package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nempay/api/internal/banksim"
	"github.com/nempay/api/internal/ledger"
	"github.com/nempay/api/internal/outbox"
	"github.com/nempay/api/internal/repository/db"
	"github.com/nempay/api/internal/statemachine"
)

// eventTypeFor maps a resulting status to its webhook event type.
func eventTypeFor(status string) string {
	switch status {
	case statemachine.StatusAuthorized:
		return "payment_intent.authorized"
	case statemachine.StatusFailed:
		return "payment_intent.failed"
	case statemachine.StatusCaptured:
		return "payment_intent.captured"
	case statemachine.StatusSettled:
		return "payment_intent.settled"
	case statemachine.StatusRefunded:
		return "payment_intent.refunded"
	case statemachine.StatusPartiallyRefunded:
		return "payment_intent.partially_refunded"
	default:
		return "payment_intent." + status
	}
}

// emit writes the outbox event for an intent's new state, using the tx-bound queries so it
// commits atomically with the status change (the outbox pattern). A marshalling slip must not
// silently drop the event, so the error is returned to fail (and roll back) the whole tx.
func emit(ctx context.Context, qtx *db.Queries, pi db.PaymentIntent) error {
	payload, err := json.Marshal(map[string]any{
		"id":              pi.ID,
		"object":          "payment_intent",
		"status":          pi.Status,
		"amount":          pi.Amount,
		"currency":        pi.Currency,
		"settlement_mode": pi.SettlementMode,
	})
	if err != nil {
		return err
	}
	return outbox.EmitEvent(ctx, qtx, pi.MerchantID, eventTypeFor(pi.Status), payload)
}

// Money-flow errors. The HTTP layer maps these to status codes; kept typed so callers assert
// the exact cause.
var (
	ErrInvalidState              = errors.New("payment intent is not in a state that allows this operation")
	ErrBankUnavailable           = errors.New("the bank could not be reached in time; the outcome is unknown")
	ErrBankDeclined              = errors.New("the bank declined the operation")
	ErrRefundExceedsCapture      = errors.New("refund amount exceeds the captured amount")
	ErrPartialRefundBeforeSettle = errors.New("a partial refund is only allowed after the intent has settled; refund in full or wait for settlement")
)

// sweepBatch bounds how many intents one settle/expiry pass claims.
const sweepBatch = 100

// Confirm authorizes an intent against the bank. It walks created → requires_confirmation →
// authorized (or → failed on decline) validating each edge, but persists only the final status.
//
// Timeout policy (the core lesson): if the bank call times out we DO NOT know whether the hold
// was placed, so we commit NO status change — the intent stays in its safe pre-auth state and
// the caller retries (idempotently). Never treat a timeout as success.
func (s *Intents) Confirm(ctx context.Context, id, merchantID uuid.UUID, token string) (db.PaymentIntent, error) {
	// DB work runs on an uncancelable context: once the bank has acted, a client disconnect must
	// not roll back the matching status change. The bank call itself keeps the cancelable ctx so
	// its deadline (and a disconnect BEFORE the bank acts) still applies.
	dbCtx := context.WithoutCancel(ctx)
	tx, err := s.pool.Begin(dbCtx)
	if err != nil {
		return db.PaymentIntent{}, err
	}
	defer tx.Rollback(dbCtx) //nolint:errcheck // no-op after a successful commit
	qtx := s.q.WithTx(tx)

	pi, err := s.lock(dbCtx, qtx, id, merchantID)
	if err != nil {
		return db.PaymentIntent{}, err
	}

	// Validate the walk to requires_confirmation (payment method now present).
	from := pi.Status
	if from == statemachine.StatusCreated {
		if !statemachine.CanTransition(from, statemachine.StatusRequiresConfirmation) {
			return db.PaymentIntent{}, ErrInvalidState
		}
		from = statemachine.StatusRequiresConfirmation
	}
	if from != statemachine.StatusRequiresConfirmation {
		return db.PaymentIntent{}, ErrInvalidState
	}

	outcome, err := s.bank.Authorize(ctx, banksim.AuthorizeRequest{
		IntentID: pi.ID, Amount: pi.Amount, Currency: pi.Currency, Token: token,
	})
	if err != nil {
		// Timeout / transport error → unknown outcome → leave the intent untouched.
		return db.PaymentIntent{}, ErrBankUnavailable
	}

	next := statemachine.StatusAuthorized
	if outcome == banksim.Declined {
		next = statemachine.StatusFailed
	}
	if !statemachine.CanTransition(statemachine.StatusRequiresConfirmation, next) {
		return db.PaymentIntent{}, ErrInvalidState
	}

	updated, err := qtx.UpdateIntentStatus(dbCtx, db.UpdateIntentStatusParams{ID: pi.ID, Status: next})
	if err != nil {
		return db.PaymentIntent{}, err
	}
	if err := emit(dbCtx, qtx, updated); err != nil {
		return db.PaymentIntent{}, err
	}
	if err := tx.Commit(dbCtx); err != nil {
		return db.PaymentIntent{}, err
	}
	return updated, nil
}

