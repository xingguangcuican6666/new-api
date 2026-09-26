package controller

import (
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/oauthserver"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// This file implements new-api's OAuth 2.0 / OpenID Connect *authorization
// server* (identity-provider) endpoints, letting third-party applications sign
// users in with their new-api account. It is the reverse of the oauth/ package,
// which makes new-api a client of external providers.
//
// The public endpoints (/oauth2/*, /.well-known/openid-configuration) speak the
// OAuth wire format: spec-compliant {error, error_description} bodies with the
// correct 4xx/401 status and Cache-Control: no-store, never the 200-wrapped
// common.Api* dashboard envelope. The consent handlers (/api/oauth-server/*)
// run under a dashboard session and do use the dashboard envelope.

// oauthServerEnabled reports whether the authorization server is turned on.
func oauthServerEnabled() bool {
	return system_setting.GetOAuthServerSettings().Enabled
}

// writeOAuthError renders an RFC 6749 §5.2 error response with no caching.
func writeOAuthError(c *gin.Context, status int, code, description string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(status, gin.H{"error": code, "error_description": description})
}

// renderAuthorizeError shows a terminal /authorize error to the end user when it
// cannot be safely redirected back to the client (unknown client or unregistered
// redirect_uri), as required by RFC 6749 §4.1.2.1.
func renderAuthorizeError(c *gin.Context, description string) {
	c.Header("Cache-Control", "no-store")
	body := "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<title>Authorization Error</title></head>" +
		"<body style=\"font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;\">" +
		"<h1 style=\"font-size:1.25rem;\">Authorization Error</h1><p>" +
		html.EscapeString(description) + "</p></body></html>"
	c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(body))
}

// oauthAppendQuery appends non-empty params to a URL's query string, preserving
// any parameters the client already encoded into the redirect URI.
func oauthAppendQuery(base string, params map[string]string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// oauthRedirectWithParams appends params to a trusted redirect URI and issues a
// 302, or renders a terminal error if the stored URI cannot be parsed.
func oauthRedirectWithParams(c *gin.Context, redirectUri string, params map[string]string) {
	target, err := oauthAppendQuery(redirectUri, params)
	if err != nil {
		renderAuthorizeError(c, "the registered redirect_uri could not be parsed")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusFound, target)
}

// OAuthAuthorize handles GET /oauth2/authorize (RFC 6749 §4.1.1). It validates
// the request, parks it as a one-time pending-authorize record, and redirects
// the browser to the dashboard consent screen, where the end user is
// authenticated (the public endpoint carries no dashboard session) and consent
// binds the request to a user.
func OAuthAuthorize(c *gin.Context) {
	if !oauthServerEnabled() {
		renderAuthorizeError(c, "The OAuth authorization server is not enabled.")
		return
	}

	clientId := strings.TrimSpace(c.Query("client_id"))
	if clientId == "" {
		renderAuthorizeError(c, "missing client_id")
		return
	}
	client, err := model.GetOAuthClientByClientId(clientId)
	if err != nil {
		renderAuthorizeError(c, "unknown client_id")
		return
	}
	if !client.IsEnabled() {
		renderAuthorizeError(c, "this application is disabled")
		return
	}

	redirectUri := strings.TrimSpace(c.Query("redirect_uri"))
	if redirectUri == "" || !client.RedirectUriRegistered(redirectUri) {
		renderAuthorizeError(c, "redirect_uri is not registered for this client")
		return
	}

	// The client and redirect_uri are now trusted, so protocol errors from here
	// are reported back to the client via the redirect (RFC 6749 §4.1.2.1).
	state := c.Query("state")

	if responseType := strings.TrimSpace(c.Query("response_type")); responseType != "code" {
		oauthRedirectWithParams(c, redirectUri, map[string]string{
			"error":             "unsupported_response_type",
			"error_description": "only response_type=code is supported",
			"state":             state,
		})
		return
	}

	requestedScopes := oauthserver.ParseScopes(c.Query("scope"))
	if len(requestedScopes) == 0 {
		requestedScopes = client.GetScopes()
	}
	for _, scope := range requestedScopes {
		if !oauthserver.IsSupportedScope(scope) {
			oauthRedirectWithParams(c, redirectUri, map[string]string{
				"error":             "invalid_scope",
				"error_description": "unknown scope: " + scope,
				"state":             state,
			})
			return
		}
	}
	if !oauthserver.ScopesSubset(requestedScopes, client.GetScopes()) {
		oauthRedirectWithParams(c, redirectUri, map[string]string{
			"error":             "invalid_scope",
			"error_description": "requested scope exceeds what this application may request",
			"state":             state,
		})
		return
	}

	// PKCE (RFC 7636). Public clients MUST use it; confidential clients may.
	codeChallenge := strings.TrimSpace(c.Query("code_challenge"))
	codeChallengeMethod := strings.TrimSpace(c.Query("code_challenge_method"))
	if codeChallenge != "" {
		if !oauthserver.ValidChallengeMethod(codeChallengeMethod) {
			oauthRedirectWithParams(c, redirectUri, map[string]string{
				"error":             "invalid_request",
				"error_description": "code_challenge_method must be S256",
				"state":             state,
			})
			return
		}
	} else if client.IsPublic {
		oauthRedirectWithParams(c, redirectUri, map[string]string{
			"error":             "invalid_request",
			"error_description": "code_challenge is required for public clients",
			"state":             state,
		})
		return
	}

	pending := oauthserver.PendingAuthorize{
		ResponseType:        "code",
		ClientId:            clientId,
		RedirectUri:         redirectUri,
		Scopes:              requestedScopes,
		State:               state,
		Nonce:               strings.TrimSpace(c.Query("nonce")),
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
	}
	requestToken, err := oauthserver.StorePendingAuthorize(pending, system_setting.GetOAuthServerSettings().GetAuthorizationCodeTTL())
	if err != nil {
		common.SysError("oauth authorize: failed to store pending request: " + err.Error())
		oauthRedirectWithParams(c, redirectUri, map[string]string{
			"error":             "server_error",
			"error_description": "failed to start authorization",
			"state":             state,
		})
		return
	}

	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusFound, "/oauth-consent?req="+url.QueryEscape(requestToken))
}

