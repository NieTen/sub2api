//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type usdtQuoteIntegrationRepository struct {
	latest *PaymentExchangeRatePoint
	err    error
	reads  int
}

func (r *usdtQuoteIntegrationRepository) Latest(context.Context) (*PaymentExchangeRatePoint, error) {
	r.reads++
	return r.latest, r.err
}
func (r *usdtQuoteIntegrationRepository) Insert(_ context.Context, point PaymentExchangeRatePoint) error {
	r.latest = &point
	return nil
}
func (r *usdtQuoteIntegrationRepository) History(context.Context, time.Time, time.Time) ([]PaymentExchangeRatePoint, error) {
	return nil, nil
}

type usdtQuoteIntegrationLoadBalancer struct {
	selection *payment.InstanceSelection
	amounts   []float64
}

func (b *usdtQuoteIntegrationLoadBalancer) GetInstanceConfig(context.Context, int64) (map[string]string, error) {
	return b.selection.Config, nil
}
func (b *usdtQuoteIntegrationLoadBalancer) SelectInstance(_ context.Context, _ string, _ payment.PaymentType, _ payment.Strategy, amount float64) (*payment.InstanceSelection, error) {
	b.amounts = append(b.amounts, amount)
	return b.selection, nil
}

type usdtQuoteIntegrationTransport struct {
	amounts []string
	names   []string
}

func (r *usdtQuoteIntegrationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	// 测试只截获本地模拟域名，防止集成验证触达真实支付接口。
	if request.URL.Host != "okpay.test" || request.URL.Path != "/shop/payLink" || request.Method != http.MethodPost {
		return nil, fmt.Errorf("测试不允许访问此地址: %s", request.URL)
	}
	var payload map[string]any
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return nil, err
	}
	amount, ok := payload["amount"].(string)
	if !ok {
		return nil, errors.New("支付金额必须以精确字符串发送")
	}
	r.amounts = append(r.amounts, amount)
	name, _ := payload["name"].(string)
	r.names = append(r.names, name)
	body := fmt.Sprintf(`{"status":"success","code":10000,"data":{"order_id":"pay-%d","pay_url":"https://okpay.test/cashier"}}`, len(r.amounts))
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

type usdtQuoteIntegrationIntentRepository struct {
	PaymentTRC20Repository
	intents map[int64]*PaymentTRC20Intent
}

func (r *usdtQuoteIntegrationIntentRepository) Allocate(_ context.Context, input PaymentTRC20CreateInput, units, _ int64) (*PaymentTRC20Intent, error) {
	intent := &PaymentTRC20Intent{OrderID: input.OrderID, OutTradeNo: input.OutTradeNo, ProviderInstanceID: input.ProviderInstanceID, WalletAddress: input.WalletAddress, BaseAmount: input.BaseAmount, AmountUnits: units + 42, CreatedAt: input.CreatedAt, ExpiresAt: input.ExpiresAt}
	r.intents[input.OrderID] = intent
	return intent, nil
}
func (r *usdtQuoteIntegrationIntentRepository) Get(_ context.Context, orderID int64) (*PaymentTRC20Intent, error) {
	intent := r.intents[orderID]
	if intent == nil {
		return nil, ErrTRC20IntentNotFound
	}
	copy := *intent
	return &copy, nil
}

type usdtQuoteIntegrationFixture struct {
	service   *PaymentService
	quoteRepo *usdtQuoteIntegrationRepository
	quoteSvc  *PaymentExchangeRateService
	settings  *paymentConfigSettingRepoStub
	balancer  *usdtQuoteIntegrationLoadBalancer
	transport *usdtQuoteIntegrationTransport
	intents   *usdtQuoteIntegrationIntentRepository
	userRepo  *mockUserRepo
	userID    int64
	method    string
	now       time.Time
}

