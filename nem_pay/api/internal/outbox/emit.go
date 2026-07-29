// Package outbox is the write side of the notification plane. EmitEvent inserts an event row in
// the SAME database transaction as the state change it announces — the heart of the outbox
// pattern. A separate worker (internal/webhook) delivers the row later, out-of-band. Never call
// a merchant's HTTP endpoint from inside the money tx: that collapses the planes and loses events
// if the merchant is down.
package outbox

import (
	"context"

	"github.com/google/uuid"

	"github.com/nempay/api/internal/repository/db"
)

// EmitEvent writes one pending outbox row using the caller's tx-bound *db.Queries. The caller
// (the money service) passes the Queries it already bound to the money tx, so the event commits
// atomically with the status change — or rolls back with it. event_id is fresh and stable: it
// travels to the receiver on every (re)delivery so they can dedupe at-least-once delivery.
func EmitEvent(ctx context.Context, q *db.Queries, merchantID uuid.UUID, eventType string, payload []byte) error {
	_, err := q.InsertOutbox(ctx, db.InsertOutboxParams{
		MerchantID: merchantID,
		EventID:    uuid.New(),
		EventType:  eventType,
		Payload:    payload,
	})
	return err
}
