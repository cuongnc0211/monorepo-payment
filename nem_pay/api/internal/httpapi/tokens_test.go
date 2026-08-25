package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// tokenResp is the subset of the /v1/tokens response the tests assert on.
type tokenResp struct {
	ID   string `json:"id"`
	Card struct {
		Brand string `json:"brand"`
		Last4 string `json:"last4"`
	} `json:"card"`
}

func TestTokens_PublishableMapsTestCards(t *testing.T) {
	f := newAPIFixture(t)
	cases := []struct{ number, wantTok, wantLast4 string }{
		{"4242 4242 4242 4242", "tok_ok", "4242"},
		{"4000000000000002", "tok_declined", "0002"},
		{"4000000000000069", "tok_timeout", "0069"},
	}
	for _, tc := range cases {
		body := `{"number":"` + tc.number + `","exp_month":12,"exp_year":2030,"cvc":"123"}`
		w := f.do(http.MethodPost, "/v1/tokens", f.pubTok, "", body)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d (%s)", tc.number, w.Code, w.Body.String())
		}
		var resp tokenResp
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.ID != tc.wantTok || resp.Card.Last4 != tc.wantLast4 {
			t.Fatalf("%s: got id=%s last4=%s, want id=%s last4=%s",
				tc.number, resp.ID, resp.Card.Last4, tc.wantTok, tc.wantLast4)
		}
	}
}

func TestTokens_SecretKeyRefused(t *testing.T) {
	f := newAPIFixture(t)
	body := `{"number":"4242424242424242","exp_month":12,"exp_year":2030,"cvc":"123"}`
	w := f.do(http.MethodPost, "/v1/tokens", f.secretTok, "", body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("secret key must be refused on /v1/tokens: want 403, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestTokens_InvalidCardNumber(t *testing.T) {
	f := newAPIFixture(t)
	body := `{"number":"1234567890123","exp_month":12,"exp_year":2030,"cvc":"123"}`
	w := f.do(http.MethodPost, "/v1/tokens", f.pubTok, "", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for a bad card number, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestTokens_CORSPreflight(t *testing.T) {
	f := newAPIFixture(t)
	req := httptest.NewRequest(http.MethodOptions, "/v1/tokens", strings.NewReader(""))
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	f.r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight: want 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("preflight: Access-Control-Allow-Origin = %q, want the merchant origin", got)
	}
}
