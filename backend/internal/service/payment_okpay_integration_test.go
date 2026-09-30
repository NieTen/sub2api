//go:build unit

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type okpayIntegrationFixture struct {
	service    *PaymentService
	order      *dbent.PaymentOrder
	userRepo   *mockUserRepo
	redeemRepo *paymentOrderLifecycleRedeemRepo
}

type okpayMerchantQueryStub struct {
	paymentFulfillmentTestProvider
	merchantIDs  []string
	genericCalls int
	responses    []*payment.QueryOrderResponse
}

func (p *okpayMerchantQueryStub) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	p.genericCalls++
	return nil, fmt.Errorf("不应使用平台流水号查 OKPay 订单")
}
func (p *okpayMerchantQueryStub) QueryOrderByMerchantOrderID(_ context.Context, orderID string) (*payment.QueryOrderResponse, error) {
	p.merchantIDs = append(p.merchantIDs, orderID)
	response := p.responses[0]
	if len(p.responses) > 1 {
		p.responses = p.responses[1:]
	}
	return response, nil
}

func newOKPayIntegrationFixture(t *testing.T) okpayIntegrationFixture {
	t.Helper()
	return newOKPayIntegrationFixtureWithAlgorithm(t, "")
}

func newOKPayIntegrationFixtureWithAlgorithm(t *testing.T, algorithm string) okpayIntegrationFixture {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	config := map[string]string{"id": "123", "token": "token+plus%2Band%zz"}
	if algorithm == "legacy_md5" {
		config["signatureAlgorithm"] = algorithm
	}
	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)
	instance, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeOKPay).SetName("OKPay 商户 A").SetConfig(string(rawConfig)).SetSupportedTypes(payment.TypeOKPay).SetPaymentMode("redirect").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
	snapshot := buildPaymentOrderProviderSnapshot(&payment.InstanceSelection{ProviderKey: payment.TypeOKPay, InstanceID: strconv.FormatInt(instance.ID, 10), PaymentMode: "redirect", Config: config}, CreateOrderRequest{})
	require.Equal(t, "123", snapshot["merchant_id"])
	require.Equal(t, "USDT", snapshot["currency"])
	require.NotContains(t, snapshot, "token")
	if algorithm == "" {
		// 模拟升级前已经创建的订单；商户当前默认协议升级也不能改变旧订单验签方式。
		delete(snapshot, "signature_algorithm")
	} else {
		require.Equal(t, algorithm, snapshot["signature_algorithm"])
	}
	order, err = client.PaymentOrder.UpdateOneID(order.ID).SetOrderType(payment.OrderTypeBalance).ClearPlanID().ClearSubscriptionGroupID().ClearSubscriptionDays().ClearPaidAt().SetOutTradeNo("site-100").SetAmount(12.3).SetPayAmount(12.3).SetPaymentType(payment.TypeOKPay).SetPaymentTradeNo("pay-900").SetProviderKey(payment.TypeOKPay).SetProviderInstanceID(strconv.FormatInt(instance.ID, 10)).SetProviderSnapshot(snapshot).Save(ctx)
	require.NoError(t, err)
	userRepo := &mockUserRepo{getByIDUser: &User{ID: order.UserID, Email: order.UserEmail, Username: order.UserName}}
	userRepo.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		require.Equal(t, order.UserID, id)
		userRepo.getByIDUser.Balance += amount
		return nil
	}
	redeemRepo := &paymentOrderLifecycleRedeemRepo{codesByCode: map[string]*RedeemCode{order.RechargeCode: {ID: 1, Code: order.RechargeCode, Type: RedeemTypeBalance, Value: order.Amount, Status: StatusUnused}}}
	svc := &PaymentService{entClient: client, registry: payment.NewRegistry(), loadBalancer: payment.NewDefaultLoadBalancer(client, nil), userRepo: userRepo, redeemService: NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)}
	return okpayIntegrationFixture{service: svc, order: order, userRepo: userRepo, redeemRepo: redeemRepo}
}

