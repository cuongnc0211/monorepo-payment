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
	r.Use(cors(corsOrigin())) // browser-facing endpoints (/v1/tokens); answers preflight before auth
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

	// Portal login is public (it mints the session); it is reachable cross-origin via the CORS
	// layer above. It is NOT under the API-key group.
	r.POST("/v1/portal/login", session.login)

	// Authenticated surface.
	v1 := r.Group("/v1")
	v1.Use(apiKeyAuth(q))

	// Tokenization: called from the browser with a PUBLISHABLE key. The card PAN reaches the
	// gateway here (never a merchant), is tokenized, and only the token flows onward.
	tokens := v1.Group("/tokens")
	tokens.Use(publishableOnly())
	tokens.POST("", (&tokenHandler{}).create)

	// Payment intents: secret-key only for all of M1 (reads included).
	pi := v1.Group("/payment_intents")
	pi.Use(secretOnly())
	pi.POST("", WithIdempotency(q, intents.create))
	pi.GET("/:id", intents.get)
	pi.GET("", intents.list)
	// Money verbs — each idempotent (task-03) and gated by the state machine (task-05).
	pi.POST("/:id/confirm", WithIdempotency(q, intents.confirm))
	pi.POST("/:id/capture", WithIdempotency(q, intents.capture))
	pi.POST("/:id/refund", WithIdempotency(q, intents.refund))
	pi.POST("/:id/release", WithIdempotency(q, intents.release)) // escrow only

	return r
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
