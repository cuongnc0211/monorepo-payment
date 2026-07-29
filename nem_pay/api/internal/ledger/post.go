package ledger

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nempay/api/internal/repository/db"
)

// Typed errors so callers (and tests) can assert the exact failure without string matching.
var (
	ErrEmpty         = errors.New("ledger: transaction has no entries")
	ErrBadEntry      = errors.New("ledger: each entry must have exactly one of debit/credit non-zero and both non-negative")
	ErrMixedCurrency = errors.New("ledger: all entries in a transaction must share one currency")
	ErrUnbalanced    = errors.New("ledger: transaction unbalanced (Σdebit != Σcredit)")
)

// Entry is one leg of a balanced transaction. Amounts are int64 minor units; exactly one of
// Debit/Credit is non-zero and both are non-negative (the entries CHECK enforces the same at
// the DB, but we validate up front to fail fast with a typed error and write nothing).
type Entry struct {
	AccountID uuid.UUID
	Debit     int64
	Credit    int64
	Currency  string
}

// PostTransaction writes one balanced transaction and its entries using the supplied
// *db.Queries. The caller binds that Queries to its own DB transaction (q.WithTx(tx)) — the
// service layer owns the tx boundary so the ledger write lands in the same tx as the state
// change it represents (see nem_pay/CLAUDE.md, M1.4). The double-entry invariant Σdebit =
// Σcredit is checked BEFORE any write, so an unbalanced posting persists nothing.
func PostTransaction(ctx context.Context, q *db.Queries, merchantID uuid.UUID, kind string, refID *uuid.UUID, entries []Entry) (uuid.UUID, error) {
	if len(entries) == 0 {
		return uuid.Nil, ErrEmpty
	}

	currency := entries[0].Currency
	var sumDebit, sumCredit int64
	for _, e := range entries {
		// exactly one side non-zero, both non-negative. (a==0)==(b==0) is true when both
		// are zero or both non-zero — either way malformed.
		if e.Debit < 0 || e.Credit < 0 || (e.Debit == 0) == (e.Credit == 0) {
			return uuid.Nil, ErrBadEntry
		}
		if e.Currency != currency {
			return uuid.Nil, ErrMixedCurrency
		}
		sumDebit += e.Debit
		sumCredit += e.Credit
	}
	if sumDebit != sumCredit {
		return uuid.Nil, ErrUnbalanced
	}

	tx, err := q.InsertTransaction(ctx, db.InsertTransactionParams{
		MerchantID:  merchantID,
		Kind:        kind,
		ReferenceID: refID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert transaction: %w", err)
	}
	for _, e := range entries {
		if _, err := q.InsertEntry(ctx, db.InsertEntryParams{
			TransactionID: tx.ID,
			AccountID:     e.AccountID,
			Debit:         e.Debit,
			Credit:        e.Credit,
			Currency:      e.Currency,
		}); err != nil {
			return uuid.Nil, fmt.Errorf("insert entry: %w", err)
		}
	}
	return tx.ID, nil
}

// Balance returns an account's derived balance (Σdebit − Σcredit) as int64 minor units.
// The query computes it in numeric (exact, never float); we narrow to int64 here because a
// single account's balance always fits — reconciliation reports that SUM across many
// accounts keep working in numeric.
func Balance(ctx context.Context, q *db.Queries, accountID uuid.UUID) (int64, error) {
	n, err := q.GetAccountBalance(ctx, accountID)
	if err != nil {
		return 0, err
	}
	return numericToInt64(n)
}

func numericToInt64(n pgtype.Numeric) (int64, error) {
	if !n.Valid {
		return 0, nil // no entries yet → zero balance
	}
	if n.NaN {
		return 0, errors.New("ledger: balance is NaN")
	}
	// Integer minor-unit sums have Exp == 0; handle a non-zero Exp defensively anyway.
	i := new(big.Int).Set(n.Int)
	switch {
	case n.Exp > 0:
		i.Mul(i, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n.Exp)), nil))
	case n.Exp < 0:
		i.Quo(i, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-n.Exp)), nil))
	}
	if !i.IsInt64() {
		return 0, fmt.Errorf("ledger: balance %s overflows int64", i.String())
	}
	return i.Int64(), nil
}
