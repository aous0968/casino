package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aous0968/casino/services/api-gateway/internal/config"
	"github.com/aous0968/casino/pkg/httpx"
	"github.com/aous0968/casino/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.LogLevel, cfg.Env).With("service", cfg.Service.Name)
	log.Info("starting gateway", "env", cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Mount reverse proxies for each backend service.
	authURL, err := url.Parse("http://localhost:8081")
	if err != nil {
		return fmt.Errorf("parse auth url: %w", err)
	}
	mux.Handle("/auth/", stripPrefixAndProxy("/auth", authURL, log))

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Service.Port),
		Handler:           logRequests(log, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// stripPrefixAndProxy creates a handler that strips the given prefix
// from the request path and forwards the rest to the upstream URL.
func stripPrefixAndProxy(prefix string, upstream *url.URL, log *slog.Logger) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			// Route the outbound request to the upstream.
			r.SetURL(upstream)

			// Set X-Forwarded-* headers based on the inbound request.
			// This appends to any existing X-Forwarded-For from prior proxies.
			r.SetXForwarded()

			// Preserve the inbound Host header. SetURL rewrites it to
			// the upstream's host by default; the original
			// NewSingleHostReverseProxy behavior kept the inbound Host.
			r.Out.Host = r.In.Host

			// Strip the route prefix. We do this on the outbound URL
			// after SetURL so the path joining doesn't reintroduce it.
			r.Out.URL.Path = strings.TrimPrefix(r.Out.URL.Path, prefix)
			if r.Out.URL.Path == "" {
				r.Out.URL.Path = "/"
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error("proxy error", "upstream", upstream.String(), "err", err)
			httpx.WriteError(w, http.StatusBadGateway, "upstream_unreachable", "backend service is unavailable")
		},
	}

	return proxy
}

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}