// oauthConsentRequest is the body of the consent approve/deny endpoints, echoing
// back the opaque pending-authorize request token minted by /oauth2/authorize.
type oauthConsentRequest struct {
	Request string `json:"request"`
}

// GetOAuthConsentContext backs GET /api/oauth-server/authorize/context. It reads
// (without consuming) a pending-authorize record so the dashboard consent screen
// can show which application is asking and for which scopes.
func GetOAuthConsentContext(c *gin.Context) {
	if !oauthServerEnabled() {
		common.ApiErrorMsg(c, "the OAuth authorization server is not enabled")
		return
	}
	pending, err := oauthserver.LoadPendingAuthorize(strings.TrimSpace(c.Query("request")))
	if err != nil {
		common.ApiErrorMsg(c, "this authorization request is invalid or has expired")
		return
	}
	client, err := model.GetOAuthClientByClientId(pending.ClientId)
	if err != nil || !client.IsEnabled() {
		common.ApiErrorMsg(c, "this application is unavailable")
		return
	}

	grant, err := model.GetOAuthUserGrant(c.GetInt("id"), client.ClientId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	alreadyAuthorized := grant != nil && oauthserver.ScopesSubset(pending.Scopes, grant.GetScopes())

	scopes := make([]gin.H, 0, len(pending.Scopes))
	for _, name := range pending.Scopes {
		scope, ok := oauthserver.LookupScope(name)
		if !ok {
			continue
		}
		scopes = append(scopes, gin.H{"name": scope.Name, "title": scope.Title, "description": scope.Description, "sensitive": scope.Sensitive})
	}

	common.ApiSuccess(c, gin.H{
		"client": gin.H{
			"name":        client.Name,
			"description": client.Description,
			"logo":        client.Logo,
			"homepage":    client.Homepage,
		},
		"scopes":             scopes,
		"already_authorized": alreadyAuthorized,
	})
}

// ApproveOAuthConsent backs POST /api/oauth-server/authorize/approve. It consumes
// the pending request, records the user's consent, mints a single-use
// authorization code bound to the user, and returns the redirect URL the SPA
// should send the browser to. Consuming the request on approve (and deny)
// prevents the request token from being replayed.
func ApproveOAuthConsent(c *gin.Context) {
	if !oauthServerEnabled() {
		common.ApiErrorMsg(c, "the OAuth authorization server is not enabled")
		return
	}
	var req oauthConsentRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Request) == "" {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	pending, err := oauthserver.ConsumePendingAuthorize(strings.TrimSpace(req.Request))
	if err != nil {
		common.ApiErrorMsg(c, "this authorization request is invalid or has expired")
		return
	}
	client, err := model.GetOAuthClientByClientId(pending.ClientId)
	if err != nil || !client.IsEnabled() {
		common.ApiErrorMsg(c, "this application is unavailable")
		return
	}
	// Re-check scopes against the client's current allow-list: it may have been
	// narrowed while the consent screen was open.
	if !oauthserver.ScopesSubset(pending.Scopes, client.GetScopes()) {
		redirect, err := oauthAppendQuery(pending.RedirectUri, map[string]string{
			"error":             "invalid_scope",
			"error_description": "requested scope exceeds what this application may request",
			"state":             pending.State,
		})
		if err != nil {
			common.ApiErrorMsg(c, "the registered redirect_uri could not be parsed")
			return
		}
		common.ApiSuccess(c, gin.H{"redirect_uri": redirect})
		return
	}

	userId := c.GetInt("id")
	if err := model.UpsertOAuthUserGrant(userId, client.ClientId, pending.Scopes); err != nil {
		common.ApiError(c, err)
		return
	}

	codeStr, err := oauthserver.IssueAuthorizationCode(oauthserver.AuthorizationCode{
		ClientId:            client.ClientId,
		RedirectUri:         pending.RedirectUri,
		Scopes:              pending.Scopes,
		Nonce:               pending.Nonce,
		CodeChallenge:       pending.CodeChallenge,
		CodeChallengeMethod: pending.CodeChallengeMethod,
		UserId:              userId,
		AuthTime:            time.Now().Unix(),
	}, system_setting.GetOAuthServerSettings().GetAuthorizationCodeTTL())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	redirect, err := oauthAppendQuery(pending.RedirectUri, map[string]string{"code": codeStr, "state": pending.State})
	if err != nil {
		common.ApiErrorMsg(c, "the registered redirect_uri could not be parsed")
		return
	}
	common.ApiSuccess(c, gin.H{"redirect_uri": redirect})
}

// DenyOAuthConsent backs POST /api/oauth-server/authorize/deny. It consumes the
// pending request and returns the redirect URL carrying error=access_denied
// (RFC 6749 §4.1.2.1).
func DenyOAuthConsent(c *gin.Context) {
	var req oauthConsentRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Request) == "" {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	pending, err := oauthserver.ConsumePendingAuthorize(strings.TrimSpace(req.Request))
	if err != nil {
		common.ApiErrorMsg(c, "this authorization request is invalid or has expired")
		return
	}
	redirect, err := oauthAppendQuery(pending.RedirectUri, map[string]string{
		"error":             "access_denied",
		"error_description": "the user denied the request",
		"state":             pending.State,
	})
	if err != nil {
		common.ApiErrorMsg(c, "the registered redirect_uri could not be parsed")
		return
	}
	common.ApiSuccess(c, gin.H{"redirect_uri": redirect})
}

// authenticateOAuthClient resolves and authenticates the client making a token
// or revocation request (RFC 6749 §2.3). Credentials arrive via HTTP Basic auth
// or as client_id/client_secret form fields; a public (PKCE-only) client
// authenticates by client_id alone. On failure it writes the RFC 6749 §5.2
// invalid_client error itself (401, adding WWW-Authenticate: Basic when Basic
// was attempted) and returns nil.
func authenticateOAuthClient(c *gin.Context) *model.OAuthClient {
	clientId, clientSecret, hasBasic := c.Request.BasicAuth()
	if !hasBasic {
		clientId = c.PostForm("client_id")
		clientSecret = c.PostForm("client_secret")
	}
	client, err := model.GetOAuthClientByClientId(strings.TrimSpace(clientId))
	if err != nil || !client.IsEnabled() {
		writeOAuthClientAuthError(c, hasBasic)
		return nil
	}
	if client.IsPublic {
		// Public clients hold no secret; PKCE binds the code to the client.
		return client
	}
	if !client.ValidateSecret(clientSecret) {
		writeOAuthClientAuthError(c, hasBasic)
		return nil
	}
	return client
}

// writeOAuthClientAuthError reports a failed client authentication per RFC 6749
// §5.2, challenging with Basic when the client attempted Basic auth.
func writeOAuthClientAuthError(c *gin.Context, usedBasic bool) {
	if usedBasic {
		c.Header("WWW-Authenticate", `Basic realm="oauth"`)
	}
	writeOAuthError(c, http.StatusUnauthorized, "invalid_client", "client authentication failed")
}

// OAuthToken handles POST /oauth2/token (RFC 6749 §3.2), issuing tokens for the
// authorization_code and refresh_token grants after authenticating the client.
func OAuthToken(c *gin.Context) {
	if !oauthServerEnabled() {
		writeOAuthError(c, http.StatusBadRequest, "invalid_request", "the OAuth authorization server is not enabled")
		return
	}
	client := authenticateOAuthClient(c)
	if client == nil {
		return
	}
	switch c.PostForm("grant_type") {
	case "authorization_code":
		handleOAuthAuthorizationCodeGrant(c, client)
	case "refresh_token":
		handleOAuthRefreshTokenGrant(c, client)
	default:
		writeOAuthError(c, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
}

// handleOAuthAuthorizationCodeGrant exchanges a consumed authorization code for
// tokens, enforcing client binding, redirect_uri match, and PKCE (RFC 6749
// §4.1.3, RFC 7636 §4.6).
func handleOAuthAuthorizationCodeGrant(c *gin.Context, client *model.OAuthClient) {
	codeStr := strings.TrimSpace(c.PostForm("code"))
	if codeStr == "" {
		writeOAuthError(c, http.StatusBadRequest, "invalid_request", "missing code")
		return
	}
	code, err := oauthserver.ConsumeAuthorizationCode(codeStr)
	if err != nil {
		writeOAuthError(c, http.StatusBadRequest, "invalid_grant", "the authorization code is invalid or has expired")
		return
	}
	if code.ClientId != client.ClientId {
		writeOAuthError(c, http.StatusBadRequest, "invalid_grant", "the authorization code was issued to another client")
		return
	}
	// redirect_uri was required at /authorize, so it must match here exactly.
	if strings.TrimSpace(c.PostForm("redirect_uri")) != code.RedirectUri {
		writeOAuthError(c, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request")
		return
	}
	if code.CodeChallenge != "" {
		verifier := strings.TrimSpace(c.PostForm("code_verifier"))
		if verifier == "" {
			writeOAuthError(c, http.StatusBadRequest, "invalid_request", "code_verifier is required")
			return
		}
		if !oauthserver.VerifyCodeChallenge(verifier, code.CodeChallenge) {
			writeOAuthError(c, http.StatusBadRequest, "invalid_grant", "code_verifier does not match the code_challenge")
			return
		}
	}
	issued, err := oauthserver.IssueForAuthorizationCode(oauthserver.GrantContext{
		Client:   client,
		UserId:   code.UserId,
		Scopes:   code.Scopes,
		Nonce:    code.Nonce,
		AuthTime: code.AuthTime,
	})
	if err != nil {
		common.SysError("oauth token: failed to issue tokens: " + err.Error())
		writeOAuthError(c, http.StatusInternalServerError, "server_error", "failed to issue tokens")
		return
	}
	writeOAuthTokenResponse(c, issued)
}

// handleOAuthRefreshTokenGrant rotates a refresh token into a fresh token set
// (RFC 6749 §6). A reused, unknown, or expired refresh token is reported as
// invalid_grant.
func handleOAuthRefreshTokenGrant(c *gin.Context, client *model.OAuthClient) {
	refreshToken := strings.TrimSpace(c.PostForm("refresh_token"))
	if refreshToken == "" {
		writeOAuthError(c, http.StatusBadRequest, "invalid_request", "missing refresh_token")
		return
	}
	issued, err := oauthserver.RefreshGrant(client, refreshToken)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrOAuthTokenNotFound),
			errors.Is(err, model.ErrOAuthRefreshReused),
			errors.Is(err, model.ErrOAuthRefreshExpired):
			writeOAuthError(c, http.StatusBadRequest, "invalid_grant", "the refresh token is invalid or has expired")
		default:
			common.SysError("oauth token: refresh failed: " + err.Error())
			writeOAuthError(c, http.StatusInternalServerError, "server_error", "failed to refresh tokens")
		}
		return
	}
	writeOAuthTokenResponse(c, issued)
}

// writeOAuthTokenResponse renders a successful RFC 6749 §5.1 token response,
// omitting the refresh and ID tokens when the grant did not produce them.
func writeOAuthTokenResponse(c *gin.Context, issued *oauthserver.IssuedTokens) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	body := gin.H{
		"access_token": issued.AccessToken,
		"token_type":   issued.TokenType,
		"expires_in":   issued.ExpiresIn,
		"scope":        issued.Scope,
	}
	if issued.RefreshToken != "" {
		body["refresh_token"] = issued.RefreshToken
	}
	if issued.IDToken != "" {
		body["id_token"] = issued.IDToken
	}
	c.JSON(http.StatusOK, body)
}

