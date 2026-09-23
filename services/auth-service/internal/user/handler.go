package user

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/aous0968/casino/pkg/httpx"
	"github.com/aous0968/casino/services/auth-service/internal/password"
	"github.com/aous0968/casino/services/auth-service/internal/token"
	"github.com/aous0968/casino/services/auth-service/internal/auth"
)

type Handler struct {
	repo       *Repo
	log        *slog.Logger
	signer     *token.Signer
	refreshTTL time.Duration
	accessTTL  time.Duration
}

func NewHandler(repo *Repo, log *slog.Logger, signer *token.Signer, refreshTTL time.Duration , accessTTL time.Duration) *Handler {
	return &Handler{
		repo:       repo,
		log:        log,
		signer:     signer,
		refreshTTL: refreshTTL,
		accessTTL:  accessTTL,
	}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}
	defer r.Body.Close()

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if fields := validate(req); len(fields) > 0 {
		httpx.WriteValidationError(w, fields)
		return
	}

	hash, err := password.Hash(req.Password)
	if err != nil {
		h.log.Error("hash password failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	u, err := h.repo.Create(r.Context(), req.Email, hash)
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			httpx.WriteError(w, http.StatusConflict, "email_taken", "this email is already registered")
			return
		}
		h.log.Error("create user failed", "err", err, "email", req.Email)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	h.log.Info("user registered", "user_id", u.ID, "email", u.Email)
	httpx.WriteJSON(w, http.StatusCreated, registerResponse{ID: u.ID, Email: u.Email})
}

func validate(req registerRequest) map[string]string {
	fields := make(map[string]string)

	if req.Email == "" {
		fields["email"] = "required"
	} else if _, err := mail.ParseAddress(req.Email); err != nil {
		fields["email"] = "invalid email format"
	} else if len(req.Email) > 254 {
		fields["email"] = "too long"
	}

	switch {
	case req.Password == "":
		fields["password"] = "required"
	case len(req.Password) < 8:
		fields["password"] = "must be at least 8 characters"
	case len(req.Password) > 128:
		fields["password"] = "must be at most 128 characters"
	}

	return fields
}
// ---------- Login ----------

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // seconds until access token expires
}

// dummyHash is a precomputed Argon2id hash used to equalize login timing
// when the user doesn't exist. Generated once at package init.
var dummyHash string

func init() {
	h, err := password.Hash("dummy-password-for-timing-equalization")
	if err != nil {
		panic("user: cannot precompute dummy hash: " + err.Error())
	}
	dummyHash = h
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}
	defer r.Body.Close()

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Email == "" || req.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_credentials", "email and password are required")
		return
	}

	u, err := h.repo.FindByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Run a dummy verify so timing is identical to the wrong-password case.
			_, _ = password.Verify(req.Password, dummyHash)
			httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
			return
		}
		h.log.Error("login: find user failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	ok, err := password.Verify(req.Password, u.PasswordHash)
	if err != nil {
		h.log.Error("login: verify password failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
		return
	}

	resp, err := h.issueTokens(r, u.ID)
	if err != nil {
		h.log.Error("login: issue tokens failed", "err", err, "user_id", u.ID)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	h.log.Info("login success", "user_id", u.ID)
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ---------- Refresh ----------

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}
	defer r.Body.Close()

	if req.RefreshToken == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_token", "refresh_token is required")
		return
	}

	hash := token.HashRefreshToken(req.RefreshToken)

	stored, err := h.repo.FindRefreshToken(r.Context(), hash)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid_token", "refresh token is invalid or expired")
			return
		}
		h.log.Error("refresh: lookup failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	// Rotation: revoke the old token before issuing a new one.
	if err := h.repo.RevokeRefreshToken(r.Context(), hash); err != nil {
		h.log.Error("refresh: revoke failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	resp, err := h.issueTokens(r, stored.UserID)
	if err != nil {
		h.log.Error("refresh: issue tokens failed", "err", err, "user_id", stored.UserID)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	h.log.Info("refresh success", "user_id", stored.UserID)
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ---------- Logout ----------

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}
	defer r.Body.Close()

	if req.RefreshToken == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_token", "refresh_token is required")
		return
	}

	hash := token.HashRefreshToken(req.RefreshToken)

	if err := h.repo.RevokeRefreshToken(r.Context(), hash); err != nil {
		h.log.Error("logout: revoke failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------- shared ----------

// issueTokens generates a fresh access + refresh pair and stores the
// refresh token's hash. Returns the wire response.
func (h *Handler) issueTokens(r *http.Request, userID string) (*tokenResponse, error) {
	access, err := h.signer.Sign(userID)
	if err != nil {
		return nil, err
	}

	raw, hash, err := token.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(h.refreshTTL)
	if err := h.repo.StoreRefreshToken(r.Context(), userID, hash, expiresAt); err != nil {
		return nil, err
	}

	return &tokenResponse{
		AccessToken:  access,
		RefreshToken: raw,
		TokenType:    "Bearer",
		ExpiresIn:    int(h.accessTTL.Seconds()), // 15 minutes, keep in sync with config
	}, nil
}

// ---------- Me ----------

type meResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		// This should be impossible if the middleware is wired correctly.
		// Log it as a programming error.
		h.log.Error("me: no user id in context")
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	u, err := h.repo.FindByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Token is valid but user no longer exists (deleted account).
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "user no longer exists")
			return
		}
		h.log.Error("me: find user failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, meResponse{ID: u.ID, Email: u.Email})
}