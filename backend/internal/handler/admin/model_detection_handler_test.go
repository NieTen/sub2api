package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelDetectionHandlerRejectsInvalidInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelDetectionHandler(nil)
	for _, tc := range []struct {
		name, method, path, body string
		handler                  gin.HandlerFunc
	}{
		{"非法账号", "POST", "/accounts/-1/run", `{"model_id":"model"}`, h.RunAccount},
		{"非法模型请求", "POST", "/accounts/1/run", `{`, h.RunAccount},
		{"非法批量模型请求", "POST", "/accounts/1/runs", `{"model_ids":"model"}`, h.RunAccountModels},
		{"非法批量计划请求", "POST", "/plans/batch", `{"model_ids":{}}`, h.CreatePlansBatch},
		{"超大请求", "POST", "/accounts/1/run", `{"model_id":"` + strings.Repeat("x", 40<<10) + `"}`, h.RunAccount},
		{"历史游标", "GET", "/accounts/1/history?before_id=-1", "", h.History},
		{"历史上限", "GET", "/accounts/1/history?limit=101", "", h.History},
		{"非法摘要列表", "GET", "/summaries?account_ids=1,invalid", "", h.Summaries},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			route := strings.Split(tc.path, "?")[0]
			if strings.HasPrefix(route, "/accounts/") {
				parts := strings.Split(route, "/")
				parts[2] = ":id"
				route = strings.Join(parts, "/")
			}
			router.Handle(tc.method, route, tc.handler)
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}
