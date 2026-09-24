package service

import (
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