func okpayIntegrationNotification(order *dbent.PaymentOrder) *payment.PaymentNotification {
	return &payment.PaymentNotification{OrderID: order.OutTradeNo, TradeNo: order.PaymentTradeNo, Amount: order.PayAmount, Status: payment.NotificationStatusSuccess, Metadata: map[string]string{"merchant_id": "123", "currency": "USDT", "unique_id": order.OutTradeNo, "type": "deposit", "status": "1"}}
}

func TestOKPayIntegrationQueriesMerchantOrderAndRetriesWithoutUsingPlatformTradeNo(t *testing.T) {
	fixture := newOKPayIntegrationFixture(t)
	query := &okpayMerchantQueryStub{paymentFulfillmentTestProvider: paymentFulfillmentTestProvider{key: payment.TypeOKPay}, responses: []*payment.QueryOrderResponse{
		{TradeNo: "pay-900", Status: payment.ProviderStatusPaid, Amount: 0},
		{TradeNo: "pay-900", Status: payment.ProviderStatusPaid, Amount: 12.3, Metadata: okpayIntegrationNotification(fixture.order).Metadata},
	}}
	restore := replacePaymentProviderFactoryForTest(t, query)
	defer restore()
	result, err := fixture.service.VerifyOrderByOutTradeNo(context.Background(), fixture.order.OutTradeNo, fixture.order.UserID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, result.Status)
	require.Equal(t, []string{"site-100", "site-100"}, query.merchantIDs)
	require.Zero(t, query.genericCalls)
	require.Equal(t, 12.3, fixture.userRepo.getByIDUser.Balance)
	require.Len(t, fixture.redeemRepo.useCalls, 1)
}

func TestOKPayIntegrationRejectsWrongSnapshotIdentityCurrencyOrderAndAmount(t *testing.T) {
	for _, key := range []string{"merchant_id", "currency", "unique_id", "missing_metadata", "amount", "platform_trade_no"} {
		t.Run(key, func(t *testing.T) {
			fixture := newOKPayIntegrationFixture(t)
			notification := okpayIntegrationNotification(fixture.order)
			switch key {
			case "missing_metadata":
				notification.Metadata = nil
			case "amount":
				notification.Amount = 12.31
			case "platform_trade_no":
				notification.TradeNo = "wrong-platform-order"
			default:
				notification.Metadata[key] = "wrong"
			}
			require.Error(t, fixture.service.HandlePaymentNotification(context.Background(), notification, payment.TypeOKPay))
			reloaded, err := fixture.service.entClient.PaymentOrder.Get(context.Background(), fixture.order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusPending, reloaded.Status)
			require.Equal(t, "pay-900", reloaded.PaymentTradeNo)
			require.Nil(t, reloaded.PaidAt)
			require.Zero(t, fixture.userRepo.getByIDUser.Balance)
			require.Empty(t, fixture.redeemRepo.useCalls)
		})
	}
}

func TestOKPayIntegrationWrongQueryTradeNoCannotReplaceOriginalOrCreditBalance(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry_%v", retry), func(t *testing.T) {
			fixture := newOKPayIntegrationFixture(t)
			responses := []*payment.QueryOrderResponse{{TradeNo: "wrong-platform-order", Status: payment.ProviderStatusPaid, Amount: 12.3, Metadata: okpayIntegrationNotification(fixture.order).Metadata}}
			if retry {
				responses = append([]*payment.QueryOrderResponse{{TradeNo: "pay-900", Status: payment.ProviderStatusPaid, Amount: 0}}, responses...)
			}
			query := &okpayMerchantQueryStub{paymentFulfillmentTestProvider: paymentFulfillmentTestProvider{key: payment.TypeOKPay}, responses: responses}
			restore := replacePaymentProviderFactoryForTest(t, query)
			defer restore()
			result, err := fixture.service.VerifyOrderByOutTradeNo(context.Background(), fixture.order.OutTradeNo, fixture.order.UserID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusPending, result.Status)
			require.Equal(t, "pay-900", result.PaymentTradeNo)
			require.Nil(t, result.PaidAt)
			require.Len(t, query.merchantIDs, len(responses))
			require.Zero(t, query.genericCalls)
			require.Zero(t, fixture.userRepo.getByIDUser.Balance)
			require.Empty(t, fixture.redeemRepo.useCalls)
		})
	}
}

