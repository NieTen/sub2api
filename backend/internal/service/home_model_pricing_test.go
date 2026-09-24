package service

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newHomeModelPricingTestService(t *testing.T) *BillingService {
	t.Helper()
	catalog := NewPricingService(nil, nil)
	var err error
	catalog.pricingData, err = catalog.parsePricingData([]byte(`{
		"custom-text": {"input_cost_per_token":0.000002,"output_cost_per_token":0.000008,"cache_read_input_token_cost":0.00000025,"cache_creation_input_token_cost":0.000009,"input_cost_per_token_flex":0.000001},
		"free-text": {"input_cost_per_token":0,"output_cost_per_token":0,"cache_read_input_token_cost":0,"input_cost_per_token_flex":0},
		"plain-text": {"input_cost_per_token":0.000001,"output_cost_per_token":0.000002},
		"fixed-image": {"output_cost_per_image":0.02},
		"token-image": {"output_cost_per_image_token":0.000032,"input_cost_per_token":0.000005},
		"zero-image": {"output_cost_per_image":0}
	}`))
	require.NoError(t, err)
	return NewBillingService(nil, catalog)
}

func TestHomeModelSystemPrices_TextUnitsAndOptionalPrices(t *testing.T) {
	svc := newHomeModelPricingTestService(t)
	queries := []HomeModelPriceQuery{
		{Name: "custom-text", Type: "text"},
		{Name: "free-text", Type: "text"},
		{Name: "plain-text", Type: "text"},
		{Name: "claude-sonnet-4", Type: "text"},
	}
	prices, err := svc.GetHomeModelSystemPrices(queries)
	require.NoError(t, err)
	require.Len(t, prices, len(queries))
	for i := range prices {
		require.Equal(t, queries[i].Name, prices[i].Name)
		require.True(t, prices[i].Found)
	}
	require.Equal(t, 2.0, *prices[0].Input)
	require.Equal(t, 8.0, *prices[0].Output)
	require.Equal(t, 0.25, *prices[0].CachedInput)
	require.Equal(t, 1.0, *prices[0].FlexInput)
	for _, value := range []*float64{prices[1].Input, prices[1].Output, prices[1].CachedInput, prices[1].FlexInput} {
		require.NotNil(t, value)
		require.Zero(t, *value)
	}
	require.Nil(t, prices[2].CachedInput)
	require.Nil(t, prices[2].FlexInput)
	require.InDelta(t, 0.3, *prices[3].CachedInput, 1e-12)
	require.Nil(t, prices[3].FlexInput, "缓存写入价不得误作 Flex 输入价")
	// 同步只读取数据，不把百万 tokens 展示值写回系统每 token 价格。
	require.Equal(t, 0.000002, svc.pricingService.pricingData["custom-text"].InputCostPerToken)
}

func TestHomeModelSystemPrices_DoesNotGuessUnknownPrices(t *testing.T) {
	svc := newHomeModelPricingTestService(t)
	queries := []HomeModelPriceQuery{
		{Name: "invented-claude-haiku-99", Type: "text"},
		{Name: "gpt-99-nonexistent", Type: "text"},
		{Name: "fixed-image", Type: "text"},
		{Name: "unknown-image", Type: "image"},
		{Name: "token-image", Type: "image"},
		{Name: "gpt-image-2", Type: "image"},
		{Name: "zero-image", Type: "image"},
	}
	prices, err := svc.GetHomeModelSystemPrices(queries)
	require.NoError(t, err)
	for i, price := range prices {
		require.Equal(t, queries[i].Name, price.Name)
		require.False(t, price.Found, price.Name)
		require.NotEmpty(t, price.Reason, price.Name)
		require.Nil(t, price.Input)
		require.Nil(t, price.Output)
		require.Nil(t, price.CachedInput)
		require.Nil(t, price.FlexInput)
		require.Nil(t, price.ResolutionPrices)
	}
}

func TestHomeModelSystemPrices_ImagePricesMatchSystemRules(t *testing.T) {
	svc := newHomeModelPricingTestService(t)
	prices, err := svc.GetHomeModelSystemPrices([]HomeModelPriceQuery{
		{Name: "fixed-image", Type: "image"},
		{Name: "grok-imagine-image-2.0", Type: "image"},
	})
	require.NoError(t, err)
	require.True(t, prices[0].Found)
	require.Equal(t, map[string]float64{"1K": 0.02, "2K": 0.03, "4K": 0.04}, prices[0].ResolutionPrices)
	require.True(t, prices[1].Found)
	require.Equal(t, map[string]float64{
		"1K": defaultGrokImagineImage20Price1K,
		"2K": defaultGrokImagineImage20Price2K,
		"4K": defaultGrokImagineImage20Price2K,
	}, prices[1].ResolutionPrices)
}

