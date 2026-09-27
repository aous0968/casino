package wallet

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/aous0968/casino/pkg/authmw"
	"github.com/aous0968/casino/pkg/httpx"
)

type Handler struct {
	repo *Repo
	log  *slog.Logger
}

func NewHandler(repo *Repo, log *slog.Logger) *Handler {
	return &Handler{repo: repo, log: log}
}

// ---------- shared types ----------

type moneyRequest struct {
	Amount int64 `json:"amount"` // minor units, always positive
}

type moneyResponse struct {
	TransactionID string `json:"transaction_id"`
	Balance       int64  `json:"balance"`  // user's new balance, minor units
	Currency      string `json:"currency"`
}

// ---------- Deposit ----------

func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.UserIDFromContext(r.Context())
	if !ok {
		h.log.Error("deposit: no user id in context")
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_idempotency_key",
			"Idempotency-Key header is required for money movements")
		return
	}

	var req moneyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}
	defer r.Body.Close()

	if req.Amount <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_amount", "amount must be positive minor units")
		return
	}

	userAcc, err := h.repo.GetOrCreateUserAccount(r.Context(), userID)
	if err != nil {
		h.log.Error("deposit: get user account", "err", err, "user_id", userID)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	externalAcc, err := h.repo.GetSystemAccount(r.Context(), "external")
	if err != nil {
		h.log.Error("deposit: get external account", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	res, err := h.repo.Transfer(r.Context(), TransferParams{
		FromAccountID:  externalAcc.ID,
		ToAccountID:    userAcc.ID,
		Amount:         req.Amount,
		Kind:           "deposit",
		IdempotencyKey: key,
		Metadata:       map[string]any{"user_id": userID},
	})
	if err != nil {
		h.log.Error("deposit: transfer", "err", err, "user_id", userID, "amount", req.Amount)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	h.log.Info("deposit",
		"user_id", userID,
		"amount", req.Amount,
		"tx_id", res.TransactionID,
	)
	httpx.WriteJSON(w, http.StatusCreated, moneyResponse{
		TransactionID: res.TransactionID,
		Balance:       res.ToBalance, // user account is the "to" side
		Currency:      userAcc.Currency,
	})
}

// ---------- Withdraw ----------

func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.UserIDFromContext(r.Context())
	if !ok {
		h.log.Error("withdraw: no user id in context")
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_idempotency_key",
			"Idempotency-Key header is required for money movements")
		return
	}

	var req moneyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}
	defer r.Body.Close()

	if req.Amount <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_amount", "amount must be positive minor units")
		return
	}

	userAcc, err := h.repo.GetOrCreateUserAccount(r.Context(), userID)
	if err != nil {
		h.log.Error("withdraw: get user account", "err", err, "user_id", userID)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	externalAcc, err := h.repo.GetSystemAccount(r.Context(), "external")
	if err != nil {
		h.log.Error("withdraw: get external account", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	res, err := h.repo.Transfer(r.Context(), TransferParams{
		FromAccountID:  userAcc.ID,
		ToAccountID:    externalAcc.ID,
		Amount:         req.Amount,
		Kind:           "withdrawal",
		IdempotencyKey: key,
		Metadata:       map[string]any{"user_id": userID},
	})
	if err != nil {
		if errors.Is(err, ErrInsufficientFunds) {
			httpx.WriteError(w, http.StatusPaymentRequired, "insufficient_funds",
				"balance is too low for this withdrawal")
			return
		}
		h.log.Error("withdraw: transfer", "err", err, "user_id", userID, "amount", req.Amount)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	h.log.Info("withdraw",
		"user_id", userID,
		"amount", req.Amount,
		"tx_id", res.TransactionID,
	)
	httpx.WriteJSON(w, http.StatusCreated, moneyResponse{
		TransactionID: res.TransactionID,
		Balance:       res.FromBalance, // user account is the "from" side
		Currency:      userAcc.Currency,
	})
}