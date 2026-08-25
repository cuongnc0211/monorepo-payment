package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// publishableOnly is the mirror of secretOnly: it guards routes that a browser calls with the
// PUBLISHABLE key. A secret key must never travel to a browser, so it is refused here — the
// opposite direction from the money endpoints.
func publishableOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		kind, ok := KeyKind(c)
		if !ok {
			respondError(c, http.StatusUnauthorized, errTypeAuth, "authentication_required",
				"authentication is required for this request", "")
			return
		}
		if kind != "publishable" {
			respondError(c, http.StatusForbidden, errTypeAuth, "publishable_key_required",
				"this endpoint must be called with a publishable key; a secret key must never be exposed to a browser", "")
			return
		}
		c.Next()
	}
}

type tokenHandler struct{}

// createTokenRequest is the POST /v1/tokens body: the raw card fields, sent straight from the
// browser. The PAN reaches the gateway (never the merchant) and is used only to derive brand/last4
// and to select the test outcome — it is never stored.
type createTokenRequest struct {
	Number   string `json:"number"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
	CVC      string `json:"cvc"`
}

// create tokenizes a card into a single-use payment-method token.
//
// Test-gateway simplification (see plans/004): the returned token is the bank-sim magic token the
// card maps to, so the existing confirm → bank-sim path is unchanged. A production gateway would
// return an opaque, single-use token backed by a vault; the response shape (id + card summary) is
// the same either way, so the merchant integration would not change.
func (h *tokenHandler) create(c *gin.Context) {
	var req createTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_body",
			"request body must be valid JSON", "")
		return
	}

	number := strings.ReplaceAll(strings.TrimSpace(req.Number), " ", "")
	switch {
	case !digitsOnly(number) || len(number) < 13 || len(number) > 19 || !luhnValid(number):
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_card_number",
			"the card number is not valid", "number")
		return
	case req.ExpMonth < 1 || req.ExpMonth > 12:
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_expiry",
			"exp_month must be between 1 and 12", "exp_month")
		return
	case req.ExpYear < 2000 || req.ExpYear > 2100:
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_expiry",
			"exp_year is out of range", "exp_year")
		return
	case len(req.CVC) < 3 || len(req.CVC) > 4 || !digitsOnly(req.CVC):
		respondError(c, http.StatusBadRequest, errTypeInvalidRequest, "invalid_cvc",
			"the security code is not valid", "cvc")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":     magicTokenForCard(number),
		"object": "token",
		"card": gin.H{
			"brand":     cardBrand(number),
			"last4":     number[len(number)-4:],
			"exp_month": req.ExpMonth,
			"exp_year":  req.ExpYear,
		},
	})
}

// magicTokenForCard maps the documented test cards to the bank-sim outcome tokens; any other valid
// card approves. These are the only "cards" the system understands — there is no real PAN
// processing anywhere.
func magicTokenForCard(number string) string {
	switch number {
	case "4000000000000002":
		return "tok_declined"
	case "4000000000000069":
		return "tok_timeout"
	default: // 4242424242424242 and any other valid card
		return "tok_ok"
	}
}

func cardBrand(number string) string {
	switch number[0] {
	case '4':
		return "visa"
	case '5':
		return "mastercard"
	case '3':
		return "amex"
	default:
		return "card"
	}
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// luhnValid runs the Luhn checksum so obviously-mistyped numbers are rejected client-and-server
// side, matching what a real card form does before tokenizing.
func luhnValid(number string) bool {
	sum, alt := 0, false
	for i := len(number) - 1; i >= 0; i-- {
		d := int(number[i] - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}
