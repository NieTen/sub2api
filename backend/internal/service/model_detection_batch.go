package service

import (
	"context"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const ModelDetectionBatchLimit = 20

func normalizeDetectionModelIDs(input []string) ([]string, error) {
	if len(input) == 0 {
		return nil, modelDetectionInvalid("请至少选择一个检测模型")
	}
	models := make([]string, 0, min(len(input), ModelDetectionBatchLimit))
	seen := make(map[string]bool, ModelDetectionBatchLimit)
	for _, raw := range input {
		model := strings.TrimSpace(raw)
		if model == "" || len(model) > 200 || strings.ContainsAny(model, "\r\n\x00") {
			return nil, modelDetectionInvalid("请选择有效的模型名称")
		}
		if seen[model] {
			continue
		}
		if len(models) >= ModelDetectionBatchLimit {
			return nil, modelDetectionInvalid("一次最多选择 20 个不同模型")
		}
		seen[model] = true
		models = append(models, model)
	}
	return models, nil
}

// prepareDetectionBatch 先验证整批配置和账号，参数错误不会留下部分计划或付费任务。
func (s *ModelDetectionService) prepareDetectionBatch(ctx context.Context, input ModelDetectionBatchPlanInput) ([]*ModelDetectionPlan, error) {
	models, err := normalizeDetectionModelIDs(input.ModelIDs)
	if err != nil {
		return nil, err
	}
	plans := make([]*ModelDetectionPlan, 0, len(models))
	for _, model := range models {
		p := &ModelDetectionPlan{AccountID: input.AccountID, ModelID: model, Enabled: input.Enabled, ScheduleType: input.ScheduleType, IntervalMinutes: input.IntervalMinutes, DailyTime: input.DailyTime, Timezone: input.Timezone, DropThreshold: input.DropThreshold, MaxResults: input.MaxResults}
		if err = validateModelDetectionPlan(p); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	account, err := s.accounts.GetByID(ctx, input.AccountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	for _, p := range plans {
		p.AccountName = account.Name
	}
	return plans, nil
}

func detectionBatchError(item *ModelDetectionBatchItem, err error) {
	code, status := infraerrors.ToHTTP(err)
	if code >= 400 && code < 500 {
		item.Error = status.Message
		item.Reason = status.Reason
		return
	}
	// 数据库和上游内部错误可能包含连接信息，不能拼入逐模型响应。
	item.Error = "模型检测操作失败，请稍后重试"
	item.Reason = "MODEL_DETECTION_FAILED"
}

// RunAccountModels 为每个模型独立排队，忙碌模型单独返回失败，调用方可仅重试失败项。
func (s *ModelDetectionService) RunAccountModels(ctx context.Context, accountID int64, modelIDs []string) (*ModelDetectionBatchResult, error) {
	plans, err := s.prepareDetectionBatch(ctx, ModelDetectionBatchPlanInput{AccountID: accountID, ModelIDs: modelIDs})
	if err != nil {
		return nil, err
	}
	result := &ModelDetectionBatchResult{Total: len(plans), Results: make([]ModelDetectionBatchItem, 0, len(plans))}
	for _, p := range plans {
		item := ModelDetectionBatchItem{ModelID: p.ModelID}
		run, runErr := s.repo.RunAccount(ctx, p)
		if runErr != nil {
			detectionBatchError(&item, runErr)
			result.Failed++
		} else {
			item.Run = run
			result.Success++
		}
		result.Results = append(result.Results, item)
	}
	return result, nil
}

func (s *ModelDetectionService) CreatePlansBatch(ctx context.Context, input ModelDetectionBatchPlanInput) (*ModelDetectionBatchResult, error) {
	plans, err := s.prepareDetectionBatch(ctx, input)
	if err != nil {
		return nil, err
	}
	result := &ModelDetectionBatchResult{Total: len(plans), Results: make([]ModelDetectionBatchItem, 0, len(plans))}
	for _, p := range plans {
		item := ModelDetectionBatchItem{ModelID: p.ModelID}
		created, createErr := s.repo.SavePlan(ctx, p)
		if createErr != nil {
			detectionBatchError(&item, createErr)
			result.Failed++
		} else {
			item.Plan = created
			result.Success++
		}
		result.Results = append(result.Results, item)
	}
	return result, nil
}
