package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type bulkEmailRouteRepository struct {
	service.BulkEmailRepository
	action string
	id     int64
	err    error
}

func (r *bulkEmailRouteRepository) Pause(_ context.Context, id int64) error {
	r.action, r.id = "pause", id
	return r.err
}

func (r *bulkEmailRouteRepository) Resume(_ context.Context, id int64) error {
	r.action, r.id = "resume", id
	return r.err
}

func TestBulkEmailAdminPauseAndResumeRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &bulkEmailRouteRepository{}
	handlers := &handler.Handlers{
		Admin:           &handler.AdminHandlers{Ops: adminhandler.NewOpsHandler(nil)},
		SupportDelivery: handler.NewSupportDeliveryHandler(nil, service.NewBulkEmailService(repo, nil, nil), nil),
	}
	router := gin.New()
	adminAuth := middleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "test-admin" {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
	})
	next := func(c *gin.Context) { c.Next() }
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, middleware.AuditLogMiddleware(next), middleware.StepUpAuthMiddleware(next), nil, nil)

	for _, tc := range []struct {
		action, id string
		admin      bool
		err        error
		status     int
		called     bool
	}{
		{"pause", "42", false, nil, http.StatusUnauthorized, false},
		{"resume", "42", false, nil, http.StatusUnauthorized, false},
		{"pause", "0", true, nil, http.StatusBadRequest, false},
		{"resume", "bad", true, nil, http.StatusBadRequest, false},
		{"pause", "42", true, nil, http.StatusOK, true},
		{"resume", "42", true, nil, http.StatusAccepted, true},
		{"pause", "42", true, service.ErrBulkEmailState, http.StatusConflict, true},
		{"resume", "42", true, service.ErrBulkEmailNotFound, http.StatusNotFound, true},
	} {
		t.Run(tc.action+"/"+http.StatusText(tc.status), func(t *testing.T) {
			repo.action, repo.id, repo.err = "", 0, tc.err
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bulk-emails/"+tc.id+"/"+tc.action, nil)
			if tc.admin {
				req.Header.Set("Authorization", "test-admin")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.called {
				require.Equal(t, tc.action, repo.action)
				require.EqualValues(t, 42, repo.id)
			} else {
				require.Empty(t, repo.action)
			}
		})
	}
}
