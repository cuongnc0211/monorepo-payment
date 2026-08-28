package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/banksim"
	"github.com/nempay/api/internal/repository/db"
	"github.com/nempay/api/internal/service"
)

// NewRouter builds the /v1 API over a pgx pool.
//
// Route protection follows nem_pay/CLAUDE.md: /v1/health is public; everything else requires a
// valid API key (apiKeyAuth), and the payment-intent routes additionally require a SECRET key
// (secretOnly) — a publishable key authenticates but is refused here. Money-mutating POSTs are
// wrapped in WithIdempotency; the money verbs (confirm/capture/refund) are added in task-05.
func NewRouter(pool *pgxpool.Pool, bank *banksim.Client) *gin.Engine {
	q := db.New(pool)
	intents := &intentHandler{svc: service.NewIntents(pool, bank)}
	session := newSessionHandler(q)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors(corsOrigins())) // browser-facing endpoints (/v1/tokens); answers preflight before auth
	r.HandleMethodNotAllowed = true // so NoMethod (405) fires instead of falling through to 404

	// Unknown path / unsupported method still return the standard error envelope, so a client
	// never has to parse Gin's default plaintext 404/405 — one error shape everywhere.
	r.NoRoute(func(c *gin.Context) {
		respondError(c, http.StatusNotFound, errTypeNotFound, "unknown_route",
			"no such endpoint", "")
	})
	r.NoMethod(func(c *gin.Context) {
		respondError(c, http.StatusMethodNotAllowed, errTypeInvalidRequest, "method_not_allowed",
			"that method is not allowed on this endpoint", "")
	})

	r.GET("/v1/health", health)

	// Public, browsable API docs (Scalar) + the raw spec. No auth — docs are public.
	r.GET("/docs", docsPage)
	r.GET("/openapi.yaml", openapiSpec)

	// Portal login is public (it mints the session); it is reachable cross-origin via the CORS
	// layer above. It is NOT under the API-key group. Refresh exchanges a refresh token for a new
	// access token — also public (the refresh token is the credential).
	r.POST("/v1/portal/login", session.login)
	r.POST("/v1/portal/refresh", session.refresh)

	// Authenticated surface. Auth is applied PER GROUP (not on the whole /v1) so the read group can
	// accept a portal session as well as an API key.
	v1 := r.Group("/v1")
	portal := &portalHandler{q: q}

	// Tokenization: browser call with a PUBLISHABLE key. The card PAN reaches the gateway here
	// (never a merchant), is tokenized, and only the token flows onward.
	tokens := v1.Group("/tokens")
	tokens.Use(apiKeyAuth(q), publishableOnly())
	tokens.POST("", (&tokenHandler{}).create)

	// Money-mutating routes: SECRET key only — a browser session can never reach these.
	piWrite := v1.Group("/payment_intents")
	piWrite.Use(apiKeyAuth(q), secretOnly())
	piWrite.POST("", WithIdempotency(q, intents.create))
	piWrite.POST("/:id/confirm", WithIdempotency(q, intents.confirm))
	piWrite.POST("/:id/capture", WithIdempotency(q, intents.capture))
	piWrite.POST("/:id/refund", WithIdempotency(q, intents.refund))
	piWrite.POST("/:id/release", WithIdempotency(q, intents.release)) // escrow only

	// Read surface: an API key OR a portal session (authAny). Every handler scopes to the
	// credential's merchant, so a session sees only its own tenant's data.
	reads := v1.Group("")
	reads.Use(session.authAny())
	reads.GET("/payment_intents", intents.list)
	reads.GET("/payment_intents/:id", intents.get)
	reads.GET("/payment_intents/:id/ledger", portal.ledger)
	reads.GET("/balances", portal.balances)
	reads.GET("/webhook_events", portal.webhookEvents)
	reads.GET("/api_keys", portal.apiKeys)

	return r
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
