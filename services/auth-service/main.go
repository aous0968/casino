package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aous0968/casino/pkg/config"
	"github.com/aous0968/casino/pkg/httpx"
	"github.com/aous0968/casino/pkg/logger"
	"github.com/aous0968/casino/pkg/postgres"
	"github.com/aous0968/casino/services/auth-service/internal/user"
	"github.com/aous0968/casino/services/auth-service/internal/token"
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
	log.Info("starting", "env", cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.New(ctx, cfg.Postgres.DSN(), cfg.Postgres.MaxConns)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	log.Info("postgres connected", "host", cfg.Postgres.Host, "db", cfg.Postgres.Database)

	// ---- Wire dependencies ----
	signer := token.NewSigner(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.AccessTokenTTL)

	userRepo := user.NewRepo(pool.Pool)
	userHandler := user.NewHandler(
		userRepo,
		log,
		signer,
		cfg.JWT.RefreshTokenTTL,
		cfg.JWT.AccessTokenTTL,
	)

	// ---- Routes ----
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			log.Warn("readiness failed", "err", err)
			httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "postgres unreachable")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	mux.HandleFunc("POST /register", userHandler.Register)
	mux.HandleFunc("POST /login", userHandler.Login)
	mux.HandleFunc("POST /refresh", userHandler.Refresh)
	mux.HandleFunc("POST /logout", userHandler.Logout)

	// ---- HTTP server ----
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

// logRequests logs method, path, and duration for every request.
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