// OAuthUserInfo handles GET/POST /oauth2/userinfo (OpenID Connect Core §5.3),
// returning the claim set the presented access token's scopes permit.
func OAuthUserInfo(c *gin.Context) {
	if !oauthServerEnabled() {
		writeOAuthError(c, http.StatusBadRequest, "invalid_request", "the OAuth authorization server is not enabled")
		return
	}
	accessToken := bearerAccessToken(c)
	if accessToken == "" {
		c.Header("WWW-Authenticate", `Bearer realm="oauth"`)
		writeOAuthError(c, http.StatusUnauthorized, "invalid_token", "a bearer access token is required")
		return
	}
	claims, err := oauthserver.UserInfo(accessToken)
	if err != nil {
		c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
		writeOAuthError(c, http.StatusUnauthorized, "invalid_token", "the access token is invalid or has expired")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, claims)
}

// bearerAccessToken extracts an OAuth access token from the Authorization:
// Bearer header (RFC 6750 §2.1, scheme matched case-insensitively) or an
// access_token form field (§2.2). URI query tokens are not accepted (§2.3).
func bearerAccessToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if len(header) >= 7 && strings.EqualFold(header[:7], "Bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return strings.TrimSpace(c.PostForm("access_token"))
}

// oauthCreateAPIKeyRequest is the optional body of POST /oauth2/keys. Every
// field is optional and bounded server-side; the client can influence only the
// spending limits and expiry, never the key value or the owning user.
type oauthCreateAPIKeyRequest struct {
	RemainQuota    int   `json:"remain_quota"`
	UnlimitedQuota bool  `json:"unlimited_quota"`
	ExpiredTime    int64 `json:"expired_time"`
}

