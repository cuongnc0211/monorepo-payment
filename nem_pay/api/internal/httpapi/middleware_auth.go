package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nempay/api/internal/apikey"
	"github.com/nempay/api/internal/repository/db"
)

// errNoKeyMatch is returned by lookupAPIKey when the token authenticates against no active key —
// distinct from a DB error so callers can map it to 401 (not 500).
var errNoKeyMatch = errors.New("no matching api key")

// lookupAPIKey resolves a raw token to its (merchant, kind) by the indexed prefix + constant-time
// hash compare. Shared by apiKeyAuth and authAny so the two credential paths stay in step.
func lookupAPIKey(ctx context.Context, q *db.Queries, token string) (uuid.UUID, string, error) {
	candidates, err := q.GetKeysByPrefix(ctx, apikey.Prefix(token))
	if err != nil {
		return uuid.Nil, "", err
	}
	hash := apikey.Hash(token)
	for _, k := range candidates {
		if apikey.Equal(k.TokenHash, hash) {
			return k.MerchantID, k.Kind, nil
		}
	}
	return uuid.Nil, "", errNoKeyMatch
}

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

		merchantID, kind, err := lookupAPIKey(c.Request.Context(), q, token)
		if errors.Is(err, errNoKeyMatch) {
			respondError(c, http.StatusUnauthorized, errTypeAuth, "invalid_api_key",
				"the provided API key is invalid or has been revoked", "")
			return
		}
		if err != nil {
			respondError(c, http.StatusInternalServerError, errTypeAPI, "auth_lookup_failed",
				"could not verify the API key", "")
			return
		}
		setAuth(c, merchantID, kind)
		c.Next()
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
