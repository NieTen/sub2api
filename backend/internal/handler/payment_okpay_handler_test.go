//go:build unit

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

const okpayHandlerJSON = `{"id":"123","status":"success","code":10000,"data":{"order_id":"pay-900","unique_id":"site-100","pay_user_id":5639813059,"amount":"12.30","coin":"USDT","status":1,"type":"deposit"},"sign":"5C3DFCBF609D8BD0E66311EC73726275"}`
const okpayHandlerForm = "id=123&status=success&code=10000&data[order_id]=pay-900&data[unique_id]=site-100&data[pay_user_id]=5639813059&data[amount]=12.30&data[coin]=USDT&data[status]=1&data[type]=deposit&sign=5C3DFCBF609D8BD0E66311EC73726275"

func TestOKPayHandlerExtractsMerchantOrderFromJSONAndForm(t *testing.T) {
	for _, body := range []string{okpayHandlerJSON, okpayHandlerForm, "data%5Bunique_id%5D=site-100"} {
		require.Equal(t, "site-100", extractOutTradeNo(body, payment.TypeOKPay))
	}
	require.Empty(t, extractOutTradeNo(`{"data":{"order_id":"platform-only"}}`, payment.TypeOKPay))
	require.Empty(t, extractOutTradeNo(`{"data":invalid}`, payment.TypeOKPay))
}

func TestOKPayHandlerUsesPinnedMerchantAndReturnsJSONAcknowledgment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	user, err := client.User.Create().SetEmail("okpay-handler@example.com").SetPasswordHash("hash").SetUsername("测试用户").Save(ctx)
	require.NoError(t, err)
	instance, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeOKPay).SetName("商户 A").SetConfig(`{"id":"123","token":"token+plus%2Band%zz"}`).SetSupportedTypes(payment.TypeOKPay).SetPaymentMode("redirect").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeOKPay).SetName("商户 B").SetConfig(`{"id":"456","token":"other-token"}`).SetSupportedTypes(payment.TypeOKPay).SetPaymentMode("redirect").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	instanceID := strconv.FormatInt(instance.ID, 10)
	_, err = client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).SetAmount(12.3).SetPayAmount(12.3).SetFeeRate(0).SetRechargeCode("OKPAY-HANDLER").SetOutTradeNo("site-100").SetPaymentType(payment.TypeOKPay).SetPaymentTradeNo("pay-900").SetOrderType(payment.OrderTypeBalance).SetStatus(payment.OrderStatusCompleted).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("site.example").SetProviderKey(payment.TypeOKPay).SetProviderInstanceID(instanceID).SetProviderSnapshot(map[string]any{"schema_version": 2, "provider_key": payment.TypeOKPay, "provider_instance_id": instanceID, "merchant_id": "123", "currency": "USDT"}).Save(ctx)
	require.NoError(t, err)
	registry := payment.NewRegistry()
	svc := service.NewPaymentService(client, registry, payment.NewDefaultLoadBalancer(client, nil), nil, nil, nil, nil, nil, nil)
	handler := NewPaymentWebhookHandler(svc, registry)
	for _, tc := range []struct {
		contentType, body string
		status            int
	}{
		{"application/json", okpayHandlerJSON, http.StatusOK},
		{"application/x-www-form-urlencoded", okpayHandlerForm, http.StatusOK},
		{"application/json", strings.Replace(okpayHandlerJSON, "12.30", "99.00", 1), http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payment/webhook/okpay", strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", tc.contentType)
		handler.OKPayNotify(c)
		require.Equal(t, tc.status, response.Code, response.Body.String())
		if tc.status == http.StatusOK {
			require.Contains(t, response.Header().Get("Content-Type"), "application/json")
			var ack map[string]string
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &ack))
			require.Equal(t, map[string]string{"status": "success"}, ack)
		}
	}
}
