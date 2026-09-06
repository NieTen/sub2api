package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSupportWebhookRejectsMissingSecretBeforeReadingBody(t *testing.T) {
	r := gin.New()
	h := &SupportDeliveryHandler{}
	r.POST("/webhook", h.TelegramWebhook)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("not-json")))
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestSupportWebhookBoundsRequestBody(t *testing.T) {
	r := gin.New()
	h := &SupportDeliveryHandler{}
	r.POST("/webhook", h.TelegramWebhook)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(strings.Repeat("x", 1024*1024+1)))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-secret-12345")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