func newUSDTQuoteIntegrationFixture(t *testing.T, providerKey, method string) *usdtQuoteIntegrationFixture {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().SetEmail("usdt-quote@example.com").SetUsername("汇率测试用户").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	config := map[string]string{"id": "123", "token": "test-token", "apiBase": "https://okpay.test/shop", "notifyUrl": "https://site.example/api/v1/payment/webhook/okpay", "paymentMode": "redirect"}
	mode := "redirect"
	if providerKey == payment.TypeUSDTTRC20 {
		config = map[string]string{"walletAddress": "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7", "apiKey": "test-key", "paymentMode": "qrcode"}
		mode = "qrcode"
	} else if providerKey == payment.TypeEasyPay {
		config = map[string]string{"pid": "123", "pkey": "test-key", "apiBase": "https://easypay.test", "notifyUrl": "https://site.example/api/v1/payment/webhook/easypay", "returnUrl": "https://site.example/payment/result", "paymentMode": "popup", "customMethods": `[{"type":"` + method + `","upstreamType":"legacy-usdt","displayName":"旧自定义渠道"}]`}
		mode = "popup"
	}
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	instance, err := client.PaymentProviderInstance.Create().SetProviderKey(providerKey).SetName("报价集成测试渠道").SetConfig(string(raw)).SetSupportedTypes(method).SetPaymentMode(mode).SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	settings := &paymentConfigSettingRepoStub{values: map[string]string{SettingPaymentEnabled: "true", SettingEnabledPaymentTypes: method, SettingBalanceRechargeMult: "0.14", SettingRechargeFeeRate: "2.5", SettingMaxPendingOrders: "20"}}
	configService := &PaymentConfigService{entClient: client, settingRepo: settings, encryptionKey: []byte("0123456789abcdef0123456789abcdef")}
	now := time.Now().UTC().Truncate(time.Second)
	repo := &usdtQuoteIntegrationRepository{latest: &PaymentExchangeRatePoint{Rate: 7.2, Source: "okx", FetchedAt: &now, ObservedAt: now, SamplePrices: []float64{7.2, 7.2}, SampleCount: 2, Aggregation: "median"}}
	quoteSvc := NewPaymentExchangeRateService(repo, configService)
	quoteSvc.now = func() time.Time { return now }
	quoteSvc.fallbackGetter = func(context.Context) (float64, error) { return 7.5, nil }
	balancer := &usdtQuoteIntegrationLoadBalancer{selection: &payment.InstanceSelection{InstanceID: strconv.FormatInt(instance.ID, 10), ProviderKey: providerKey, SupportedTypes: method, PaymentMode: mode, Config: config}}
	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: StatusActive}}
	userRepo.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		userRepo.getByIDUser.Balance += amount
		return nil
	}
	intents := &usdtQuoteIntegrationIntentRepository{intents: map[int64]*PaymentTRC20Intent{}}
	svc := &PaymentService{entClient: client, configService: configService, registry: payment.NewRegistry(), loadBalancer: balancer, userRepo: userRepo, exchangeRateSvc: quoteSvc, trc20Svc: NewPaymentTRC20Service(intents)}
	transport := &usdtQuoteIntegrationTransport{}
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous })
	return &usdtQuoteIntegrationFixture{service: svc, quoteRepo: repo, quoteSvc: quoteSvc, settings: settings, balancer: balancer, transport: transport, intents: intents, userRepo: userRepo, userID: user.ID, method: method, now: now}
}

func (f *usdtQuoteIntegrationFixture) create(t *testing.T) (*CreateOrderResponse, *dbent.PaymentOrder) {
	t.Helper()
	response, err := f.service.CreateOrder(context.Background(), CreateOrderRequest{UserID: f.userID, PaymentType: f.method, Amount: 10, OrderType: payment.OrderTypeBalance, SrcHost: "site.example", ClientIP: "127.0.0.1"})
	require.NoError(t, err)
	order, err := f.service.entClient.PaymentOrder.Get(context.Background(), response.OrderID)
	require.NoError(t, err)
	return response, order
}

