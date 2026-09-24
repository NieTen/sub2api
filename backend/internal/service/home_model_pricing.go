package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// HomeModelPriceQuery 只查询管理员已经选择的模型，不会导入系统目录中的其他模型。
type HomeModelPriceQuery struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// HomeModelSystemPrice 使用首页展示单位：美元/百万 tokens 或美元/张。
// 未找到可确定的价格时不返回价格字段，避免覆盖管理员手工填写的价格。
type HomeModelSystemPrice struct {
	Name             string             `json:"name"`
	Type             string             `json:"type"`
	Found            bool               `json:"found"`
	Input            *float64           `json:"input,omitempty"`
	Output           *float64           `json:"output,omitempty"`
	CachedInput      *float64           `json:"cachedInput"`
	FlexInput        *float64           `json:"flexInput"`
	ResolutionPrices map[string]float64 `json:"resolutionPrices,omitempty"`
	Reason           string             `json:"reason,omitempty"`
}

// GetHomeModelSystemPrices 按请求顺序读取当前系统标准价格，不修改首页配置、系统计费或模型列表。
func (s *BillingService) GetHomeModelSystemPrices(models []HomeModelPriceQuery) ([]HomeModelSystemPrice, error) {
	if models == nil {
		return nil, infraerrors.BadRequest("INVALID_HOME_MODEL_PRICE_QUERY", "models 不能缺失或为 null")
	}
	if len(models) > 500 {
		return nil, infraerrors.BadRequest("INVALID_HOME_MODEL_PRICE_QUERY", "模型数量不能超过 500")
	}
	for i, model := range models {
		name := strings.TrimSpace(model.Name)
		if name == "" || utf8.RuneCountInString(name) > 200 {
			return nil, infraerrors.BadRequest("INVALID_HOME_MODEL_PRICE_QUERY", fmt.Sprintf("第 %d 个模型名称必须为 1 至 200 个字符", i+1))
		}
		if model.Type != "text" && model.Type != "image" {
			return nil, infraerrors.BadRequest("INVALID_HOME_MODEL_PRICE_QUERY", fmt.Sprintf("第 %d 个模型类型无效", i+1))
		}
	}
	if s == nil {
		return nil, fmt.Errorf("首页模型系统价格服务不可用")
	}
	results := make([]HomeModelSystemPrice, 0, len(models))
	for _, model := range models {
		result := HomeModelSystemPrice{Name: strings.TrimSpace(model.Name), Type: model.Type}
		if model.Type == "image" {
			s.populateHomeImagePrice(&result)
		} else {
			s.populateHomeTextPrice(&result)
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *BillingService) populateHomeTextPrice(result *HomeModelSystemPrice) {
	if !s.HasIdentifiedTokenPricing(result.Name) {
		result.Reason = "系统未配置此模型的文本价格，请手动填写"
		return
	}
	pricing, err := s.GetModelPricing(result.Name)
	if err != nil {
		result.Reason = "系统暂时无法读取此模型价格，请保留手动价格"
		return
	}
	input := pricing.InputPricePerToken * 1e6
	output := pricing.OutputPricePerToken * 1e6
	cached := pricing.CacheReadPricePerToken * 1e6
	if !validHomeModelPrice(input) || !validHomeModelPrice(output) || !validHomeModelPrice(cached) {
		result.Reason = "系统价格不是有效的非负数字，请手动填写"
		return
	}
	identified := s.pricingService.GetIdentifiedModelPricing(result.Name)
	var flex *float64
	if identified != nil && identified.InputCostPerTokenFlex != nil {
		value := *identified.InputCostPerTokenFlex * 1e6
		if !validHomeModelPrice(value) {
			result.Reason = "系统 Flex 价格不是有效的非负数字，请手动填写"
			return
		}
		flex = &value
	}
	result.Found = true
	result.Input = &input
	result.Output = &output
	// 缓存读取与缓存写入是不同价格；缺少读取价时保持 null，显式免费价保留为 0。
	if cached > 0 || (identified != nil && !identified.TokenPricingAbsent && identified.CacheReadInputTokenCostExplicit) {
		result.CachedInput = &cached
	}
	result.FlexInput = flex
}

func (s *BillingService) populateHomeImagePrice(result *HomeModelSystemPrice) {
	_, hasGrokPrice := getDefaultGrokImagineImagePrice(result.Name, ImageBillingSize1K)
	identified := s.pricingService.GetIdentifiedModelPricing(result.Name)
	// 图片 token 单价不能换算为固定每张价，也不能使用未知模型的通用兜底价。
	if !hasGrokPrice && (identified == nil || identified.OutputCostPerImage <= 0) {
		result.Reason = "系统未配置此模型的固定分辨率图片价格，请手动填写"
		return
	}
	prices := make(map[string]float64, 3)
	for _, size := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
		price := s.getDefaultImagePrice(result.Name, size)
		if !validHomeModelPrice(price) {
			result.Reason = "系统图片价格不是有效的非负数字，请手动填写"
			return
		}
		prices[size] = price
	}
	result.Found = true
	result.ResolutionPrices = prices
}
