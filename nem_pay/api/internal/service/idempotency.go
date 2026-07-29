// Package service holds NemPay's business logic and owns DB transaction boundaries.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nempay/api/internal/repository/db"
)

// Fingerprint identifies a request for idempotent replay. The same (method, requestURI, body)
// must reproduce the stored response; the SAME key with a DIFFERENT request is a client misuse
// and is rejected (422). requestURI includes the query string (pass URL.RequestURI()), so two
// requests differing only in query params don't collide. Only method + URI + body are hashed —
// never volatile transport headers — so a retry differing only in, say, a trace header is not
// falsely rejected.
func Fingerprint(method, requestURI string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{0}) // NUL separators so ("GET","/a") and ("GE","T/a") can't collide.
	h.Write([]byte(requestURI))
	h.Write([]byte{0})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// SweepOrphanedKeys deletes idempotency claims stuck 'in_flight' longer than olderThan — rows
// left by a process that crashed mid-request. Scheduled from the worker (M1.5). Without it, a
// crash would wedge every subsequent retry of that key at 409 permanently. olderThan must be
// comfortably larger than the longest possible handler duration, so a still-running request is
// never swept out from under itself.
func SweepOrphanedKeys(ctx context.Context, q *db.Queries, olderThan time.Duration) error {
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-olderThan), Valid: true}
	return q.ResetOrphans(ctx, cutoff)
}

// ExpireCompletedKeys enforces the ~24h key lifetime by dropping completed rows older than
// olderThan (the worker passes 24h). Keeps the table bounded and lets a key reused after its
// lifetime run fresh instead of replaying a stale response.
func ExpireCompletedKeys(ctx context.Context, q *db.Queries, olderThan time.Duration) error {
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-olderThan), Valid: true}
	return q.ExpireCompleted(ctx, cutoff)
}
