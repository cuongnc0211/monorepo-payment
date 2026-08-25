package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/nempay/api/internal/repository/db"
)

// seedUser inserts a portal user for the fixture's merchant with a known password.
func (f *apiFixture) seedUser(t *testing.T, email, password string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := db.New(f.pool).InsertUser(context.Background(), db.InsertUserParams{
		MerchantID: f.merchantID, Email: email, PasswordHash: string(hash),
	}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func TestLogin_HappyPath(t *testing.T) {
	f := newAPIFixture(t)
	f.seedUser(t, "owner@test.example", "s3cret-pw")

	w := f.do(http.MethodPost, "/v1/portal/login", "", "",
		`{"email":"owner@test.example","password":"s3cret-pw"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Token    string `json:"token"`
		Merchant struct {
			ID string `json:"id"`
		} `json:"merchant"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected a session token")
	}
	if resp.Merchant.ID != f.merchantID.String() {
		t.Fatalf("merchant = %s, want %s", resp.Merchant.ID, f.merchantID)
	}
}

func TestLogin_WrongPasswordAndUnknownEmailBothReject(t *testing.T) {
	f := newAPIFixture(t)
	f.seedUser(t, "owner@test.example", "s3cret-pw")

	wrong := f.do(http.MethodPost, "/v1/portal/login", "", "",
		`{"email":"owner@test.example","password":"nope"}`)
	unknown := f.do(http.MethodPost, "/v1/portal/login", "", "",
		`{"email":"ghost@test.example","password":"whatever"}`)

	for name, w := range map[string]int{"wrong-password": wrong.Code, "unknown-email": unknown.Code} {
		if w != http.StatusUnauthorized {
			t.Fatalf("%s: want 401, got %d", name, w)
		}
	}
}

func TestSession_IssueVerifyRoundTrip(t *testing.T) {
	f := newAPIFixture(t)
	h := newSessionHandler(db.New(f.pool))

	tok, _, err := h.issueToken(f.merchantID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := h.verifyToken(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got != f.merchantID {
		t.Fatalf("verify merchant = %s, want %s", got, f.merchantID)
	}
	if _, err := h.verifyToken(tok + "tampered"); err == nil {
		t.Fatal("a tampered token must not verify")
	}
}

// A session token must never satisfy a money-mutating route (it is not an API key). This is the
// guard behind AC10: a browser session cannot move money.
func TestSession_RefusedOnMoneyRoute(t *testing.T) {
	f := newAPIFixture(t)
	f.seedUser(t, "owner@test.example", "s3cret-pw")

	login := f.do(http.MethodPost, "/v1/portal/login", "", "",
		`{"email":"owner@test.example","password":"s3cret-pw"}`)
	var lr struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &lr)

	w := f.do(http.MethodPost, "/v1/payment_intents", lr.Token, "idem-1",
		`{"amount":1000,"currency":"USD"}`)
	if w.Code == http.StatusOK {
		t.Fatalf("a session token must not be accepted on POST /v1/payment_intents; got 200")
	}
}
