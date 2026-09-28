package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/modeltrace"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ModelDetectionHandler struct {
	service *service.ModelDetectionService
}

func NewModelDetectionHandler(s *service.ModelDetectionService) *ModelDetectionHandler {
	return &ModelDetectionHandler{service: s}
}

type modelDetectionAccountRunRequest struct {
	ModelID        string `json:"model_id"`
	ReferenceModel string `json:"reference_model"`
}

type modelDetectionAccountRunsRequest struct {
	ModelIDs []string `json:"model_ids"`
}

func detectionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "检测记录或账号 ID 无效")
		return 0, false
	}
	return id, true
}

func (h *ModelDetectionHandler) Catalog(c *gin.Context) {
	response.Success(c, gin.H{"reference_models": modeltrace.Models(), "suite_version": service.ModelDetectionSuiteVersion, "requests_per_run": 4, "request_timeout_seconds": service.ModelDetectionRequestTimeoutSeconds, "limitation": modeltrace.Limitation})
}
func (h *ModelDetectionHandler) ListPlans(c *gin.Context) {
	var id int64
	if raw := c.Query("account_id"); raw != "" {
		var err error
		id, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "账号 ID 无效")
			return
		}
	}
	items, err := h.service.ListPlans(c.Request.Context(), id)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, items)
}
func (h *ModelDetectionHandler) SavePlan(c *gin.Context) {
	var p service.ModelDetectionPlan
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if err := c.ShouldBindJSON(&p); err != nil {
		response.BadRequest(c, "模型检测配置格式无效")
		return
	}
	p.ID = 0
	if c.Request.Method == http.MethodPut {
		id, ok := detectionID(c)
		if !ok {
			return
		}
		p.ID = id
	}
	item, err := h.service.SavePlan(c.Request.Context(), &p)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, item)
}
func (h *ModelDetectionHandler) RunPlan(c *gin.Context) {
	id, ok := detectionID(c)
	if !ok {
		return
	}
	item, err := h.service.RunPlan(c.Request.Context(), id)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, item)
}
func (h *ModelDetectionHandler) RunAccount(c *gin.Context) {
	id, ok := detectionID(c)
	if !ok {
		return
	}
	var req modelDetectionAccountRunRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请选择需要检测的模型")
		return
	}
	item, err := h.service.RunAccount(c.Request.Context(), id, req.ModelID, req.ReferenceModel)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, item)
}

func (h *ModelDetectionHandler) RunAccountModels(c *gin.Context) {
	id, ok := detectionID(c)
	if !ok {
		return
	}
	var req modelDetectionAccountRunsRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请选择有效的检测模型列表")
		return
	}
	result, err := h.service.RunAccountModels(c.Request.Context(), id, req.ModelIDs)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}

func (h *ModelDetectionHandler) CreatePlansBatch(c *gin.Context) {
	var req service.ModelDetectionBatchPlanInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "批量检测计划格式无效")
		return
	}
	result, err := h.service.CreatePlansBatch(c.Request.Context(), req)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}
func (h *ModelDetectionHandler) ResetBaseline(c *gin.Context) {
	id, ok := detectionID(c)
	if !ok {
		return
	}
	item, err := h.service.ResetBaseline(c.Request.Context(), id)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, item)
}
func (h *ModelDetectionHandler) History(c *gin.Context) {
	id, ok := detectionID(c)
	if !ok {
		return
	}
	limit := 20
	var before int64
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 || value > 100 {
			response.BadRequest(c, "历史记录条数须为 1～100")
			return
		}
		limit = value
	}
	if raw := c.Query("before_id"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			response.BadRequest(c, "历史分页游标无效")
			return
		}
		before = value
	}
	items, err := h.service.History(c.Request.Context(), id, limit, before)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, items)
}
func (h *ModelDetectionHandler) GetRun(c *gin.Context) {
	id, ok := detectionID(c)
	if !ok {
		return
	}
	item, err := h.service.Run(c.Request.Context(), id)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, item)
}
func (h *ModelDetectionHandler) Summaries(c *gin.Context) {
	raw := c.Query("account_ids")
	if len(raw) > 4000 {
		response.BadRequest(c, "一次最多查询 200 个账号")
		return
	}
	ids := []int64{}
	seen := map[int64]bool{}
	if strings.TrimSpace(raw) != "" {
		for _, part := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || id <= 0 {
				response.BadRequest(c, "账号 ID 列表无效")
				return
			}
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}
	items, err := h.service.Summaries(c.Request.Context(), ids)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, items)
}
func (h *ModelDetectionHandler) Overview(c *gin.Context) {
	item, err := h.service.Overview(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, item)
}
