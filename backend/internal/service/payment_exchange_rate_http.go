package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const okxC2CExchangeRateEndpoint = "https://www.okx.com/v3/c2c/tradingOrders/books?quoteCurrency=CNY&baseCurrency=USDT&side=sell&paymentMethod=all&userType=all&receivingAds=false"
const paymentExchangeRateMaxResponseBytes = 2 * 1024 * 1024

type okxC2CExchangeRateResponse struct {
	Code json.RawMessage        `json:"code"`
	Data okxC2CExchangeRateData `json:"data"`
}

type okxC2CExchangeRateData struct {
	Sell []okxC2CExchangeRateOffer `json:"sell"`
}

type okxC2CExchangeRateOffer struct {
	Price json.RawMessage `json:"price"`
}

func (s *PaymentExchangeRateService) fetchOKX(ctx context.Context, fetchedAt time.Time) (*PaymentExchangeRatePoint, error) {
	endpoint, err := url.Parse(s.endpoint)
	if err != nil {
		return nil, errors.New("OKX 汇率接口地址无效")
	}
	query := endpoint.Query()
	query.Set("t", strconv.FormatInt(fetchedAt.UnixMilli(), 10))
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("创建 OKX 汇率请求失败")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("User-Agent", "Sub2API-ExchangeRate/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("OKX 汇率请求中断: %w", ctx.Err())
		}
		return nil, errors.New("OKX 汇率网络请求失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OKX 汇率接口返回 HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, paymentExchangeRateMaxResponseBytes+1))
	if err != nil {
		return nil, errors.New("读取 OKX 汇率响应失败")
	}
	if len(body) > paymentExchangeRateMaxResponseBytes {
		return nil, errors.New("OKX 汇率响应超过允许大小")
	}
	var payload okxC2CExchangeRateResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("OKX 汇率响应不是有效 JSON")
	}
	code := bytes.TrimSpace(payload.Code)
	if !bytes.Equal(code, []byte("0")) && !bytes.Equal(code, []byte(`"0"`)) {
		return nil, errors.New("OKX 汇率接口业务状态不是成功")
	}
	prices := make([]float64, 0, 10)
	for _, offer := range payload.Data.Sell {
		raw := strings.TrimSpace(string(offer.Price))
		if strings.HasPrefix(raw, `"`) {
			if err := json.Unmarshal(offer.Price, &raw); err != nil {
				continue
			}
		}
		price, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || !validPaymentExchangeRate(price) {
			continue
		}
		prices = append(prices, price)
		if len(prices) == 10 {
			break
		}
	}
	if len(prices) == 0 {
		return nil, errors.New("OKX 卖单没有 1～100 范围内的有效价格")
	}
	// 保留采样顺序用于追踪；中位数只对副本排序。
	sortedPrices := append([]float64{}, prices...)
	sort.Float64s(sortedPrices)
	middle := len(sortedPrices) / 2
	rate := sortedPrices[middle]
	if len(sortedPrices)%2 == 0 {
		rate, _ = decimal.NewFromFloat(sortedPrices[middle-1]).Add(decimal.NewFromFloat(rate)).Div(decimal.NewFromInt(2)).Float64()
	}
	return &PaymentExchangeRatePoint{Rate: rate, Source: PaymentExchangeRateSourceOKX, FetchedAt: &fetchedAt, ObservedAt: fetchedAt, SamplePrices: prices, SampleCount: len(prices), Aggregation: "median_first_10_sell"}, nil
}
