package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nempay/api/internal/repository/db"
	"github.com/nempay/api/internal/service"
)

type intentHandler struct {
	svc *service.Intents
}

// createIntentRequest is the POST /v1/payment_intents body. Money is amount (int64 minor units)
// + currency; metadata is arbitrary caller JSON.
type createIntentRequest struct {
	Amount   int64           `json:"amount"`
	Currency string          `json:"currency"`
	Metadata json.RawMessage `json:"metadata"`
	// Escrow mode (optional). When escrow is true, payee (a UUID) and application_fee are required.
	Escrow         bool    `json:"escrow"`
	Payee          string  `json:"payee"`
	ApplicationFee *int64  `json:"application_fee"`
}

// intentResponse is the public shape of a payment intent — deliberately hand-mapped from the DB
// row so response JSON is stable and clean (RFC3339 timestamps, no pgtype internals leaking).
type intentResponse struct {
	ID             string          `json:"id"`
	Object         string          `json:"object"`
	Amount         int64           `json:"amount"`
	Currency       string          `json:"currency"`
	Status         string          `json:"status"`
	SettlementMode string          `json:"settlement_mode"`
	Payee          *string         `json:"payee,omitempty"`
	ApplicationFee *int64          `json:"application_fee,omitempty"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func toIntentResponse(pi db.PaymentIntent) intentResponse {
	meta := pi.Metadata
	if len(meta) == 0 {
		meta = []byte("{}")
	}
	var payee *string
	if pi.PayeeID != nil {
		s := pi.PayeeID.String()
		payee = &s
	}
	return intentResponse{
		ID:             pi.ID.String(),
		Object:         "payment_intent",
		Amount:         pi.Amount,
		Currency:       pi.Currency,
		Status:         pi.Status,
		SettlementMode: pi.SettlementMode,
		Payee:          payee,
		ApplicationFee: pi.ApplicationFee,
		Metadata:       json.RawMessage(meta),
		CreatedAt:      pi.CreatedAt.Time,
		UpdatedAt:      pi.UpdatedAt.Time,
	}
}

func (h *intentHandler) create(c *gin.Context) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required",
			"authentication is required for this request", "")
		return
	}

	var req createIntentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_body",
			"request body is not valid JSON", "")
		return
	}

	in := service.CreateInput{
		Amount:         req.Amount,
		Currency:       req.Currency,
		Metadata:       req.Metadata,
		Escrow:         req.Escrow,
		ApplicationFee: req.ApplicationFee,
	}
	if req.Escrow {
		payee, perr := uuid.Parse(req.Payee)
		if perr != nil {
			respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_payee",
				"payee must be a valid UUID for an escrow intent", "payee")
			return
		}
		in.Payee = &payee
	}

	pi, err := h.svc.Create(c.Request.Context(), merchantID, in)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidAmount):
			respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_amount", err.Error(), "amount")
		case errors.Is(err, service.ErrInvalidCurrency):
			respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_currency", err.Error(), "currency")
		case errors.Is(err, service.ErrPayeeRequired):
			respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "payee_required", err.Error(), "payee")
		case errors.Is(err, service.ErrInvalidFee):
			respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_application_fee", err.Error(), "application_fee")
		default:
			respondError(c, http.StatusInternalServerError, errTypeAPI, "create_failed",
				"could not create the payment intent", "")
		}
		return
	}
	c.JSON(http.StatusOK, toIntentResponse(pi))
}

func (h *intentHandler) get(c *gin.Context) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required",
			"authentication is required for this request", "")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_id",
			"the payment intent id is not a valid UUID", "id")
		return
	}
	pi, err := h.svc.Get(c.Request.Context(), id, merchantID)
	if errors.Is(err, service.ErrIntentNotFound) {
		respondError(c, http.StatusNotFound, errTypeNotFound, "intent_not_found",
			"no payment intent with that id", "id")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "read_failed",
			"could not read the payment intent", "")
		return
	}
	c.JSON(http.StatusOK, toIntentResponse(pi))
}

func (h *intentHandler) list(c *gin.Context) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required",
			"authentication is required for this request", "")
		return
	}
	limit := atoi32(c.Query("limit"))
	offset := atoi32(c.Query("offset"))

	rows, err := h.svc.List(c.Request.Context(), merchantID, c.Query("status"), limit, offset)
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "list_failed",
			"could not list payment intents", "")
		return
	}
	data := make([]intentResponse, 0, len(rows))
	for _, pi := range rows {
		data = append(data, toIntentResponse(pi))
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

// confirmRequest carries the tokenized payment method (bank-sim magic token in dev/tests).
type confirmRequest struct {
	Token string `json:"token"`
}

// refundRequest carries the amount (minor units) to refund; must be ≤ the captured amount.
type refundRequest struct {
	Amount int64 `json:"amount"`
}

func (h *intentHandler) confirm(c *gin.Context) {
	merchantID, id, ok := h.authAndID(c)
	if !ok {
		return
	}
	var req confirmRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "missing_token",
			"a payment-method token is required to confirm", "token")
		return
	}
	pi, err := h.svc.Confirm(c.Request.Context(), id, merchantID, req.Token)
	if mapMoneyError(c, err) {
		return
	}
	c.JSON(http.StatusOK, toIntentResponse(pi))
}

func (h *intentHandler) capture(c *gin.Context) {
	merchantID, id, ok := h.authAndID(c)
	if !ok {
		return
	}
	pi, err := h.svc.Capture(c.Request.Context(), id, merchantID)
	if mapMoneyError(c, err) {
		return
	}
	c.JSON(http.StatusOK, toIntentResponse(pi))
}

func (h *intentHandler) release(c *gin.Context) {
	merchantID, id, ok := h.authAndID(c)
	if !ok {
		return
	}
	pi, err := h.svc.Release(c.Request.Context(), id, merchantID)
	if mapMoneyError(c, err) {
		return
	}
	c.JSON(http.StatusOK, toIntentResponse(pi))
}

func (h *intentHandler) refund(c *gin.Context) {
	merchantID, id, ok := h.authAndID(c)
	if !ok {
		return
	}
	var req refundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_body",
			"request body is not valid JSON", "")
		return
	}
	pi, err := h.svc.Refund(c.Request.Context(), id, merchantID, req.Amount)
	if mapMoneyError(c, err) {
		return
	}
	c.JSON(http.StatusOK, toIntentResponse(pi))
}

// authAndID resolves the authenticated merchant and the :id path param, writing the error
// response itself (and returning ok=false) when either is missing/invalid.
func (h *intentHandler) authAndID(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required",
			"authentication is required for this request", "")
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_id",
			"the payment intent id is not a valid UUID", "id")
		return uuid.Nil, uuid.Nil, false
	}
	return merchantID, id, true
}

// mapMoneyError renders the envelope for a money-service error and returns true if it handled
// one (nil → false, so the caller proceeds to render success).
func mapMoneyError(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, service.ErrIntentNotFound):
		respondError(c, http.StatusNotFound, errTypeNotFound, "intent_not_found", "no payment intent with that id", "id")
	case errors.Is(err, service.ErrInvalidState):
		respondError(c, http.StatusConflict, errTypeInvalidRequest, "invalid_state", err.Error(), "")
	case errors.Is(err, service.ErrRefundExceedsCapture):
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "refund_too_large", err.Error(), "amount")
	case errors.Is(err, service.ErrPartialRefundBeforeSettle):
		respondError(c, http.StatusUnprocessableEntity, errTypeInvalidRequest, "partial_refund_before_settle", err.Error(), "amount")
	case errors.Is(err, service.ErrPartialEscrowRefund):
		respondError(c, http.StatusUnprocessableEntity, errTypeInvalidRequest, "partial_escrow_refund", err.Error(), "amount")
	case errors.Is(err, service.ErrInvalidAmount):
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_amount", err.Error(), "amount")
	case errors.Is(err, service.ErrBankDeclined):
		respondError(c, http.StatusPaymentRequired, errTypeInvalidRequest, "bank_declined", err.Error(), "")
	case errors.Is(err, service.ErrBankUnavailable):
		// 504: the outcome is unknown; the intent is unchanged and the request is safe to retry
		// (a 5xx also makes the idempotency wrapper release the claim, so the retry re-runs).
		respondError(c, http.StatusGatewayTimeout, errTypeAPI, "bank_unavailable", err.Error(), "")
	default:
		respondError(c, http.StatusInternalServerError, errTypeAPI, "internal_error", "an unexpected error occurred", "")
	}
	return true
}

// atoi32 parses a query int into int32 without wrapping, returning 0 on empty/invalid/out-of-range
// (the service applies defaults and clamps limit/offset).
func atoi32(s string) int32 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}
