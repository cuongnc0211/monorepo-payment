package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Request-scoped values the auth middleware (task-04) sets and downstream handlers read.
// Kept behind typed accessors so no handler reaches into gin.Context with a raw string key.
const (
	ctxMerchantID = "nempay.merchant_id"
	ctxKeyKind    = "nempay.api_key_kind"
)

// setAuth records the authenticated merchant and the kind of key used ('publishable' or
// 'secret'). Called by the auth middleware once a key is verified.
func setAuth(c *gin.Context, merchantID uuid.UUID, kind string) {
	c.Set(ctxMerchantID, merchantID)
	c.Set(ctxKeyKind, kind)
}

// MerchantID returns the authenticated merchant, or ok=false if the request was not
// authenticated (a programming error on a protected route, treated as 401 by callers).
func MerchantID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(ctxMerchantID)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

// KeyKind returns the kind of API key that authenticated the request.
func KeyKind(c *gin.Context) (string, bool) {
	v, ok := c.Get(ctxKeyKind)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
