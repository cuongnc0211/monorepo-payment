package devseed

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/nempay/api/internal/repository/db"
)

func newSeedDB(t *testing.T) (*pgxpool.Pool, *db.Queries) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping devseed DB tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE api_keys, webhook_endpoints, merchants CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool, db.New(pool)
}

func TestRun_SeedsWebhookEndpointIdempotently(t *testing.T) {
	pool, q := newSeedDB(t)
	ctx := context.Background()
	merchantID := uuid.MustParse(fixedMerchantStr)

	t.Setenv("NEMPAY_DEV_WEBHOOK_URL", "http://host.docker.internal:3000/webhooks/nem_pay")
	t.Setenv("NEMPAY_DEV_WEBHOOK_SECRET", "whsec_test")

	if err := Run(ctx, pool); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}

	eps, err := q.ListActiveEndpoints(ctx, merchantID)
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("want exactly 1 webhook endpoint after two runs, got %d", len(eps))
	}
	if eps[0].Url != "http://host.docker.internal:3000/webhooks/nem_pay" || eps[0].Secret != "whsec_test" {
		t.Fatalf("unexpected endpoint: %+v", eps[0])
	}
}

func TestRun_NoWebhookEndpointWhenEnvUnset(t *testing.T) {
	pool, q := newSeedDB(t)
	ctx := context.Background()
	merchantID := uuid.MustParse(fixedMerchantStr)

	// Ensure the vars are unset for this test.
	os.Unsetenv("NEMPAY_DEV_WEBHOOK_URL")
	os.Unsetenv("NEMPAY_DEV_WEBHOOK_SECRET")

	if err := Run(ctx, pool); err != nil {
		t.Fatalf("run: %v", err)
	}
	eps, err := q.ListActiveEndpoints(ctx, merchantID)
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(eps) != 0 {
		t.Fatalf("want no webhook endpoint when env unset, got %d", len(eps))
	}
}

// Seeds a portal user for each of the two merchants, idempotently, with a bcrypt-verifiable
// password — the data foundation for the portal's tenant-isolation tests (spec 005 AC2/AC3).
func TestRun_SeedsPortalUsersIdempotently(t *testing.T) {
	pool, q := newSeedDB(t)
	ctx := context.Background()

	if err := Run(ctx, pool); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}

	userA, err := q.GetUserByEmail(ctx, devUserAEmail)
	if err != nil {
		t.Fatalf("get user A: %v", err)
	}
	userB, err := q.GetUserByEmail(ctx, devUserBEmail)
	if err != nil {
		t.Fatalf("get user B: %v", err)
	}

	// Each user is bound to its own merchant — the two must differ (tenant isolation setup).
	if userA.MerchantID != uuid.MustParse(fixedMerchantStr) {
		t.Fatalf("user A merchant = %s, want %s", userA.MerchantID, fixedMerchantStr)
	}
	if userB.MerchantID != uuid.MustParse(fixedMerchantBStr) {
		t.Fatalf("user B merchant = %s, want %s", userB.MerchantID, fixedMerchantBStr)
	}
	if userA.MerchantID == userB.MerchantID {
		t.Fatal("the two dev users must belong to different merchants")
	}

	// The stored hash verifies against the known dev password, and no plaintext is stored.
	if err := bcrypt.CompareHashAndPassword([]byte(userA.PasswordHash), []byte(devUserPassword)); err != nil {
		t.Fatalf("password hash does not verify: %v", err)
	}
	if userA.PasswordHash == devUserPassword {
		t.Fatal("password stored in plaintext")
	}

	// Idempotent: a second Run did not duplicate either user.
	for _, email := range []string{devUserAEmail, devUserBEmail} {
		n, err := q.CountUsersByEmail(ctx, email)
		if err != nil {
			t.Fatalf("count %s: %v", email, err)
		}
		if n != 1 {
			t.Fatalf("want exactly 1 user for %s after two runs, got %d", email, n)
		}
	}
}
