package handler

import (
	"context"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type SupportDeliveryHandler struct {
	delivery  *service.SupportDeliveryService
	bulk      *service.BulkEmailService
	community CommunityTelegramWebhook
}

// CommunityTelegramWebhook 复用同一机器人回调，社群事件与工单回复分别处理。
type CommunityTelegramWebhook interface {
	HandleTelegramWebhook(context.Context, string, []byte) error
}

func NewSupportDeliveryHandler(delivery *service.SupportDeliveryService, bulk *service.BulkEmailService, community *service.CommunityService) *SupportDeliveryHandler {
	return &SupportDeliveryHandler{delivery: delivery, bulk: bulk, community: community}
}

func (h *SupportDeliveryHandler) GetSettings(c *gin.Context) {
	settings, err := h.delivery.GetSettings(c.Request.Context())
	if !response.ErrorFrom(c, err) {
		response.Success(c, settings)
	}
}

func (h *SupportDeliveryHandler) UpdateSettings(c *gin.Context) {
	var input service.SupportDeliverySettings
	if !supportBindJSON(c, &input) {
		return
	}
	settings, err := h.delivery.UpdateSettings(c.Request.Context(), input)
	if !response.ErrorFrom(c, err) {
		response.Success(c, settings)
	}
}

func (h *SupportDeliveryHandler) TelegramWebhook(c *gin.Context) {
	secret := c.GetHeader("X-Telegram-Bot-Api-Secret-Token")
	if len(secret) < 16 || len(secret) > 256 {
		response.Forbidden(c, "Telegram 回调验证失败")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.BadRequest(c, "回调请求过大或无法读取")
		return
	}
	if h.community != nil {
		if response.ErrorFrom(c, h.community.HandleTelegramWebhook(c.Request.Context(), secret, data)) {
			return
		}
	}
	err = h.delivery.HandleTelegramWebhook(c.Request.Context(), secret, data)
	if !response.ErrorFrom(c, err) {
		response.Success(c, gin.H{"ok": true})
	}
}

func (h *SupportDeliveryHandler) CreateBatch(c *gin.Context) {
	actor, ok := supportTicketActor(c, true)
	if !ok {
		return
	}
	var input service.BulkEmailCreateInput
	if !supportBindJSON(c, &input) {
		return
	}
	batch, err := h.bulk.Create(c.Request.Context(), actor.UserID, input)
	if !response.ErrorFrom(c, err) {
		response.Created(c, batch)
	}
}

func (h *SupportDeliveryHandler) ListBatches(c *gin.Context) {
	page, size := response.ParsePagination(c)
	if size > 100 {
		size = 100
	}
	items, total, err := h.bulk.List(c.Request.Context(), page, size)
	if !response.ErrorFrom(c, err) {
		response.Paginated(c, items, total, page, size)
	}
}

func (h *SupportDeliveryHandler) GetBatch(c *gin.Context) {
	id, ok := supportRequestID(c)
	if !ok {
		return
	}
	page, size := response.ParsePagination(c)
	if size > 100 {
		size = 100
	}
	batch, recipients, total, err := h.bulk.Get(c.Request.Context(), id, page, size)
	if !response.ErrorFrom(c, err) {
		response.Success(c, gin.H{
			"batch":      batch,
			"recipients": gin.H{"items": recipients, "total": total, "page": page, "page_size": size},
		})
	}
}

func (h *SupportDeliveryHandler) StartBatch(c *gin.Context) {
	id, ok := supportRequestID(c)
	if !ok {
		return
	}
	if !response.ErrorFrom(c, h.bulk.StartBatch(c.Request.Context(), id)) {
		response.Accepted(c, gin.H{"message": "批量邮件已加入发送队列"})
	}
}

func (h *SupportDeliveryHandler) BatchImage(c *gin.Context) {
	id, ok := supportRequestID(c)
	if !ok {
		return
	}
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil || index < 0 || index >= service.SupportTicketMaxAttachments {
		response.BadRequest(c, "图片编号无效")
		return
	}
	img, err := h.bulk.GetImage(c.Request.Context(), id, index)
	if response.ErrorFrom(c, err) {
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": img.FileName}))
	c.Data(http.StatusOK, img.MimeType, img.Data)
}

func (h *SupportDeliveryHandler) RetryBatch(c *gin.Context) {
	id, ok := supportRequestID(c)
	if !ok {
		return
	}
	if !response.ErrorFrom(c, h.bulk.Retry(c.Request.Context(), id)) {
		response.Accepted(c, gin.H{"message": "失败收件人已加入重试队列"})
	}
}
