package wallet

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/pgconn"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// ---------- lookups ----------

// GetOrCreateUserAccount returns the user's USD account, creating it if needed.
func (r *Repo) GetOrCreateUserAccount(ctx context.Context, userID string) (*Account, error) {
	const insert = `
		INSERT INTO wallet.accounts (owner_type, owner_id)
		VALUES ('user', $1)
		ON CONFLICT (owner_type, owner_id, currency) DO NOTHING
	`
	if _, err := r.pool.Exec(ctx, insert, userID); err != nil {
		return nil, fmt.Errorf("wallet: ensure user account: %w", err)
	}

	const selectQ = `
		SELECT id, owner_type, owner_id, currency, balance
		FROM wallet.accounts
		WHERE owner_type = 'user' AND owner_id = $1
	`
	a := &Account{}
	err := r.pool.QueryRow(ctx, selectQ, userID).
		Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.Currency, &a.Balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("wallet: load user account: %w", err)
	}
	return a, nil
}

// GetSystemAccount returns a named system account like "house" or "external".
func (r *Repo) GetSystemAccount(ctx context.Context, name string) (*Account, error) {
	const q = `
		SELECT id, owner_type, owner_id, currency, balance
		FROM wallet.accounts
		WHERE owner_type = 'system' AND owner_id = $1
	`
	a := &Account{}
	err := r.pool.QueryRow(ctx, q, name).
		Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.Currency, &a.Balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("wallet: load system account %q: %w", name, err)
	}
	return a, nil
}

// ---------- the atomic transfer ----------

// Transfer moves Amount from FromAccountID to ToAccountID atomically.
// Creates a transaction row and two matching ledger entries. Updates both
// account balances. All or nothing.
func (r *Repo) Transfer(ctx context.Context, p TransferParams) (*TransferResult, error) {
	if p.Amount <= 0 {
		return nil, ErrInvalidAmount
	}
	if p.FromAccountID == p.ToAccountID {
		return nil, ErrSameAccount
	}

	// Fast path: if we've seen this key, return the original result.
	if p.IdempotencyKey != "" {
		if res, err := r.findByKey(ctx, p.IdempotencyKey); err == nil {
			return res, nil
		} else if !errors.Is(err, ErrAccountNotFound) {
			return nil, err
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("wallet: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	ids := []string{p.FromAccountID, p.ToAccountID}
	sort.Strings(ids)

	locked := make(map[string]*Account, 2)
	for _, id := range ids {
		a, err := lockAccount(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		locked[id] = a
	}
	from := locked[p.FromAccountID]
	to := locked[p.ToAccountID]

	if from.Currency != to.Currency {
		return nil, ErrCurrencyMismatch
	}
	
	if from.OwnerType == "user" && from.Balance < p.Amount {
		return nil, ErrInsufficientFunds
	}

	// Insert transaction. If the idempotency key already exists (race),
	// Postgres returns 23505 and we fall back to the lookup.
	txID, err := insertTransaction(ctx, tx, p)
	if err != nil {
		if errors.Is(err, ErrDuplicateKey) && p.IdempotencyKey != "" {
			_ = tx.Rollback(ctx)
			return r.findByKey(ctx, p.IdempotencyKey)
		}
		return nil, err
	}

	if err := insertEntry(ctx, tx, txID, p.FromAccountID, -p.Amount); err != nil {
		return nil, err
	}
	if err := insertEntry(ctx, tx, txID, p.ToAccountID, +p.Amount); err != nil {
		return nil, err
	}

	newFrom, err := updateBalance(ctx, tx, p.FromAccountID, -p.Amount)
	if err != nil {
		return nil, err
	}
	newTo, err := updateBalance(ctx, tx, p.ToAccountID, +p.Amount)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("wallet: commit: %w", err)
	}

	return &TransferResult{
		TransactionID: txID,
		FromBalance:   newFrom,
		ToBalance:     newTo,
	}, nil
}
// ---------- helpers used inside Transfer ----------

func lockAccount(ctx context.Context, tx pgx.Tx, id string) (*Account, error) {
	const q = `
		SELECT id, owner_type, owner_id, currency, balance
		FROM wallet.accounts
		WHERE id = $1
		FOR UPDATE
	`
	a := &Account{}
	err := tx.QueryRow(ctx, q, id).
		Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.Currency, &a.Balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("wallet: lock account %s: %w", id, err)
	}
	return a, nil
}

func insertTransaction(ctx context.Context, tx pgx.Tx, p TransferParams) (string, error) {
	const q = `
		INSERT INTO wallet.transactions (kind, idempotency_key, metadata)
		VALUES ($1, $2, $3)
		RETURNING id
	`

	var keyArg any
	if p.IdempotencyKey != "" {
		keyArg = p.IdempotencyKey
	} else {
		keyArg = nil
	}

	var id string
	err := tx.QueryRow(ctx, q, p.Kind, keyArg, p.Metadata).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", ErrDuplicateKey
		}
		return "", fmt.Errorf("wallet: insert transaction: %w", err)
	}
	return id, nil
}

func insertEntry(ctx context.Context, tx pgx.Tx, txID, accountID string, amount int64) error {
	const q = `
		INSERT INTO wallet.ledger_entries (transaction_id, account_id, amount)
		VALUES ($1, $2, $3)
	`
	if _, err := tx.Exec(ctx, q, txID, accountID, amount); err != nil {
		return fmt.Errorf("wallet: insert entry: %w", err)
	}
	return nil
}

func updateBalance(ctx context.Context, tx pgx.Tx, accountID string, delta int64) (int64, error) {
	const q = `
		UPDATE wallet.accounts
		SET balance = balance + $2, updated_at = NOW()
		WHERE id = $1
		RETURNING balance
	`
	var newBalance int64
	if err := tx.QueryRow(ctx, q, accountID, delta).Scan(&newBalance); err != nil {
		return 0, fmt.Errorf("wallet: update balance %s: %w", accountID, err)
	}
	return newBalance, nil
}

// findByKey returns the result of a previous transfer identified by its
// idempotency key. Reconstructs the response from the stored transaction.
func (r *Repo) findByKey(ctx context.Context, key string) (*TransferResult, error) {
	const q = `
		SELECT
			t.id,
			COALESCE((SELECT balance FROM wallet.accounts WHERE id = le_debit.account_id), 0),
			COALESCE((SELECT balance FROM wallet.accounts WHERE id = le_credit.account_id), 0)
		FROM wallet.transactions t
		JOIN LATERAL (
			SELECT * FROM wallet.ledger_entries
			WHERE transaction_id = t.id AND amount < 0 LIMIT 1
		) le_debit ON TRUE
		JOIN LATERAL (
			SELECT * FROM wallet.ledger_entries
			WHERE transaction_id = t.id AND amount > 0 LIMIT 1
		) le_credit ON TRUE
		WHERE t.idempotency_key = $1
	`

	var (
		txID      string
		fromBal   int64
		toBal     int64
	)

	err := r.pool.QueryRow(ctx, q, key).Scan(&txID, &fromBal, &toBal)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIdempotencyKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("wallet: find by idempotency key: %w", err)
	}

	return &TransferResult{
		TransactionID: txID,
		FromBalance:   fromBal,
		ToBalance:     toBal,
	}, nil
}