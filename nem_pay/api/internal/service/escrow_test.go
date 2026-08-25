package service

import (
	"context"
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

func ptr64(v int64) *int64 { return &v }
