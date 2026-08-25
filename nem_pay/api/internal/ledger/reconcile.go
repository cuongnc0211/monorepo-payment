package ledger

import (
	"context"

	"github.com/google/uuid"

	"github.com/nempay/api/internal/repository/db"
)

// SumByKind returns the summed derived balance across ALL of a merchant's accounts of one kind and
// currency (e.g. every per-intent escrow_liability), as int64 minor units.
func SumByKind(ctx context.Context, q *db.Queries, merchantID uuid.UUID, kind, currency string) (int64, error) {
	n, err := q.SumAccountBalanceByKind(ctx, db.SumAccountBalanceByKindParams{
		MerchantID: merchantID, Kind: kind, Currency: currency,
	})
	if err != nil {
		return 0, err
	}
	return numericToInt64(n)
}

// EscrowSegregationHolds proves the segregation invariant for a merchant/currency: the segregated
// cash on hand must back exactly the outstanding escrow liabilities. In the debit-normal
// convention an escrow_liability carries a negative balance, so the check is
// segregated_cash + Σ escrow_liability == 0. Released/refunded intents contribute 0 to both sides,
// so no state-scoping is needed. (A captured-not-yet-settled liability is instead backed by the
// acquirer receivable — that transit window is outside this "cash on hand" invariant.)
func EscrowSegregationHolds(ctx context.Context, q *db.Queries, merchantID uuid.UUID, currency string) (segregated, liability int64, ok bool, err error) {
	segregated, err = SumByKind(ctx, q, merchantID, KindSegregatedCash, currency)
	if err != nil {
		return 0, 0, false, err
	}
	liability, err = SumByKind(ctx, q, merchantID, KindEscrowLiability, currency)
	if err != nil {
		return 0, 0, false, err
	}
	return segregated, liability, segregated+liability == 0, nil
}
