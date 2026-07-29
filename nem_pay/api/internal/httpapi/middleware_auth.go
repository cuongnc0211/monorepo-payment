package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/nempay/api/internal/apikey"
	"github.com/nempay/api/internal/repository/db"
)

// apiKeyAuth authenticates a request from its `Authorization: Bearer <token>` header. It
// narrows candidates by the indexed prefix, then confirms with a constant-time hash compare,
// and attaches (merchant_id, key kind) to the context. It does NOT decide whether the route
// allows the key's kind — secretOnly does that — so the same middleware serves both key kinds.
func apiKeyAuth(q *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			respondError(c, http.StatusUnauthorized, errTypeAuth, "missing_api_key",
				"provide your API key as 'Authorization: Bearer <key>'", "")
			return
		}

		candidates, err := q.GetKeysByPrefix(c.Request.Context(), apikey.Prefix(token))
		if err != nil {
			respondError(c, http.StatusInternalServerError, errTypeAPI, "auth_lookup_failed",
				"could not verify the API key", "")
			return
		}

		hash := apikey.Hash(token)
		for _, k := range candidates {
			if apikey.Equal(k.TokenHash, hash) {
				setAuth(c, k.MerchantID, k.Kind)
				c.Next()
				return
			}
		}
		respondError(c, http.StatusUnauthorized, errTypeAuth, "invalid_api_key",
			"the provided API key is invalid or has been revoked", "")
	}
}

// secretOnly rejects any route it guards unless a SECRET key authenticated the request — a
// publishable key must never perform a money-mutating (or otherwise privileged) action.
func secretOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		kind, ok := KeyKind(c)
		if !ok {
			respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required",
				"authentication is required for this request", "")
			return
		}
		if kind != "secret" {
			respondError(c, http.StatusForbidden, errTypeAuth, "secret_key_required",
				"this endpoint requires a secret API key; a publishable key cannot be used", "")
			return
		}
		c.Next()
	}
}

// bearerToken extracts the token from an Authorization: Bearer header (case-insensitive scheme).
func bearerToken(c *gin.Context) (string, bool) {
	h := c.GetHeader("Authorization")
	if h == "" {
		return "", false
	}
	const scheme = "bearer "
	if len(h) <= len(scheme) || !strings.EqualFold(h[:len(scheme)], scheme) {
		return "", false
	}
	token := strings.TrimSpace(h[len(scheme):])
	return token, token != ""
}
