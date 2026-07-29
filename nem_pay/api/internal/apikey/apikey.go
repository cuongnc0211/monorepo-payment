// Package apikey handles the non-secret bookkeeping around merchant API keys: deriving the
// indexed lookup prefix and hashing the raw token for storage/comparison. The raw token is
// never persisted — only its hash — so a leaked api_keys table yields no usable credentials.
package apikey

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// PrefixLen is how many leading characters form the indexed, non-secret lookup prefix. It must
// be long enough to reach PAST the static label ("sk_test_", 8 chars) into the token's RANDOM
// part, so the prefix is per-key SELECTIVE — otherwise every secret key shares one prefix and
// the "narrow by prefix" lookup degrades to scanning all secret keys on the auth hot path. The
// prefix is still non-secret: it reveals a slice of the token but authentication requires the
// full token to hash-match. (Real tokens must therefore carry random entropy in chars [8:16).)
const PrefixLen = 16

// Prefix returns the lookup prefix for a token (the whole token if shorter than PrefixLen).
func Prefix(token string) string {
	if len(token) < PrefixLen {
		return token
	}
	return token[:PrefixLen]
}

// Hash returns the hex-encoded SHA-256 of a raw token, the form stored in api_keys.token_hash.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Equal compares two token hashes in constant time, so a caller can't learn the stored hash
// byte-by-byte from response timing.
func Equal(hashA, hashB string) bool {
	return subtle.ConstantTimeCompare([]byte(hashA), []byte(hashB)) == 1
}
