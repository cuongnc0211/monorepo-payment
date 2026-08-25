package httpapi

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// corsOrigin is the single browser origin allowed to call the browser-facing endpoints
// (currently /v1/tokens) cross-origin. A merchant's card page runs there and tokenizes directly
// against the gateway, exactly as a page would talk to Stripe.js. Defaults to the local NemLuxury
// dev server; override with NEMPAY_CORS_ORIGIN.
func corsOrigin() string {
	if v := os.Getenv("NEMPAY_CORS_ORIGIN"); v != "" {
		return v
	}
	return "http://localhost:3000"
}

// cors sets CORS headers for the configured origin and answers preflight (OPTIONS) requests before
// authentication runs — a preflight carries no Authorization header, so it must not be rejected by
// apiKeyAuth. Only the exact configured origin is echoed back; any other origin gets no CORS grant.
func cors(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if reqOrigin := c.GetHeader("Origin"); reqOrigin != "" && reqOrigin == origin {
			c.Header("Access-Control-Allow-Origin", reqOrigin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
