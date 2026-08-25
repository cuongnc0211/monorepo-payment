package httpapi

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// corsOrigins is the set of browser origins allowed to call the browser-facing endpoints
// (/v1/tokens for the merchant checkout, and the whole read surface + login for the portal).
// Defaults to the local NemLuxury dev server and the local portal dev server; override with a
// comma-separated NEMPAY_CORS_ORIGINS.
func corsOrigins() map[string]bool {
	raw := os.Getenv("NEMPAY_CORS_ORIGINS")
	if raw == "" {
		raw = "http://localhost:3000,http://localhost:5173"
	}
	set := map[string]bool{}
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			set[o] = true
		}
	}
	return set
}

// cors echoes CORS headers for an allowed origin and answers preflight (OPTIONS) before auth runs
// — a preflight carries no Authorization header, so it must not be rejected by the auth middleware.
// Only an exact-match origin is granted; any other origin gets no CORS headers.
func cors(allowed map[string]bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if origin := c.GetHeader("Origin"); origin != "" && allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
