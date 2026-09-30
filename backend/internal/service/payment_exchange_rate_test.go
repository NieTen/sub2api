package service

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type paymentExchangeRateMemoryRepository struct {
	mu       sync.Mutex
	points   []PaymentExchangeRatePoint
	readErr  error
	writeErr error
	since    time.Time
	until    time.Time
}

func (r *paymentExchangeRateMemoryRepository) Latest(context.Context) (*PaymentExchangeRatePoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return nil, r.readErr
	}
	if len(r.points) == 0 {
		return nil, nil
	}
	point := r.points[len(r.points)-1]
	return &point, nil
}

func (r *paymentExchangeRateMemoryRepository) Insert(_ context.Context, point PaymentExchangeRatePoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.writeErr != nil {
		return r.writeErr
	}
	r.points = append(r.points, point)
	return nil
}

func (r *paymentExchangeRateMemoryRepository) History(_ context.Context, since, until time.Time) ([]PaymentExchangeRatePoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.since, r.until = since, until
	return append([]PaymentExchangeRatePoint{}, r.points...), r.readErr
}

type paymentExchangeRateTransport func(*http.Request) (*http.Response, error)

func (f paymentExchangeRateTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func exchangeRateTestService(repo PaymentExchangeRateRepository, now time.Time, fallback float64) *PaymentExchangeRateService {
	svc := NewPaymentExchangeRateService(repo, nil)
	svc.now = func() time.Time { return now }
	svc.fallbackGetter = func(context.Context) (float64, error) { return fallback, nil }
	return svc
}

func exchangeRateTestResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

func TestPaymentExchangeRateOKXUsesFirstTenValidSellPricesAndDynamicTimestamp(t *testing.T) {
	now := time.Date(2026, 9, 30, 8, 20, 10, 123000000, time.UTC)
	svc := exchangeRateTestService(nil, now, 7)
	requests := 0
	svc.client.Transport = paymentExchangeRateTransport(func(request *http.Request) (*http.Response, error) {
		requests++
		require.Equal(t, http.MethodGet, request.Method)
		require.Equal(t, strconv.FormatInt(now.Add(time.Duration(requests-1)*time.Minute).UnixMilli(), 10), request.URL.Query().Get("t"))
		require.Equal(t, "sell", request.URL.Query().Get("side"))
		require.Equal(t, "CNY", request.URL.Query().Get("quoteCurrency"))
		require.Equal(t, "USDT", request.URL.Query().Get("baseCurrency"))
		return exchangeRateTestResponse(`{"code":0,"data":{"buy":[{"price":"99"}],"sell":[{"price":null},{"price":"NaN"},{"price":"+Inf"},{"price":"-1"},{"price":"0.99"},{"price":"100.01"},{"price":"9"},{"price":"5"},{"price":7},{"price":"6"},{"price":"8"},{"price":"6.5"},{"price":"7.5"},{"price":"5.5"},{"price":"8.5"},{"price":"9.5"},{"price":"1"}]}}`), nil
	})
	for i := 0; i < 2; i++ {
		point, err := svc.fetchOKX(context.Background(), now.Add(time.Duration(i)*time.Minute))
		require.NoError(t, err)
		require.Equal(t, 7.25, point.Rate)
		require.Equal(t, []float64{9, 5, 7, 6, 8, 6.5, 7.5, 5.5, 8.5, 9.5}, point.SamplePrices)
		require.Equal(t, 10, point.SampleCount)
		require.Equal(t, "median_first_10_sell", point.Aggregation)
		require.Equal(t, PaymentExchangeRateSourceOKX, point.Source)
	}
}

func TestPaymentExchangeRateOKXRejectsInvalidResponses(t *testing.T) {
	for _, body := range []string{
		`{"data":{"sell":[{"price":"7"}]}}`,
		`{"code":1,"data":{"sell":[{"price":"7"}]}}`,
		`{"code":null,"data":{"sell":[{"price":"7"}]}}`,
		`{"code":0,"data":{"buy":[{"price":"7"}],"sell":[]}}`,
		`{"code":0,"data":{"sell":[{"price":"0"},{"price":"Infinity"},{"price":"101"},{"price":false}]}}`,
		`{"code":0,"data":{"sell":[{"price":"7"}]}} trailing`,
		`<html>Access denied</html>`,
	} {
		t.Run(body, func(t *testing.T) {
			svc := exchangeRateTestService(nil, time.Now(), 7)
			svc.client.Transport = paymentExchangeRateTransport(func(*http.Request) (*http.Response, error) { return exchangeRateTestResponse(body), nil })
			_, err := svc.fetchOKX(context.Background(), time.Now())
			require.Error(t, err)
		})
	}
	svc := exchangeRateTestService(nil, time.Now(), 7)
	svc.client.Transport = paymentExchangeRateTransport(func(*http.Request) (*http.Response, error) {
		return exchangeRateTestResponse(`{"code":"0","data":{"sell":[{"price":"7.2"},{"price":"6.8"},{"price":"7.1"}]}}`), nil
	})
	point, err := svc.fetchOKX(context.Background(), time.Now())
	require.NoError(t, err)
	require.Equal(t, 7.1, point.Rate)
}

func TestPaymentExchangeRateQuoteUsesLatestAttemptAndCurrentFallback(t *testing.T) {
	now := time.Now().UTC()
	fetchedAt := now.Add(-time.Minute)
	repo := &paymentExchangeRateMemoryRepository{points: []PaymentExchangeRatePoint{{Rate: 6.8, Source: "okx", FetchedAt: &fetchedAt, ObservedAt: fetchedAt, SamplePrices: []float64{6.8}, SampleCount: 1}}}
	svc := exchangeRateTestService(repo, now, 7)
	svc.client.Transport = paymentExchangeRateTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("查询当前报价不能访问外部接口")
		return nil, nil
	})
	quote, err := svc.GetQuote(context.Background())
	require.NoError(t, err)
	require.Equal(t, 6.8, quote.Rate)
	require.Equal(t, "okx", quote.Source)
	// 最新失败不能被之前仍在有效期内的成功记录掩盖；后台修改兜底立即生效。
	repo.points = append(repo.points, PaymentExchangeRatePoint{Rate: 9, Source: "fallback", FetchedAt: &now, ObservedAt: now, FallbackReason: "网络失败"})
	quote, err = svc.GetQuote(context.Background())
	require.NoError(t, err)
	require.Equal(t, 7.0, quote.Rate)
	require.Equal(t, "fallback", quote.Source)
	require.Equal(t, "网络失败", quote.FallbackReason)
	require.Equal(t, now, quote.ObservedAt)
	require.Equal(t, now, *quote.FetchedAt)
	require.Empty(t, quote.SamplePrices)
	svc.fallbackGetter = func(context.Context) (float64, error) { return 7.2, nil }
	quote, err = svc.GetQuote(context.Background())
	require.NoError(t, err)
	require.Equal(t, 7.2, quote.Rate)
}

