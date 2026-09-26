package oauthserver

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestAPIKeysScopeIsSensitiveActionOnly(t *testing.T) {
	// The api_keys scope must be advertised as sensitive so the consent screen can
	// highlight it, and it must NOT produce identity claims (it authorizes an
	// action, not a userinfo field).
	scope, ok := LookupScope(ScopeAPIKeys)
	require.True(t, ok, "api_keys must be in the scope catalog")
	assert.True(t, scope.Sensitive, "api_keys must be marked sensitive")
	assert.False(t, scope.OIDC)

	user := &model.User{Username: "carol", Email: "carol@example.com"}
	assert.Empty(t, ClaimsForScopes(user, []string{ScopeAPIKeys}))
}

// setupOAuthTokenTestDB gives AuthorizeAPIKeyCreation an isolated in-memory DB
// holding only the oauth_tokens table, restoring the process DB on cleanup.
func setupOAuthTokenTestDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.OAuthToken{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		_ = sqlDB.Close()
	})
}

func insertOAuthAccessToken(t *testing.T, plain, scopes string, expiresAt int64, revoked bool) {
	t.Helper()
	rec := &model.OAuthToken{
		GrantId:          "grant-" + plain,
		ClientId:         "client-abc",
		UserId:           42,
		Scopes:           scopes,
		AccessExpiresAt:  expiresAt,
		RefreshExpiresAt: expiresAt,
		Revoked:          revoked,
	}
	rec.SetAccessToken(plain)
	require.NoError(t, rec.Insert())
}

func TestAuthorizeAPIKeyCreation(t *testing.T) {
	setupOAuthTokenTestDB(t)
	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()

	insertOAuthAccessToken(t, "at_with_scope", "openid api_keys", future, false)
	insertOAuthAccessToken(t, "at_no_scope", "openid profile", future, false)
	insertOAuthAccessToken(t, "at_revoked", "api_keys", future, true)
	insertOAuthAccessToken(t, "at_expired", "api_keys", past, false)

	// Success: an active token carrying api_keys resolves to its owner and client.
	grant, err := AuthorizeAPIKeyCreation("at_with_scope")
	require.NoError(t, err)
	require.NotNil(t, grant)
	assert.Equal(t, 42, grant.UserId)
	assert.Equal(t, "client-abc", grant.ClientId)

	// A valid, active token WITHOUT the scope is rejected with insufficient_scope,
	// never silently allowed (the scope is the only gate on key creation).
	_, err = AuthorizeAPIKeyCreation("at_no_scope")
	assert.ErrorIs(t, err, ErrOAuthInsufficientScope)

	// Unknown, revoked, and expired tokens are all indistinguishable "not found".
	_, err = AuthorizeAPIKeyCreation("at_unknown")
	assert.ErrorIs(t, err, model.ErrOAuthTokenNotFound)
	_, err = AuthorizeAPIKeyCreation("at_revoked")
	assert.ErrorIs(t, err, model.ErrOAuthTokenNotFound)
	_, err = AuthorizeAPIKeyCreation("at_expired")
	assert.ErrorIs(t, err, model.ErrOAuthTokenNotFound)
	_, err = AuthorizeAPIKeyCreation("")
	assert.ErrorIs(t, err, model.ErrOAuthTokenNotFound)
}
