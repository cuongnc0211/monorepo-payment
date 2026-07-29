package webhook

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/repository/db"
)

func newDeliverDB(t *testing.T) (*pgxpool.Pool, *db.Queries) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping webhook DB tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE webhook_deliveries, outbox, webhook_endpoints, merchants CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool, db.New(pool)
}

// seed makes a merchant, an endpoint pointing at url with secret, and one pending outbox row.
func seed(t *testing.T, q *db.Queries, endpointURL, secret string, payload []byte) (uuid.UUID, db.Outbox) {
	t.Helper()
	ctx := context.Background()
	m, err := q.CreateMerchant(ctx, "Webhook Merchant")
	if err != nil {
		t.Fatalf("merchant: %v", err)
	}
	if endpointURL != "" {
		if _, err := q.InsertWebhookEndpoint(ctx, db.InsertWebhookEndpointParams{
			MerchantID: m.ID, Url: endpointURL, Secret: secret,
		}); err != nil {
			t.Fatalf("endpoint: %v", err)
		}
	}
	row, err := q.InsertOutbox(ctx, db.InsertOutboxParams{
		MerchantID: m.ID, EventID: uuid.New(), EventType: "payment_intent.captured", Payload: payload,
	})
	if err != nil {
		t.Fatalf("outbox: %v", err)
	}
	return m.ID, row
}

func TestProcessDue_DeliversSignedPayload(t *testing.T) {
	pool, q := newDeliverDB(t)
	ctx := context.Background()
	const secret = "whsec_abc"
	payload := []byte(`{"object":"payment_intent","status":"captured"}`)

	var gotBody []byte
	var gotSig, gotEventID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get(SignatureHeader)
		gotEventID = r.Header.Get(EventIDHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, row := seed(t, q, srv.URL, secret, payload)

	p := NewProcessor(pool)
	n, err := p.ProcessDue(ctx, 10)
	if err != nil || n != 1 {
		t.Fatalf("ProcessDue n=%d err=%v", n, err)
	}

	if !Verify(secret, gotBody, gotSig) {
		t.Fatalf("delivered signature does not verify: sig=%q", gotSig)
	}
	if gotEventID != row.EventID.String() {
		t.Fatalf("event-id header %q != row event_id %q", gotEventID, row.EventID)
	}
	got, _ := q.GetOutbox(ctx, row.ID)
	if got.Status != "delivered" {
		t.Fatalf("outbox status want delivered, got %s", got.Status)
	}
	if nd := countDeliveries(t, pool, row.ID); nd != 1 {
		t.Fatalf("want 1 delivery log row, got %d", nd)
	}
}

func TestProcessDue_RetryThenDeadWithBackoff(t *testing.T) {
	pool, q := newDeliverDB(t)
	ctx := context.Background()

	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError) // always fail
	}))
	defer srv.Close()

	_, row := seed(t, q, srv.URL, "s", []byte(`{}`))

	p := NewProcessor(pool)
	p.maxAttempts = 3
	p.baseBackoff = 0 // so the row is immediately due again for the next ProcessDue
	p.leaseSeconds = 0
	// Write next_attempt_at safely in the past so the "due" comparison is robust against any
	// skew between this host's clock and the DB container's clock.
	p.now = func() time.Time { return time.Now().Add(-time.Hour) }

	// Attempt 1 and 2 → still pending, attempts grow, next_attempt_at set.
	for i := 1; i <= 2; i++ {
		if _, err := p.ProcessDue(ctx, 10); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		got, _ := q.GetOutbox(ctx, row.ID)
		if got.Status != "pending" || int(got.Attempts) != i {
			t.Fatalf("after attempt %d want pending/attempts=%d, got %s/%d", i, i, got.Status, got.Attempts)
		}
	}
	// Attempt 3 → dead.
	if _, err := p.ProcessDue(ctx, 10); err != nil {
		t.Fatalf("attempt 3: %v", err)
	}
	got, _ := q.GetOutbox(ctx, row.ID)
	if got.Status != "dead" || got.Attempts != 3 {
		t.Fatalf("want dead/attempts=3, got %s/%d", got.Status, got.Attempts)
	}
	if hits != 3 {
		t.Fatalf("endpoint hit %d times, want 3", hits)
	}
	if nd := countDeliveries(t, pool, row.ID); nd != 3 {
		t.Fatalf("want 3 delivery log rows, got %d", nd)
	}
	// A dead row is no longer due.
	n, _ := p.ProcessDue(ctx, 10)
	if n != 0 {
		t.Fatalf("dead row was re-claimed: n=%d", n)
	}
}

func TestDeliverByID_SkipsAlreadyDelivered(t *testing.T) {
	pool, q := newDeliverDB(t)
	ctx := context.Background()

	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, row := seed(t, q, srv.URL, "s", []byte(`{}`))
	if err := q.MarkOutboxDelivered(ctx, row.ID); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if err := NewProcessor(pool).DeliverByID(ctx, row.ID); err != nil {
		t.Fatalf("DeliverByID: %v", err)
	}
	if hit {
		t.Fatalf("already-delivered row was re-delivered")
	}
}

func TestProcessDue_NoEndpointRetiresRow(t *testing.T) {
	pool, q := newDeliverDB(t)
	ctx := context.Background()
	_, row := seed(t, q, "", "", []byte(`{}`)) // no endpoint configured

	if _, err := NewProcessor(pool).ProcessDue(ctx, 10); err != nil {
		t.Fatalf("ProcessDue: %v", err)
	}
	got, _ := q.GetOutbox(ctx, row.ID)
	if got.Status != "delivered" {
		t.Fatalf("no-endpoint row want delivered (retired), got %s", got.Status)
	}
}

func countDeliveries(t *testing.T, pool *pgxpool.Pool, outboxID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM webhook_deliveries WHERE outbox_id=$1", outboxID).Scan(&n); err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	return n
}
