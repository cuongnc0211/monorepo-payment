package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/apikey"
	"github.com/nempay/api/internal/banksim"
	"github.com/nempay/api/internal/repository/db"
)

// newBankStub starts an in-test double of bank-sim: /authorize honors the magic token
// (tok_declined → declined, tok_timeout → sleep past the client deadline, else approved) and
// /capture always approves. It lives here (not an import of the bank-sim service) because the
// api reaches the bank over HTTP only.
func newBankStub(t *testing.T) *banksim.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/authorize") {
			var body struct {
				Token string `json:"token"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			switch body.Token {
			case "tok_declined":
				_, _ = w.Write([]byte(`{"status":"declined"}`))
			case "tok_timeout":
				time.Sleep(500 * time.Millisecond) // > the 100ms client timeout below
				_, _ = w.Write([]byte(`{"status":"approved"}`))
			default:
				_, _ = w.Write([]byte(`{"status":"approved"}`))
			}
			return
		}
		_, _ = w.Write([]byte(`{"status":"approved"}`)) // /capture
	}))
	t.Cleanup(srv.Close)
	return banksim.New(srv.URL, 100*time.Millisecond)
}

// apiFixture spins up the real router over a test DB and seeds one merchant with a secret and a
// publishable key, returning the raw tokens for use in requests. Skips without TEST_DATABASE_URL.
type apiFixture struct {
	r          *gin.Engine
	pool       *pgxpool.Pool
	merchantID uuid.UUID
	secretTok  string
	pubTok     string
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping API DB tests")
	}
	gin.SetMode(gin.TestMode)
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE payment_intents, api_keys, merchants, idempotency_keys, outbox, webhook_endpoints, webhook_deliveries CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	q := db.New(pool)
	m, err := q.CreateMerchant(ctx, "Test Merchant")
	if err != nil {
		t.Fatalf("merchant: %v", err)
	}
	secretTok := "sk_test_" + uuid.NewString()
	pubTok := "pk_test_" + uuid.NewString()
	for _, k := range []struct{ kind, tok string }{{"secret", secretTok}, {"publishable", pubTok}} {
		if _, err := q.InsertAPIKey(ctx, db.InsertAPIKeyParams{
			MerchantID: m.ID, Kind: k.kind, TokenPrefix: apikey.Prefix(k.tok), TokenHash: apikey.Hash(k.tok),
		}); err != nil {
			t.Fatalf("key %s: %v", k.kind, err)
		}
	}
	return &apiFixture{r: NewRouter(pool, newBankStub(t)), pool: pool, merchantID: m.ID, secretTok: secretTok, pubTok: pubTok}
}

func (f *apiFixture) do(method, path, token, idemKey, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.r.ServeHTTP(w, req)
	return w
}

func TestCreateIntent_HappyPath(t *testing.T) {
	f := newAPIFixture(t)
	w := f.do(http.MethodPost, "/v1/payment_intents", f.secretTok, "k1", `{"amount":250000,"currency":"USD"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var resp intentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "created" || resp.SettlementMode != "direct" || resp.Amount != 250000 {
		t.Fatalf("unexpected intent: %+v", resp)
	}
	if resp.Object != "payment_intent" {
		t.Fatalf("want object payment_intent, got %q", resp.Object)
	}
}

func TestCreateIntent_PublishableKeyForbidden(t *testing.T) {
	f := newAPIFixture(t)
	w := f.do(http.MethodPost, "/v1/payment_intents", f.pubTok, "k1", `{"amount":100,"currency":"USD"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("publishable key on create want 403, got %d", w.Code)
	}
}

func TestCreateIntent_MissingKeyUnauthorized(t *testing.T) {
	f := newAPIFixture(t)
	w := f.do(http.MethodPost, "/v1/payment_intents", "", "k1", `{"amount":100,"currency":"USD"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no key want 401, got %d", w.Code)
	}
}

func TestCreateIntent_InvalidAmount(t *testing.T) {
	f := newAPIFixture(t)
	w := f.do(http.MethodPost, "/v1/payment_intents", f.secretTok, "k1", `{"amount":0,"currency":"USD"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("amount 0 want 400, got %d", w.Code)
	}
	assertErrorEnvelope(t, w.Body.Bytes())
}

func TestGetIntent_NotFoundAndEnvelopeConsistent(t *testing.T) {
	f := newAPIFixture(t)

	// 404 for an unknown id.
	nf := f.do(http.MethodGet, "/v1/payment_intents/"+uuid.NewString(), f.secretTok, "", "")
	if nf.Code != http.StatusNotFound {
		t.Fatalf("unknown intent want 404, got %d", nf.Code)
	}
	// The envelope shape is identical across a 404, a 400, and an auth error.
	assertErrorEnvelope(t, nf.Body.Bytes())
	badID := f.do(http.MethodGet, "/v1/payment_intents/not-a-uuid", f.secretTok, "", "")
	if badID.Code != http.StatusBadRequest {
		t.Fatalf("bad id want 400, got %d", badID.Code)
	}
	assertErrorEnvelope(t, badID.Body.Bytes())
	noAuth := f.do(http.MethodGet, "/v1/payment_intents", "", "", "")
	if noAuth.Code != http.StatusUnauthorized {
		t.Fatalf("no auth want 401, got %d", noAuth.Code)
	}
	assertErrorEnvelope(t, noAuth.Body.Bytes())
}

func TestGetIntent_RoundTrip(t *testing.T) {
	f := newAPIFixture(t)
	created := f.do(http.MethodPost, "/v1/payment_intents", f.secretTok, "k1", `{"amount":999,"currency":"EUR"}`)
	var resp intentResponse
	_ = json.Unmarshal(created.Body.Bytes(), &resp)

	got := f.do(http.MethodGet, "/v1/payment_intents/"+resp.ID, f.secretTok, "", "")
	if got.Code != http.StatusOK {
		t.Fatalf("get want 200, got %d (%s)", got.Code, got.Body.String())
	}
	var back intentResponse
	_ = json.Unmarshal(got.Body.Bytes(), &back)
	if back.ID != resp.ID || back.Amount != 999 || back.Currency != "EUR" {
		t.Fatalf("round trip mismatch: %+v", back)
	}
}

func TestListIntents(t *testing.T) {
	f := newAPIFixture(t)
	for i, cur := range []string{"USD", "USD", "GBP"} {
		_ = i
		f.do(http.MethodPost, "/v1/payment_intents", f.secretTok, uuid.NewString(), `{"amount":100,"currency":"`+cur+`"}`)
	}
	w := f.do(http.MethodGet, "/v1/payment_intents", f.secretTok, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list want 200, got %d", w.Code)
	}
	var out struct {
		Object string           `json:"object"`
		Data   []intentResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Object != "list" || len(out.Data) != 3 {
		t.Fatalf("want list of 3, got object=%q n=%d", out.Object, len(out.Data))
	}
}

// One merchant must never read another's intent, even with the exact id. The scoping lives in
// SQL (GetIntent WHERE merchant_id = $2); this proves it end to end.
func TestGetIntent_CrossTenantIsolation(t *testing.T) {
	f := newAPIFixture(t)
	ctx := context.Background()

	// Merchant A creates an intent.
	created := f.do(http.MethodPost, "/v1/payment_intents", f.secretTok, "k1", `{"amount":500,"currency":"USD"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("create want 200, got %d", created.Code)
	}
	var a intentResponse
	_ = json.Unmarshal(created.Body.Bytes(), &a)

	// Merchant B (a second merchant + secret key in the same DB) tries to fetch A's intent.
	q := db.New(f.pool)
	mB, err := q.CreateMerchant(ctx, "Merchant B")
	if err != nil {
		t.Fatalf("merchant B: %v", err)
	}
	bTok := "sk_test_" + uuid.NewString()
	if _, err := q.InsertAPIKey(ctx, db.InsertAPIKeyParams{
		MerchantID: mB.ID, Kind: "secret", TokenPrefix: apikey.Prefix(bTok), TokenHash: apikey.Hash(bTok),
	}); err != nil {
		t.Fatalf("key B: %v", err)
	}

	w := f.do(http.MethodGet, "/v1/payment_intents/"+a.ID, bTok, "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant read want 404, got %d (%s)", w.Code, w.Body.String())
	}
}

// Unknown routes and wrong methods return the standard error envelope, not Gin's plaintext.
func TestUnknownRouteAndMethodUseEnvelope(t *testing.T) {
	f := newAPIFixture(t)

	nr := f.do(http.MethodGet, "/v1/does_not_exist", f.secretTok, "", "")
	if nr.Code != http.StatusNotFound {
		t.Fatalf("unknown route want 404, got %d", nr.Code)
	}
	assertErrorEnvelope(t, nr.Body.Bytes())

	nm := f.do(http.MethodDelete, "/v1/payment_intents", f.secretTok, "", "")
	if nm.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method want 405, got %d", nm.Code)
	}
	assertErrorEnvelope(t, nm.Body.Bytes())
}

// assertErrorEnvelope checks the { "error": { type, code, message } } shape — the same shape
// every failure returns, so a client parses one thing everywhere.
func assertErrorEnvelope(t *testing.T, body []byte) {
	t.Helper()
	var env struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("error body not JSON: %v (%s)", err, body)
	}
	if env.Error.Type == "" || env.Error.Code == "" || env.Error.Message == "" {
		t.Fatalf("incomplete error envelope: %s", body)
	}
}
