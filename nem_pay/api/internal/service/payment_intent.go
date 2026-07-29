package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/banksim"
	"github.com/nempay/api/internal/repository/db"
	"github.com/nempay/api/internal/statemachine"
)

// Validation errors returned by the intent service; the HTTP layer maps these to 400s with the
// error envelope. Kept as typed sentinels so callers assert the exact cause, never a string.
var (
	ErrInvalidAmount   = errors.New("amount must be a positive integer in minor units")
	ErrInvalidCurrency = errors.New("currency must be a 3-letter ISO-4217 code")
	ErrIntentNotFound  = errors.New("payment intent not found")
)

// defaultListLimit / maxListLimit bound the list endpoint's page size.
const (
	defaultListLimit = 20
	maxListLimit     = 100
)

// Intents is the payment-intent service. It owns the DB transaction boundary for money
// mutations (confirm/capture/refund/settle in money.go); for the create/read/list here a single
// statement suffices, so they run directly on the pool-bound queries.
type Intents struct {
	pool *pgxpool.Pool
	q    *db.Queries
	bank *banksim.Client
}

// NewIntents builds the service over a pgx pool and a bank-sim client.
func NewIntents(pool *pgxpool.Pool, bank *banksim.Client) *Intents {
	return &Intents{pool: pool, q: db.New(pool), bank: bank}
}

// Create validates and inserts a new direct-mode intent in status 'created'. metadata may be
// nil (stored as an empty JSON object).
func (s *Intents) Create(ctx context.Context, merchantID uuid.UUID, amount int64, currency string, metadata []byte) (db.PaymentIntent, error) {
	if amount <= 0 {
		return db.PaymentIntent{}, ErrInvalidAmount
	}
	currency, ok := normalizeCurrency(currency)
	if !ok {
		return db.PaymentIntent{}, ErrInvalidCurrency
	}
	if len(metadata) == 0 {
		metadata = []byte("{}")
	}
	return s.q.CreateIntent(ctx, db.CreateIntentParams{
		MerchantID:     merchantID,
		Amount:         amount,
		Currency:       currency,
		SettlementMode: statemachine.SettlementDirect,
		Metadata:       metadata,
	})
}

// normalizeCurrency upper-cases the code and accepts it only if it is exactly three ASCII
// letters — matching the ISO-4217 shape the column and API claim (length alone would let "1$3"
// or lowercase through and store them unnormalized).
func normalizeCurrency(cur string) (string, bool) {
	if len(cur) != 3 {
		return "", false
	}
	out := []byte(cur)
	for i, b := range out {
		switch {
		case b >= 'a' && b <= 'z':
			out[i] = b - ('a' - 'A')
		case b >= 'A' && b <= 'Z':
			// already uppercase
		default:
			return "", false
		}
	}
	return string(out), true
}

// Get returns one intent scoped to the merchant, or ErrIntentNotFound.
func (s *Intents) Get(ctx context.Context, id, merchantID uuid.UUID) (db.PaymentIntent, error) {
	pi, err := s.q.GetIntent(ctx, db.GetIntentParams{ID: id, MerchantID: merchantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PaymentIntent{}, ErrIntentNotFound
	}
	return pi, err
}

// List returns a merchant's intents (newest first) with an optional status filter and paging.
// limit is clamped to [1, maxListLimit]; a limit <= 0 uses the default.
func (s *Intents) List(ctx context.Context, merchantID uuid.UUID, status string, limit, offset int32) ([]db.PaymentIntent, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	if offset < 0 {
		offset = 0
	}
	var statusFilter *string
	if status != "" {
		statusFilter = &status
	}
	return s.q.ListIntents(ctx, db.ListIntentsParams{
		MerchantID: merchantID,
		Status:     statusFilter,
		Lim:        limit,
		Off:        offset,
	})
}
