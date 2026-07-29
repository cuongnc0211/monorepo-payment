package webhook

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/repository/db"
)

// Headers carried on every delivery. Event-Id lets receivers dedupe at-least-once delivery;
// the signature proves authenticity.
const (
	EventIDHeader   = "X-NemPay-Event-Id"
	EventTypeHeader = "X-NemPay-Event-Type"
)

// Processor delivers due outbox rows to merchant endpoints and drives each row's delivery state
// (pending → delivered, or retry with backoff, or dead after maxAttempts). The DB outbox is the
// single retry authority; asynq (in cmd/worker) is only the transport that triggers ProcessDue.
type Processor struct {
	pool         *pgxpool.Pool
	q            *db.Queries
	hc           *http.Client
	maxAttempts  int
	baseBackoff  time.Duration
	leaseSeconds float64
	now          func() time.Time
}

// NewProcessor builds a processor with sensible delivery defaults.
func NewProcessor(pool *pgxpool.Pool) *Processor {
	return &Processor{
		pool:         pool,
		q:            db.New(pool),
		hc:           &http.Client{Timeout: 10 * time.Second}, // merchants handle webhooks synchronously — give them room
		maxAttempts:  6,
		baseBackoff:  1 * time.Second,
		leaseSeconds: 30,
		now:          time.Now,
	}
}

// ProcessDue claims up to limit due rows (leasing them) and delivers each. Returns the number
// claimed. Safe to run concurrently: ClaimDueOutbox uses FOR UPDATE SKIP LOCKED.
func (p *Processor) ProcessDue(ctx context.Context, limit int32) (int, error) {
	rows, err := p.q.ClaimDueOutbox(ctx, db.ClaimDueOutboxParams{LeaseSeconds: p.leaseSeconds, Lim: limit})
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		if err := p.deliverRow(ctx, row); err != nil {
			return 0, err // a DB-bookkeeping failure; the lease will make the row due again
		}
	}
	return len(rows), nil
}

// Dispatch claims up to limit due rows (leasing them) and hands each row id to enqueue — e.g.
// the worker pushes an asynq task, which later calls DeliverByID. This is the out-of-band path:
// the DB outbox owns retry/backoff/dead-letter; asynq is only the transport. ProcessDue is the
// inline equivalent (claim + deliver in one process) used by tests and simple deployments.
func (p *Processor) Dispatch(ctx context.Context, limit int32, enqueue func(uuid.UUID) error) (int, error) {
	rows, err := p.q.ClaimDueOutbox(ctx, db.ClaimDueOutboxParams{LeaseSeconds: p.leaseSeconds, Lim: limit})
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		if err := enqueue(row.ID); err != nil {
			return 0, err // lease lapses → row becomes due again → re-dispatched (at-least-once)
		}
	}
	return len(rows), nil
}

// DeliverByID delivers one outbox row if it is still pending (used by the asynq handler). A row
// already delivered/dead is a no-op, which dedupes SEQUENTIAL redelivery. Concurrent redelivery
// of the same row is prevented upstream by the dispatcher's per-row asynq TaskID; delivery is
// at-least-once regardless, so receivers still dedupe on event_id.
func (p *Processor) DeliverByID(ctx context.Context, id uuid.UUID) error {
	row, err := p.q.GetOutbox(ctx, id)
	if err != nil {
		return err
	}
	if row.Status != "pending" {
		return nil
	}
	return p.deliverRow(ctx, row)
}

func (p *Processor) deliverRow(ctx context.Context, row db.Outbox) error {
	endpoints, err := p.q.ListActiveEndpoints(ctx, row.MerchantID)
	if err != nil {
		return err
	}
	// No endpoint configured (e.g. the standalone gateway with no merchant): nothing to deliver
	// to, so retire the row rather than letting it accrue forever. M1 assumes at most one active
	// endpoint per merchant; multiple endpoints is a later lesson.
	if len(endpoints) == 0 {
		return p.q.MarkOutboxDelivered(ctx, row.ID)
	}
	ep := endpoints[0]

	attempt := int32(row.Attempts + 1)
	code, ok, deliverErr := p.post(ctx, ep.Url, ep.Secret, row)

	var codePtr *int32
	if code != 0 {
		c := int32(code)
		codePtr = &c
	}
	var errPtr *string
	if deliverErr != nil {
		s := deliverErr.Error()
		errPtr = &s
	}
	if logErr := p.q.InsertDelivery(ctx, db.InsertDeliveryParams{
		OutboxID: row.ID, Attempt: attempt, StatusCode: codePtr, Ok: ok, Error: errPtr,
	}); logErr != nil {
		return logErr
	}

	if ok {
		return p.q.MarkOutboxDelivered(ctx, row.ID)
	}
	reason := "non-2xx response"
	if deliverErr != nil {
		reason = deliverErr.Error()
	}
	if int(attempt) >= p.maxAttempts {
		return p.q.MarkOutboxDead(ctx, db.MarkOutboxDeadParams{ID: row.ID, LastError: &reason})
	}
	next := pgtype.Timestamptz{Time: p.now().Add(p.backoff(int(attempt))), Valid: true}
	return p.q.MarkOutboxRetry(ctx, db.MarkOutboxRetryParams{ID: row.ID, NextAttemptAt: next, LastError: &reason})
}

// post signs and POSTs the payload. Returns (statusCode, delivered-ok, transport-error). A 2xx is
// success; anything else (or a transport error) is a failure to retry.
func (p *Processor) post(ctx context.Context, url, secret string, row db.Outbox) (int, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(row.Payload))
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SignatureHeader, Sign(secret, row.Payload))
	req.Header.Set(EventIDHeader, row.EventID.String())
	req.Header.Set(EventTypeHeader, row.EventType)

	resp, err := p.hc.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !ok {
		return resp.StatusCode, false, fmt.Errorf("endpoint returned %d", resp.StatusCode)
	}
	return resp.StatusCode, true, nil
}

// backoff is exponential in the attempt number, capped, so a persistently-down endpoint is
// retried with growing gaps rather than hammered.
func (p *Processor) backoff(attempt int) time.Duration {
	d := p.baseBackoff
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= 5*time.Minute {
			return 5 * time.Minute
		}
	}
	return d
}
