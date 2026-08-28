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

// sessionHandler issues and verifies portal login sessions. Human auth for the read-only portal:
// a session resolves to exactly one merchant. Two HS256 tokens, distinguished by a `typ` claim —
// a short-lived ACCESS token used on requests, and a longer-lived REFRESH token exchanged for a new
// access token when it expires (spec 007). Both are held in memory by the portal (never storage).
type sessionHandler struct {
	q          *db.Queries
	secret     []byte
	ttl        time.Duration // access token TTL
	refreshTTL time.Duration
}

func newSessionHandler(q *db.Queries) *sessionHandler {
	return &sessionHandler{q: q, secret: []byte(jwtSecret()), ttl: jwtTTL(), refreshTTL: jwtRefreshTTL()}
}

// jwtSecret / jwtTTL / jwtRefreshTTL are read from the environment with dev defaults (mirrors
// corsOrigin), so NewRouter's signature stays stable. The default secret is obviously-dev;
// production MUST set it.
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
	return 15 * time.Minute
}

func jwtRefreshTTL() time.Duration {
	if v := os.Getenv("NEMPAY_JWT_REFRESH_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 12 * time.Hour
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
	if err == nil {
		var refresh string
		refresh, err = h.issueRefreshToken(user.MerchantID)
		if err == nil {
			merchant, mErr := h.q.GetMerchant(c.Request.Context(), user.MerchantID)
			if mErr != nil {
				respondError(c, http.StatusInternalServerError, errTypeAPI, "merchant_lookup_failed",
					"could not load the merchant", "")
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"token":         token,
				"refresh_token": refresh,
				"expires_at":    exp.UTC(),
				"merchant":      gin.H{"id": merchant.ID.String(), "name": merchant.Name},
			})
			return
		}
	}
	respondError(c, http.StatusInternalServerError, errTypeAPI, "token_error",
		"could not issue a session", "")
}

type refreshRequestBody struct {
	RefreshToken string `json:"refresh_token"`
}

// refresh exchanges a valid REFRESH token for a new ACCESS token, scoped to the same merchant. No
// credentials; an access token (or anything not a refresh token) is refused (401).
func (h *sessionHandler) refresh(c *gin.Context) {
	var req refreshRequestBody
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_body",
			"a refresh_token is required", "")
		return
	}
	merchantID, err := h.verifyRefresh(req.RefreshToken)
	if err != nil {
		respondError(c, http.StatusUnauthorized, errTypeAuth, "invalid_refresh",
			"the refresh token is invalid or has expired", "")
		return
	}
	token, exp, err := h.issueToken(merchantID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, errTypeAPI, "token_error",
			"could not issue a session", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "expires_at": exp.UTC()})
}

func invalidCredentials(c *gin.Context) {
	respondError(c, http.StatusUnauthorized, errTypeAuth, "invalid_credentials",
		"the email or password is incorrect", "")
}

// issueToken mints a short-lived ACCESS token (used on requests).
func (h *sessionHandler) issueToken(merchantID uuid.UUID) (string, time.Time, error) {
	return h.issue(merchantID, "access", h.ttl)
}

// issueRefreshToken mints a longer-lived REFRESH token (exchanged at /v1/portal/refresh).
func (h *sessionHandler) issueRefreshToken(merchantID uuid.UUID) (string, error) {
	tok, _, err := h.issue(merchantID, "refresh", h.refreshTTL)
	return tok, err
}

func (h *sessionHandler) issue(merchantID uuid.UUID, typ string, ttl time.Duration) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(ttl)
	claims := jwt.MapClaims{
		"merchant_id": merchantID.String(),
		"typ":         typ,
		"iat":         now.Unix(),
		"exp":         exp.Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(h.secret)
	return signed, exp, err
}

// parseToken validates signature + expiry (rejecting non-HMAC algorithms — alg-confusion defence)
// and returns the token's merchant and `typ`.
func (h *sessionHandler) parseToken(tokenStr string) (uuid.UUID, string, error) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return h.secret, nil
	})
	if err != nil || !tok.Valid {
		return uuid.Nil, "", errors.New("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, "", errors.New("invalid claims")
	}
	mid, ok := claims["merchant_id"].(string)
	if !ok {
		return uuid.Nil, "", errors.New("no merchant in token")
	}
	typ, _ := claims["typ"].(string)
	merchantID, err := uuid.Parse(mid)
	return merchantID, typ, err
}

// verifyToken accepts only an ACCESS token (used by authAny sessions). A refresh token is refused
// here, so the two types are non-interchangeable.
func (h *sessionHandler) verifyToken(tokenStr string) (uuid.UUID, error) {
	merchantID, typ, err := h.parseToken(tokenStr)
	if err != nil {
		return uuid.Nil, err
	}
	if typ != "access" {
		return uuid.Nil, errors.New("not an access token")
	}
	return merchantID, nil
}

// verifyRefresh accepts only a REFRESH token (used by the refresh endpoint).
func (h *sessionHandler) verifyRefresh(tokenStr string) (uuid.UUID, error) {
	merchantID, typ, err := h.parseToken(tokenStr)
	if err != nil {
		return uuid.Nil, err
	}
	if typ != "refresh" {
		return uuid.Nil, errors.New("not a refresh token")
	}
	return merchantID, nil
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