// OAuthCreateAPIKey handles POST /oauth2/keys. A third-party application holding
// an access token granted the api_keys scope creates a new-api API key (relay
// token) that belongs to the token's resource owner and receives its value once.
// This mirrors the dashboard's AddToken, except the owning user is taken from the
// validated access token (never a request field) and the same server-side quota
// and per-user count limits apply.
func OAuthCreateAPIKey(c *gin.Context) {
	if !oauthServerEnabled() {
		writeOAuthError(c, http.StatusBadRequest, "invalid_request", "the OAuth authorization server is not enabled")
		return
	}
	accessToken := bearerAccessToken(c)
	if accessToken == "" {
		c.Header("WWW-Authenticate", `Bearer realm="oauth"`)
		writeOAuthError(c, http.StatusUnauthorized, "invalid_token", "a bearer access token is required")
		return
	}
	grant, err := oauthserver.AuthorizeAPIKeyCreation(accessToken)
	if err != nil {
		if errors.Is(err, oauthserver.ErrOAuthInsufficientScope) {
			c.Header("WWW-Authenticate", `Bearer error="insufficient_scope", scope="api_keys"`)
			writeOAuthError(c, http.StatusForbidden, "insufficient_scope", "the api_keys scope is required to create API keys")
			return
		}
		c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
		writeOAuthError(c, http.StatusUnauthorized, "invalid_token", "the access token is invalid or has expired")
		return
	}
	var req oauthCreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		writeOAuthError(c, http.StatusBadRequest, "invalid_request", "the request body is invalid")
		return
	}
	if !req.UnlimitedQuota {
		if req.RemainQuota < 0 {
			writeOAuthError(c, http.StatusBadRequest, "invalid_request", "remain_quota must not be negative")
			return
		}
		if maxQuota := maxTokenQuota(); req.RemainQuota > maxQuota {
			writeOAuthError(c, http.StatusBadRequest, "invalid_request", fmt.Sprintf("remain_quota exceeds the maximum of %d", maxQuota))
			return
		}
	}

	maxTokens := operation_setting.GetMaxUserTokens()
	count, err := model.CountUserTokens(grant.UserId)
	if err != nil {
		common.SysError("oauth create api key: count tokens failed: " + err.Error())
		writeOAuthError(c, http.StatusInternalServerError, "server_error", "failed to create the API key")
		return
	}
	if int(count) >= maxTokens {
		writeOAuthError(c, http.StatusForbidden, "access_denied", "the account has reached its maximum number of API keys")
		return
	}
	// Name the key after the requesting application so the user can recognize and
	// revoke it from their key list; fall back to a generic label. Keep it within
	// the same 50-byte bound the dashboard enforces, truncating on a UTF-8 boundary.
	name := "OAuth application"
	if client, cErr := model.GetOAuthClientByClientId(grant.ClientId); cErr == nil && strings.TrimSpace(client.Name) != "" {
		name = strings.TrimSpace(client.Name)
	}
	if len(name) > 50 {
		name = name[:50]
		for len(name) > 0 && !utf8.ValidString(name) {
			name = name[:len(name)-1]
		}
	}

	expiredTime := req.ExpiredTime
	if expiredTime == 0 {
		// A zero expiry would place the key at the Unix epoch (already expired);
		// treat an omitted expiry as "never", matching the token model default.
		expiredTime = -1
	}

	key, err := common.GenerateKey()
	if err != nil {
		common.SysError("oauth create api key: generate key failed: " + err.Error())
		writeOAuthError(c, http.StatusInternalServerError, "server_error", "failed to create the API key")
		return
	}
	now := common.GetTimestamp()
	token := model.Token{
		UserId:         grant.UserId,
		Name:           name,
		Key:            key,
		CreatedTime:    now,
		AccessedTime:   now,
		ExpiredTime:    expiredTime,
		RemainQuota:    req.RemainQuota,
		UnlimitedQuota: req.UnlimitedQuota,
	}
	if err := token.Insert(); err != nil {
		common.SysError("oauth create api key: insert failed: " + err.Error())
		writeOAuthError(c, http.StatusInternalServerError, "server_error", "failed to create the API key")
		return
	}
	// Audit context only: identify who minted a key for whom. Never the key value.
	common.SysLog(fmt.Sprintf("oauth: client %s created API key id=%d for user %d", grant.ClientId, token.Id, grant.UserId))

	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusCreated, gin.H{
		"key":             "sk-" + token.Key,
		"id":              token.Id,
		"name":            token.Name,
		"unlimited_quota": token.UnlimitedQuota,
		"remain_quota":    token.RemainQuota,
		"expires_at":      token.ExpiredTime,
	})
}

