package wallet

import "errors"

var (
	ErrAccountNotFound   	  = errors.New("wallet: account not found")
	ErrInsufficientFunds 	  = errors.New("wallet: insufficient funds")
	ErrSameAccount       	  = errors.New("wallet: from and to accounts are the same")
	ErrInvalidAmount     	  = errors.New("wallet: amount must be non-zero")
	ErrDuplicateKey 	 	  = errors.New("wallet: duplicate idempotency key")
	ErrIdempotencyKeyNotFound = errors.New("wallet: idempotency key not found")
	ErrCurrencyMismatch 	  = errors.New("wallet: accounts have different currencies")
)

type Account struct {
	ID        string
	OwnerType string // "user" or "system"
	OwnerID   string
	Currency  string
	Balance   int64 // minor units
}

type TransferParams struct {
	FromAccountID  string
	ToAccountID    string
	Amount         int64  // positive; direction is from→to
	IdempotencyKey string // required for public endpoints; may be empty for internal jobs
	Kind           string // "deposit", "withdrawal", "bet", "win", "bonus"
	Metadata      map[string]any
}

type TransferResult struct {
	TransactionID string
	FromBalance   int64 // new balance after transfer
	ToBalance     int64
}