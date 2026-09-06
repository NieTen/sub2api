package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// CommunityOperations 将认证和请求解析与社群业务分离。
type CommunityOperations interface {
	Get(context.Context, int64) (*service.CommunityState, error)
	StartVerification(context.Context, int64) (*service.CommunityState, error)
	CreateInvite(context.Context, int64, service.CommunityInviteInput) (*service.CommunityState, error)
	GetSettings(context.Context) (*service.CommunitySettings, error)
	UpdateSettings(context.Context, service.CommunitySettings) (*service.CommunitySettings, error)
	ListMembers(context.Context, service.CommunityMemberFilter) (*service.CommunityMemberPage, error)
}

type CommunityHandler struct {
	community CommunityOperations
}

func NewCommunityHandler(community *service.CommunityService) *CommunityHandler {
	return &CommunityHandler{community: community}
}

func (h *CommunityHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	actor, ok := supportTicketActor(c, false)
	if !ok {
		return
	}
	state, err := h.community.Get(c.Request.Context(), actor.UserID)
	if !response.ErrorFrom(c, err) {
		response.Success(c, state)
	}
}

func (h *CommunityHandler) StartVerification(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	actor, ok := supportTicketActor(c, false)
	if !ok {
		return
	}
	state, err := h.community.StartVerification(c.Request.Context(), actor.UserID)
	if !response.ErrorFrom(c, err) {
		response.Success(c, state)
	}
}

func (h *CommunityHandler) CreateInvite(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	actor, ok := supportTicketActor(c, false)
	if !ok {
		return
	}
	var input service.CommunityInviteInput
	if !supportBindJSON(c, &input) {
		return
	}
	state, err := h.community.CreateInvite(c.Request.Context(), actor.UserID, input)
	if !response.ErrorFrom(c, err) {
		response.Success(c, state)
	}
}

func (h *CommunityHandler) GetSettings(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	settings, err := h.community.GetSettings(c.Request.Context())
	if !response.ErrorFrom(c, err) {
		response.Success(c, settings)
	}
}

// ListMembers 仅挂载在管理员路由组，返回网站用户的当前社群状态。
func (h *CommunityHandler) ListMembers(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	page, pageSize := response.ParsePagination(c)
	result, err := h.community.ListMembers(c.Request.Context(), service.CommunityMemberFilter{
		Page: page, PageSize: pageSize, Search: c.Query("search"), Status: c.Query("status"),
	})
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}

func (h *CommunityHandler) UpdateSettings(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	var input service.CommunitySettings
	if !supportBindJSON(c, &input) {
		return
	}
	settings, err := h.community.UpdateSettings(c.Request.Context(), input)
	if !response.ErrorFrom(c, err) {
		response.Success(c, settings)
	}
}
