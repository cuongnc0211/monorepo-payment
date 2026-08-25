package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/nempay/api/internal/service"
)

// drive an escrow intent to held_in_escrow using the service directly (the settle sweep is not an
// HTTP endpoint), so the HTTP tests can exercise the /release endpoint itself.
func (f *apiFixture) holdEscrowViaService(t *testing.T, amount, fee int64, payee uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	svc := service.NewIntents(f.pool, newBankStub(t))
	pi, err := svc.Create(ctx, f.merchantID, service.CreateInput{
		Amount: amount, Currency: "USD", Escrow: true, Payee: &payee, ApplicationFee: &fee,
	})
	if err != nil {
		t.Fatalf("create escrow: %v", err)
	}
	if _, err := svc.Confirm(ctx, pi.ID, f.merchantID, "tok_ok"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if _, err := svc.Capture(ctx, pi.ID, f.merchantID); err != nil {
		t.Fatalf("capture: %v", err)
	}
	if _, err := svc.SettleDueIntents(ctx, 0); err != nil {
		t.Fatalf("settle: %v", err)
	}
	return pi.ID
}

func TestEscrowHTTP_CreateValidation(t *testing.T) {
	f := newAPIFixture(t)

	// Valid escrow create → 200 with escrow fields echoed.
	body := `{"amount":250000,"currency":"USD","escrow":true,"payee":"` + uuid.NewString() + `","application_fee":5000}`
	w := f.do(http.MethodPost, "/v1/payment_intents", f.secretTok, "e1", body)
	if w.Code != http.StatusOK {
		t.Fatalf("escrow create want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var resp intentResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.SettlementMode != "escrow" || resp.Payee == nil || resp.ApplicationFee == nil || *resp.ApplicationFee != 5000 {
		t.Fatalf("escrow fields not echoed: %+v", resp)
	}

	// Missing payee → 400.
	bad := `{"amount":1000,"currency":"USD","escrow":true,"application_fee":100}`
	if w := f.do(http.MethodPost, "/v1/payment_intents", f.secretTok, "e2", bad); w.Code != http.StatusBadRequest {
		t.Fatalf("escrow missing payee want 400, got %d", w.Code)
	}

	// fee > amount → 400.
	bad2 := `{"amount":1000,"currency":"USD","escrow":true,"payee":"` + uuid.NewString() + `","application_fee":2000}`
	if w := f.do(http.MethodPost, "/v1/payment_intents", f.secretTok, "e3", bad2); w.Code != http.StatusBadRequest {
		t.Fatalf("escrow fee>amount want 400, got %d", w.Code)
	}
}

func TestEscrowHTTP_ReleaseEndpoint(t *testing.T) {
	f := newAPIFixture(t)
	id := f.holdEscrowViaService(t, 250000, 5000, uuid.New()).String()

	// Release via HTTP → 200, released.
	rel := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/release", f.secretTok, "rel-1", "")
	if rel.Code != http.StatusOK {
		t.Fatalf("release want 200, got %d (%s)", rel.Code, rel.Body.String())
	}
	var resp intentResponse
	_ = json.Unmarshal(rel.Body.Bytes(), &resp)
	if resp.Status != "released" {
		t.Fatalf("want released, got %s", resp.Status)
	}

	// Same Idempotency-Key → replayed.
	replay := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/release", f.secretTok, "rel-1", "")
	if replay.Code != http.StatusOK || replay.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("release replay want 200+replayed, got %d", replay.Code)
	}

	// A distinct release of the now-released intent → 409.
	again := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/release", f.secretTok, "rel-2", "")
	if again.Code != http.StatusConflict {
		t.Fatalf("re-release want 409, got %d", again.Code)
	}
}

func TestEscrowHTTP_ReleaseRejectedForDirect(t *testing.T) {
	f := newAPIFixture(t)
	// A direct intent driven to captured cannot be released.
	svc := service.NewIntents(f.pool, newBankStub(t))
	ctx := context.Background()
	pi, _ := svc.Create(ctx, f.merchantID, service.CreateInput{Amount: 1000, Currency: "USD"})
	_, _ = svc.Confirm(ctx, pi.ID, f.merchantID, "tok_ok")
	_, _ = svc.Capture(ctx, pi.ID, f.merchantID)

	w := f.do(http.MethodPost, "/v1/payment_intents/"+pi.ID.String()+"/release", f.secretTok, "d1", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("release direct intent want 409, got %d", w.Code)
	}
}
