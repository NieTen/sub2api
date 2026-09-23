package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// GetHomeModels 返回首页 I2 模型目录，无需登录。
// GET /api/v1/settings/home-models
func (h *SettingHandler) GetHomeModels(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	models, err := h.settingService.GetHomeModels(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, models)
}
