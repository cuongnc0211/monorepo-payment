package service

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/nempay/api/internal/ledger"
	"github.com/nempay/api/internal/repository/db"
	"github.com/nempay/api/internal/statemachine"
)

// mkEscrowIntent inserts an escrow intent directly (bypassing Create validation) so capture/
// settle/release tests can control payee and fee.
func mkEscrowIntent(t *testing.T, q *db.Queries, merchantID, payee uuid.UUID, amount, fee int64) db.PaymentIntent {
	t.Helper()
	pi, err := q.CreateIntent(context.Background(), db.CreateIntentParams{
		MerchantID: merchantID, Amount: amount, Currency: "USD",
		SettlementMode: statemachine.SettlementEscrow, PayeeID: &payee, ApplicationFee: &fee,
		Metadata: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("create escrow intent: %v", err)
	}
	return pi
}

// balRef returns the derived balance of a per-reference account (e.g. escrow_liability(intent)).
func balRef(t *testing.T, q *db.Queries, merchantID uuid.UUID, typ, kind string, ref uuid.UUID) int64 {
	t.Helper()
	ctx := context.Background()
	a, err := q.GetOrCreatePerRefAccount(ctx, db.GetOrCreatePerRefAccountParams{
		MerchantID: merchantID, Type: typ, Kind: kind, Currency: "USD", ReferenceID: &ref,
	})
	if err != nil {
		t.Fatalf("per-ref account %s: %v", kind, err)
	}
	b, err := ledger.Balance(ctx, q, a.ID)
	if err != nil {
		t.Fatalf("balance %s: %v", kind, err)
	}
	return b
}

func TestEscrow_CreateValidation(t *testing.T) {
	s, _, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	payee := uuid.New()

	// Missing payee → rejected.
	if _, err := s.Create(ctx, merchant, CreateInput{Amount: 1000, Currency: "USD", Escrow: true, ApplicationFee: ptr64(100)}); err != ErrPayeeRequired {
		t.Fatalf("missing payee want ErrPayeeRequired, got %v", err)
	}
	// Fee > amount → rejected.
	if _, err := s.Create(ctx, merchant, CreateInput{Amount: 1000, Currency: "USD", Escrow: true, Payee: &payee, ApplicationFee: ptr64(2000)}); err != ErrInvalidFee {
		t.Fatalf("fee>amount want ErrInvalidFee, got %v", err)
	}
	// Valid escrow → stored with mode/payee/fee.
	pi, err := s.Create(ctx, merchant, CreateInput{Amount: 1000, Currency: "USD", Escrow: true, Payee: &payee, ApplicationFee: ptr64(150)})
	if err != nil {
		t.Fatalf("valid escrow create: %v", err)
	}
	if pi.SettlementMode != statemachine.SettlementEscrow || pi.PayeeID == nil || *pi.PayeeID != payee ||
		pi.ApplicationFee == nil || *pi.ApplicationFee != 150 {
		t.Fatalf("escrow fields not stored: mode=%s payee=%v fee=%v", pi.SettlementMode, pi.PayeeID, pi.ApplicationFee)
	}
	// fee == amount is allowed.
	if _, err := s.Create(ctx, merchant, CreateInput{Amount: 1000, Currency: "USD", Escrow: true, Payee: &payee, ApplicationFee: ptr64(1000)}); err != nil {
		t.Fatalf("fee==amount should be allowed, got %v", err)
	}
}

// Escrow capture holds the amount as a per-intent liability (not merchant_payable), backed by the
// acquirer receivable. The intent moves to `captured`.
func TestEscrow_CaptureHoldsAsLiability(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	const amt = 250000
	payee := uuid.New()
	pi := mkEscrowIntent(t, q, merchant, payee, amt, 5000)

	if _, err := s.Confirm(ctx, pi.ID, merchant, "tok_ok"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	got, err := s.Capture(ctx, pi.ID, merchant)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if got.Status != statemachine.StatusCaptured {
		t.Fatalf("want captured, got %s", got.Status)
	}
	// acquirer_receivable (asset) = +amt ; escrow_liability(intent) (liability) = −amt.
	if b := bal(t, q, merchant, ledger.TypeAsset, ledger.KindAcquirerReceivable); b != amt {
		t.Fatalf("acquirer_receivable want %d, got %d", amt, b)
	}
	if b := balRef(t, q, merchant, ledger.TypeLiability, ledger.KindEscrowLiability, pi.ID); b != -amt {
		t.Fatalf("escrow_liability want %d, got %d", -amt, b)
	}
	// No merchant_payable created for an escrow intent.
	if b := bal(t, q, merchant, ledger.TypeLiability, ledger.KindMerchantPayable); b != 0 {
		t.Fatalf("merchant_payable should be untouched for escrow, got %d", b)
	}
}

// The settle sweep moves an escrow intent's funds into a segregated account and holds them
// (→ held_in_escrow); the escrow_liability stays, now backed by segregated cash, not the receivable.
func TestEscrow_SettleIntoSegregation(t *testing.T) {
	s, q, pool, merchant := newMoneyFixture(t)
	ctx := context.Background()
	const amt = 250000
	payee := uuid.New()
	pi := mkEscrowIntent(t, q, merchant, payee, amt, 5000)
	mustConfirmCapture(t, s, pi.ID, merchant)

	if b := bal(t, q, merchant, ledger.TypeAsset, ledger.KindSegregatedCash); b != 0 {
		t.Fatalf("segregated_cash before settle want 0, got %d", b)
	}

	n, err := s.SettleDueIntents(ctx, 0)
	if err != nil || n != 1 {
		t.Fatalf("settle want 1, got n=%d err=%v", n, err)
	}
	got, _ := s.Get(ctx, pi.ID, merchant)
	if got.Status != statemachine.StatusHeldInEscrow {
		t.Fatalf("want held_in_escrow, got %s", got.Status)
	}
	if b := bal(t, q, merchant, ledger.TypeAsset, ledger.KindSegregatedCash); b != amt {
		t.Fatalf("segregated_cash want %d, got %d", amt, b)
	}
	if b := bal(t, q, merchant, ledger.TypeAsset, ledger.KindAcquirerReceivable); b != 0 {
		t.Fatalf("acquirer_receivable after settle want 0, got %d", b)
	}
	if b := balRef(t, q, merchant, ledger.TypeLiability, ledger.KindEscrowLiability, pi.ID); b != -amt {
		t.Fatalf("escrow_liability after settle want %d (still held), got %d", -amt, b)
	}
	// One held_in_escrow event emitted.
	var n2 int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE event_type='payment_intent.held_in_escrow'").Scan(&n2)
	if n2 != 1 {
		t.Fatalf("want 1 held_in_escrow event, got %d", n2)
	}
}

// heldEscrow drives an escrow intent all the way to held_in_escrow.
func heldEscrow(t *testing.T, s *Intents, q *db.Queries, merchant, payee uuid.UUID, amt, fee int64) db.PaymentIntent {
	t.Helper()
	ctx := context.Background()
	pi := mkEscrowIntent(t, q, merchant, payee, amt, fee)
	mustConfirmCapture(t, s, pi.ID, merchant)
	if _, err := s.SettleDueIntents(ctx, 0); err != nil {
		t.Fatalf("settle: %v", err)
	}
	got, _ := s.Get(ctx, pi.ID, merchant)
	if got.Status != statemachine.StatusHeldInEscrow {
		t.Fatalf("setup: want held_in_escrow, got %s", got.Status)
	}
	return got
}

func TestEscrow_Release(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	const amt, fee = 250000, 5000
	payee := uuid.New()
	pi := heldEscrow(t, s, q, merchant, payee, amt, fee)

	got, err := s.Release(ctx, pi.ID, merchant)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if got.Status != statemachine.StatusReleased {
		t.Fatalf("want released, got %s", got.Status)
	}
	// escrow_liability=0, segregated_cash=0, platform_cash=+amt, payee=-(amt-fee), revenue=-fee.
	if b := balRef(t, q, merchant, ledger.TypeLiability, ledger.KindEscrowLiability, pi.ID); b != 0 {
		t.Fatalf("escrow_liability after release want 0, got %d", b)
	}
	if b := bal(t, q, merchant, ledger.TypeAsset, ledger.KindSegregatedCash); b != 0 {
		t.Fatalf("segregated_cash after release want 0, got %d", b)
	}
	if b := bal(t, q, merchant, ledger.TypeAsset, ledger.KindPlatformCash); b != amt {
		t.Fatalf("platform_cash after release want %d, got %d", amt, b)
	}
	if b := balRef(t, q, merchant, ledger.TypeLiability, ledger.KindPayableToPayee, payee); b != -(amt - fee) {
		t.Fatalf("payable_to_payee want %d, got %d", -(amt - fee), b)
	}
	if b := bal(t, q, merchant, ledger.TypeRevenue, ledger.KindPlatformRevenue); b != -fee {
		t.Fatalf("platform_revenue want %d, got %d", -fee, b)
	}
}

func TestEscrow_ReleaseFeeEdges(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()

	// fee == amount → payee accrues 0, all becomes revenue.
	p1 := uuid.New()
	pi1 := heldEscrow(t, s, q, merchant, p1, 1000, 1000)
	if _, err := s.Release(ctx, pi1.ID, merchant); err != nil {
		t.Fatalf("release fee==amount: %v", err)
	}
	if b := balRef(t, q, merchant, ledger.TypeLiability, ledger.KindPayableToPayee, p1); b != 0 {
		t.Fatalf("payee should accrue 0 when fee==amount, got %d", b)
	}
	if b := bal(t, q, merchant, ledger.TypeRevenue, ledger.KindPlatformRevenue); b != -1000 {
		t.Fatalf("revenue want -1000, got %d", b)
	}

	// fee == 0 → no revenue, payee gets all.
	p2 := uuid.New()
	pi2 := heldEscrow(t, s, q, merchant, p2, 1000, 0)
	if _, err := s.Release(ctx, pi2.ID, merchant); err != nil {
		t.Fatalf("release fee==0: %v", err)
	}
	if b := balRef(t, q, merchant, ledger.TypeLiability, ledger.KindPayableToPayee, p2); b != -1000 {
		t.Fatalf("payee want -1000 when fee==0, got %d", b)
	}
}

func TestEscrow_ReleaseIdempotentAndConcurrent(t *testing.T) {
	s, q, pool, merchant := newMoneyFixture(t)
	ctx := context.Background()
	payee := uuid.New()
	pi := heldEscrow(t, s, q, merchant, payee, 5000, 500)

	const N = 6
	var wg sync.WaitGroup
	var ok int64
	var mu sync.Mutex
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Release(ctx, pi.ID, merchant); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("want exactly 1 successful release, got %d", ok)
	}
	var nRel int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE kind='release'").Scan(&nRel); err != nil {
		t.Fatalf("count: %v", err)
	}
	if nRel != 1 {
		t.Fatalf("want 1 release ledger tx, got %d", nRel)
	}
	// A further release of the already-released intent is rejected.
	if _, err := s.Release(ctx, pi.ID, merchant); err != ErrInvalidState {
		t.Fatalf("re-release want ErrInvalidState, got %v", err)
	}
}

