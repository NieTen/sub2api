package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 仅替换存储边界，使用真实公开处理器、管理处理器和配置服务验证 HTTP 契约。
type homeModelsAPIRepository struct {
	service.SettingRepository
	value    string
	writeErr error
}

func (r *homeModelsAPIRepository) GetValue(context.Context, string) (string, error) {
	if r.value == "" {
		return "", service.ErrSettingNotFound
	}
	return r.value, nil
}

func (r *homeModelsAPIRepository) Set(_ context.Context, _ string, value string) error {
	if r.writeErr != nil {
		return r.writeErr
	}
	r.value = value
	return nil
}

func newHomeModelsAPIRouter(repo *homeModelsAPIRepository) *gin.Engine {
	settings := service.NewSettingService(repo, nil)
	publicHandler := handler.NewSettingHandler(settings, "test")
	adminHandler := admin.NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/settings/home-models", publicHandler.GetHomeModels)
	router.GET("/api/v1/admin/settings/home-models", adminHandler.GetHomeModels)
	router.PUT("/api/v1/admin/settings/home-models", adminHandler.UpdateHomeModels)
	return router
}

func callHomeModelsAPI(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	router.ServeHTTP(result, request)
	return result
}

type homeModelsAPIResponse struct {
	Code int                 `json:"code"`
	Data []service.HomeModel `json:"data"`
}

func readHomeModelsAPIResponse(t *testing.T, result *httptest.ResponseRecorder) homeModelsAPIResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	var payload homeModelsAPIResponse
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &payload))
	require.Zero(t, payload.Code)
	return payload
}

func TestHomeModelsAPI_SaveReadAndClear(t *testing.T) {
	repo := &homeModelsAPIRepository{}
	router := newHomeModelsAPIRouter(repo)
	initial := callHomeModelsAPI(router, http.MethodGet, "/api/v1/settings/home-models", "")
	require.Len(t, readHomeModelsAPIResponse(t, initial).Data, 14)
	require.Equal(t, "no-store", initial.Header().Get("Cache-Control"))

	body := `{"models":[{"name":" 后台文本模型 ","vendor":"自定义厂商","type":"text","input":0,"output":3.125,"cachedInput":null,"flexInput":0.000001},{"name":"图片模型","vendor":"OpenAI","type":"image","resolutionPrices":{"1K":0,"2K":0.25,"4K":1}}]}`
	saved := readHomeModelsAPIResponse(t, callHomeModelsAPI(router, http.MethodPut, "/api/v1/admin/settings/home-models", body))
	require.Len(t, saved.Data, 2)
	require.Equal(t, "后台文本模型", saved.Data[0].Name)
	require.NotNil(t, saved.Data[0].Input)
	require.Zero(t, *saved.Data[0].Input)
	require.Nil(t, saved.Data[0].CachedInput)
	require.Equal(t, 0.25, *saved.Data[1].ResolutionPrices["2K"])

	// 重建服务与处理器后再读，确认依赖已保存的数据而非当前请求内存。
	router = newHomeModelsAPIRouter(repo)
	for _, path := range []string{"/api/v1/settings/home-models", "/api/v1/admin/settings/home-models"} {
		loaded := readHomeModelsAPIResponse(t, callHomeModelsAPI(router, http.MethodGet, path, ""))
		require.Equal(t, saved.Data, loaded.Data)
	}
	cleared := readHomeModelsAPIResponse(t, callHomeModelsAPI(router, http.MethodPut, "/api/v1/admin/settings/home-models", `{"models":[]}`))
	require.NotNil(t, cleared.Data)
	require.Empty(t, cleared.Data)
	loaded := readHomeModelsAPIResponse(t, callHomeModelsAPI(newHomeModelsAPIRouter(repo), http.MethodGet, "/api/v1/settings/home-models", ""))
	require.NotNil(t, loaded.Data)
	require.Empty(t, loaded.Data)
}

func TestHomeModelsAPI_InvalidRequestsDoNotOverwrite(t *testing.T) {
	for name, body := range map[string]string{
		"缺少列表":      `{}`,
		"列表为null":   `{"models":null}`,
		"缺少文本价格":    `{"models":[{"name":"text","vendor":"OpenAI","type":"text"}]}`,
		"负数价格":      `{"models":[{"name":"text","vendor":"OpenAI","type":"text","input":-1,"output":1}]}`,
		"图片价格为null": `{"models":[{"name":"image","vendor":"OpenAI","type":"image","resolutionPrices":{"1K":null,"2K":0,"4K":1}}]}`,
		"错误JSON":    `{"models":`,
	} {
		t.Run(name, func(t *testing.T) {
			repo := &homeModelsAPIRepository{value: "[]"}
			result := callHomeModelsAPI(newHomeModelsAPIRouter(repo), http.MethodPut, "/api/v1/admin/settings/home-models", body)
			require.Equal(t, http.StatusBadRequest, result.Code, result.Body.String())
			require.Equal(t, "[]", repo.value)
		})
	}
}

func TestHomeModelsAPI_WriteFailureReturnsError(t *testing.T) {
	repo := &homeModelsAPIRepository{value: "[]", writeErr: errors.New("存储不可用")}
	result := callHomeModelsAPI(newHomeModelsAPIRouter(repo), http.MethodPut, "/api/v1/admin/settings/home-models", `{"models":[]}`)
	require.Equal(t, http.StatusInternalServerError, result.Code, result.Body.String())
	require.Equal(t, "[]", repo.value)
}
