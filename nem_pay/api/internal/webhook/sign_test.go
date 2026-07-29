package webhook

import (
	"strings"
	"testing"
)

func TestSign_DeterministicAndPrefixed(t *testing.T) {
	body := []byte(`{"event":"payment_intent.captured"}`)
	sig := Sign("whsec_test", body)
	if !strings.HasPrefix(sig, "sha256=") {
		t.Fatalf("signature missing sha256= prefix: %q", sig)
	}
	if sig != Sign("whsec_test", body) {
		t.Fatalf("signature is not deterministic")
	}
}

func TestVerify_AcceptsGoodRejectsTampered(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"amount":250000}`)
	sig := Sign(secret, body)

	if !Verify(secret, body, sig) {
		t.Fatalf("valid signature rejected")
	}
	if Verify("wrong_secret", body, sig) {
		t.Fatalf("signature verified under the wrong secret")
	}
	if Verify(secret, []byte(`{"amount":250001}`), sig) {
		t.Fatalf("signature verified over a tampered body")
	}
}