func TestEscrow_ReleaseNonHeldRejected(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()

	// Escrow intent only captured (not settled) → cannot release.
	payee := uuid.New()
	pi := mkEscrowIntent(t, q, merchant, payee, 1000, 100)
	mustConfirmCapture(t, s, pi.ID, merchant)
	if _, err := s.Release(ctx, pi.ID, merchant); err != ErrInvalidState {
		t.Fatalf("release captured (not held) want ErrInvalidState, got %v", err)
	}

	// Direct intent cannot be released.
	d := mkIntent(t, q, merchant, 1000)
	mustConfirmCapture(t, s, d.ID, merchant)
	if _, err := s.Release(ctx, d.ID, merchant); err != ErrInvalidState {
		t.Fatalf("release direct intent want ErrInvalidState, got %v", err)
	}
}

// Full refund from escrow returns the held money to the payer: the escrow liability is discharged
// and segregated cash reduced, both to zero; the intent becomes refunded.
func TestEscrow_FullRefundFromHeld(t *testing.T) {
	s, q, pool, merchant := newMoneyFixture(t)
	ctx := context.Background()
	const amt = 1000
	payee := uuid.New()
	pi := heldEscrow(t, s, q, merchant, payee, amt, 100)

	got, err := s.Refund(ctx, pi.ID, merchant, amt)
	if err != nil {
		t.Fatalf("full escrow refund: %v", err)
	}
	if got.Status != statemachine.StatusRefunded {
		t.Fatalf("want refunded, got %s", got.Status)
	}
	if b := balRef(t, q, merchant, ledger.TypeLiability, ledger.KindEscrowLiability, pi.ID); b != 0 {
		t.Fatalf("escrow_liability after refund want 0, got %d", b)
	}
	if b := bal(t, q, merchant, ledger.TypeAsset, ledger.KindSegregatedCash); b != 0 {
		t.Fatalf("segregated_cash after refund want 0, got %d", b)
	}
	var n int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE kind='refund' AND reference_id=$1", pi.ID).Scan(&n)
	if n != 1 {
		t.Fatalf("want 1 refund tx, got %d", n)
	}
}

func TestEscrow_PartialRefundRejected(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	payee := uuid.New()
	pi := heldEscrow(t, s, q, merchant, payee, 1000, 100)
	if _, err := s.Refund(ctx, pi.ID, merchant, 400); err != ErrPartialEscrowRefund {
		t.Fatalf("partial escrow refund want ErrPartialEscrowRefund, got %v", err)
	}
}

func TestEscrow_RefundAfterReleaseRejected(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	payee := uuid.New()
	pi := heldEscrow(t, s, q, merchant, payee, 1000, 100)
	if _, err := s.Release(ctx, pi.ID, merchant); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := s.Refund(ctx, pi.ID, merchant, 1000); err != ErrInvalidState {
		t.Fatalf("refund after release want ErrInvalidState, got %v", err)
	}
}

func ptr64(v int64) *int64 { return &v }
