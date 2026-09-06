package handler

import (
	"context"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// SupportTicketOperations 将请求解析与工单业务分离，同时便于验证权限边界。
type SupportTicketOperations interface {
	Create(context.Context, service.SupportTicketActor, service.CreateSupportTicketInput) (*service.SupportTicketDetail, error)
	List(context.Context, service.SupportTicketActor, int, int, string) (*service.SupportTicketPage, error)
	Get(context.Context, service.SupportTicketActor, int64, int64, int) (*service.SupportTicketDetail, error)
	Reply(context.Context, service.SupportTicketActor, int64, service.SupportTicketReplyInput) (*service.SupportTicketMessage, error)
	SetStatus(context.Context, service.SupportTicketActor, int64, string) (*service.SupportTicket, error)
	UploadAttachment(context.Context, service.SupportTicketActor, string, []byte) (*service.SupportTicketAttachment, error)
	GetAttachment(context.Context, service.SupportTicketActor, int64) (*service.SupportTicketAttachment, error)
	DeleteAttachment(context.Context, service.SupportTicketActor, int64) error
}

type SupportTicketHandler struct {
	tickets SupportTicketOperations
}

type supportTicketStatusRequest struct {
	Status string `json:"status"`
}

func NewSupportTicketHandler(tickets *service.SupportTicketService) *SupportTicketHandler {
	return &SupportTicketHandler{tickets: tickets}
}

// admin 参数只由通过管理员认证的路由设置，不能从请求体或查询参数获得。
func supportTicketActor(c *gin.Context, admin bool) (service.SupportTicketActor, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "请先登录")
		return service.SupportTicketActor{}, false
	}
	return service.SupportTicketActor{UserID: subject.UserID, IsAdmin: admin}, true
}

func supportRequestID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的记录编号")
		return 0, false
	}
	return id, true
}

func supportBindJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	if err := c.ShouldBindJSON(target); err != nil {
		response.BadRequest(c, "请求格式错误或内容过长")
		return false
	}
	return true
}

func (h *SupportTicketHandler) List(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := supportTicketActor(c, admin)
		if !ok {
			return
		}
		page, size := response.ParsePagination(c)
		result, err := h.tickets.List(c.Request.Context(), actor, page, size, c.Query("status"))
		if !response.ErrorFrom(c, err) {
			response.Success(c, result)
		}
	}
}

func (h *SupportTicketHandler) Create(c *gin.Context) {
	actor, ok := supportTicketActor(c, false)
	if !ok {
		return
	}
	var input service.CreateSupportTicketInput
	if !supportBindJSON(c, &input) {
		return
	}
	result, err := h.tickets.Create(c.Request.Context(), actor, input)
	if !response.ErrorFrom(c, err) {
		response.Created(c, result)
	}
}

func (h *SupportTicketHandler) Get(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := supportTicketActor(c, admin)
		if !ok {
			return
		}
		id, ok := supportRequestID(c)
		if !ok {
			return
		}
		after, err := strconv.ParseInt(c.DefaultQuery("after_message_id", "0"), 10, 64)
		limit, limitErr := strconv.Atoi(c.DefaultQuery("limit", "50"))
		if err != nil || after < 0 || limitErr != nil || limit < 1 || limit > 100 {
			response.BadRequest(c, "消息分页参数无效")
			return
		}
		result, err := h.tickets.Get(c.Request.Context(), actor, id, after, limit)
		if !response.ErrorFrom(c, err) {
			response.Success(c, result)
		}
	}
}

func (h *SupportTicketHandler) Reply(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := supportTicketActor(c, admin)
		if !ok {
			return
		}
		id, ok := supportRequestID(c)
		if !ok {
			return
		}
		var input service.SupportTicketReplyInput
		if !supportBindJSON(c, &input) {
			return
		}
		result, err := h.tickets.Reply(c.Request.Context(), actor, id, input)
		if !response.ErrorFrom(c, err) {
			response.Created(c, result)
		}
	}
}

func (h *SupportTicketHandler) SetStatus(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := supportTicketActor(c, admin)
		if !ok {
			return
		}
		id, ok := supportRequestID(c)
		if !ok {
			return
		}
		var input supportTicketStatusRequest
		if !supportBindJSON(c, &input) {
			return
		}
		result, err := h.tickets.SetStatus(c.Request.Context(), actor, id, input.Status)
		if !response.ErrorFrom(c, err) {
			response.Success(c, result)
		}
	}
}

func (h *SupportTicketHandler) Upload(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := supportTicketActor(c, admin)
		if !ok {
			return
		}
		const maxImageSize = 5 * 1024 * 1024
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImageSize+64*1024)
		if err := c.Request.ParseMultipartForm(maxImageSize + 64*1024); err != nil {
			response.BadRequest(c, "请上传不超过 5 MB 的图片")
			return
		}
		defer c.Request.MultipartForm.RemoveAll()
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			response.BadRequest(c, "缺少图片文件")
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxImageSize+1))
		if err != nil || len(data) > maxImageSize {
			response.BadRequest(c, "图片读取失败或超过 5 MB")
			return
		}
		result, err := h.tickets.UploadAttachment(c.Request.Context(), actor, header.Filename, data)
		if !response.ErrorFrom(c, err) {
			response.Created(c, result)
		}
	}
}

func (h *SupportTicketHandler) Attachment(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := supportTicketActor(c, admin)
		if !ok {
			return
		}
		id, ok := supportRequestID(c)
		if !ok {
			return
		}
		attachment, err := h.tickets.GetAttachment(c.Request.Context(), actor, id)
		if response.ErrorFrom(c, err) {
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": attachment.FileName}))
		c.Data(http.StatusOK, attachment.MimeType, attachment.Data)
	}
}

func (h *SupportTicketHandler) DeleteAttachment(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := supportTicketActor(c, admin)
		if !ok {
			return
		}
		id, ok := supportRequestID(c)
		if !ok {
			return
		}
		if !response.ErrorFrom(c, h.tickets.DeleteAttachment(c.Request.Context(), actor, id)) {
			response.Success(c, gin.H{"message": "图片已移除"})
		}
	}
}
