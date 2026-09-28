package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// 所有检测数据和变更均位于现有管理员认证及审计路由组中。
func registerModelDetectionRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	if h.Admin.ModelDetection == nil {
		return
	}
	api := admin.Group("/model-detection")
	api.GET("/catalog", h.Admin.ModelDetection.Catalog)
	api.GET("/plans", h.Admin.ModelDetection.ListPlans)
	api.POST("/plans", h.Admin.ModelDetection.SavePlan)
	api.PUT("/plans/:id", h.Admin.ModelDetection.SavePlan)
	api.POST("/plans/:id/run", h.Admin.ModelDetection.RunPlan)
	api.POST("/plans/:id/reset-baseline", h.Admin.ModelDetection.ResetBaseline)
	api.POST("/accounts/:id/run", h.Admin.ModelDetection.RunAccount)
	api.GET("/accounts/:id/history", h.Admin.ModelDetection.History)
	api.GET("/runs/:id", h.Admin.ModelDetection.GetRun)
	api.GET("/summaries", h.Admin.ModelDetection.Summaries)
	api.GET("/overview", h.Admin.ModelDetection.Overview)
}
