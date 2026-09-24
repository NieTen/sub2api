package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newHomeModelPricingRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewChannelHandler(nil, service.NewBillingService(nil, nil), nil)
	router.POST("/api/v1/admin/channels/model-pricing/batch", h.GetHomeModelSystemPrices)
	return router
}

func callHomeModelPricingAPI(body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/channels/model-pricing/batch", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	newHomeModelPricingRouter().ServeHTTP(result, request)
	return result
}

func TestHomeModelPricingAPI_ReturnsOrderedStandardEnvelope(t *testing.T) {
	result := callHomeModelPricingAPI(`{"models":[{"name":"claude-sonnet-4","type":"text"},{"name":"invented-claude-haiku-99","type":"text"},{"name":"grok-imagine-image-2.0","type":"image"}]}`)
	require.Equal(t, http.StatusOK, result.Code)
	require.Equal(t, "no-store", result.Header().Get("Cache-Control"))
	var body struct {
		Code int                            `json:"code"`
		Data []service.HomeModelSystemPrice `json:"data"`
	}
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &body))
	require.Zero(t, body.Code)
	require.Len(t, body.Data, 3)
	require.Equal(t, "claude-sonnet-4", body.Data[0].Name)
	require.True(t, body.Data[0].Found)
	require.Equal(t, 3.0, *body.Data[0].Input)
	require.InDelta(t, 0.3, *body.Data[0].CachedInput, 1e-12)
	require.Nil(t, body.Data[0].FlexInput)
	require.Equal(t, "invented-claude-haiku-99", body.Data[1].Name)
	require.False(t, body.Data[1].Found)
	require.NotEmpty(t, body.Data[1].Reason)
	require.Equal(t, "grok-imagine-image-2.0", body.Data[2].Name)
	require.True(t, body.Data[2].Found)
	require.Len(t, body.Data[2].ResolutionPrices, 3)
}

func TestHomeModelPricingAPI_EmptyArrayAndInvalidRequests(t *testing.T) {
	result := callHomeModelPricingAPI(`{"models":[]}`)
	require.Equal(t, http.StatusOK, result.Code)
	require.Contains(t, result.Body.String(), `"data":[]`)
	for _, body := range []string{
		`{}`, `null`, `{"models":null}`, `{"models":"bad"}`, `{"models":[{}]}`,
		`{"models":[{"name":" ","type":"text"}]}`,
		`{"models":[{"name":"claude-sonnet-4","type":"video"}]}`,
		`{"models":[]} {"models":[]}`,
		`{"models":[` + strings.TrimSuffix(strings.Repeat(`{"name":"claude-sonnet-4","type":"text"},`, 501), ",") + `]}`,
	} {
		result := callHomeModelPricingAPI(body)
		require.Equal(t, http.StatusBadRequest, result.Code, body)
	}
}

func TestHomeModelPricingAPI_EnforcesFullBodySizeLimit(t *testing.T) {
	result := callHomeModelPricingAPI(`{"models":[]}` + strings.Repeat(" ", homeModelPricingRequestBodyLimit))
	require.Equal(t, http.StatusRequestEntityTooLarge, result.Code)
}