func TestPaymentExchangeRateStaleAndFutureQuotesFallBack(t *testing.T) {
	now := time.Now().UTC()
	for _, age := range []time.Duration{30 * time.Minute, time.Hour, -time.Minute} {
		fetchedAt := now.Add(-age)
		repo := &paymentExchangeRateMemoryRepository{points: []PaymentExchangeRatePoint{{Rate: 6.8, Source: "okx", FetchedAt: &fetchedAt, ObservedAt: fetchedAt}}}
		svc := exchangeRateTestService(repo, now, 7)
		quote, err := svc.GetQuote(context.Background())
		require.NoError(t, err)
		require.Equal(t, "fallback", quote.Source)
		require.Equal(t, 7.0, quote.Rate)
	}
}

func TestPaymentExchangeRateRecoversAfterSystemClockCorrection(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	repo := &paymentExchangeRateMemoryRepository{points: []PaymentExchangeRatePoint{{Rate: 8, Source: "okx", FetchedAt: &future, ObservedAt: future, SamplePrices: []float64{8}, SampleCount: 1}}}
	svc := exchangeRateTestService(repo, now, 7)
	quote, err := svc.GetQuote(context.Background())
	require.NoError(t, err)
	require.Equal(t, "fallback", quote.Source)
	svc.client.Transport = paymentExchangeRateTransport(func(*http.Request) (*http.Response, error) {
		return exchangeRateTestResponse(`{"code":0,"data":{"sell":[{"price":"6.8"}]}}`), nil
	})
	// 发现历史时间在未来时立即重采；最近持久化的有效报价应覆盖旧记录。
	delay, err := svc.refreshIfDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, PaymentExchangeRateInterval, delay)
	require.Len(t, repo.points, 2)
	quote, err = svc.GetQuote(context.Background())
	require.NoError(t, err)
	require.Equal(t, "okx", quote.Source)
	require.Equal(t, 6.8, quote.Rate)
	require.Equal(t, now, *quote.FetchedAt)
	require.Empty(t, quote.FallbackReason)
}

