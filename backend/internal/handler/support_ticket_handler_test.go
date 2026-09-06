package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type supportTicketHandlerStub struct {
	SupportTicketOperations
	actor  service.SupportTicketActor
	called bool
}

func (s *supportTicketHandlerStub) Get(_ context.Context, actor service.SupportTicketActor, _ int64, _ int64, _ int) (*service.SupportTicketDetail, error) {
	s.actor, s.called = actor, true
	return nil, service.ErrSupportTicketNotFound
}

func (s *supportTicketHandlerStub) GetAttachment(_ context.Context, actor service.SupportTicketActor, _ int64) (*service.SupportTicketAttachment, error) {
	s.actor, s.called = actor, true
	return &service.SupportTicketAttachment{FileName: "图片.png", MimeType: "image/png", Data: []byte("image")}, nil
}

func supportTicketTestRouter(h gin.HandlerFunc, authenticated bool) *gin.Engine {
	r := gin.New()
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		})
	}
	r.Any("/tickets/:id", h)
	return r
}

func TestSupportTicketHandlerDoesNotTrustRequestedActor(t *testing.T) {
	stub := &supportTicketHandlerStub{}
	h := &SupportTicketHandler{tickets: stub}
	r := supportTicketTestRouter(h.Get(false), true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tickets/8?user_id=1&is_admin=true", nil))
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Equal(t, service.SupportTicketActor{UserID: 42, IsAdmin: false}, stub.actor)
}

func TestSupportTicketHandlerRequiresLogin(t *testing.T) {
	stub := &supportTicketHandlerStub{}
	h := &SupportTicketHandler{tickets: stub}
	w := httptest.NewRecorder()
	supportTicketTestRouter(h.Get(false), false).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tickets/8", nil))
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.False(t, stub.called)
}

func TestSupportTicketHandlerRejectsInvalidCursor(t *testing.T) {
	for _, query := range []string{"after_message_id=-1", "after_message_id=hello", "limit=100000"} {
		t.Run(query, func(t *testing.T) {
			stub := &supportTicketHandlerStub{}
			h := &SupportTicketHandler{tickets: stub}
			w := httptest.NewRecorder()
			supportTicketTestRouter(h.Get(false), true).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tickets/8?"+query, nil))
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.False(t, stub.called)
		})
	}
}

func TestSupportTicketAttachmentHasPrivateCacheHeaders(t *testing.T) {
	h := &SupportTicketHandler{tickets: &supportTicketHandlerStub{}}
	w := httptest.NewRecorder()
	supportTicketTestRouter(h.Attachment(false), true).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tickets/8", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "image/png", w.Header().Get("Content-Type"))
}

func TestSupportTicketUploadRejectsOversizedFile(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	f, err := w.CreateFormFile("file", "large.png")
	require.NoError(t, err)
	_, err = f.Write(bytes.Repeat([]byte("x"), service.SupportTicketMaxImageBytes+1))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	h := &SupportTicketHandler{}
	r := supportTicketTestRouter(h.Upload(false), true)
	req := httptest.NewRequest(http.MethodPost, "/tickets/8", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestSupportBindJSONLimitsBody(t *testing.T) {
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		var input supportTicketStatusRequest
		if supportBindJSON(c, &input) {
			c.Status(http.StatusOK)
		}
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"status":"`+strings.Repeat("x", 270*1024)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
