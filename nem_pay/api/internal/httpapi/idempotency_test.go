package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/repository/db"
	"github.com/nempay/api/internal/service"
)

// newIdemDB connects to TEST_DATABASE_URL (see Makefile `test-db`), clears the table, and
// returns queries + pool. Skips when unset so the suite stays green without a database.
func newIdemDB(t *testing.T) (*db.Queries, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping idempotency DB tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "TRUNCATE idempotency_keys"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db.New(pool), pool
}

// engine wires a fake auth middleware (fixed merchant) + a counter handler behind
// WithIdempotency. The counter is the "side effect" we assert runs exactly once.
func engine(q *db.Queries, merchant uuid.UUID, counter *int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) { setAuth(c, merchant, "secret") })
	r.POST("/v1/thing", WithIdempotency(q, func(c *gin.Context) {
		atomic.AddInt64(counter, 1)
		time.Sleep(30 * time.Millisecond) // widen the concurrency window
		c.JSON(http.StatusOK, gin.H{"ok": true, "n": atomic.LoadInt64(counter)})
	}))
	return r
}

func post(r *gin.Engine, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/thing", strings.NewReader(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestIdempotency_MissingHeader400(t *testing.T) {
	q, _ := newIdemDB(t)
	var n int64
	r := engine(q, uuid.New(), &n)

	w := post(r, "", `{"a":1}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	if n != 0 {
		t.Fatalf("handler ran without a key: counter=%d", n)
	}
}

func TestIdempotency_ReplayIsByteIdentical(t *testing.T) {
	q, _ := newIdemDB(t)
	var n int64
	r := engine(q, uuid.New(), &n)

	first := post(r, "key-1", `{"a":1}`)
	second := post(r, "key-1", `{"a":1}`)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("want 200/200, got %d/%d", first.Code, second.Code)
	}
	if n != 1 {
		t.Fatalf("side effect ran %d times, want 1", n)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay body differs:\n first=%s\nsecond=%s", first.Body.String(), second.Body.String())
	}
	if second.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay missing Idempotent-Replayed header")
	}
}

func TestIdempotency_SameKeyDifferentBody422(t *testing.T) {
	q, _ := newIdemDB(t)
	var n int64
	r := engine(q, uuid.New(), &n)

	if w := post(r, "key-2", `{"a":1}`); w.Code != http.StatusOK {
		t.Fatalf("first call want 200, got %d", w.Code)
	}
	w := post(r, "key-2", `{"a":2}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for reused key, got %d", w.Code)
	}
	if n != 1 {
		t.Fatalf("side effect ran %d times, want 1", n)
	}
}

// The core correctness lesson: concurrent identical requests execute the side effect once.
func TestIdempotency_ConcurrentSingleExecution(t *testing.T) {
	q, _ := newIdemDB(t)
	var n int64
	r := engine(q, uuid.New(), &n)

	const N = 8
	var wg sync.WaitGroup
	codes := make([]int, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = post(r, "race", `{"a":1}`).Code
		}(i)
	}
	wg.Wait()

	if n != 1 {
		t.Fatalf("side effect ran %d times under concurrency, want exactly 1", n)
	}
	var ok, conflict int
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if ok < 1 {
		t.Fatalf("no request succeeded")
	}
	if ok+conflict != N {
		t.Fatalf("codes did not partition into ok/409: ok=%d conflict=%d", ok, conflict)
	}
}

// A panicking handler must release its claim (via the wrapper's recover), so the retry is not
// wedged at 409 until the sweeper — and it re-panics so gin.Recovery still renders a 500.
func TestIdempotency_PanicReleasesClaim(t *testing.T) {
	q, _ := newIdemDB(t)
	merchant := uuid.New()
	var calls int64

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) { setAuth(c, merchant, "secret") })
	r.POST("/v1/thing", WithIdempotency(q, func(c *gin.Context) {
		n := atomic.AddInt64(&calls, 1)
		if n == 1 {
			panic("boom") // first attempt blows up mid-handler
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}))

	if w := post(r, "panic-key", `{"a":1}`); w.Code != http.StatusInternalServerError {
		t.Fatalf("first attempt want 500, got %d", w.Code)
	}
	// Claim released → retry re-runs (not a 409 replay) and succeeds.
	if w := post(r, "panic-key", `{"a":1}`); w.Code != http.StatusOK {
		t.Fatalf("retry after panic want 200, got %d", w.Code)
	}
	if calls != 2 {
		t.Fatalf("handler ran %d times, want 2 (panic + successful retry)", calls)
	}
}

// Completed keys expire per the ~24h lifetime: after expiry the same key runs fresh.
func TestIdempotency_ExpireCompletedRunsFresh(t *testing.T) {
	q, _ := newIdemDB(t)
	var n int64
	r := engine(q, uuid.New(), &n)
	ctx := context.Background()

	if w := post(r, "exp", `{"a":1}`); w.Code != http.StatusOK {
		t.Fatalf("first call want 200, got %d", w.Code)
	}
	// Expire everything completed (olderThan 0 → all completed rows qualify).
	if err := service.ExpireCompletedKeys(ctx, q, 0); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if w := post(r, "exp", `{"a":1}`); w.Code != http.StatusOK {
		t.Fatalf("post-expiry call want 200, got %d", w.Code)
	}
	if n != 2 {
		t.Fatalf("side effect ran %d times, want 2 (expiry let the key run fresh)", n)
	}
}

// Orphan sweep: a wedged in_flight row is freed, and the retry then runs.
func TestIdempotency_SweeperFreesOrphan(t *testing.T) {
	q, pool := newIdemDB(t)
	var n int64
	merchant := uuid.New()
	r := engine(q, merchant, &n)
	ctx := context.Background()

	// Simulate a crash: a claim left in_flight, never completed.
	if _, err := q.InsertInFlight(ctx, db.InsertInFlightParams{
		MerchantID:  merchant,
		IdemKey:     "wedged",
		RequestHash: service.Fingerprint(http.MethodPost, "/v1/thing", []byte(`{"a":1}`)),
	}); err != nil {
		t.Fatalf("seed in_flight: %v", err)
	}

	// Before the sweep, a retry is correctly blocked (409).
	if w := post(r, "wedged", `{"a":1}`); w.Code != http.StatusConflict {
		t.Fatalf("pre-sweep want 409, got %d", w.Code)
	}

	// Sweep everything in_flight (olderThan 0 → cutoff is now, all qualify).
	if err := service.SweepOrphanedKeys(ctx, q, 0); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	_ = pool

	// Retry now succeeds and the side effect runs.
	if w := post(r, "wedged", `{"a":1}`); w.Code != http.StatusOK {
		t.Fatalf("post-sweep want 200, got %d", w.Code)
	}
	if n != 1 {
		t.Fatalf("side effect ran %d times, want 1", n)
	}
}
