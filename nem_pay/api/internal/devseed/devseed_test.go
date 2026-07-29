package devseed

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

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