// Capture captures an authorized intent and posts the capture transaction in the SAME tx as the
// status change: Dr acquirer_receivable / Cr merchant_payable (direct mode). The FOR UPDATE lock
// makes concurrent captures safe: the second blocks, then sees 'captured' and is rejected.
func (s *Intents) Capture(ctx context.Context, id, merchantID uuid.UUID) (db.PaymentIntent, error) {
	dbCtx := context.WithoutCancel(ctx) // see Confirm: don't let a client disconnect roll back a captured intent
	tx, err := s.pool.Begin(dbCtx)
	if err != nil {
		return db.PaymentIntent{}, err
	}
	defer tx.Rollback(dbCtx) //nolint:errcheck
	qtx := s.q.WithTx(tx)

	pi, err := s.lock(dbCtx, qtx, id, merchantID)
	if err != nil {
		return db.PaymentIntent{}, err
	}
	if !statemachine.CanTransition(pi.Status, statemachine.StatusCaptured) {
		return db.PaymentIntent{}, ErrInvalidState
	}

	outcome, err := s.bank.Capture(ctx, pi.ID, pi.Amount)
	if err != nil {
		return db.PaymentIntent{}, ErrBankUnavailable
	}
	if outcome != banksim.Approved {
		return db.PaymentIntent{}, ErrBankDeclined
	}

	recv, err := s.account(dbCtx, qtx, merchantID, ledger.TypeAsset, ledger.KindAcquirerReceivable, pi.Currency)
	if err != nil {
		return db.PaymentIntent{}, err
	}
	payable, err := s.account(dbCtx, qtx, merchantID, ledger.TypeLiability, ledger.KindMerchantPayable, pi.Currency)
	if err != nil {
		return db.PaymentIntent{}, err
	}
	if _, err := ledger.PostTransaction(dbCtx, qtx, merchantID, "capture", &pi.ID, []ledger.Entry{
		{AccountID: recv, Debit: pi.Amount, Currency: pi.Currency},
		{AccountID: payable, Credit: pi.Amount, Currency: pi.Currency},
	}); err != nil {
		return db.PaymentIntent{}, err
	}

	updated, err := qtx.UpdateIntentStatus(dbCtx, db.UpdateIntentStatusParams{ID: pi.ID, Status: statemachine.StatusCaptured})
	if err != nil {
		return db.PaymentIntent{}, err
	}
	if err := emit(dbCtx, qtx, updated); err != nil {
		return db.PaymentIntent{}, err
	}
	if err := tx.Commit(dbCtx); err != nil {
		return db.PaymentIntent{}, err
	}
	return updated, nil
}

// Refund reverses a captured (or settled) intent. Amount must be > 0 and ≤ the captured amount.
// The credit side depends on whether cash has settled yet: pre-settle it reverses the receivable,
// post-settle it comes out of platform_cash. A full refund → refunded, a partial → partially_
// refunded (both terminal, so a second refund is rejected by the state machine — no over-refund).
func (s *Intents) Refund(ctx context.Context, id, merchantID uuid.UUID, amount int64) (db.PaymentIntent, error) {
	if amount <= 0 {
		return db.PaymentIntent{}, ErrInvalidAmount
	}
	dbCtx := context.WithoutCancel(ctx)
	tx, err := s.pool.Begin(dbCtx)
	if err != nil {
		return db.PaymentIntent{}, err
	}
	defer tx.Rollback(dbCtx) //nolint:errcheck
	qtx := s.q.WithTx(tx)

	pi, err := s.lock(dbCtx, qtx, id, merchantID)
	if err != nil {
		return db.PaymentIntent{}, err
	}
	if amount > pi.Amount {
		return db.PaymentIntent{}, ErrRefundExceedsCapture
	}
	// A PARTIAL refund before settlement would leave acquirer_receivable = amount − refund and
	// flip the intent to the terminal partially_refunded, which the settle sweep never selects —
	// so that receivable would never convert to cash and the ledger would diverge from the bank.
	// Disallow it: refund in full pre-settle, or take the partial after settlement (cash side).
	if pi.Status == statemachine.StatusCaptured && amount < pi.Amount {
		return db.PaymentIntent{}, ErrPartialRefundBeforeSettle
	}
	next := statemachine.StatusPartiallyRefunded
	if amount == pi.Amount {
		next = statemachine.StatusRefunded
	}
	if !statemachine.CanTransition(pi.Status, next) {
		return db.PaymentIntent{}, ErrInvalidState
	}

	payable, err := s.account(dbCtx, qtx, merchantID, ledger.TypeLiability, ledger.KindMerchantPayable, pi.Currency)
	if err != nil {
		return db.PaymentIntent{}, err
	}
	// Credit acquirer_receivable pre-settle, platform_cash post-settle.
	creditKind := ledger.KindAcquirerReceivable
	if pi.Status == statemachine.StatusSettled {
		creditKind = ledger.KindPlatformCash
	}
	credit, err := s.account(dbCtx, qtx, merchantID, ledger.TypeAsset, creditKind, pi.Currency)
	if err != nil {
		return db.PaymentIntent{}, err
	}
	if _, err := ledger.PostTransaction(dbCtx, qtx, merchantID, "refund", &pi.ID, []ledger.Entry{
		{AccountID: payable, Debit: amount, Currency: pi.Currency},
		{AccountID: credit, Credit: amount, Currency: pi.Currency},
	}); err != nil {
		return db.PaymentIntent{}, err
	}

	updated, err := qtx.UpdateIntentStatus(dbCtx, db.UpdateIntentStatusParams{ID: pi.ID, Status: next})
	if err != nil {
		return db.PaymentIntent{}, err
	}
	if err := emit(dbCtx, qtx, updated); err != nil {
		return db.PaymentIntent{}, err
	}
	if err := tx.Commit(dbCtx); err != nil {
		return db.PaymentIntent{}, err
	}
	return updated, nil
}