func TestPaymentExchangeRateWithoutValidFallbackBlocksQuote(t *testing.T) {
	for _, rate := range []float64{0, -1, 0.99, 100.01, math.NaN(), math.Inf(1)} {
		svc := exchangeRateTestService(&paymentExchangeRateMemoryRepository{}, time.Now(), rate)
		quote, err := svc.GetQuote(context.Background())
		require.ErrorIs(t, err, ErrPaymentExchangeRateUnavailable)
		require.Equal(t, "unavailable", quote.Source)
		require.Zero(t, quote.Rate)
		require.Nil(t, quote.FetchedAt)
	}
	svc := exchangeRateTestService(&paymentExchangeRateMemoryRepository{readErr: errors.New("数据库错误")}, time.Now(), 7)
	quote, err := svc.GetQuote(context.Background())
	require.NoError(t, err)
	require.Equal(t, "fallback", quote.Source)
	require.Contains(t, quote.FallbackReason, "读取实时报价失败")
	svc.fallbackGetter = func(context.Context) (float64, error) { return 7, errors.New("配置读取错误") }
	quote, err = svc.GetQuote(context.Background())
	require.ErrorIs(t, err, ErrPaymentExchangeRateUnavailable)
	require.Equal(t, "unavailable", quote.Source)
}

func TestPaymentExchangeRateWorkerPersistsFailuresAndResumesFromDatabase(t *testing.T) {
	now := time.Now().UTC()
	repo := &paymentExchangeRateMemoryRepository{}
	svc := exchangeRateTestService(repo, now, 7.2)
	calls := 0
	svc.client.Transport = paymentExchangeRateTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("模拟网络错误")
	})
	delay, err := svc.refreshIfDue(context.Background())
	require.Error(t, err)
	require.Equal(t, PaymentExchangeRateInterval, delay)
	require.Len(t, repo.points, 1)
	require.Equal(t, "fallback", repo.points[0].Source)
	require.Equal(t, 7.2, repo.points[0].Rate)
	require.Equal(t, now, *repo.points[0].FetchedAt)

	restarted := exchangeRateTestService(repo, now.Add(10*time.Minute), 0)
	restarted.client = svc.client
	delay, err = restarted.refreshIfDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 20*time.Minute, delay)
	require.Equal(t, 1, calls)
	restarted.now = func() time.Time { return now.Add(30 * time.Minute) }
	_, err = restarted.refreshIfDue(context.Background())
	require.Error(t, err)
	require.Len(t, repo.points, 2)
	require.Equal(t, "unavailable", repo.points[1].Source)
	require.Zero(t, repo.points[1].Rate)
	require.NotEmpty(t, repo.points[1].FallbackReason)
}

func TestPaymentExchangeRateWorkerSingleLeaderAndSuccessfulHistory(t *testing.T) {
	now := time.Now().UTC()
	repo := &paymentExchangeRateMemoryRepository{}
	svc := exchangeRateTestService(repo, now, 7)
	cache := &fakeLeaderLockCache{}
	svc.SetLeaderLock(cache, nil)
	_, err := cache.TryAcquireLeaderLock(context.Background(), paymentExchangeRateLeaderLockKey, "peer", time.Minute)
	require.NoError(t, err)
	calls := 0
	svc.client.Transport = paymentExchangeRateTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return exchangeRateTestResponse(`{"code":0,"data":{"sell":[{"price":"6.9"}]}}`), nil
	})
	_, err = svc.refreshIfDue(context.Background())
	require.NoError(t, err)
	require.Zero(t, calls)
	require.Empty(t, repo.points)
	require.NoError(t, cache.ReleaseLeaderLock(context.Background(), paymentExchangeRateLeaderLockKey, "peer"))
	_, err = svc.refreshIfDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Len(t, repo.points, 1)
	require.Equal(t, "okx", repo.points[0].Source)
	require.Equal(t, 6.9, repo.points[0].Rate)
	// 下一实例获得锁后仍需检查数据库，不能在同一采集周期重复请求。
	peer := exchangeRateTestService(repo, now, 7)
	peer.SetLeaderLock(cache, nil)
	peer.client = svc.client
	_, err = peer.refreshIfDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	history, err := svc.History(context.Background())
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, now.Add(-72*time.Hour), repo.since)
	require.Equal(t, now, repo.until)
}

func TestPaymentExchangeRateStopCancelsInFlightRequest(t *testing.T) {
	svc := exchangeRateTestService(&paymentExchangeRateMemoryRepository{}, time.Now(), 7)
	started := make(chan struct{})
	svc.client.Transport = paymentExchangeRateTransport(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	svc.Start()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("后台采集未启动")
	}
	stopped := make(chan struct{})
	go func() { svc.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("停止服务未及时取消外部请求")
	}
	svc.Stop()
}
