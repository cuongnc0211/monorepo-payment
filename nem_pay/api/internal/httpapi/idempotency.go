package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/nempay/api/internal/repository/db"
	"github.com/nempay/api/internal/service"
)

const (
	pgUniqueViolation = "23505"
	// maxBody caps the request body the wrapper buffers for fingerprinting. Money payloads are
	// tiny; the cap stops a giant body from amplifying memory before the handler even runs.
	maxBody = 1 << 20 // 1 MiB
)

// bodyCapture tees the handler's response into a buffer so the first ('in_flight') caller can
// store exactly what it returned, for byte-identical replay on a later retry.
type bodyCapture struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (w bodyCapture) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w bodyCapture) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// WithIdempotency wraps a money-mutating handler with insert-first idempotency.
//
// It claims (merchant_id, Idempotency-Key) via the UNIQUE constraint BEFORE running the
// handler. The claim is committed on its own — deliberately NOT inside the handler's money tx
// — because a competing retry must see the claim immediately to lose the race; a row invisible
// until commit could not arbitrate anything. On a duplicate the wrapper replays the stored
// response (completed), returns 409 (still in flight) or 422 (key reused for a different body).
func WithIdempotency(q *db.Queries, handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		merchantID, ok := MerchantID(c)
		if !ok {
			respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required",
				"authentication is required for this request", "")
			return
		}

		key := c.GetHeader("Idempotency-Key")
		if key == "" {
			respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "missing_idempotency_key",
				"an Idempotency-Key header is required for this request", "Idempotency-Key")
			return
		}

		// Read then restore the body so the wrapped handler can still bind it. A read error
		// means we can't fingerprint reliably, so reject rather than proceed on a truncated body.
		var body []byte
		if c.Request.Body != nil {
			var err error
			body, err = io.ReadAll(io.LimitReader(c.Request.Body, maxBody+1))
			if err != nil {
				respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_body",
					"could not read the request body", "")
				return
			}
			if len(body) > maxBody {
				respondError(c, http.StatusRequestEntityTooLarge, errTypeInvalidRequest, "body_too_large",
					"request body exceeds the maximum size", "")
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
		}
		hash := service.Fingerprint(c.Request.Method, c.Request.URL.RequestURI(), body)

		ctx := c.Request.Context()
		claim, err := q.InsertInFlight(ctx, db.InsertInFlightParams{
			MerchantID:  merchantID,
			IdemKey:     key,
			RequestHash: hash,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
				replayOrConflict(c, q, merchantID, key, hash)
				return
			}
			respondError(c, http.StatusInternalServerError, errTypeAPI, "idempotency_claim_failed",
				"could not record the idempotency key", "")
			return
		}

		// Bookkeeping (release/store) must survive a client disconnect: if we used the request
		// context here, a cancel could leave the row wedged 'in_flight' even though the money
		// side effect committed, and the retry could then never replay. WithoutCancel keeps the
		// deadline-free parent values while dropping cancellation.
		writeBack := context.WithoutCancel(ctx)

		// If the handler panics, gin.Recovery unwinds past us; release the claim first so a retry
		// can re-run promptly (rather than waiting on the sweeper), then re-panic to let Recovery
		// render the 500.
		defer func() {
			if r := recover(); r != nil {
				_ = q.DeleteKey(writeBack, claim.ID)
				panic(r)
			}
		}()

		// First caller: run the handler, capturing its response for storage.
		buf := &bytes.Buffer{}
		c.Writer = bodyCapture{ResponseWriter: c.Writer, buf: buf}
		handler(c)

		status := c.Writer.Status()
		// Only a "final" response is stored. A 5xx is treated as transient: release the claim so
		// a retry can re-run instead of being wedged replaying a server error.
		if status >= http.StatusInternalServerError {
			_ = q.DeleteKey(writeBack, claim.ID)
			return
		}
		code := int32(status)
		_ = q.MarkCompleted(writeBack, db.MarkCompletedParams{
			ID:           claim.ID,
			ResponseCode: &code,
			ResponseBody: buf.Bytes(),
		})
	}
}

// replayOrConflict resolves a duplicate claim (the InsertInFlight hit 23505).
func replayOrConflict(c *gin.Context, q *db.Queries, merchantID uuid.UUID, key, hash string) {
	rec, err := q.GetByKey(c.Request.Context(), db.GetByKeyParams{MerchantID: merchantID, IdemKey: key})
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "idempotency_lookup_failed",
			"could not read the idempotency key", "")
		return
	}

	// Same key, different request → misuse.
	if rec.RequestHash != hash {
		respondError(c, http.StatusUnprocessableEntity, errTypeIdempotency, "idempotency_key_reused",
			"this Idempotency-Key was already used for a request with a different body", "Idempotency-Key")
		return
	}

	// Original still running → tell the client to back off and retry.
	if rec.Status == "in_flight" {
		respondError(c, http.StatusConflict, errTypeIdempotency, "request_in_flight",
			"a request with this Idempotency-Key is still being processed", "Idempotency-Key")
		return
	}

	// Completed → replay the stored response verbatim. Scope note: we replay the exact body bytes
	// and status code, plus a fixed JSON content-type. The /v1 surface is JSON-only and sets no
	// response-specific headers (e.g. Location), so body+status is the full contract here; revisit
	// if a handler ever needs to replay custom headers.
	code := http.StatusOK
	if rec.ResponseCode != nil {
		code = int(*rec.ResponseCode)
	}
	c.Header("Idempotent-Replayed", "true")
	if len(rec.ResponseBody) > 0 {
		c.Data(code, "application/json; charset=utf-8", rec.ResponseBody)
	} else {
		c.Status(code)
	}
	c.Abort()
}
