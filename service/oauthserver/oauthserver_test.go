package oauthserver

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidChallengeMethod(t *testing.T) {
	// Only S256 is accepted; plain and empty are rejected (RFC 7636 §7.2).
	assert.True(t, ValidChallengeMethod(PKCEMethodS256))
	assert.False(t, ValidChallengeMethod("plain"))
	assert.False(t, ValidChallengeMethod(""))
}

func TestVerifyCodeChallenge(t *testing.T) {
	// RFC 7636 Appendix B worked example: challenge = BASE64URL(SHA256(verifier)).
	const (
		verifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)
	assert.True(t, VerifyCodeChallenge(verifier, challenge))
	assert.False(t, VerifyCodeChallenge(verifier+"tampered", challenge))
	assert.False(t, VerifyCodeChallenge("", challenge))
	assert.False(t, VerifyCodeChallenge(verifier, ""))
}

func TestParseScopes(t *testing.T) {
	// De-duplicates on first-seen order, dropping empty/whitespace tokens.
	assert.Equal(t, []string{"openid", "profile", "email"}, ParseScopes("openid profile email"))
	assert.Equal(t, []string{"openid", "profile"}, ParseScopes("  openid   profile  openid "))
	assert.Empty(t, ParseScopes("   "))
	assert.Empty(t, ParseScopes(""))
}

func TestScopesSubsetAndSupport(t *testing.T) {
	allow := []string{"openid", "profile", "email"}
	assert.True(t, ScopesSubset([]string{"openid", "email"}, allow))
	assert.True(t, ScopesSubset(nil, allow))
	assert.False(t, ScopesSubset([]string{"openid", "groups"}, allow))

	assert.True(t, IsSupportedScope("openid"))
	assert.True(t, IsSupportedScope("groups"))
	assert.False(t, IsSupportedScope("payments"))

	assert.Equal(t, []string{"openid", "email"}, FilterSupported([]string{"openid", "payments", "email"}))
	assert.Empty(t, FilterSupported([]string{"payments"}))
}

func TestNormalizeScopeString(t *testing.T) {
	assert.Equal(t, "openid profile", NormalizeScopeString("  openid   openid profile "))
	assert.Equal(t, "", NormalizeScopeString("   "))
}

func TestClaimsForScopes(t *testing.T) {
	user := &model.User{
		Username:    "alice",
		DisplayName: "Alice Example",
		Email:       "alice@example.com",
		Role:        10,
		Group:       "default",
		Groups:      "vip,staff",
	}

	claims := ClaimsForScopes(user, []string{"profile", "email", "groups"})
	assert.Equal(t, "alice", claims["preferred_username"])
	assert.Equal(t, "Alice Example", claims["name"])
	assert.Equal(t, "alice@example.com", claims["email"])
	assert.Equal(t, true, claims["email_verified"])
	assert.Equal(t, 10, claims["role"])
	// Primary group plus extra groups, de-duplicated and sorted.
	assert.Equal(t, []string{"default", "staff", "vip"}, claims["groups"])

	// The openid scope alone produces no identity claims here (sub is added by
	// the ID-token/userinfo assembler, not by ClaimsForScopes).
	assert.Empty(t, ClaimsForScopes(user, []string{"openid"}))
	assert.Empty(t, ClaimsForScopes(user, nil))

	// DisplayName falls back to username; an empty email yields no email claim.
	minimal := &model.User{Username: "bob"}
	c2 := ClaimsForScopes(minimal, []string{"profile", "email"})
	assert.Equal(t, "bob", c2["name"])
	_, hasEmail := c2["email"]
	assert.False(t, hasEmail)
	_, hasVerified := c2["email_verified"]
	assert.False(t, hasVerified)
}

func TestMetadataDerivesEndpoints(t *testing.T) {
	const issuer = "https://id.example.com"
	meta := Metadata(issuer)
	assert.Equal(t, issuer, meta["issuer"])
	assert.Equal(t, issuer+"/oauth2/authorize", meta["authorization_endpoint"])
	assert.Equal(t, issuer+"/oauth2/token", meta["token_endpoint"])
	assert.Equal(t, issuer+"/oauth2/userinfo", meta["userinfo_endpoint"])
	assert.Equal(t, issuer+"/oauth2/jwks", meta["jwks_uri"])
	assert.Equal(t, issuer+"/oauth2/revoke", meta["revocation_endpoint"])
	assert.Equal(t, []string{"code"}, meta["response_types_supported"])
	assert.Equal(t, []string{"authorization_code", "refresh_token"}, meta["grant_types_supported"])
	assert.Equal(t, []string{PKCEMethodS256}, meta["code_challenge_methods_supported"])
	require.Contains(t, meta, "scopes_supported")
	assert.Contains(t, meta["scopes_supported"], ScopeOpenID)
}
