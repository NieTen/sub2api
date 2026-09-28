package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelDetectionRoutesUseAdminGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/admin")
	group.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Admin") != "yes" {
			c.AbortWithStatus(http.StatusForbidden)
		}
	})
	h := &handler.Handlers{Admin: &handler.AdminHandlers{ModelDetection: admin.NewModelDetectionHandler(nil)}}
	registerModelDetectionRoutes(group, h)
	require.Len(t, router.Routes(), 11)
	for _, route := range router.Routes() {
		req := httptest.NewRequest(route.Method, route.Path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code, route.Path)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/model-detection/catalog", nil)
	req.Header.Set("X-Test-Admin", "yes")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"requests_per_run":4`)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/model-detection/catalog", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
