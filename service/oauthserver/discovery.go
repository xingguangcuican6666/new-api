package oauthserver

// Metadata builds the OpenID Connect discovery document
// (.well-known/openid-configuration) for the given issuer identifier. All
// endpoint URLs are derived from the issuer so a single ServerAddress / Issuer
// setting drives the whole surface.
func Metadata(issuer string) map[string]any {
	return map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth2/authorize",
		"token_endpoint":                        issuer + "/oauth2/token",
		"userinfo_endpoint":                     issuer + "/oauth2/userinfo",
		"jwks_uri":                              issuer + "/oauth2/jwks",
		"revocation_endpoint":                   issuer + "/oauth2/revoke",
		"scopes_supported":                      SupportedScopeNames(),
		"response_types_supported":              []string{"code"},
		"response_modes_supported":              []string{"query"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
		"code_challenge_methods_supported":      []string{PKCEMethodS256},
		"claims_supported": []string{
			"iss", "sub", "aud", "exp", "iat", "auth_time", "nonce",
			"preferred_username", "name", "email", "email_verified", "groups", "role",
		},
	}
}
