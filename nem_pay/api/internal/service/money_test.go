package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/banksim"
	"github.com/nempay/api/internal/ledger"
	"github.com/nempay/api/internal/repository/db"
	"github.com/nempay/api/internal/statemachine"
)

func newMoneyFixture(t *testing.T) (*Intents, *db.Queries, *pgxpool.Pool, uuid.UUID) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping money DB tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE entries, transactions, accounts, payment_intents, api_keys, merchants, idempotency_keys, outbox, webhook_endpoints, webhook_deliveries CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	q := db.New(pool)
	m, err := q.CreateMerchant(ctx, "Money Merchant")
	if err != nil {
		t.Fatalf("merchant: %v", err)
	}
	return NewIntents(pool, bankStub(t)), q, pool, m.ID
}

// bankStub is an httptest double honoring the magic tokens; capture always approves.
func bankStub(t *testing.T) *banksim.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/authorize") {
			var b struct {
				Token string `json:"token"`
			}
			_ = json.NewDecoder(r.Body).Decode(&b)
			switch b.Token {
			case "tok_declined":
				_, _ = w.Write([]byte(`{"status":"declined"}`))
			case "tok_timeout":
				time.Sleep(300 * time.Millisecond)
				_, _ = w.Write([]byte(`{"status":"approved"}`))
			default:
				_, _ = w.Write([]byte(`{"status":"approved"}`))
			}
			return
		}
		_, _ = w.Write([]byte(`{"status":"approved"}`))
	}))
	t.Cleanup(srv.Close)
	return banksim.New(srv.URL, 80*time.Millisecond)
}