// SettleDueIntents settles captured intents older than olderThan, one tx per intent, and returns
// how many settled. Models the acquirer's async batched payout: capture ≠ cash in hand; settle
// converts the receivable into platform_cash. Settle ≠ payout — merchant_payable still stands.
// Callable directly so tests force-run it without waiting on wall time.
func (s *Intents) SettleDueIntents(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-olderThan), Valid: true}
	due, err := s.q.ListCapturedBefore(ctx, db.ListCapturedBeforeParams{UpdatedAt: cutoff, Limit: sweepBatch})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, pi := range due {
		if err := s.settleOne(ctx, pi.ID, pi.MerchantID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Intents) settleOne(ctx context.Context, id, merchantID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	qtx := s.q.WithTx(tx)

	pi, err := s.lock(ctx, qtx, id, merchantID)
	if err != nil {
		return err
	}
	// Re-check under the lock: another worker may have settled it already.
	if pi.Status != statemachine.StatusCaptured {
		return nil
	}
	if !statemachine.CanTransition(pi.Status, statemachine.StatusSettled) {
		return ErrInvalidState
	}
	cash, err := s.account(ctx, qtx, merchantID, ledger.TypeAsset, ledger.KindPlatformCash, pi.Currency)
	if err != nil {
		return err
	}
	recv, err := s.account(ctx, qtx, merchantID, ledger.TypeAsset, ledger.KindAcquirerReceivable, pi.Currency)
	if err != nil {
		return err
	}
	if _, err := ledger.PostTransaction(ctx, qtx, merchantID, "settle", &pi.ID, []ledger.Entry{
		{AccountID: cash, Debit: pi.Amount, Currency: pi.Currency},
		{AccountID: recv, Credit: pi.Amount, Currency: pi.Currency},
	}); err != nil {
		return err
	}
	updated, err := qtx.UpdateIntentStatus(ctx, db.UpdateIntentStatusParams{ID: pi.ID, Status: statemachine.StatusSettled})
	if err != nil {
		return err
	}
	if err := emit(ctx, qtx, updated); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ExpireStaleIntents fails PRE-AUTHORIZATION intents (created/requires_confirmation) older than
// olderThan, so abandoned intents don't linger. Authorized intents are excluded on purpose: they
// hold funds at the acquirer and failing them without a bank void would leak the hold (see
// ListStaleBefore). No ledger post — nothing was captured.
func (s *Intents) ExpireStaleIntents(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-olderThan), Valid: true}
	stale, err := s.q.ListStaleBefore(ctx, db.ListStaleBeforeParams{UpdatedAt: cutoff, Limit: sweepBatch})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, pi := range stale {
		if err := s.expireOne(ctx, pi.ID, pi.MerchantID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Intents) expireOne(ctx context.Context, id, merchantID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	qtx := s.q.WithTx(tx)

	pi, err := s.lock(ctx, qtx, id, merchantID)
	if err != nil {
		return err
	}
	if !statemachine.CanTransition(pi.Status, statemachine.StatusFailed) {
		return nil // already advanced past a pre-capture state under the lock
	}
	updated, err := qtx.UpdateIntentStatus(ctx, db.UpdateIntentStatusParams{ID: pi.ID, Status: statemachine.StatusFailed})
	if err != nil {
		return err
	}
	if err := emit(ctx, qtx, updated); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// lock row-locks an intent scoped to its merchant, mapping "no rows" to ErrIntentNotFound.
func (s *Intents) lock(ctx context.Context, qtx *db.Queries, id, merchantID uuid.UUID) (db.PaymentIntent, error) {
	pi, err := qtx.LockIntentForUpdate(ctx, db.LockIntentForUpdateParams{ID: id, MerchantID: merchantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PaymentIntent{}, ErrIntentNotFound
	}
	return pi, err
}

// account get-or-creates a singleton (per merchant+currency) ledger account inside the tx.
func (s *Intents) account(ctx context.Context, qtx *db.Queries, merchantID uuid.UUID, typ, kind, currency string) (uuid.UUID, error) {
	a, err := qtx.GetOrCreateSingletonAccount(ctx, db.GetOrCreateSingletonAccountParams{
		MerchantID: merchantID, Type: typ, Kind: kind, Currency: currency,
	})
	return a.ID, err
}
