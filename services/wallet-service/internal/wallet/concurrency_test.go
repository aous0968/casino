//go:build integration

package wallet

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func requireTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestConcurrentWithdrawalsCannotOverdraw(t *testing.T) {
	pool := requireTestPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()

	userID := "test-" + uuid.NewString()

	userAcc, err := repo.GetOrCreateUserAccount(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	external, err := repo.GetSystemAccount(ctx, "external")
	if err != nil {
		t.Fatal(err)
	}

	// Seed the account with $100.
	if _, err := repo.Transfer(ctx, TransferParams{
		FromAccountID: external.ID,
		ToAccountID:   userAcc.ID,
		Amount:        10000,
		Kind:          "deposit",
	}); err != nil {
		t.Fatal(err)
	}

	// Fire 5 concurrent $30 withdrawals. Only 3 should succeed.
	const workers = 5
	const amount = 3000

	var (
		wg             sync.WaitGroup
		successes      int32
		insufficient   int32
		otherErrs      []error
		errsMu         sync.Mutex
	)

	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // all goroutines wait for the same starting gun
			_, err := repo.Transfer(ctx, TransferParams{
				FromAccountID: userAcc.ID,
				ToAccountID:   external.ID,
				Amount:        amount,
				Kind:          "withdrawal",
			})
			switch {
			case err == nil:
				atomic.AddInt32(&successes, 1)
			case errors.Is(err, ErrInsufficientFunds):
				atomic.AddInt32(&insufficient, 1)
			default:
				errsMu.Lock()
				otherErrs = append(otherErrs, err)
				errsMu.Unlock()
			}
		}()
	}

	close(start) // fire!
	wg.Wait()

	if len(otherErrs) > 0 {
		t.Fatalf("unexpected errors: %v", otherErrs)
	}
	if successes != 3 {
		t.Fatalf("expected 3 successes, got %d (insufficient=%d)", successes, insufficient)
	}
	if insufficient != 2 {
		t.Fatalf("expected 2 insufficient, got %d", insufficient)
	}

	// Final balance must be exactly $10.
	final, err := repo.GetUserAccount(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Balance != 1000 {
		t.Fatalf("expected balance 1000, got %d", final.Balance)
	}
}

func TestConcurrentSameIdempotencyKey(t *testing.T) {
	pool := requireTestPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()

	userID := "test-" + uuid.NewString()

	userAcc, err := repo.GetOrCreateUserAccount(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	external, err := repo.GetSystemAccount(ctx, "external")
	if err != nil {
		t.Fatal(err)
	}

	key := uuid.NewString()
	const workers = 5

	var (
		wg     sync.WaitGroup
		txIDs  sync.Map // string -> struct{}
		errsMu sync.Mutex
		errs   []error
	)

	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := repo.Transfer(ctx, TransferParams{
				FromAccountID:  external.ID,
				ToAccountID:    userAcc.ID,
				Amount:         1000,
				Kind:           "deposit",
				IdempotencyKey: key,
			})
			if err != nil {
				errsMu.Lock()
				errs = append(errs, err)
				errsMu.Unlock()
				return
			}
			txIDs.Store(res.TransactionID, struct{}{})
		}()
	}
	close(start)
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	// All workers should have gotten the SAME transaction ID.
	var count int
	txIDs.Range(func(_, _ any) bool { count++; return true })
	if count != 1 {
		t.Fatalf("expected 1 unique transaction ID, got %d", count)
	}

	// Balance must be exactly $10, not $50.
	final, err := repo.GetUserAccount(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Balance != 1000 {
		t.Fatalf("expected balance 1000, got %d", final.Balance)
	}
}