func TestUSDTQuoteCalculatesCNYRechargeAndCompatibleSubscriptionPrice(t *testing.T) {
	quote := &PaymentExchangeRateQuote{Rate: 7.2, Source: "okx"}
	for _, tc := range []struct {
		name, orderType, mode                          string
		price, subscriptionRate, cnyBase, cnyPay, usdt float64
	}{
		{"人民币充值", payment.OrderTypeBalance, "balance_cny", 10, 0, 10, 10.25, 1.43},
		{"订阅沿用旧数值", payment.OrderTypeSubscription, "subscription_legacy_cny", 10, 0, 10, 10.25, 1.43},
		{"订阅显式人民币换算", payment.OrderTypeSubscription, "subscription_cny", 10, 7.2, 72, 73.8, 10.25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			details, err := calculateUSDTExchangePayment(tc.price, 2.5, tc.orderType, tc.subscriptionRate, quote)
			require.NoError(t, err)
			require.Equal(t, tc.usdt, details.USDTPayAmount)
			require.Equal(t, tc.cnyBase, details.CNYBaseAmount)
			require.Equal(t, tc.cnyPay, details.CNYPayAmount)
			require.Equal(t, tc.mode, details.PricingMode)
		})
	}
	for _, rate := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		_, err := calculateUSDTExchangePayment(10, 0, payment.OrderTypeBalance, 0, &PaymentExchangeRateQuote{Rate: rate, Source: "okx"})
		require.Error(t, err)
	}
}

func TestUSDTQuoteNativeOrdersLockQuoteAndCreditCNYBalance(t *testing.T) {
	for _, providerKey := range []string{payment.TypeOKPay, payment.TypeUSDTTRC20} {
		t.Run(providerKey, func(t *testing.T) {
			fixture := newUSDTQuoteIntegrationFixture(t, providerKey, providerKey)
			response, order := fixture.create(t)
			require.Equal(t, 1.43, response.PayAmount)
			require.Equal(t, 1.43, order.PayAmount)
			require.Equal(t, 1.4, order.Amount, "充值输入是人民币，余额仍应用全局充值倍率")
			details := PaymentOrderUSDTExchange(order)
			require.NotNil(t, details)
			require.Equal(t, 7.2, details.Rate)
			require.Equal(t, "okx", details.Source)
			require.Equal(t, 10.0, details.CNYBaseAmount)
			require.Equal(t, 10.25, details.CNYPayAmount)
			require.Equal(t, "USDT", PaymentOrderCurrency(order))
			require.Equal(t, []float64{1.43}, fixture.balancer.amounts, "实例限额选择必须使用实际付款USDT金额，不能先用人民币排除可用渠道")
			if providerKey == payment.TypeOKPay {
				require.Equal(t, []string{"1.43"}, fixture.transport.amounts)
				require.Equal(t, []string{"Sub2API 1.43 USDT"}, fixture.transport.names)
			} else {
				require.Equal(t, "1.430042", PaymentOrderTransferDetails(order).PaymentAmountExact)
				require.Equal(t, int64(1430042), fixture.intents.intents[order.ID].AmountUnits)
			}
			// 报价刷新只影响新订单，数据库中的旧订单金额、来源和时间必须锁定。
			fixture.quoteRepo.latest.Rate = 8
			_, newOrder := fixture.create(t)
			require.Equal(t, 1.29, newOrder.PayAmount)
			reloaded, err := fixture.service.entClient.PaymentOrder.Get(context.Background(), order.ID)
			require.NoError(t, err)
			require.Equal(t, 1.43, reloaded.PayAmount)
			require.Equal(t, details, PaymentOrderUSDTExchange(reloaded))
			redeemRepo := &paymentOrderLifecycleRedeemRepo{codesByCode: map[string]*RedeemCode{order.RechargeCode: {ID: 1, Code: order.RechargeCode, Type: RedeemTypeBalance, Value: order.Amount, Status: StatusUnused}}}
			fixture.service.redeemService = NewRedeemService(redeemRepo, fixture.userRepo, nil, nil, nil, fixture.service.entClient, nil, nil)
			notification := &payment.PaymentNotification{OrderID: order.OutTradeNo, TradeNo: order.PaymentTradeNo, Amount: 1.43, Status: payment.NotificationStatusSuccess, Metadata: map[string]string{"merchant_id": "123", "currency": "USDT", "unique_id": order.OutTradeNo}}
			if providerKey == payment.TypeUSDTTRC20 {
				intent := fixture.intents.intents[order.ID]
				paidAt := intent.CreatedAt.Add(time.Millisecond)
				intent.TransactionHash, intent.TransferredAt = strings.Repeat("a", 64), &paidAt
				notification = trc20Notification(intent)
			}
			require.NoError(t, fixture.service.HandlePaymentNotification(context.Background(), notification, providerKey))
			require.NoError(t, fixture.service.HandlePaymentNotification(context.Background(), notification, providerKey))
			require.Equal(t, 1.4, fixture.userRepo.getByIDUser.Balance)
			require.Len(t, redeemRepo.useCalls, 1)
		})
	}
}

