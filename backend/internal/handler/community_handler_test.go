package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type communityHandlerStub struct {
	CommunityOperations
	userID    int64
	input     service.CommunityInviteInput
	called    bool
	filter    service.CommunityMemberFilter
	inviteErr error
}

func (s *communityHandlerStub) Get(_ context.Context, userID int64) (*service.CommunityState, error) {
	s.userID, s.called = userID, true
	return &service.CommunityState{}, nil
}

func (s *communityHandlerStub) StartVerification(_ context.Context, userID int64) (*service.CommunityState, error) {
	s.userID, s.called = userID, true
	return &service.CommunityState{}, nil
}

func (s *communityHandlerStub) CreateInvite(_ context.Context, userID int64, input service.CommunityInviteInput) (*service.CommunityState, error) {
	s.userID, s.input, s.called = userID, input, true
	if s.inviteErr != nil {
		return nil, s.inviteErr
	}
	return &service.CommunityState{}, nil
}

func TestCommunityHandlerVIPRejectionCannotBeOverriddenByClient(t *testing.T) {
	stub := &communityHandlerStub{inviteErr: service.ErrCommunityVIPRequired}
	h := &CommunityHandler{community: stub}
	req := httptest.NewRequest(http.MethodPost, "/tickets/1?user_id=7", strings.NewReader(`{"eligible":true,"require_paid_recharge":false,"user_id":7}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	supportTicketTestRouter(h.CreateInvite, true).ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), `"reason":"COMMUNITY_VIP_REQUIRED"`)
	require.NotContains(t, w.Body.String(), "https://t.me/")
	require.Equal(t, int64(42), stub.userID)
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
}

func (s *communityHandlerStub) ListMembers(_ context.Context, filter service.CommunityMemberFilter) (*service.CommunityMemberPage, error) {
	s.filter, s.called = filter, true
	return &service.CommunityMemberPage{}, nil
}

func TestCommunityHandlerDirectInvitationUsesSessionWithoutVerification(t *testing.T) {
	stub := &communityHandlerStub{}
	h := &CommunityHandler{community: stub}
	req := httptest.NewRequest(http.MethodPost, "/tickets/1?user_id=7", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	supportTicketTestRouter(h.CreateInvite, true).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(42), stub.userID)
	require.Empty(t, stub.input.ChallengeID)
	require.Zero(t, stub.input.TelegramUserID)
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
}

func TestCommunityMemberListParsesFiltersAndDisablesCaching(t *testing.T) {
	stub := &communityHandlerStub{}
	h := &CommunityHandler{community: stub}
	r := gin.New()
	r.GET("/admin/community/members", h.ListMembers)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/community/members?page=2&page_size=30&status=not_joined&search=sample%40example.com", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, service.CommunityMemberFilter{Page: 2, PageSize: 30, Status: "not_joined", Search: "sample@example.com"}, stub.filter)
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
}

func TestCommunityHandlerRequiresLogin(t *testing.T) {
	for _, action := range []string{"get", "verify", "invite"} {
		t.Run(action, func(t *testing.T) {
			stub := &communityHandlerStub{}
			h := &CommunityHandler{community: stub}
			handler := map[string]gin.HandlerFunc{"get": h.Get, "verify": h.StartVerification, "invite": h.CreateInvite}[action]
			w := httptest.NewRecorder()
			supportTicketTestRouter(handler, false).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tickets/1", strings.NewReader(`{}`)))
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.False(t, stub.called)
		})
	}
}

func TestCommunityHandlerUsesAuthenticatedOwner(t *testing.T) {
	for _, action := range []string{"get", "verify", "invite"} {
		t.Run(action, func(t *testing.T) {
			stub := &communityHandlerStub{}
			h := &CommunityHandler{community: stub}
			handler := map[string]gin.HandlerFunc{"get": h.Get, "verify": h.StartVerification, "invite": h.CreateInvite}[action]
			req := httptest.NewRequest(http.MethodPost, "/tickets/1?user_id=7", strings.NewReader(`{"user_id":7,"challenge_id":"mine","telegram_user_id":1234567890123}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			supportTicketTestRouter(handler, true).ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, int64(42), stub.userID)
			require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
			if action == "invite" {
				require.Equal(t, "mine", stub.input.ChallengeID)
				require.Equal(t, int64(1234567890123), stub.input.TelegramUserID)
			}
		})
	}
}

func TestCommunityHandlerRejectsMalformedConfirmation(t *testing.T) {
	for _, body := range []string{`{"telegram_user_id":"wrong"}`, `{"telegram_user_id":1.5}`, `{"challenge_id":"` + strings.Repeat("x", 270*1024) + `"}`} {
		stub := &communityHandlerStub{}
		h := &CommunityHandler{community: stub}
		req := httptest.NewRequest(http.MethodPost, "/tickets/1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		supportTicketTestRouter(h.CreateInvite, true).ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.False(t, stub.called)
	}
}
