package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/nempay/api/internal/repository/db"
)

// Webhook-endpoint management — the portal's first WRITES. Configuration only: these touch no
// ledger and no balance, so a portal session that can call them still cannot move money (spec 008,
// preserving 005's invariant). Tenant-scoped; the signing secret is write-only (never returned).

func endpointJSON(e db.WebhookEndpoint) gin.H {
	return gin.H{
		"id":         e.ID.String(),
		"url":        e.Url,
		"active":     !e.DisabledAt.Valid,
		"created_at": e.CreatedAt.Time,
	}
}

func (h *portalHandler) listEndpoints(c *gin.Context) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required", "authentication is required", "")
		return
	}
	rows, err := h.q.ListEndpoints(c.Request.Context(), merchantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "endpoints_failed", "could not load webhook endpoints", "")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, e := range rows {
		out = append(out, endpointJSON(e)) // never includes the secret
	}
	c.JSON(http.StatusOK, gin.H{"webhook_endpoints": out})
}

type createEndpointRequest struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

func (h *portalHandler) createEndpoint(c *gin.Context) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required", "authentication is required", "")
		return
	}
	var req createEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_body", "request body must be valid JSON", "")
		return
	}
	if !validEndpointURL(req.URL) {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_url", "url must be a valid http(s) URL", "url")
		return
	}
	if strings.TrimSpace(req.Secret) == "" {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_secret", "a signing secret is required", "secret")
		return
	}
	e, err := h.q.InsertWebhookEndpoint(c.Request.Context(), db.InsertWebhookEndpointParams{
		MerchantID: merchantID, Url: req.URL, Secret: req.Secret,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "endpoint_create_failed", "could not create the endpoint", "")
		return
	}
	c.JSON(http.StatusOK, endpointJSON(e))
}

func (h *portalHandler) disableEndpoint(c *gin.Context) {
	merchantID, ok := MerchantID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required", "authentication is required", "")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_id", "the endpoint id is not a valid uuid", "id")
		return
	}
	e, err := h.q.DisableEndpoint(c.Request.Context(), db.DisableEndpointParams{ID: id, MerchantID: merchantID})
	if errors.Is(err, pgx.ErrNoRows) {
		respondError(c, http.StatusNotFound, errTypeNotFound, "endpoint_not_found", "no such active endpoint", "id")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "endpoint_disable_failed", "could not disable the endpoint", "")
		return
	}
	c.JSON(http.StatusOK, endpointJSON(e))
}

// validEndpointURL accepts a parseable absolute http(s) URL with a host.
func validEndpointURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
