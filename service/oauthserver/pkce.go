package oauthserver

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// PKCEMethodS256 is the only code challenge method new-api accepts. The plain
// method is intentionally unsupported (OAuth 2.1 / RFC 7636 §7.2 discourage it).
const PKCEMethodS256 = "S256"

// ValidChallengeMethod reports whether method is a code challenge method the
// server supports. An empty method is not valid here; callers decide whether
// PKCE is optional for a given client.
func ValidChallengeMethod(method string) bool {
	return method == PKCEMethodS256
}

// VerifyCodeChallenge checks a PKCE code_verifier against the stored challenge
// using S256 (RFC 7636 §4.6): BASE64URL(SHA256(verifier)) must equal challenge.
// The comparison is constant time to avoid leaking the challenge.
func VerifyCodeChallenge(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}
