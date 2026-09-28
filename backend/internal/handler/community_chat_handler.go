package handler

import (
	"context"
	"mime"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CommunityChatOperations interface {
	ListChatMessages(context.Context, service.SupportTicketActor, service.CommunityChatFilter) (*service.CommunityChatPage, error)
	SendChatMessage(context.Context, service.SupportTicketActor, service.CommunityChatSendInput) (*service.CommunityChatMessage, error)
	ChatPerson(context.Context, service.SupportTicketActor, int64) (*service.CommunityChatPerson, error)
	ChatAvatar(context.Context, service.SupportTicketActor, int64) (*service.CommunityChatFile, error)
	ChatMedia(context.Context, service.SupportTicketActor, int64) (*service.CommunityChatFile, error)
	Unbind(context.Context, service.SupportTicketActor, int64, int64) (*service.CommunityUnbindResult, error)
}

func (h *CommunityHandler) ChatMessages(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	actor, ok := supportTicketActor(c, true)
	if !ok {
		return
	}
	values := map[string]int64{}
	for _, key := range []string{"before_id", "after_id", "limit"} {
		if raw := c.Query(key); raw != "" {
			v, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || v <= 0 {
				response.BadRequest(c, "消息分页参数无效")
				return
			}
			values[key] = v
		}
	}
	if values["limit"] > 100 {
		response.BadRequest(c, "每页最多读取 100 条群消息")
		return
	}
	page, err := h.community.ListChatMessages(c.Request.Context(), actor, service.CommunityChatFilter{BeforeID: values["before_id"], AfterID: values["after_id"], Limit: int(values["limit"])})
	if !response.ErrorFrom(c, err) {
		response.Success(c, page)
	}
}
func (h *CommunityHandler) SendChatMessage(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	actor, ok := supportTicketActor(c, true)
	if !ok {
		return
	}
	var input service.CommunityChatSendInput
	if !supportBindJSON(c, &input) {
		return
	}
	result, err := h.community.SendChatMessage(c.Request.Context(), actor, input)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}
func (h *CommunityHandler) ChatPerson(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	actor, ok := supportTicketActor(c, true)
	if !ok {
		return
	}
	id, ok := supportRequestID(c)
	if !ok {
		return
	}
	person, err := h.community.ChatPerson(c.Request.Context(), actor, id)
	if !response.ErrorFrom(c, err) {
		response.Success(c, person)
	}
}
func (h *CommunityHandler) ChatFile(avatar bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		actor, ok := supportTicketActor(c, true)
		if !ok {
			return
		}
		id, ok := supportRequestID(c)
		if !ok {
			return
		}
		var file *service.CommunityChatFile
		var err error
		if avatar {
			file, err = h.community.ChatAvatar(c.Request.Context(), actor, id)
		} else {
			file, err = h.community.ChatMedia(c.Request.Context(), actor, id)
		}
		if response.ErrorFrom(c, err) {
			return
		}
		disposition := "inline"
		if file.MimeType == "application/octet-stream" {
			disposition = "attachment"
		}
		c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.FileName}))
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
		c.Data(http.StatusOK, file.MimeType, file.Data)
	}
}

type communityUnbindRequest struct {
	TicketID int64 `json:"ticket_id"`
}

func (h *CommunityHandler) Unbind(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	actor, ok := supportTicketActor(c, true)
	if !ok {
		return
	}
	id, ok := supportRequestID(c)
	if !ok {
		return
	}
	var input communityUnbindRequest
	if !supportBindJSON(c, &input) {
		return
	}
	result, err := h.community.Unbind(c.Request.Context(), actor, id, input.TicketID)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}
