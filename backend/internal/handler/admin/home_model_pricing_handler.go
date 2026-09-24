package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const homeModelPricingRequestBodyLimit = 1 << 20

// HomeModelPricingRequest 中的 nil 表示 models 缺失或为 null，空数组表示合法的空查询。
type HomeModelPricingRequest struct {
	Models []service.HomeModelPriceQuery `json:"models"`
}

// GetHomeModelSystemPrices 批量查询首页模型可用的系统价格，不保存任何设置。
// POST /api/v1/admin/channels/model-pricing/batch
func (h *ChannelHandler) GetHomeModelSystemPrices(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	// 完整读取受限请求体，确保超限的尾随内容也无法绕过 1 MiB 限制。
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, homeModelPricingRequestBodyLimit))
	if err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			response.Error(c, http.StatusRequestEntityTooLarge, "请求体不能超过 1 MiB")
		} else {
			response.BadRequest(c, "无法读取请求 JSON")
		}
		return
	}
	var request HomeModelPricingRequest
	if err := json.Unmarshal(body, &request); err != nil {
		response.BadRequest(c, "请求 JSON 无效: "+err.Error())
		return
	}
	prices, err := h.billingService.GetHomeModelSystemPrices(request.Models)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, prices)
}
