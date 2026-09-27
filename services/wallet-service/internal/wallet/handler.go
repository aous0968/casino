package wallet

import (
	"fmt"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
	"strconv"
	"strings"
	"encoding/base64"

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

// ---------- Balance ----------

type balanceResponse struct {
	Balance  int64  `json:"balance"`
	Currency string `json:"currency"`
}

func (h *Handler) Balance(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.UserIDFromContext(r.Context())
	if !ok {
		h.log.Error("balance: no user id in context")
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	acc, err := h.repo.GetUserAccount(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			// No account yet → user has never transacted. Zero balance.
			httpx.WriteJSON(w, http.StatusOK, balanceResponse{Balance: 0, Currency: "USD"})
			return
		}
		h.log.Error("balance: get account", "err", err, "user_id", userID)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, balanceResponse{
		Balance:  acc.Balance,
		Currency: acc.Currency,
	})
}

// ---------- Transactions ----------

type transactionItem struct {
	ID        string         `json:"id"`
	Amount    int64          `json:"amount"`
	Kind      string         `json:"kind"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type transactionsResponse struct {
	Items      []transactionItem `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

func (h *Handler) Transactions(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.UserIDFromContext(r.Context())
	if !ok {
		h.log.Error("transactions: no user id in context")
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	acc, err := h.repo.GetUserAccount(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			// No account yet → empty history.
			httpx.WriteJSON(w, http.StatusOK, transactionsResponse{Items: []transactionItem{}})
			return
		}
		h.log.Error("transactions: get account", "err", err, "user_id", userID)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"), 20, 100)
	cursorTS, cursorID := decodeCursor(r.URL.Query().Get("cursor"))

	// Fetch one extra to detect if there's a next page.
	entries, err := h.repo.ListEntries(r.Context(), acc.ID, limit+1, cursorTS, cursorID)
	if err != nil {
		h.log.Error("transactions: list", "err", err, "user_id", userID)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	var nextCursor string
	if len(entries) > limit {
		entries = entries[:limit]
		last := entries[len(entries)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	items := make([]transactionItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, transactionItem{
			ID:        e.ID,
			Amount:    e.Amount,
			Kind:      e.Kind,
			Metadata:  e.Metadata,
			CreatedAt: e.CreatedAt,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, transactionsResponse{
		Items:      items,
		NextCursor: nextCursor,
	})
}

// ---------- pagination helpers ----------

func parseLimit(raw string, def, max int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func encodeCursor(ts time.Time, id string) string {
	raw := fmt.Sprintf("%d|%s", ts.UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (time.Time, string) {
	if s == "" {
		return time.Time{}, ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, ""
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, ""
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, ""
	}
	return time.Unix(0, nanos), parts[1]
}