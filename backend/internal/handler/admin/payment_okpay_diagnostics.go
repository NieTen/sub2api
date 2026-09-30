package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// DiagnoseOKPayProvider 只读检查已保存实例的商户认证，不接收或返回凭据。
func (h *PaymentHandler) DiagnoseOKPayProvider(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	result, err := h.configService.DiagnoseOKPayProvider(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
