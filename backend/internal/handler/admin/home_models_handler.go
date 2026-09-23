package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const homeModelsAdminRequestBodyLimit = 1 << 20

// GetHomeModels 返回管理员可编辑的首页 I2 模型目录。
// GET /api/v1/admin/settings/home-models
func (h *SettingHandler) GetHomeModels(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	models, err := h.settingService.GetHomeModels(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, models)
}

// HomeModelsUpdateRequest 是管理员首页模型目录保存请求。
// 使用切片指针区分 models:null（非法）和 models:[]（合法清空）。
type HomeModelsUpdateRequest struct {
	Models *[]service.HomeModel `json:"models"`
}

// UpdateHomeModels 校验并保存首页 I2 模型目录。
// PUT /api/v1/admin/settings/home-models
func (h *SettingHandler) UpdateHomeModels(c *gin.Context) {
	request := HomeModelsUpdateRequest{}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, homeModelsAdminRequestBodyLimit)
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "请求 JSON 无效: "+err.Error())
		return
	}
	if err := h.settingService.SaveHomeModels(c.Request.Context(), request.ModelsValue()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	models, err := h.settingService.GetHomeModels(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, models)
}

func (r HomeModelsUpdateRequest) ModelsValue() []service.HomeModel {
	if r.Models == nil {
		return nil
	}
	return *r.Models
}
