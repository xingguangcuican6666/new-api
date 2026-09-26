package oauthserver

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

// Scope is a single OAuth scope the authorization server understands. Title and
// Description are English fallbacks (used in discovery metadata and for clients
// that do not localize); the frontend consent screen localizes by Name.
type Scope struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// OIDC marks scopes that map to OpenID Connect standard claims.
	OIDC bool `json:"oidc"`
	// Sensitive marks scopes that grant an action beyond reading identity claims
	// (for example, minting API keys). The consent screen highlights these so the
	// user notices the elevated request before approving.
	Sensitive bool `json:"sensitive"`
}

// ScopeOpenID is required for any OpenID Connect flow (issues an ID token).
const ScopeOpenID = "openid"

// ScopeAPIKeys authorizes a client to create API keys (relay tokens) on the
// resource owner's behalf and receive their value, so the client can call the
// API as the user. It is an action authorization, not an identity claim, so it
// is marked Sensitive and implies no ClaimsForScopes entry.
const ScopeAPIKeys = "api_keys"

// supportedScopes is the authorization server's scope catalog. Adding a scope
// here makes it available for clients to allow and for users to grant; wire any
// new claims it implies into ClaimsForScopes, and mark it Sensitive when it
// grants an action rather than exposing an identity claim.
var supportedScopes = []Scope{
	{Name: ScopeOpenID, Title: "Sign you in", Description: "Verify your identity and sign you in", OIDC: true},
	{Name: "profile", Title: "Basic profile", Description: "Your username and display name", OIDC: true},
	{Name: "email", Title: "Email address", Description: "Your email address", OIDC: true},
	{Name: "groups", Title: "Groups and role", Description: "Your account groups and role", OIDC: false},
	{Name: ScopeAPIKeys, Title: "Create API keys", Description: "Create API keys on your account and use them to call the API on your behalf", OIDC: false, Sensitive: true},
}

var scopeIndex = func() map[string]Scope {
	m := make(map[string]Scope, len(supportedScopes))
	for _, s := range supportedScopes {
		m[s.Name] = s
	}
	return m
}()

// SupportedScopes returns the catalog in a stable order.
func SupportedScopes() []Scope {
	out := make([]Scope, len(supportedScopes))
	copy(out, supportedScopes)
	return out
}

// SupportedScopeNames returns just the scope identifiers, for discovery metadata.
func SupportedScopeNames() []string {
	names := make([]string, len(supportedScopes))
	for i, s := range supportedScopes {
		names[i] = s.Name
	}
	return names
}

// IsSupportedScope reports whether a scope is in the catalog.
func IsSupportedScope(name string) bool {
	_, ok := scopeIndex[name]
	return ok
}

// LookupScope returns the catalog entry for a scope name.
func LookupScope(name string) (Scope, bool) {
	s, ok := scopeIndex[name]
	return s, ok
}

// ParseScopes splits a space-delimited scope string into a de-duplicated,
// order-preserving slice. Empty and whitespace-only tokens are dropped.
func ParseScopes(raw string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, tok := range strings.Fields(raw) {
		if _, dup := seen[tok]; dup {
			continue
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}
	return out
}

// JoinScopes renders a scope slice back to the space-delimited wire format.
func JoinScopes(scopes []string) string {
	return strings.Join(scopes, " ")
}

// NormalizeScopeString parses and rejoins so stored scope strings are canonical.
func NormalizeScopeString(raw string) string {
	return JoinScopes(ParseScopes(raw))
}

// FilterSupported keeps only scopes present in the catalog, preserving order.
func FilterSupported(scopes []string) []string {
	var out []string
	for _, s := range scopes {
		if IsSupportedScope(s) {
			out = append(out, s)
		}
	}
	return out
}

// ScopesSubset reports whether every scope in want is present in allow.
func ScopesSubset(want, allow []string) bool {
	allowed := make(map[string]struct{}, len(allow))
	for _, s := range allow {
		allowed[s] = struct{}{}
	}
	for _, s := range want {
		if _, ok := allowed[s]; !ok {
			return false
		}
	}
	return true
}

// ContainsScope reports whether scopes includes name.
func ContainsScope(scopes []string, name string) bool {
	for _, s := range scopes {
		if s == name {
			return true
		}
	}
	return false
}

// ClaimsForScopes builds the userinfo / ID-token claim set granted by scopes for
// a given user. The "sub" claim is always included when any claims are produced;
// callers requiring OIDC must ensure ScopeOpenID was granted. Group and role
// data is only exposed when the "groups" scope is present, matching consent.
func ClaimsForScopes(user *model.User, scopes []string) map[string]any {
	claims := make(map[string]any)
	for _, scope := range scopes {
		switch scope {
		case "profile":
			claims["preferred_username"] = user.Username
			if user.DisplayName != "" {
				claims["name"] = user.DisplayName
			} else {
				claims["name"] = user.Username
			}
		case "email":
			if user.Email != "" {
				claims["email"] = user.Email
				// new-api verifies email addresses at bind time; a stored,
				// non-empty address is treated as verified.
				claims["email_verified"] = true
			}
		case "groups":
			groups := []string{user.Group}
			groups = append(groups, user.GetExtraGroups()...)
			claims["groups"] = dedupeNonEmpty(groups)
			claims["role"] = user.Role
		}
	}
	return claims
}

func dedupeNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	var out []string
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
