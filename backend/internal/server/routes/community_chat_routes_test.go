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

type communityChatRouteUserRepository struct {
	service.UserRepository
	user *service.User
}

func (r *communityChatRouteUserRepository) GetByID(_ context.Context, id int64) (*service.User, error) {
	if id != r.user.ID {
		return nil, service.ErrUserNotFound
	}
	user := *r.user
	return &user, nil
}

func (r *communityChatRouteUserRepository) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

func TestCommunityChatAdminRoutesRequireAdminAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "community-route-test-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	user := &service.User{ID: 42, Email: "user@example.com", Role: service.RoleUser, Status: service.StatusActive, Concurrency: 1}
	userService := service.NewUserService(&communityChatRouteUserRepository{user: user}, nil, nil, nil)
	userToken, err := authService.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	// 即使令牌仍保留旧管理员角色，数据库中的普通用户身份也必须拒绝。
	staleAdmin := *user
	staleAdmin.Role = service.RoleAdmin
	staleAdminToken, err := authService.GenerateToken(context.Background(), &staleAdmin)
	require.NoError(t, err)

	router := gin.New()
	handlers := &handler.Handlers{
		Community: handler.NewCommunityHandler(nil),
		Admin:     &handler.AdminHandlers{Ops: adminhandler.NewOpsHandler(nil)},
	}
	auditReached := false
	auditLog := middleware.AuditLogMiddleware(func(c *gin.Context) { auditReached = true; c.Next() })
	stepUp := middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, middleware.NewAdminAuthMiddleware(authService, userService, nil, nil), auditLog, stepUp, nil, nil)

	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/community/messages"},
		{http.MethodPost, "/api/v1/admin/community/messages"},
		{http.MethodGet, "/api/v1/admin/community/messages/8/media"},
		{http.MethodGet, "/api/v1/admin/community/telegram-users/5939067819"},
		{http.MethodGet, "/api/v1/admin/community/telegram-users/5939067819/avatar"},
		{http.MethodPost, "/api/v1/admin/community/members/8/unbind"},
	} {
		for _, tc := range []struct {
			name, token, reason string
			status              int
		}{
			{name: "unauthenticated", reason: "UNAUTHORIZED", status: http.StatusUnauthorized},
			{name: "invalid-token", token: "invalid-token", reason: "INVALID_TOKEN", status: http.StatusUnauthorized},
			{name: "ordinary-user", token: userToken, reason: "FORBIDDEN", status: http.StatusForbidden},
			{name: "stale-admin-role", token: staleAdminToken, reason: "FORBIDDEN", status: http.StatusForbidden},
		} {
			t.Run(endpoint.method+"/"+endpoint.path+"/"+tc.name, func(t *testing.T) {
				req := httptest.NewRequest(endpoint.method, endpoint.path+"?user_id=1&is_admin=true&group_chat_id=-100123", strings.NewReader(`{"text":"测试","ticket_id":17,"user_id":1,"is_admin":true,"group_chat_id":"-100123"}`))
				req.Header.Set("Content-Type", "application/json")
				if tc.token != "" {
					req.Header.Set("Authorization", "Bearer "+tc.token)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, tc.status, w.Code, w.Body.String())
				require.Contains(t, w.Body.String(), `"code":"`+tc.reason+`"`)
				require.False(t, auditReached)
			})
		}
	}
}
