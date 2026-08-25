package httpapi

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/nempay/api/internal/repository/db"
)

// sessionHandler issues and verifies portal login sessions (short-lived HS256 JWTs). Human auth for
// the read-only portal: a session resolves to exactly one merchant. Deliberately no refresh token
// for the first cut (re-login on expiry) — see plans/005.
type sessionHandler struct {
	q      *db.Queries
	secret []byte
	ttl    time.Duration
}

func newSessionHandler(q *db.Queries) *sessionHandler {
	return &sessionHandler{q: q, secret: []byte(jwtSecret()), ttl: jwtTTL()}
}

// jwtSecret / jwtTTL are read from the environment with dev defaults (mirrors corsOrigin), so
// NewRouter's signature stays stable. The default secret is obviously-dev; production MUST set it.
func jwtSecret() string {
	if v := os.Getenv("NEMPAY_JWT_SECRET"); v != "" {
		return v
	}
	return "dev-only-insecure-jwt-secret-change-me"
}

func jwtTTL() time.Duration {
	if v := os.Getenv("NEMPAY_JWT_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return time.Hour
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// login authenticates a portal user and returns a session token. A wrong password and an unknown
// email return the same 401, so the endpoint does not reveal which emails exist.
func (h *sessionHandler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" || req.Password == "" {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_body",
			"email and password are required", "")
		return
	}

	user, err := h.q.GetUserByEmail(c.Request.Context(), req.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		invalidCredentials(c)
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "auth_lookup_failed",
			"could not verify credentials", "")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		invalidCredentials(c)
		return
	}

	token, exp, err := h.issueToken(user.MerchantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "token_error",
			"could not issue a session", "")
		return
	}
	merchant, err := h.q.GetMerchant(c.Request.Context(), user.MerchantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "merchant_lookup_failed",
			"could not load the merchant", "")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"expires_at": exp.UTC(),
		"merchant":   gin.H{"id": merchant.ID.String(), "name": merchant.Name},
	})
}

func invalidCredentials(c *gin.Context) {
	respondError(c, http.StatusUnauthorized, errTypeAuth, "invalid_credentials",
		"the email or password is incorrect", "")
}

func (h *sessionHandler) issueToken(merchantID uuid.UUID) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(h.ttl)
	claims := jwt.MapClaims{
		"merchant_id": merchantID.String(),
		"iat":         now.Unix(),
		"exp":         exp.Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(h.secret)
	return signed, exp, err
}

// verifyToken validates a session JWT and returns its merchant. Rejects any non-HMAC algorithm
// (defends against alg-confusion), a bad signature, or expiry.
func (h *sessionHandler) verifyToken(tokenStr string) (uuid.UUID, error) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return h.secret, nil
	})
	if err != nil || !tok.Valid {
		return uuid.Nil, errors.New("invalid session token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, errors.New("invalid claims")
	}
	mid, ok := claims["merchant_id"].(string)
	if !ok {
		return uuid.Nil, errors.New("no merchant in token")
	}
	return uuid.Parse(mid)
}

// authAny guards the read surface: it accepts EITHER an API key (pk_/sk_) OR a portal session JWT,
// resolving to a merchant either way. Money-mutating routes do NOT use this — they keep apiKeyAuth
// + secretOnly, so a browser session can never move money.
func (h *sessionHandler) authAny() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			respondError(c, http.StatusUnauthorized, errTypeAuth, "missing_credentials",
				"provide an API key or a session token as 'Authorization: Bearer <token>'", "")
			return
		}

		if strings.HasPrefix(token, "pk_") || strings.HasPrefix(token, "sk_") {
			merchantID, kind, err := lookupAPIKey(c.Request.Context(), h.q, token)
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
			return
		}

		merchantID, err := h.verifyToken(token)
		if err != nil {
			respondError(c, http.StatusUnauthorized, errTypeAuth, "invalid_session",
				"your session is invalid or has expired", "")
			return
		}
		setAuth(c, merchantID, "session")
		c.Next()
	}
}
