package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// 工单接口沿用所在路由组的认证、审计与限流规则。
func registerSupportTicketRoutes(group *gin.RouterGroup, h *handler.Handlers, admin bool) {
	tickets := group.Group("/tickets")
	tickets.GET("", h.SupportTicket.List(admin))
	if !admin {
		tickets.POST("", h.SupportTicket.Create)
	}
	tickets.POST("/attachments", h.SupportTicket.Upload(admin))
	tickets.GET("/attachments/:id", h.SupportTicket.Attachment(admin))
	tickets.DELETE("/attachments/:id", h.SupportTicket.DeleteAttachment(admin))
	tickets.GET("/:id", h.SupportTicket.Get(admin))
	tickets.POST("/:id/messages", h.SupportTicket.Reply(admin))
	tickets.PATCH("/:id/status", h.SupportTicket.SetStatus(admin))
}

func registerSupportAdminRoutes(group *gin.RouterGroup, h *handler.Handlers) {
	group.GET("/community/settings", h.Community.GetSettings)
	group.GET("/community/members", h.Community.ListMembers)
	group.PUT("/community/settings", h.Community.UpdateSettings)
	registerSupportTicketRoutes(group, h, true)
	group.GET("/support/settings", h.SupportDelivery.GetSettings)
	group.PUT("/support/settings", h.SupportDelivery.UpdateSettings)
	bulk := group.Group("/bulk-emails")
	bulk.GET("", h.SupportDelivery.ListBatches)
	bulk.POST("", h.SupportDelivery.CreateBatch)
	bulk.GET("/:id", h.SupportDelivery.GetBatch)
	bulk.GET("/:id/images/:index", h.SupportDelivery.BatchImage)
	bulk.POST("/:id/start", h.SupportDelivery.StartBatch)
	bulk.POST("/:id/retry", h.SupportDelivery.RetryBatch)
}
