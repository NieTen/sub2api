package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type pluginAdaptationRouteUserRepository struct {
	service.UserRepository
	user *service.User
}

func (r *pluginAdaptationRouteUserRepository) GetByID(_ context.Context, id int64) (*service.User, error) {
	if id != r.user.ID {
		return nil, service.ErrUserNotFound
	}
	copy := *r.user
	return &copy, nil
}

func (r *pluginAdaptationRouteUserRepository) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

func TestPluginHostAdaptationRoutesRequireCurrentAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "plugin-adaptation-route-test", ExpireHour: 1}}
	auth := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	user := &service.User{ID: 42, Email: "fixture@example.invalid", Role: service.RoleUser, Status: service.StatusActive, Concurrency: 1}
	users := service.NewUserService(&pluginAdaptationRouteUserRepository{user: user}, nil, nil, nil)
	userToken, err := auth.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	staleAdmin := *user
	staleAdmin.Role = service.RoleAdmin
	staleToken, err := auth.GenerateToken(context.Background(), &staleAdmin)
	require.NoError(t, err)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		Plugin: adminhandler.NewPluginHandler(nil), Ops: adminhandler.NewOpsHandler(nil),
	}}
	auditReached := false
	audit := middleware.AuditLogMiddleware(func(c *gin.Context) { auditReached = true; c.Next() })
	stepUp := middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, middleware.NewAdminAuthMiddleware(auth, users, nil, nil), audit, stepUp, nil, nil)
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodPut, "/api/v1/admin/plugins/17/host-adaptation"},
		{http.MethodGet, "/api/v1/admin/plugins/17/resources"},
		{http.MethodPost, "/api/v1/admin/plugins/17/actions"},
	} {
		for _, identity := range []struct {
			name, token string
			status      int
		}{
			{"未登录", "", http.StatusUnauthorized},
			{"无效令牌", "invalid-token", http.StatusUnauthorized},
			{"普通用户", userToken, http.StatusForbidden},
			{"已降权的旧管理员令牌", staleToken, http.StatusForbidden},
		} {
			t.Run(endpoint.method+endpoint.path+identity.name, func(t *testing.T) {
				request := httptest.NewRequest(endpoint.method, endpoint.path+"?is_admin=true", strings.NewReader(`{"enabled":true,"request_id":"test","is_admin":true}`))
				request.Header.Set("Content-Type", "application/json")
				if identity.token != "" {
					request.Header.Set("Authorization", "Bearer "+identity.token)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				require.Equal(t, identity.status, response.Code, response.Body.String())
				require.False(t, auditReached, "非管理员不应进入插件操作链")
			})
		}
	}
}

func TestPluginHostAdaptationRoutesApplyStepUpOnlyToMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{Plugin: adminhandler.NewPluginHandler(nil)}}
	stepUpCalls := 0
	stepUp := middleware.StepUpAuthMiddleware(func(c *gin.Context) {
		stepUpCalls++
		if c.GetHeader("X-Test-Step-Up") != "passed" {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	})
	registerPluginRoutes(router.Group("/admin"), handlers, stepUp)
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodPut, "/admin/plugins/invalid/host-adaptation"},
		{http.MethodPost, "/admin/plugins/invalid/actions"},
	} {
		for _, verified := range []bool{false, true} {
			request := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(`{"enabled":true}`))
			if verified {
				request.Header.Set("X-Test-Step-Up", "passed")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if verified {
				require.Equal(t, http.StatusBadRequest, response.Code, "通过验证后才进入 ID 参数校验")
			} else {
				require.Equal(t, http.StatusForbidden, response.Code)
			}
		}
	}
	require.Equal(t, 4, stepUpCalls)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/plugins/invalid/resources", nil))
	require.Equal(t, http.StatusBadRequest, response.Code, "读取目录直接进入参数校验")
	require.Equal(t, 4, stepUpCalls, "被动目录读取不能弹出二次验证")
}
