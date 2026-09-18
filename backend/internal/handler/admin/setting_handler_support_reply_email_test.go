package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSupportReplyEmailSettingDefaultsAndPersists(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	// 升级后的旧数据库没有此键时，读取和保存其他设置均应保持默认开启。
	get := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(get)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	h.GetSettings(c)
	require.Equal(t, http.StatusOK, get.Code)
	require.Contains(t, get.Body.String(), `"support_ticket_reply_email_enabled":true`)

	preserved := doUpdateSettings(t, h, map[string]any{"site_name": "工单站点"}, nil)
	require.Equal(t, http.StatusOK, preserved.Code)
	require.Contains(t, preserved.Body.String(), `"support_ticket_reply_email_enabled":true`)

	disabled := doUpdateSettings(t, h, map[string]any{"support_ticket_reply_email_enabled": false}, nil)
	require.Equal(t, http.StatusOK, disabled.Code)
	require.Equal(t, "false", repo.values[service.SettingKeySupportTicketReplyEmailEnabled])
	require.Contains(t, disabled.Body.String(), `"support_ticket_reply_email_enabled":false`)

	// 旧客户端不携带新开关时，不得重新开启管理员已经关闭的通知。
	omitted := doUpdateSettings(t, h, map[string]any{"smtp_host": "smtp.example.com"}, nil)
	require.Equal(t, http.StatusOK, omitted.Code)
	require.Equal(t, "false", repo.values[service.SettingKeySupportTicketReplyEmailEnabled])
	require.Contains(t, omitted.Body.String(), `"support_ticket_reply_email_enabled":false`)

	enabled := doUpdateSettings(t, h, map[string]any{"support_ticket_reply_email_enabled": true}, nil)
	require.Equal(t, http.StatusOK, enabled.Code)
	require.Equal(t, "true", repo.values[service.SettingKeySupportTicketReplyEmailEnabled])
	require.Contains(t, enabled.Body.String(), `"support_ticket_reply_email_enabled":true`)
	require.Equal(t, "smtp.example.com", repo.values[service.SettingKeySMTPHost])
}
