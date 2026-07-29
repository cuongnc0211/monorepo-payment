package outbox

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/repository/db"
)

func newEmitDB(t *testing.T) (*pgxpool.Pool, *db.Queries) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping outbox DB tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "TRUNCATE outbox CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool, db.New(pool)
}

func countOutbox(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM outbox").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// The heart of the outbox pattern: the event is bound to the caller's tx. Roll the tx back and
// the event must NOT exist — no phantom notifications for changes that never committed.
func TestEmitEvent_RollbackLeavesNoEvent(t *testing.T) {
	pool, q := newEmitDB(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := EmitEvent(ctx, q.WithTx(tx), uuid.New(), "payment_intent.captured", []byte(`{}`)); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if n := countOutbox(t, pool); n != 0 {
		t.Fatalf("rolled-back tx left %d outbox rows, want 0", n)
	}
}

func TestEmitEvent_CommitWritesPendingEvent(t *testing.T) {
	pool, q := newEmitDB(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := EmitEvent(ctx, q.WithTx(tx), uuid.New(), "payment_intent.captured", []byte(`{"a":1}`)); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if n := countOutbox(t, pool); n != 1 {
		t.Fatalf("want 1 outbox row, got %d", n)
	}
	var status, eventType string
	var eventID uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT status, event_type, event_id FROM outbox").Scan(&status, &eventType, &eventID); err != nil {
		t.Fatalf("read: %v", err)
	}
	if status != "pending" || eventType != "payment_intent.captured" || eventID == uuid.Nil {
		t.Fatalf("unexpected row: status=%s type=%s id=%s", status, eventType, eventID)
	}
}
