package user

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"

	"github.com/aous0968/casino/pkg/httpx"
	"github.com/aous0968/casino/services/auth-service/internal/password"
)

type Handler struct {
	repo *Repo
	log  *slog.Logger
}

func NewHandler(repo *Repo, log *slog.Logger) *Handler {
	return &Handler{repo: repo, log: log}
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