func TestHomeModelSystemPrices_ValidationAndEmptyResult(t *testing.T) {
	svc := newHomeModelPricingTestService(t)
	prices, err := svc.GetHomeModelSystemPrices([]HomeModelPriceQuery{})
	require.NoError(t, err)
	require.NotNil(t, prices)
	require.Empty(t, prices)
	for _, invalid := range [][]HomeModelPriceQuery{
		nil,
		make([]HomeModelPriceQuery, 501),
		{{Name: " ", Type: "text"}},
		{{Name: strings.Repeat("模", 201), Type: "text"}},
		{{Name: "custom-text", Type: "video"}},
	} {
		_, err := svc.GetHomeModelSystemPrices(invalid)
		require.Error(t, err)
	}
	maxQueries := make([]HomeModelPriceQuery, 500)
	for i := range maxQueries {
		maxQueries[i] = HomeModelPriceQuery{Name: "plain-text", Type: "text"}
	}
	prices, err = svc.GetHomeModelSystemPrices(maxQueries)
	require.NoError(t, err)
	require.Len(t, prices, 500)
}

func TestHomeModelSystemPrices_InvalidCatalogValueDoesNotOverwrite(t *testing.T) {
	svc := newHomeModelPricingTestService(t)
	svc.pricingService.pricingData["custom-text"].InputCostPerToken = math.Inf(1)
	prices, err := svc.GetHomeModelSystemPrices([]HomeModelPriceQuery{{Name: "custom-text", Type: "text"}})
	require.NoError(t, err)
	require.False(t, prices[0].Found)
	require.Nil(t, prices[0].Input)
}

func TestHomeModelSystemPrices_DecimalConversionProducesCleanJSON(t *testing.T) {
	// 从 JSON 读取运行时 float64，避免编译器把常量乘法直接折叠为准确的十进制预期值。
	catalog := NewPricingService(nil, nil)
	var err error
	catalog.pricingData, err = catalog.parsePricingData([]byte(`{
		"fractional-text": {"input_cost_per_token":0.0000002,"output_cost_per_token":0.0000001,"cache_read_input_token_cost":0.00000005,"input_cost_per_token_flex":0.00000005},
		"fractional-image": {"output_cost_per_image":0.1},
		"tiny-text": {"input_cost_per_token":0.0000000000000123456789,"output_cost_per_token":0.0000000000000000123456789,"cache_read_input_token_cost":0.00000000000000000001,"input_cost_per_token_flex":0.00000000000000000123},
		"tiny-image": {"output_cost_per_image":0.0000000000123456789}
	}`))
	require.NoError(t, err)
	svc := NewBillingService(nil, catalog)
	prices, err := svc.GetHomeModelSystemPrices([]HomeModelPriceQuery{
		{Name: "fractional-text", Type: "text"},
		{Name: "fractional-image", Type: "image"},
		{Name: "tiny-text", Type: "text"},
		{Name: "tiny-image", Type: "image"},
	})
	require.NoError(t, err)
	for _, price := range prices {
		require.True(t, price.Found, price.Name)
	}
	encoded, err := json.Marshal(prices)
	require.NoError(t, err)
	require.JSONEq(t, `[
		{"name":"fractional-text","type":"text","found":true,"input":0.2,"output":0.1,"cachedInput":0.05,"flexInput":0.05},
		{"name":"fractional-image","type":"image","found":true,"cachedInput":null,"flexInput":null,"resolutionPrices":{"1K":0.1,"2K":0.15,"4K":0.2}},
		{"name":"tiny-text","type":"text","found":true,"input":0.0000000123456789,"output":0.0000000000123456789,"cachedInput":0.00000000000001,"flexInput":0.00000000000123},
		{"name":"tiny-image","type":"image","found":true,"cachedInput":null,"flexInput":null,"resolutionPrices":{"1K":0.0000000000123456789,"2K":0.00000000001851851835,"4K":0.0000000000246913578}}
	]`, string(encoded))
	require.Contains(t, string(encoded), `"input":0.2,"output":0.1,"cachedInput":0.05,"flexInput":0.05`)
	require.Contains(t, string(encoded), `"resolutionPrices":{"1K":0.1,"2K":0.15,"4K":0.2}`)
	// 展示换算不修改目录中的每 token 价或每张基础价。
	require.Equal(t, 0.0000002, catalog.pricingData["fractional-text"].InputCostPerToken)
	require.Equal(t, 0.1, catalog.pricingData["fractional-image"].OutputCostPerImage)
}
