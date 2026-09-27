package wallet

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type VerifyReport struct {
	RanAt time.Time

	// Transactions whose ledger entries don't sum to zero.
	UnbalancedTransactions []string

	// Accounts whose cached balance differs from the ledger sum.
	// Format: account_id: cached=X, ledger=Y
	DriftedAccounts []string

	// If total != 0, we have created or destroyed money.
	TotalBalance int64

	// Duration of the whole run.
	Duration time.Duration
}

func (r VerifyReport) OK() bool {
	return len(r.UnbalancedTransactions) == 0 &&
		len(r.DriftedAccounts) == 0 &&
		r.TotalBalance == 0
}

type Verifier struct {
	repo *Repo
	log  *slog.Logger
}

func NewVerifier(repo *Repo, log *slog.Logger) *Verifier {
	return &Verifier{repo: repo, log: log}
}

// Run executes all three checks against the whole ledger.
// Note: this scans all entries. Fine for a learning project; a production
// system would check only recent transactions and sample historical ones.
func (v *Verifier) Run(ctx context.Context) (*VerifyReport, error) {
	start := time.Now()
	report := &VerifyReport{RanAt: start}

	// 1. Every transaction's entries must sum to zero.
	rows, err := v.repo.pool.Query(ctx, `
		SELECT transaction_id
		FROM wallet.ledger_entries
		GROUP BY transaction_id
		HAVING SUM(amount) <> 0
	`)
	if err != nil {
		return nil, fmt.Errorf("verifier: check transaction sums: %w", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("verifier: scan tx: %w", err)
		}
		report.UnbalancedTransactions = append(report.UnbalancedTransactions, id)
	}
	rows.Close()

	// 2. Each account's cached balance must equal the sum of its entries.
	rows, err = v.repo.pool.Query(ctx, `
		SELECT a.id, a.balance, COALESCE(SUM(le.amount), 0) AS ledger_sum
		FROM wallet.accounts a
		LEFT JOIN wallet.ledger_entries le ON le.account_id = a.id
		GROUP BY a.id, a.balance
		HAVING a.balance <> COALESCE(SUM(le.amount), 0)
	`)
	if err != nil {
		return nil, fmt.Errorf("verifier: check account balances: %w", err)
	}
	for rows.Next() {
		var (
			id        string
			cached    int64
			ledgerSum int64
		)
		if err := rows.Scan(&id, &cached, &ledgerSum); err != nil {
			rows.Close()
			return nil, fmt.Errorf("verifier: scan account: %w", err)
		}
		report.DriftedAccounts = append(report.DriftedAccounts,
			fmt.Sprintf("%s: cached=%d, ledger=%d", id, cached, ledgerSum))
	}
	rows.Close()

	// 3. Sum of all balances must be zero.
	err = v.repo.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(balance), 0) FROM wallet.accounts`,
	).Scan(&report.TotalBalance)
	if err != nil {
		return nil, fmt.Errorf("verifier: check total: %w", err)
	}

	report.Duration = time.Since(start)
	return report, nil
}

// RunLoop runs the verifier every interval until ctx is cancelled.
func (v *Verifier) RunLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report, err := v.Run(ctx)
			if err != nil {
				v.log.Error("verifier: run failed", "err", err)
				continue
			}
			if report.OK() {
				v.log.Info("verifier: ok", "duration_ms", report.Duration.Milliseconds())
				continue
			}
			// Invariant violated — this is the loudest possible log line.
			v.log.Error("verifier: INVARIANT VIOLATION",
				"unbalanced_transactions", report.UnbalancedTransactions,
				"drifted_accounts", report.DriftedAccounts,
				"total_balance", report.TotalBalance,
			)
		}
	}
}