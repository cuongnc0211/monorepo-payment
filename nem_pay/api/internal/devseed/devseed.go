// Package devseed inserts a known merchant + API keys for local development, so the gateway is
// drivable by curl the moment `docker-compose up` finishes — with no merchant app present
// (spec AC1). It runs ONLY when NEMPAY_DEV_SEED is set, and never from tests, so production and
// the test suite are never seeded with predictable credentials.
package devseed

import (
	"context"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/apikey"
	"github.com/nempay/api/internal/repository/db"
)

// Fixed, obviously-non-production dev identifiers. The raw tokens are printed on boot; only
// their hashes are stored.
const (
	devMerchantName  = "NemPay Dev Merchant"
	devSecretKey     = "sk_test_nempay_secret"
	devPublishKey    = "pk_test_nempay_publishable"
	fixedMerchantStr = "aaaaaaaa-0000-4000-8000-000000000001"
)

// Run seeds the dev merchant and keys idempotently. Safe to call on every boot.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	merchantID := uuid.MustParse(fixedMerchantStr)
	q := db.New(pool)

	// Upsert the merchant at a fixed id (raw exec: CreateMerchant generates a random id).
	if _, err := pool.Exec(ctx,
		`INSERT INTO merchants (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		merchantID, devMerchantName,
	); err != nil {
		return err
	}

	// Insert keys only if the merchant has none yet (keeps re-runs idempotent without a unique
	// constraint on token_hash).
	n, err := q.CountKeysForMerchant(ctx, merchantID)
	if err != nil {
		return err
	}
	if n == 0 {
		for _, k := range []struct{ kind, token string }{
			{"secret", devSecretKey},
			{"publishable", devPublishKey},
		} {
			if _, err := q.InsertAPIKey(ctx, db.InsertAPIKeyParams{
				MerchantID:  merchantID,
				Kind:        k.kind,
				TokenPrefix: apikey.Prefix(k.token),
				TokenHash:   apikey.Hash(k.token),
			}); err != nil {
				return err
			}
		}
	}

	// Register a webhook endpoint from env, so NemPay can deliver events to a merchant app (e.g.
	// NemLuxury) out of the box. The secret here MUST match the merchant's NEMPAY_WEBHOOK_SECRET.
	// Idempotent: inserted only if the merchant has no active endpoint yet. Skipped when unset, so
	// the standalone gateway is unchanged.
	if url, secret := os.Getenv("NEMPAY_DEV_WEBHOOK_URL"), os.Getenv("NEMPAY_DEV_WEBHOOK_SECRET"); url != "" && secret != "" {
		eps, err := q.ListActiveEndpoints(ctx, merchantID)
		if err != nil {
			return err
		}
		if len(eps) == 0 {
			if _, err := q.InsertWebhookEndpoint(ctx, db.InsertWebhookEndpointParams{
				MerchantID: merchantID, Url: url, Secret: secret,
			}); err != nil {
				return err
			}
		}
		log.Printf("dev webhook endpoint: %s", url)
	}

	log.Printf("WARNING: NEMPAY_DEV_SEED is set — inserting predictable dev credentials. "+
		"Never enable this outside local development. merchant=%s", merchantID)
	log.Printf("dev secret key:      %s", devSecretKey)
	log.Printf("dev publishable key: %s", devPublishKey)
	return nil
}
