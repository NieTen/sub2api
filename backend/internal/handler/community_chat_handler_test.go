package handler

import (
	"context"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type communityChatHandlerStub struct {
	CommunityOperations
	actor    service.SupportTicketActor
	filter   service.CommunityChatFilter
	input    service.CommunityChatSendInput
	id       int64
	ticketID int64
	called   bool
	file     *service.CommunityChatFile
	err      error
}

func (s *communityChatHandlerStub) ListChatMessages(_ context.Context, actor service.SupportTicketActor, filter service.CommunityChatFilter) (*service.CommunityChatPage, error) {
	s.actor, s.filter, s.called = actor, filter, true
	return &service.CommunityChatPage{}, s.err
}

func (s *communityChatHandlerStub) SendChatMessage(_ context.Context, actor service.SupportTicketActor, input service.CommunityChatSendInput) (*service.CommunityChatMessage, error) {
	s.actor, s.input, s.called = actor, input, true
	return &service.CommunityChatMessage{}, s.err
}

func (s *communityChatHandlerStub) ChatPerson(_ context.Context, actor service.SupportTicketActor, id int64) (*service.CommunityChatPerson, error) {
	s.actor, s.id, s.called = actor, id, true
	return &service.CommunityChatPerson{TelegramUserID: id}, s.err
}

func (s *communityChatHandlerStub) ChatAvatar(_ context.Context, actor service.SupportTicketActor, id int64) (*service.CommunityChatFile, error) {
	s.actor, s.id, s.called = actor, id, true
	return s.file, s.err
}

func (s *communityChatHandlerStub) ChatMedia(_ context.Context, actor service.SupportTicketActor, id int64) (*service.CommunityChatFile, error) {
	s.actor, s.id, s.called = actor, id, true
	return s.file, s.err
}

func (s *communityChatHandlerStub) Unbind(_ context.Context, actor service.SupportTicketActor, id, ticketID int64) (*service.CommunityUnbindResult, error) {
	s.actor, s.id, s.ticketID, s.called = actor, id, ticketID, true
	return &service.CommunityUnbindResult{UserID: id, TicketID: ticketID, Unbound: true}, s.err
}

func communityChatTestHandler(h *CommunityHandler, action string) gin.HandlerFunc {
	return map[string]gin.HandlerFunc{
		"messages": h.ChatMessages,
		"send":     h.SendChatMessage,
		"person":   h.ChatPerson,
		"avatar":   h.ChatFile(true),
		"media":    h.ChatFile(false),
		"unbind":   h.Unbind,
	}[action]
}

func TestCommunityChatHandlerRequiresLogin(t *testing.T) {
	for _, action := range []string{"messages", "send", "person", "avatar", "media", "unbind"} {
		t.Run(action, func(t *testing.T) {
			stub := &communityChatHandlerStub{}
			h := &CommunityHandler{community: stub}
			req := httptest.NewRequest(http.MethodPost, "/tickets/8?user_id=1&is_admin=true&group_chat_id=-100123", strings.NewReader(`{"user_id":1,"is_admin":true,"ticket_id":9}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			supportTicketTestRouter(communityChatTestHandler(h, action), false).ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.False(t, stub.called)
			require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
		})
	}
}

func TestCommunityChatHandlerUsesSessionAndIgnoresRequestedGroup(t *testing.T) {
	for _, action := range []string{"messages", "send", "person", "avatar", "media", "unbind"} {
		t.Run(action, func(t *testing.T) {
			stub := &communityChatHandlerStub{file: &service.CommunityChatFile{FileName: "头像.png", MimeType: "image/png", Data: []byte("test-image")}}
			h := &CommunityHandler{community: stub}
			body := `{"text":"测试群消息","client_request_id":"request-test-123456","ticket_id":17,"user_id":999,"telegram_user_id":777,"admin_user_id":1,"is_admin":false,"group_chat_id":"-100999","bot_id":2}`
			req := httptest.NewRequest(http.MethodPost, "/tickets/5939067819?user_id=999&admin_user_id=1&is_admin=false&group_chat_id=-100999&bot_id=2&before_id=123&limit=25", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			supportTicketTestRouter(communityChatTestHandler(h, action), true).ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.True(t, stub.called)
			require.Equal(t, service.SupportTicketActor{UserID: 42, IsAdmin: true}, stub.actor)
			require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
			switch action {
			case "messages":
				require.Equal(t, service.CommunityChatFilter{BeforeID: 123, Limit: 25}, stub.filter)
			case "send":
				require.Equal(t, service.CommunityChatSendInput{Text: "测试群消息", ClientRequestID: "request-test-123456"}, stub.input)
			default:
				require.Equal(t, int64(5939067819), stub.id)
			}
			if action == "unbind" {
				require.Equal(t, int64(17), stub.ticketID)
			}
		})
	}
}

func TestCommunityChatHandlerRejectsInvalidPagination(t *testing.T) {
	for _, query := range []string{"before_id=0", "before_id=-1", "after_id=1.5", "after_id=9223372036854775808", "limit=101", "limit=-1", "limit=abc"} {
		t.Run(query, func(t *testing.T) {
			stub := &communityChatHandlerStub{}
			h := &CommunityHandler{community: stub}
			w := httptest.NewRecorder()
			supportTicketTestRouter(h.ChatMessages, true).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tickets/8?"+query, nil))
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.False(t, stub.called)
		})
	}
}

func TestCommunityChatHandlerRejectsInvalidResourceID(t *testing.T) {
	for _, action := range []string{"person", "avatar", "media", "unbind"} {
		for _, id := range []string{"0", "-1", "1.5", "abc", "9223372036854775808"} {
			t.Run(action+"/"+id, func(t *testing.T) {
				stub := &communityChatHandlerStub{}
				h := &CommunityHandler{community: stub}
				req := httptest.NewRequest(http.MethodPost, "/tickets/"+id, strings.NewReader(`{"ticket_id":17}`))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				supportTicketTestRouter(communityChatTestHandler(h, action), true).ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.False(t, stub.called)
			})
		}
	}
}

func TestCommunityChatHandlerRejectsMalformedBody(t *testing.T) {
	for _, tc := range []struct{ action, body string }{
		{"send", `{"text":true}`},
		{"send", `{"text":"` + strings.Repeat("x", 270*1024) + `"}`},
		{"unbind", `{"ticket_id":1.5}`},
		{"unbind", `{"ticket_id":9223372036854775808}`},
	} {
		t.Run(tc.action, func(t *testing.T) {
			stub := &communityChatHandlerStub{}
			h := &CommunityHandler{community: stub}
			req := httptest.NewRequest(http.MethodPost, "/tickets/8", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			supportTicketTestRouter(communityChatTestHandler(h, tc.action), true).ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.False(t, stub.called)
		})
	}
}

func TestCommunityChatHandlerPreservesPermissionErrors(t *testing.T) {
	for _, action := range []string{"messages", "send", "person", "avatar", "media", "unbind"} {
		t.Run(action, func(t *testing.T) {
			stub := &communityChatHandlerStub{err: service.ErrCommunityChatForbidden}
			h := &CommunityHandler{community: stub}
			req := httptest.NewRequest(http.MethodPost, "/tickets/8", strings.NewReader(`{"text":"测试","ticket_id":17}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			supportTicketTestRouter(communityChatTestHandler(h, action), true).ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
			require.Contains(t, w.Body.String(), `"reason":"COMMUNITY_CHAT_FORBIDDEN"`)
			require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
		})
	}
}

func TestCommunityChatHandlerFileSecurityHeaders(t *testing.T) {
	for _, action := range []string{"avatar", "media"} {
		for _, tc := range []struct{ mimeType, disposition string }{
			{"image/png", "inline"},
			{"application/octet-stream", "attachment"},
		} {
			t.Run(action+"/"+tc.disposition, func(t *testing.T) {
				stub := &communityChatHandlerStub{file: &service.CommunityChatFile{FileName: "群消息附件.png", MimeType: tc.mimeType, Data: []byte("fixture-content")}}
				h := &CommunityHandler{community: stub}
				w := httptest.NewRecorder()
				supportTicketTestRouter(communityChatTestHandler(h, action), true).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tickets/8", nil))
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, "fixture-content", w.Body.String())
				require.Equal(t, tc.mimeType, w.Header().Get("Content-Type"))
				require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
				require.Equal(t, "default-src 'none'; sandbox", w.Header().Get("Content-Security-Policy"))
				require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
				disposition, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
				require.NoError(t, err)
				require.Equal(t, tc.disposition, disposition)
				require.Equal(t, "群消息附件.png", params["filename"])
			})
		}
	}
}
