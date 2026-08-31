package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardRegistersCreditGrantsRoutesWithTokenAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetDashboardRouter(engine)

	routes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, path := range []string{
		"/dashboard/billing/credit_grants",
		"/v1/dashboard/billing/credit_grants",
	} {
		_, registered := routes[http.MethodGet+" "+path]
		require.True(t, registered, path)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		engine.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusUnauthorized, recorder.Code, path)
	}
}
