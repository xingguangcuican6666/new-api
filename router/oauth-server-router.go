package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetOAuthServerRouter registers new-api's public OAuth 2.0 / OpenID Connect
// authorization-server endpoints. They speak the OAuth wire format (not the
// dashboard JSON envelope) and are reached without a dashboard session, so the
// group carries its own CORS and rate limiting rather than inheriting the /api
// chain. Credential-sensitive endpoints (authorize, token, revoke, keys) are
// rate limited; discovery, JWKS, and userinfo are not (the first two are
// cacheable and userinfo already requires a valid bearer token).
func SetOAuthServerRouter(router *gin.Engine) {
	oauthRouter := router.Group("")
	oauthRouter.Use(middleware.CORS())
	oauthRouter.Use(middleware.RouteTag("oauth-server"))
	{
		oauthRouter.GET("/.well-known/openid-configuration", controller.OAuthDiscovery)
		oauthRouter.GET("/oauth2/jwks", controller.OAuthJWKS)
		oauthRouter.GET("/oauth2/authorize", middleware.CriticalRateLimit(), controller.OAuthAuthorize)
		oauthRouter.POST("/oauth2/token", middleware.CriticalRateLimit(), controller.OAuthToken)
		oauthRouter.GET("/oauth2/userinfo", controller.OAuthUserInfo)
		oauthRouter.POST("/oauth2/userinfo", controller.OAuthUserInfo)
		oauthRouter.POST("/oauth2/revoke", middleware.CriticalRateLimit(), controller.OAuthRevoke)
		oauthRouter.POST("/oauth2/keys", middleware.CriticalRateLimit(), controller.OAuthCreateAPIKey)
	}
}