// OAuthDiscovery serves GET /.well-known/openid-configuration (OpenID Connect
// Discovery 1.0), advertising the server's endpoints and capabilities.
func OAuthDiscovery(c *gin.Context) {
	if !oauthServerEnabled() {
		writeOAuthError(c, http.StatusNotFound, "not_found", "the OAuth authorization server is not enabled")
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, oauthserver.Metadata(system_setting.GetOAuthServerSettings().GetIssuer()))
}

// OAuthJWKS serves GET /oauth2/jwks, the key set relying parties use to verify
// ID token signatures.
func OAuthJWKS(c *gin.Context) {
	if !oauthServerEnabled() {
		writeOAuthError(c, http.StatusNotFound, "not_found", "the OAuth authorization server is not enabled")
		return
	}
	jwks, err := oauthserver.JWKS()
	if err != nil {
		common.SysError("oauth jwks: " + err.Error())
		writeOAuthError(c, http.StatusInternalServerError, "server_error", "failed to build the key set")
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, jwks)
}

// OAuthRevoke handles POST /oauth2/revoke (RFC 7009), revoking the presented
// access or refresh token when it belongs to the authenticated client. Per §2.2
// the endpoint returns 200 even for an unknown token.
func OAuthRevoke(c *gin.Context) {
	if !oauthServerEnabled() {
		writeOAuthError(c, http.StatusBadRequest, "invalid_request", "the OAuth authorization server is not enabled")
		return
	}
	client := authenticateOAuthClient(c)
	if client == nil {
		return
	}
	if err := model.RevokeOAuthTokenForClient(client.ClientId, strings.TrimSpace(c.PostForm("token"))); err != nil {
		common.SysError("oauth revoke: " + err.Error())
		writeOAuthError(c, http.StatusInternalServerError, "server_error", "failed to revoke the token")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
}
