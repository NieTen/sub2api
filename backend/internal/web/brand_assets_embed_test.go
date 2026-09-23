//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrandHomeAssetsEmbedded(t *testing.T) {
	server, err := NewFrontendServer(&mockSettingsProvider{settings: map[string]string{}})
	require.NoError(t, err)

	for name, handler := range map[string]gin.HandlerFunc{
		"settings_server": server.Middleware(),
		"legacy_server":   ServeEmbeddedFrontend(),
	} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.Use(handler)
			for path, contentType := range map[string]string{
				"i2.html":         "text/html",
				"i2.js":           "javascript",
				"model-data.json": "application/json",
			} {
				t.Run(path, func(t *testing.T) {
					// 确认品牌首页及依赖真实随二进制发布，缺失时不能悄悄返回 SPA 首页。
					expected, err := frontendFS.ReadFile("dist/" + path)
					require.NoError(t, err)
					response := httptest.NewRecorder()
					router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+path, nil))
					assert.Equal(t, http.StatusOK, response.Code)
					assert.Contains(t, response.Header().Get("Content-Type"), contentType)
					assert.Equal(t, string(expected), response.Body.String())
				})
			}
		})
	}
}
