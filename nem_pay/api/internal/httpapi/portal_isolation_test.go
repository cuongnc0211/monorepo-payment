package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/nempay/api/internal/apikey"
	"github.com/nempay/api/internal/repository/db"
)

// twoMerchants stands up two full tenants (A and B), each with a portal user, a secret key, a
// payment intent, and a webhook event, so tenant isolation can be exercised end to end.
type twoMerchants struct {
	r                *gin.Engine
	aToken           string // portal session for merchant A
	aSecret          string // A's raw secret key (must never leak via reads)
	aIntent, bIntent uuid.UUID
	aEvent, bEvent   uuid.UUID
	aKeyID, bKeyID   uuid.UUID
}

func newTwoMerchants(t *testing.T) *twoMerchants {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping portal isolation tests")
	}
	gin.SetMode(gin.TestMode)
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE payment_intents, api_keys, users, merchants, outbox, webhook_deliveries, webhook_endpoints CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	q := db.New(pool)

	f := &twoMerchants{r: NewRouter(pool, newBankStub(t))}
	const password = "pw-abcdef"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)

	mkMerchant := func(name, email, secret string) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
		m, err := q.CreateMerchant(ctx, name)
		if err != nil {
			t.Fatalf("merchant: %v", err)
		}
		if _, err := q.InsertUser(ctx, db.InsertUserParams{MerchantID: m.ID, Email: email, PasswordHash: string(hash)}); err != nil {
			t.Fatalf("user: %v", err)
		}
		key, err := q.InsertAPIKey(ctx, db.InsertAPIKeyParams{
			MerchantID: m.ID, Kind: "secret", TokenPrefix: apikey.Prefix(secret), TokenHash: apikey.Hash(secret),
		})
		if err != nil {
			t.Fatalf("key: %v", err)
		}
		var intentID, eventID uuid.UUID
		if err := pool.QueryRow(ctx,
			"INSERT INTO payment_intents (merchant_id, amount, currency) VALUES ($1,$2,$3) RETURNING id",
			m.ID, 250000, "USD").Scan(&intentID); err != nil {
			t.Fatalf("intent: %v", err)
		}
		if err := pool.QueryRow(ctx,
			"INSERT INTO outbox (merchant_id, event_id, event_type, payload) VALUES ($1,$2,$3,'{}') RETURNING event_id",
			m.ID, uuid.New(), "payment_intent.captured").Scan(&eventID); err != nil {
			t.Fatalf("outbox: %v", err)
		}
		return m.ID, key.ID, intentID, eventID
	}

	// Realistic-length secrets: longer than the stored prefix, so the full key is never a substring
	// of the masked prefix the endpoint returns.
	aSecret := "sk_test_" + strings.Repeat("a", 24)
	bSecret := "sk_test_" + strings.Repeat("b", 24)
	_, f.aKeyID, f.aIntent, f.aEvent = mkMerchant("Merchant A", "a@test.example", aSecret)
	_, f.bKeyID, f.bIntent, f.bEvent = mkMerchant("Merchant B", "b@test.example", bSecret)
	f.aSecret = aSecret

	// Sign in as merchant A to get a portal session token.
	w := f.get(http.MethodPost, "/v1/portal/login", "", `{"email":"a@test.example","password":"pw-abcdef"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("login A: %d (%s)", w.Code, w.Body.String())
	}
	var lr struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &lr)
	f.aToken = lr.Token
	return f
}

func (f *twoMerchants) get(method, path, bearer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.r.ServeHTTP(w, req)
	return w
}

// AC2: a session for A sees only A's data; B's resources are 404 by id, never disclosed.
func TestPortalIsolation_ReadsAreTenantScoped(t *testing.T) {
	f := newTwoMerchants(t)

	// Payments list: A's intent present, B's absent.
	body := f.get(http.MethodGet, "/v1/payment_intents", f.aToken, "").Body.String()
	if !strings.Contains(body, f.aIntent.String()) || strings.Contains(body, f.bIntent.String()) {
		t.Fatalf("payments list leaked across tenants: %s", body)
	}

	// B's intent by id → 404 (both detail and ledger).
	if code := f.get(http.MethodGet, "/v1/payment_intents/"+f.bIntent.String(), f.aToken, "").Code; code != http.StatusNotFound {
		t.Fatalf("A fetching B's intent: want 404, got %d", code)
	}
	if code := f.get(http.MethodGet, "/v1/payment_intents/"+f.bIntent.String()+"/ledger", f.aToken, "").Code; code != http.StatusNotFound {
		t.Fatalf("A fetching B's ledger: want 404, got %d", code)
	}

	// Webhook events: A's event present, B's absent.
	wh := f.get(http.MethodGet, "/v1/webhook_events", f.aToken, "").Body.String()
	if !strings.Contains(wh, f.aEvent.String()) || strings.Contains(wh, f.bEvent.String()) {
		t.Fatalf("webhook events leaked across tenants: %s", wh)
	}

	// API keys: A's key present, B's absent.
	keys := f.get(http.MethodGet, "/v1/api_keys", f.aToken, "").Body.String()
	if !strings.Contains(keys, f.aKeyID.String()) || strings.Contains(keys, f.bKeyID.String()) {
		t.Fatalf("api keys leaked across tenants: %s", keys)
	}
}

// AC7/AC8: the api-keys endpoint never returns a secret or a hash.
func TestPortalIsolation_ApiKeysMasked(t *testing.T) {
	f := newTwoMerchants(t)
	body := f.get(http.MethodGet, "/v1/api_keys", f.aToken, "").Body.String()
	if strings.Contains(body, f.aSecret) || strings.Contains(body, "token_hash") {
		t.Fatalf("api keys response leaked a secret or hash: %s", body)
	}
}

// AC1: no credential → no data.
func TestPortalIsolation_UnauthenticatedDenied(t *testing.T) {
	f := newTwoMerchants(t)
	for _, path := range []string{"/v1/balances", "/v1/webhook_events", "/v1/api_keys", "/v1/payment_intents"} {
		if code := f.get(http.MethodGet, path, "", "").Code; code != http.StatusUnauthorized {
			t.Fatalf("%s without a credential: want 401, got %d", path, code)
		}
	}
}

// AC10: a portal session must not reach money-mutating routes.
func TestPortalIsolation_SessionCannotMoveMoney(t *testing.T) {
	f := newTwoMerchants(t)
	w := f.get(http.MethodPost, "/v1/payment_intents", f.aToken, `{"amount":1000,"currency":"USD"}`)
	if w.Code == http.StatusOK {
		t.Fatalf("a session must be refused on POST /v1/payment_intents; got 200")
	}
}
