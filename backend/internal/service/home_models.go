package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// SettingKeyHomeModelCatalog 是首页 I2 模型目录在 settings 表中的键。
const SettingKeyHomeModelCatalog = "home_model_catalog"

// HomeModel 是首页 I2 展示用的模型和价格条目。
// 输入输出价格单位为美元/百万 tokens，图片价格单位为美元/张。
type HomeModel struct {
	Name             string              `json:"name"`
	Vendor           string              `json:"vendor"`
	Type             string              `json:"type"`
	Input            *float64            `json:"input,omitempty"`
	Output           *float64            `json:"output,omitempty"`
	CachedInput      *float64            `json:"cachedInput"`
	FlexInput        *float64            `json:"flexInput"`
	ResolutionPrices map[string]*float64 `json:"resolutionPrices,omitempty"`
}

// GetHomeModels 只返回管理员配置的首页模型，尚未配置时返回空目录；已配置但损坏时直接返回错误。
func (s *SettingService) GetHomeModels(ctx context.Context) ([]HomeModel, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("home model catalog setting repository is unavailable")
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyHomeModelCatalog)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return []HomeModel{}, nil
		}
		return nil, fmt.Errorf("get home model catalog: %w", err)
	}
	return parseHomeModelsJSON([]byte(raw))
}

// SaveHomeModels 校验并持久化首页模型目录。nil 表示请求中的 models 为 null，空切片表示合法清空。
func (s *SettingService) SaveHomeModels(ctx context.Context, models []HomeModel) error {
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("home model catalog setting repository is unavailable")
	}
	if err := validateHomeModels(models); err != nil {
		return err
	}
	raw, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("marshal home model catalog: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyHomeModelCatalog, string(raw)); err != nil {
		return fmt.Errorf("save home model catalog: %w", err)
	}
	return nil
}

func parseHomeModelsJSON(raw []byte) ([]HomeModel, error) {
	var models []HomeModel
	if err := json.Unmarshal(raw, &models); err != nil {
		return nil, infraerrors.BadRequest("INVALID_HOME_MODEL_CATALOG", "首页模型目录不是有效 JSON").WithCause(err)
	}
	if models == nil {
		return nil, infraerrors.BadRequest("INVALID_HOME_MODEL_CATALOG", "首页模型目录不能为 null")
	}
	if err := validateHomeModels(models); err != nil {
		return nil, err
	}
	return models, nil
}

func validateHomeModels(models []HomeModel) error {
	if models == nil {
		return infraerrors.BadRequest("INVALID_HOME_MODELS", "models 不能为 null")
	}
	if len(models) > 500 {
		return infraerrors.BadRequest("INVALID_HOME_MODELS", "模型数量不能超过 500")
	}
	seen := make(map[string]struct{}, len(models))
	for i := range models {
		model := &models[i]
		model.Name = strings.TrimSpace(model.Name)
		model.Vendor = strings.TrimSpace(model.Vendor)
		model.Type = strings.ToLower(strings.TrimSpace(model.Type))
		if model.Name == "" {
			return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("第 %d 个模型名称不能为空", i+1))
		}
		if utf8.RuneCountInString(model.Name) > 200 {
			return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("第 %d 个模型名称不能超过 200 个字符", i+1))
		}
		if model.Vendor == "" {
			return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("第 %d 个模型厂商不能为空", i+1))
		}
		if utf8.RuneCountInString(model.Vendor) > 40 {
			return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("第 %d 个模型厂商不能超过 40 个字符", i+1))
		}
		if model.Type != "text" && model.Type != "image" {
			return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("第 %d 个模型类型无效", i+1))
		}
		key := model.Name + "\x00" + model.Type
		if _, exists := seen[key]; exists {
			return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("模型名称和类型重复：%s", model.Name))
		}
		seen[key] = struct{}{}

		if model.Type == "text" {
			if model.Input == nil || model.Output == nil {
				return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("文本模型 %s 必须配置 input 和 output", model.Name))
			}
			if !validHomeModelPrice(*model.Input) || !validHomeModelPrice(*model.Output) {
				return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("文本模型 %s 的 input/output 必须是非负有限数字", model.Name))
			}
			if model.CachedInput != nil && !validHomeModelPrice(*model.CachedInput) {
				return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("文本模型 %s 的 cachedInput 必须是非负有限数字", model.Name))
			}
			if model.FlexInput != nil && !validHomeModelPrice(*model.FlexInput) {
				return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("文本模型 %s 的 flexInput 必须是非负有限数字", model.Name))
			}
			continue
		}

		if len(model.ResolutionPrices) != 3 {
			return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("图片模型 %s 必须配置 1K、2K、4K 价格", model.Name))
		}
		for _, resolution := range []string{"1K", "2K", "4K"} {
			price, ok := model.ResolutionPrices[resolution]
			if !ok || price == nil || !validHomeModelPrice(*price) {
				return infraerrors.BadRequest("INVALID_HOME_MODELS", fmt.Sprintf("图片模型 %s 的 %s 价格必须是非负有限数字", model.Name, resolution))
			}
		}
	}
	return nil
}

func validHomeModelPrice(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
