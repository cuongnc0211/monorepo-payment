// Package webhook is the read side of the notification plane: signing and delivering outbox
// events to merchant endpoints out-of-band.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignatureHeader is the header carrying the payload signature.
const SignatureHeader = "X-NemPay-Signature"

// Sign returns the value for X-NemPay-Signature: "sha256=" + hex(HMAC-SHA256(secret, body)).
// The receiver recomputes this over the raw body with the shared secret to prove the payload is
// authentic and unmodified — the same scheme Stripe uses.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether sig matches the HMAC of body under secret, in constant time. Provided
// for receivers (and tests); the gateway itself only signs.
func Verify(secret string, body []byte, sig string) bool {
	return hmac.Equal([]byte(sig), []byte(Sign(secret, body)))
}