func TestOKPayIntegrationPinnedMerchantVerifiesAndDuplicateCallbackCreditsOnce(t *testing.T) {
	fixture := newOKPayIntegrationFixture(t)
	ctx := context.Background()
	_, err := fixture.service.entClient.PaymentProviderInstance.Create().SetProviderKey(payment.TypeOKPay).SetName("OKPay 商户 B").SetConfig(`{"id":"456","token":"different-token"}`).SetSupportedTypes(payment.TypeOKPay).SetPaymentMode("redirect").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	providers, err := fixture.service.GetWebhookProviders(ctx, payment.TypeOKPay, "site-100")
	require.NoError(t, err)
	require.Len(t, providers, 1)
	// 签名来自原生 PHP 黄金样本，对错误的商户实例验签会失败。
	raw := `{"id":"123","status":"success","code":10000,"data":{"order_id":"pay-900","unique_id":"site-100","pay_user_id":5639813059,"amount":"12.30","coin":"USDT","status":1,"type":"deposit"},"sign":"5C3DFCBF609D8BD0E66311EC73726275"}`
	notification, err := providers[0].VerifyNotification(ctx, raw, nil)
	require.NoError(t, err)
	require.Equal(t, "123", notification.Metadata["merchant_id"])
	require.NoError(t, fixture.service.HandlePaymentNotification(ctx, notification, payment.TypeOKPay))
	require.NoError(t, fixture.service.HandlePaymentNotification(ctx, notification, payment.TypeOKPay))
	reloaded, err := fixture.service.entClient.PaymentOrder.Get(ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Equal(t, "pay-900", reloaded.PaymentTradeNo)
	require.Equal(t, 12.3, fixture.userRepo.getByIDUser.Balance)
	require.Len(t, fixture.redeemRepo.useCalls, 1)
	_, err = fixture.service.GetWebhookProviders(ctx, payment.TypeOKPay, "unknown-order")
	require.Error(t, err, "多个商户实例不能随机选择一个验签")
}

// 使用标准库独立构造对照签名，不复用待验证的服务商签名实现。
func okpayIntegrationHMACSignature(fields map[string]string, token string) string {
	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		if key != "sign" && value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+fields[key])
	}
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(strings.Join(parts, "&")))
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
}

func okpayIntegrationHMACCallback(t *testing.T, changes map[string]string) string {
	t.Helper()
	fields := map[string]string{
		"id": "123", "status": "success", "code": "200", "data.order_id": "pay-900",
		"data.unique_id": "site-100", "data.pay_user_id": "5639813059", "data.amount": "12.30",
		"data.coin": "USDT", "data.status": "1", "data.type": "deposit",
	}
	for key, value := range changes {
		fields[key] = value
	}
	data := make(map[string]string)
	for key, value := range fields {
		if strings.HasPrefix(key, "data.") {
			data[strings.TrimPrefix(key, "data.")] = value
		}
	}
	raw, err := json.Marshal(map[string]any{
		"id": fields["id"], "status": fields["status"], "code": fields["code"], "data": data,
		"sign": okpayIntegrationHMACSignature(fields, "token+plus%2Band%zz"),
	})
	require.NoError(t, err)
	return string(raw)
}

