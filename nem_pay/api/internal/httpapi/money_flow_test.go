package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"
)

// createIntent is a helper: create a fresh intent and return its id.
func (f *apiFixture) createIntent(t *testing.T, amount int64, currency string) string {
	t.Helper()
	w := f.do(http.MethodPost, "/v1/payment_intents", f.secretTok, uuid.NewString(),
		`{"amount":`+strconv.FormatInt(amount, 10)+`,"currency":"`+currency+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("create want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var r intentResponse
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	return r.ID
}

func statusOf(t *testing.T, w interface{ Bytes() []byte }) string {
	t.Helper()
	var r intentResponse
	_ = json.Unmarshal(w.Bytes(), &r)
	return r.Status
}

func TestLifecycle_CreateConfirmCaptureRefund(t *testing.T) {
	f := newAPIFixture(t)
	id := f.createIntent(t, 250000, "USD")

	confirm := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/confirm", f.secretTok, uuid.NewString(), `{"token":"tok_ok"}`)
	if confirm.Code != http.StatusOK || statusOf(t, confirm.Body) != "authorized" {
		t.Fatalf("confirm want 200/authorized, got %d/%s", confirm.Code, statusOf(t, confirm.Body))
	}
	capture := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/capture", f.secretTok, uuid.NewString(), "")
	if capture.Code != http.StatusOK || statusOf(t, capture.Body) != "captured" {
		t.Fatalf("capture want 200/captured, got %d/%s", capture.Code, statusOf(t, capture.Body))
	}
	refund := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/refund", f.secretTok, uuid.NewString(), `{"amount":250000}`)
	if refund.Code != http.StatusOK || statusOf(t, refund.Body) != "refunded" {
		t.Fatalf("refund want 200/refunded, got %d/%s (%s)", refund.Code, statusOf(t, refund.Body), refund.Body.String())
	}
}

func TestConfirm_Declined(t *testing.T) {
	f := newAPIFixture(t)
	id := f.createIntent(t, 100, "USD")
	w := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/confirm", f.secretTok, uuid.NewString(), `{"token":"tok_declined"}`)
	if w.Code != http.StatusOK || statusOf(t, w.Body) != "failed" {
		t.Fatalf("declined confirm want 200/failed, got %d/%s", w.Code, statusOf(t, w.Body))
	}
}

func TestConfirm_TimeoutLeavesIntentUnchanged(t *testing.T) {
	f := newAPIFixture(t)
	id := f.createIntent(t, 100, "USD")

	to := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/confirm", f.secretTok, uuid.NewString(), `{"token":"tok_timeout"}`)
	if to.Code != http.StatusGatewayTimeout {
		t.Fatalf("timeout confirm want 504, got %d (%s)", to.Code, to.Body.String())
	}
	// The intent must be untouched (safe/reconcilable), still 'created'.
	got := f.do(http.MethodGet, "/v1/payment_intents/"+id, f.secretTok, "", "")
	if s := statusOf(t, got.Body); s != "created" {
		t.Fatalf("after timeout want status created, got %s", s)
	}
}

func TestCapture_IllegalStateRejected(t *testing.T) {
	f := newAPIFixture(t)
	id := f.createIntent(t, 100, "USD")
	// Capture straight after create (never authorized) → 409.
	w := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/capture", f.secretTok, uuid.NewString(), "")
	if w.Code != http.StatusConflict {
		t.Fatalf("illegal capture want 409, got %d", w.Code)
	}
}

func TestConfirm_IdempotentReplay(t *testing.T) {
	f := newAPIFixture(t)
	id := f.createIntent(t, 100, "USD")
	key := uuid.NewString()
	first := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/confirm", f.secretTok, key, `{"token":"tok_ok"}`)
	second := f.do(http.MethodPost, "/v1/payment_intents/"+id+"/confirm", f.secretTok, key, `{"token":"tok_ok"}`)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("want 200/200, got %d/%d", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay body differs")
	}
	if second.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("second confirm was not a replay")
	}
}