func TestUSDTQuoteExpiredFailedAndUnavailableRatesUseExplicitFallback(t *testing.T) {
	for _, scenario := range []string{"expired", "last_refresh_failed", "repository_failed", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newUSDTQuoteIntegrationFixture(t, payment.TypeOKPay, payment.TypeOKPay)
			switch scenario {
			case "expired":
				past := fixture.now.Add(-31 * time.Minute)
				fixture.quoteRepo.latest.FetchedAt = &past
				fixture.quoteRepo.latest.ObservedAt = past
			case "last_refresh_failed":
				fixture.quoteRepo.latest.Source = "fallback"
				fixture.quoteRepo.latest.FallbackReason = "upstream_failed"
			case "repository_failed":
				fixture.quoteRepo.err = errors.New("报价库暂时不可用")
			case "empty":
				fixture.quoteRepo.latest = nil
			}
			_, order := fixture.create(t)
			details := PaymentOrderUSDTExchange(order)
			require.NotNil(t, details)
			require.Equal(t, "fallback", details.Source)
			require.Equal(t, 7.5, details.Rate, "失败或过期后不能拿旧实时汇率冒充本单有效报价")
			require.NotEmpty(t, details.FallbackReason)
			require.Equal(t, 1.37, order.PayAmount)
		})
	}
	fixture := newUSDTQuoteIntegrationFixture(t, payment.TypeOKPay, payment.TypeOKPay)
	fixture.quoteRepo.latest = nil
	fixture.quoteSvc.fallbackGetter = func(context.Context) (float64, error) { return 0, nil }
	_, err := fixture.service.CreateOrder(context.Background(), CreateOrderRequest{UserID: fixture.userID, PaymentType: fixture.method, Amount: 10, OrderType: payment.OrderTypeBalance})
	require.Error(t, err)
	count, err := fixture.service.entClient.PaymentOrder.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	require.Empty(t, fixture.transport.amounts)
}

func TestUSDTQuoteDoesNotChangeEasyPayCustomMethodWithNativeName(t *testing.T) {
	for _, method := range []string{payment.TypeOKPay, payment.TypeUSDTTRC20} {
		t.Run(method, func(t *testing.T) {
			fixture := newUSDTQuoteIntegrationFixture(t, payment.TypeEasyPay, method)
			fixture.quoteRepo.latest = nil
			fixture.quoteSvc.fallbackGetter = func(context.Context) (float64, error) { return 0, errors.New("不应该查询易支付汇率") }
			response, order := fixture.create(t)
			require.Equal(t, 10.25, order.PayAmount)
			require.Equal(t, 1.4, order.Amount)
			require.Equal(t, "CNY", PaymentOrderCurrency(order))
			require.Nil(t, PaymentOrderUSDTExchange(order))
			parsed, err := url.Parse(response.PayURL)
			require.NoError(t, err)
			require.Equal(t, "10.25", parsed.Query().Get("money"))
			require.Equal(t, "legacy-usdt", parsed.Query().Get("type"))
		})
	}
}