func TestOKPayIntegrationHMACSnapshotVerifiesPinnedMerchantAndCreditsDuplicateOnce(t *testing.T) {
	fixture := newOKPayIntegrationFixtureWithAlgorithm(t, "hmac_sha256")
	ctx := context.Background()
	_, err := fixture.service.entClient.PaymentProviderInstance.Create().SetProviderKey(payment.TypeOKPay).
		SetName("另一商户").SetConfig(`{"id":"456","token":"other-token"}`).
		SetSupportedTypes(payment.TypeOKPay).SetPaymentMode("redirect").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	// 已保存的全局协议后续切回旧版，也不能影响新订单固定的 HMAC 验签。
	instanceID, err := strconv.ParseInt(*fixture.order.ProviderInstanceID, 10, 64)
	require.NoError(t, err)
	_, err = fixture.service.entClient.PaymentProviderInstance.UpdateOneID(instanceID).
		SetConfig(`{"id":"123","token":"token+plus%2Band%zz","signatureAlgorithm":"legacy_md5"}`).Save(ctx)
	require.NoError(t, err)
	providers, err := fixture.service.GetWebhookProviders(ctx, payment.TypeOKPay, fixture.order.OutTradeNo)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	notification, err := providers[0].VerifyNotification(ctx, okpayIntegrationHMACCallback(t, nil), nil)
	require.NoError(t, err)
	require.Equal(t, "123", notification.Metadata["merchant_id"])
	for repeat := 0; repeat < 2; repeat++ {
		require.NoError(t, fixture.service.HandlePaymentNotification(ctx, notification, payment.TypeOKPay))
	}
	reloaded, err := fixture.service.entClient.PaymentOrder.Get(ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Equal(t, "pay-900", reloaded.PaymentTradeNo)
	require.Equal(t, 12.3, fixture.userRepo.getByIDUser.Balance)
	require.Len(t, fixture.redeemRepo.useCalls, 1)
}

func TestOKPayIntegrationHMACSignedWrongIdentityCurrencyOrAmountDoesNotCredit(t *testing.T) {
	for _, key := range []string{"id", "data.coin", "data.amount"} {
		t.Run(key, func(t *testing.T) {
			fixture := newOKPayIntegrationFixtureWithAlgorithm(t, "hmac_sha256")
			ctx := context.Background()
			providers, err := fixture.service.GetWebhookProviders(ctx, payment.TypeOKPay, fixture.order.OutTradeNo)
			require.NoError(t, err)
			require.Len(t, providers, 1)
			changes := map[string]string{key: "wrong"}
			if key == "data.amount" {
				changes[key] = "12.31"
			}
			// 错误字段也使用正确密钥重新签名，验证业务校验能独立阻止入账。
			notification, verifyErr := providers[0].VerifyNotification(ctx, okpayIntegrationHMACCallback(t, changes), nil)
			if key == "data.amount" {
				require.NoError(t, verifyErr)
				require.Error(t, fixture.service.HandlePaymentNotification(ctx, notification, payment.TypeOKPay))
			} else {
				require.Error(t, verifyErr)
				require.Nil(t, notification)
			}
			reloaded, err := fixture.service.entClient.PaymentOrder.Get(ctx, fixture.order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusPending, reloaded.Status)
			require.Nil(t, reloaded.PaidAt)
			require.Equal(t, "pay-900", reloaded.PaymentTradeNo)
			require.Zero(t, fixture.userRepo.getByIDUser.Balance)
			require.Empty(t, fixture.redeemRepo.useCalls)
		})
	}
}

func TestOKPayIntegrationExplicitLegacySnapshotStillAcceptsMD5(t *testing.T) {
	fixture := newOKPayIntegrationFixtureWithAlgorithm(t, "legacy_md5")
	ctx := context.Background()
	providers, err := fixture.service.GetWebhookProviders(ctx, payment.TypeOKPay, fixture.order.OutTradeNo)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	raw := `{"id":"123","status":"success","code":10000,"data":{"order_id":"pay-900","unique_id":"site-100","pay_user_id":5639813059,"amount":"12.30","coin":"USDT","status":1,"type":"deposit"},"sign":"5C3DFCBF609D8BD0E66311EC73726275"}`
	notification, err := providers[0].VerifyNotification(ctx, raw, nil)
	require.NoError(t, err)
	require.NoError(t, fixture.service.HandlePaymentNotification(ctx, notification, payment.TypeOKPay))
	require.Equal(t, 12.3, fixture.userRepo.getByIDUser.Balance)
	require.Len(t, fixture.redeemRepo.useCalls, 1)
}
