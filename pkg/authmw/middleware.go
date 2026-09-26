package authmw

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aous0968/casino/pkg/httpx"
	"github.com/aous0968/casino/pkg/token"
)

// ctxKey is a private type used only as a context key.
// Using an unexported type prevents collisions with other packages.
type ctxKey int

const userIDKey ctxKey = iota

// UserIDFromContext returns the authenticated user's ID, if present.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(userIDKey)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// RequireAuth verifies the Bearer token in the Authorization header.
// On success, the user ID is placed in the request context and the
// next handler is called. On failure, a 401 is returned.
func RequireAuth(verifier *token.Verifier, log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			unauthorized(w)
			return
		}

		const prefix = "bearer "
		if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
			unauthorized(w)
			return
		}

		raw := header[len(prefix):]
		if raw == "" {
			unauthorized(w)
			return
		}

		claims, err := verifier.Verify(raw)
		if err != nil {
			// Don't tell the client whether it was expired or invalid.
			// Log it so we can debug, but return the same 401 either way.
			log.Debug("auth: token rejected", "err", err, "path", r.URL.Path)
			unauthorized(w)
			return
		}

		if claims.Subject == "" {
			log.Warn("auth: token has no subject")
			unauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, claims.Subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="auth"`)
	httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid access token")
}