func mkIntent(t *testing.T, q *db.Queries, merchantID uuid.UUID, amount int64) db.PaymentIntent {
	t.Helper()
	pi, err := q.CreateIntent(context.Background(), db.CreateIntentParams{
		MerchantID: merchantID, Amount: amount, Currency: "USD",
		SettlementMode: statemachine.SettlementDirect, Metadata: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("create intent: %v", err)
	}
	return pi
}

func bal(t *testing.T, q *db.Queries, merchantID uuid.UUID, typ, kind string) int64 {
	t.Helper()
	ctx := context.Background()
	a, err := q.GetOrCreateSingletonAccount(ctx, db.GetOrCreateSingletonAccountParams{
		MerchantID: merchantID, Type: typ, Kind: kind, Currency: "USD",
	})
	if err != nil {
		t.Fatalf("account %s: %v", kind, err)
	}
	b, err := ledger.Balance(ctx, q, a.ID)
	if err != nil {
		t.Fatalf("balance %s: %v", kind, err)
	}
	return b
}

// The full happy path drives the derived ledger balances exactly as the plan specifies.
func TestMoney_LifecycleBalances(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	const amt = 250000
	pi := mkIntent(t, q, merchant, amt)

	if _, err := s.Confirm(ctx, pi.ID, merchant, "tok_ok"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	// Authorize posts no ledger entries.
	if got := bal(t, q, merchant, ledger.TypeAsset, ledger.KindAcquirerReceivable); got != 0 {
		t.Fatalf("post-authorize receivable want 0, got %d", got)
	}

	if _, err := s.Capture(ctx, pi.ID, merchant); err != nil {
		t.Fatalf("capture: %v", err)
	}
	// capture: Dr acquirer_receivable / Cr merchant_payable.
	if got := bal(t, q, merchant, ledger.TypeAsset, ledger.KindAcquirerReceivable); got != amt {
		t.Fatalf("receivable want %d, got %d", amt, got)
	}
	if got := bal(t, q, merchant, ledger.TypeLiability, ledger.KindMerchantPayable); got != -amt {
		t.Fatalf("merchant_payable want %d, got %d", -amt, got)
	}
	if got := bal(t, q, merchant, ledger.TypeAsset, ledger.KindPlatformCash); got != 0 {
		t.Fatalf("platform_cash before settle want 0, got %d", got)
	}

	// settle sweep (olderThan 0 → the captured intent qualifies).
	n, err := s.SettleDueIntents(ctx, 0)
	if err != nil || n != 1 {
		t.Fatalf("settle want 1, got n=%d err=%v", n, err)
	}
	// settle: Dr platform_cash / Cr acquirer_receivable → receivable clears, cash rises,
	// merchant_payable still stands (settle ≠ payout).
	if got := bal(t, q, merchant, ledger.TypeAsset, ledger.KindAcquirerReceivable); got != 0 {
		t.Fatalf("receivable after settle want 0, got %d", got)
	}
	if got := bal(t, q, merchant, ledger.TypeAsset, ledger.KindPlatformCash); got != amt {
		t.Fatalf("platform_cash after settle want %d, got %d", amt, got)
	}
	if got := bal(t, q, merchant, ledger.TypeLiability, ledger.KindMerchantPayable); got != -amt {
		t.Fatalf("merchant_payable after settle want %d (payout deferred), got %d", -amt, got)
	}

	// full refund post-settle: Dr merchant_payable / Cr platform_cash → both clear to zero.
	if _, err := s.Refund(ctx, pi.ID, merchant, amt); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if got := bal(t, q, merchant, ledger.TypeAsset, ledger.KindPlatformCash); got != 0 {
		t.Fatalf("platform_cash after refund want 0, got %d", got)
	}
	if got := bal(t, q, merchant, ledger.TypeLiability, ledger.KindMerchantPayable); got != 0 {
		t.Fatalf("merchant_payable after refund want 0, got %d", got)
	}
}

func TestMoney_DeclinedNoLedger(t *testing.T) {
	s, q, pool, merchant := newMoneyFixture(t)
	ctx := context.Background()
	pi := mkIntent(t, q, merchant, 100)

	got, err := s.Confirm(ctx, pi.ID, merchant, "tok_declined")
	if err != nil {
		t.Fatalf("confirm(declined) returned error: %v", err)
	}
	if got.Status != statemachine.StatusFailed {
		t.Fatalf("want failed, got %s", got.Status)
	}
	var nTx int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transactions").Scan(&nTx); err != nil {
		t.Fatalf("count: %v", err)
	}
	if nTx != 0 {
		t.Fatalf("declined authorize posted %d ledger tx, want 0", nTx)
	}
}

func TestMoney_TimeoutLeavesUnchanged(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	pi := mkIntent(t, q, merchant, 100)

	_, err := s.Confirm(ctx, pi.ID, merchant, "tok_timeout")
	if err != ErrBankUnavailable {
		t.Fatalf("want ErrBankUnavailable, got %v", err)
	}
	got, err := s.Get(ctx, pi.ID, merchant)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != statemachine.StatusCreated {
		t.Fatalf("after timeout want status created, got %s", got.Status)
	}
}

func TestMoney_IllegalCaptureRejected(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	pi := mkIntent(t, q, merchant, 100) // status 'created', never authorized
	if _, err := s.Capture(ctx, pi.ID, merchant); err != ErrInvalidState {
		t.Fatalf("capture from created want ErrInvalidState, got %v", err)
	}
}

func TestMoney_RefundExceedsCapture(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	pi := mkIntent(t, q, merchant, 1000)
	mustConfirmCapture(t, s, pi.ID, merchant)
	if _, err := s.Refund(ctx, pi.ID, merchant, 2000); err != ErrRefundExceedsCapture {
		t.Fatalf("over-refund want ErrRefundExceedsCapture, got %v", err)
	}
}

// Concurrent captures on the same intent: exactly one succeeds (FOR UPDATE serialises them),
// exactly one balanced capture transaction results.
func TestMoney_ConcurrentCaptureSingleLedgerTx(t *testing.T) {
	s, q, pool, merchant := newMoneyFixture(t)
	ctx := context.Background()
	pi := mkIntent(t, q, merchant, 5000)
	if _, err := s.Confirm(ctx, pi.ID, merchant, "tok_ok"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	const N = 6
	var wg sync.WaitGroup
	var okCount int
	var mu sync.Mutex
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Capture(ctx, pi.ID, merchant); err == nil {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if okCount != 1 {
		t.Fatalf("want exactly 1 successful capture, got %d", okCount)
	}
	var nCap int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE kind='capture'").Scan(&nCap); err != nil {
		t.Fatalf("count: %v", err)
	}
	if nCap != 1 {
		t.Fatalf("want 1 capture ledger tx, got %d", nCap)
	}
}

// Property: every ledger transaction is balanced (Σdebit − Σcredit = 0 per transaction).
func TestMoney_EveryTransactionBalanced(t *testing.T) {
	s, q, pool, merchant := newMoneyFixture(t)
	ctx := context.Background()
	pi := mkIntent(t, q, merchant, 4200)
	mustConfirmCapture(t, s, pi.ID, merchant)
	if _, err := s.SettleDueIntents(ctx, 0); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if _, err := s.Refund(ctx, pi.ID, merchant, 4200); err != nil {
		t.Fatalf("refund: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT t.id, COALESCE(SUM(e.debit),0) - COALESCE(SUM(e.credit),0)
		FROM transactions t JOIN entries e ON e.transaction_id = t.id
		GROUP BY t.id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id uuid.UUID
		var net int64
		if err := rows.Scan(&id, &net); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if net != 0 {
			t.Fatalf("transaction %s is unbalanced: net=%d", id, net)
		}
		n++
	}
	if n != 3 { // capture, settle, refund
		t.Fatalf("want 3 transactions, got %d", n)
	}
}

// Two DIFFERENT intents of the SAME merchant captured concurrently both first-create the shared
// singleton accounts. This is the get-or-create race (they don't share the intent lock); both
// must succeed and the accounts must be created exactly once.
func TestMoney_ConcurrentFirstCaptureTwoIntents(t *testing.T) {
	s, q, pool, merchant := newMoneyFixture(t)
	ctx := context.Background()
	pi1 := mkIntent(t, q, merchant, 1000)
	pi2 := mkIntent(t, q, merchant, 2000)
	if _, err := s.Confirm(ctx, pi1.ID, merchant, "tok_ok"); err != nil {
		t.Fatalf("confirm 1: %v", err)
	}
	if _, err := s.Confirm(ctx, pi2.ID, merchant, "tok_ok"); err != nil {
		t.Fatalf("confirm 2: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []uuid.UUID{pi1.ID, pi2.ID} {
		wg.Add(1)
		go func(i int, id uuid.UUID) {
			defer wg.Done()
			_, errs[i] = s.Capture(ctx, id, merchant)
		}(i, id)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent capture %d failed: %v", i, err)
		}
	}
	// Exactly one acquirer_receivable + one merchant_payable account for the merchant.
	var nAcct int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM accounts WHERE merchant_id=$1 AND kind IN ('acquirer_receivable','merchant_payable')",
		merchant).Scan(&nAcct); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if nAcct != 2 {
		t.Fatalf("want 2 singleton accounts, got %d", nAcct)
	}
}

// A partial refund before settlement is refused (it would strand the receivable); a full refund
// pre-settle is allowed.
func TestMoney_PartialRefundBeforeSettleRejected(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	pi := mkIntent(t, q, merchant, 1000)
	mustConfirmCapture(t, s, pi.ID, merchant)

	if _, err := s.Refund(ctx, pi.ID, merchant, 400); err != ErrPartialRefundBeforeSettle {
		t.Fatalf("partial refund pre-settle want ErrPartialRefundBeforeSettle, got %v", err)
	}
	// Full refund pre-settle is fine.
	if _, err := s.Refund(ctx, pi.ID, merchant, 1000); err != nil {
		t.Fatalf("full refund pre-settle: %v", err)
	}
}

// A partial refund AFTER settlement is allowed and reverses the cash side exactly.
func TestMoney_PartialRefundAfterSettle(t *testing.T) {
	s, q, _, merchant := newMoneyFixture(t)
	ctx := context.Background()
	const amt = 1000
	pi := mkIntent(t, q, merchant, amt)
	mustConfirmCapture(t, s, pi.ID, merchant)
	if _, err := s.SettleDueIntents(ctx, 0); err != nil {
		t.Fatalf("settle: %v", err)
	}
	got, err := s.Refund(ctx, pi.ID, merchant, 400)
	if err != nil {
		t.Fatalf("partial refund post-settle: %v", err)
	}
	if got.Status != statemachine.StatusPartiallyRefunded {
		t.Fatalf("want partially_refunded, got %s", got.Status)
	}
	// cash: amt − 400 ; merchant_payable: −(amt − 400)
	if b := bal(t, q, merchant, ledger.TypeAsset, ledger.KindPlatformCash); b != amt-400 {
		t.Fatalf("platform_cash want %d, got %d", amt-400, b)
	}
	if b := bal(t, q, merchant, ledger.TypeLiability, ledger.KindMerchantPayable); b != -(amt - 400) {
		t.Fatalf("merchant_payable want %d, got %d", -(amt - 400), b)
	}
}

// A captured intent writes exactly one 'payment_intent.captured' outbox event, in the same tx.
func TestMoney_CaptureEmitsOutboxEvent(t *testing.T) {
	s, q, pool, merchant := newMoneyFixture(t)
	ctx := context.Background()
	pi := mkIntent(t, q, merchant, 1000)
	mustConfirmCapture(t, s, pi.ID, merchant)

	var nCaptured int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM outbox WHERE event_type='payment_intent.captured' AND status='pending'").Scan(&nCaptured); err != nil {
		t.Fatalf("count: %v", err)
	}
	if nCaptured != 1 {
		t.Fatalf("want 1 captured event, got %d", nCaptured)
	}
	// confirm also emitted an authorized event → 2 events total for this intent.
	var nTotal int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM outbox").Scan(&nTotal)
	if nTotal != 2 {
		t.Fatalf("want 2 outbox events (authorized+captured), got %d", nTotal)
	}
}

func mustConfirmCapture(t *testing.T, s *Intents, id, merchant uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Confirm(ctx, id, merchant, "tok_ok"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if _, err := s.Capture(ctx, id, merchant); err != nil {
		t.Fatalf("capture: %v", err)
	}
}
