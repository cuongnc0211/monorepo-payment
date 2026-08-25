package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nempay/api/internal/repository/db"
)

// portalHandler serves the read-only, tenant-scoped views the merchant portal renders. Every read
// is scoped to the credential's merchant (from authAny); there is no mutation here.
type portalHandler struct {
	q *db.Queries
}

// balances — derived balances by (type, kind, currency). The portal displays these int64 minor
// units; it never recomputes them (spec AC5/AC6-money-math).
func (h *portalHandler) balances(c *gin.Context) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required", "authentication is required", "")
		return
	}
	rows, err := h.q.BalancesForMerchant(c.Request.Context(), merchantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "balances_failed", "could not load balances", "")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{"type": r.Type, "kind": r.Kind, "currency": r.Currency, "balance": r.Balance})
	}
	c.JSON(http.StatusOK, gin.H{"balances": out})
}

// webhookEvents — the merchant's emitted events, newest first, paginated. Delivery state is derived
// by the client from status + attempts.
func (h *portalHandler) webhookEvents(c *gin.Context) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required", "authentication is required", "")
		return
	}
	limit, offset := paginate(c)
	rows, err := h.q.ListWebhookEventsForMerchant(c.Request.Context(), db.ListWebhookEventsForMerchantParams{
		MerchantID: merchantID, Limit: limit, Offset: offset,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "webhooks_failed", "could not load webhook events", "")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{
			"event_id": r.EventID.String(), "event_type": r.EventType, "status": r.Status,
			"attempts": r.Attempts, "last_error": r.LastError, "created_at": r.CreatedAt.Time,
		})
	}
	c.JSON(http.StatusOK, gin.H{"webhook_events": out})
}

// apiKeys — the merchant's keys, secret masked (only the non-secret prefix is ever returned).
func (h *portalHandler) apiKeys(c *gin.Context) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required", "authentication is required", "")
		return
	}
	rows, err := h.q.ListAPIKeysForMerchant(c.Request.Context(), merchantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "api_keys_failed", "could not load API keys", "")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{
			"id": r.ID.String(), "kind": r.Kind, "prefix": r.TokenPrefix,
			"masked": r.TokenPrefix + "••••", "created_at": r.CreatedAt.Time, "revoked_at": tsOrNil(r.RevokedAt),
		})
	}
	c.JSON(http.StatusOK, gin.H{"api_keys": out})
}

// ledger — the transaction(s)/entries backing one intent (capture, settle, refund, ...). 404 if the
// intent is not this merchant's, so another merchant's id discloses nothing.
func (h *portalHandler) ledger(c *gin.Context) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required", "authentication is required", "")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_id", "the payment intent id is not a valid uuid", "id")
		return
	}
	if _, err := h.q.GetIntent(c.Request.Context(), db.GetIntentParams{ID: id, MerchantID: merchantID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(c, http.StatusNotFound, errTypeNotFound, "intent_not_found", "no such payment intent", "id")
			return
		}
		respondError(c, http.StatusInternalServerError, errTypeAPI, "ledger_failed", "could not load the ledger", "")
		return
	}

	rows, err := h.q.LedgerForIntent(c.Request.Context(), db.LedgerForIntentParams{MerchantID: merchantID, ReferenceID: &id})
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "ledger_failed", "could not load the ledger", "")
		return
	}

	// Group flat rows into transactions, each carrying its entries.
	type entry struct {
		AccountType string `json:"account_type"`
		AccountKind string `json:"account_kind"`
		Debit       int64  `json:"debit"`
		Credit      int64  `json:"credit"`
		Currency    string `json:"currency"`
	}
	type txn struct {
		ID        string    `json:"id"`
		Kind      string    `json:"kind"`
		CreatedAt time.Time `json:"created_at"`
		Entries   []entry   `json:"entries"`
	}
	var txns []txn
	byID := map[uuid.UUID]int{}
	for _, r := range rows {
		i, seen := byID[r.TransactionID]
		if !seen {
			txns = append(txns, txn{ID: r.TransactionID.String(), Kind: r.TransactionKind, CreatedAt: r.TransactionCreatedAt.Time})
			i = len(txns) - 1
			byID[r.TransactionID] = i
		}
		txns[i].Entries = append(txns[i].Entries, entry{
			AccountType: r.AccountType, AccountKind: r.AccountKind, Debit: r.Debit, Credit: r.Credit, Currency: r.Currency,
		})
	}
	if txns == nil {
		txns = []txn{}
	}
	c.JSON(http.StatusOK, gin.H{"transactions": txns})
}

// paginate reads ?limit (1..100, default 20) and ?offset (>=0, default 0).
func paginate(c *gin.Context) (limit, offset int32) {
	limit, offset = 20, 0
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		if v > 100 {
			v = 100
		}
		limit = int32(v)
	}
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v > 0 {
		offset = int32(v)
	}
	return
}

func tsOrNil(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