func TestUSDTQuoteDailyLimitPreservesLegacyOrderAndUsesNewCNYSnapshot(t *testing.T) {
	legacy := &dbent.PaymentOrder{OrderType: payment.OrderTypeBalance, PaymentType: payment.TypeOKPay, PayAmount: 1.43, Amount: 1.4, ProviderSnapshot: map[string]any{"currency": "USDT", "provider_key": payment.TypeOKPay}}
	require.Equal(t, 1.43, paymentOrderDailyLimitAmount(legacy), "旧订单不能根据今天的费率补造历史人民币金额")
	require.Nil(t, PaymentOrderUSDTExchange(legacy))
	current := *legacy
	current.ProviderSnapshot = map[string]any{"currency": "USDT", "provider_key": payment.TypeOKPay, "usdt_exchange": &PaymentOrderExchangeDetails{Rate: 7.2, Source: "okx", CNYBaseAmount: 10, CNYPayAmount: 10.25, USDTPayAmount: 1.43}}
	require.Equal(t, 10.25, paymentOrderDailyLimitAmount(&current))
	legacy.OrderType = payment.OrderTypeSubscription
	legacy.Amount = 12.34
	require.Equal(t, 12.34, paymentOrderDailyLimitAmount(legacy), "既有订阅额度语义保持不变")
}

func TestUSDTQuoteDailyLimitRejectsNewOrderUsingTotalCNYWithFee(t *testing.T) {
	for _, limit := range []string{"20", "20.4"} {
		t.Run(limit, func(t *testing.T) {
			fixture := newUSDTQuoteIntegrationFixture(t, payment.TypeOKPay, payment.TypeOKPay)
			fixture.settings.values[SettingDailyRechargeLimit] = limit
			_, previous := fixture.create(t)
			_, err := fixture.service.entClient.PaymentOrder.UpdateOneID(previous.ID).
				SetStatus(OrderStatusCompleted).SetPaidAt(time.Now()).Save(context.Background())
			require.NoError(t, err)
			_, err = fixture.service.CreateOrder(context.Background(), CreateOrderRequest{UserID: fixture.userID, PaymentType: fixture.method, Amount: 10, OrderType: payment.OrderTypeBalance, SrcHost: "site.example"})
			require.Error(t, err, "历史10.25元与新单10.25元合计20.50元，不能用1.43U或未含费10元检查限额")
			require.Equal(t, "DAILY_LIMIT_EXCEEDED", infraerrors.FromError(err).Reason)
			count, countErr := fixture.service.entClient.PaymentOrder.Query().Count(context.Background())
			require.NoError(t, countErr)
			require.Equal(t, 1, count)
			require.Equal(t, []string{"1.43"}, fixture.transport.amounts, "超额订单不能向支付平台下单")
		})
	}
}

func TestUSDTQuoteFallbackConfigPreservesPrecisionAndRejectsInvalidValues(t *testing.T) {
	ctx := context.Background()
	repo := &paymentConfigSettingRepoStub{values: map[string]string{SettingUSDTCNYFallbackRate: "7"}}
	svc := &PaymentConfigService{settingRepo: repo}
	for _, rate := range []float64{7.1234, 0, 1, 100} {
		require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{USDTCNYFallbackRate: &rate}))
		stored, err := svc.GetUSDTCNYFallbackRate(ctx)
		require.NoError(t, err)
		require.Equal(t, rate, stored)
		config, err := svc.GetPaymentConfig(ctx)
		require.NoError(t, err)
		require.Equal(t, rate, config.USDTCNYFallbackRate)
	}
	previous := repo.values[SettingUSDTCNYFallbackRate]
	for _, invalid := range []float64{math.NaN(), math.Inf(1), -1, 0.9, 101} {
		err := svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{USDTCNYFallbackRate: &invalid})
		require.Error(t, err)
		require.Equal(t, "INVALID_USDT_CNY_FALLBACK_RATE", infraerrors.FromError(err).Reason)
		require.Equal(t, previous, repo.values[SettingUSDTCNYFallbackRate], "非法更新不能覆盖最后有效配置")
	}
	// 对已有错误配置采用不可用状态，不能将负数或非有限数用于金额除法。
	for _, invalid := range []string{"NaN", "+Inf", "-1", "101", "bad"} {
		repo.values[SettingUSDTCNYFallbackRate] = invalid
		stored, err := svc.GetUSDTCNYFallbackRate(ctx)
		require.NoError(t, err)
		require.Zero(t, stored)
	}
}
