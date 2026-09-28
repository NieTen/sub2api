package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/modelquality"
	"github.com/Wei-Shaw/sub2api/internal/pkg/modeltrace"
)

type ModelDetectionService struct {
	repo             ModelDetectionRepository
	accounts         AccountRepository
	probe            ModelDetectionProbe
	qualityChallenge func() modelquality.Challenge
	mu               sync.Mutex
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

func NewModelDetectionService(repo ModelDetectionRepository, accounts AccountRepository, probe ModelDetectionProbe) *ModelDetectionService {
	return &ModelDetectionService{repo: repo, accounts: accounts, probe: probe, qualityChallenge: modelquality.Generate}
}

func modelDetectionInvalid(message string) error {
	err := ErrModelDetectionInvalid.WithMetadata(nil)
	err.Message = message
	return err
}

// ModelDetectionNextRun 按完成时间重新安排；每日任务采用计划时区，避免服务器时区影响。
func ModelDetectionNextRun(plan *ModelDetectionPlan, from time.Time) (*time.Time, error) {
	if !plan.Enabled {
		return nil, nil
	}
	if plan.ScheduleType == "interval" {
		next := from.Add(time.Duration(plan.IntervalMinutes) * time.Minute)
		return &next, nil
	}
	loc, err := time.LoadLocation(plan.Timezone)
	if err != nil {
		return nil, err
	}
	clock, err := time.Parse("15:04", plan.DailyTime)
	if err != nil {
		return nil, err
	}
	local := from.In(loc)
	// DST 跳时按该时区的实际时间安排，下次必须严格晚于当前时刻。
	for offset := 0; offset < 3; offset++ {
		day := local.AddDate(0, 0, offset)
		next := time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
		if next.After(from) {
			return &next, nil
		}
	}
	return nil, fmt.Errorf("无法计算每日检测时间")
}

func (s *ModelDetectionService) validatePlan(ctx context.Context, p *ModelDetectionPlan) error {
	p.ModelID = strings.TrimSpace(p.ModelID)
	p.ReferenceModel = strings.TrimSpace(p.ReferenceModel)
	if p.ReferenceModel == "" {
		for _, m := range modeltrace.Models() {
			if m.ID == p.ModelID {
				p.ReferenceModel = m.ID
				break
			}
		}
	}
	if p.AccountID <= 0 || p.ModelID == "" || len(p.ModelID) > 200 || strings.ContainsAny(p.ModelID, "\r\n\x00") {
		return modelDetectionInvalid("请选择账号并填写有效的模型名称")
	}
	if p.ScheduleType == "" {
		p.ScheduleType = "interval"
	}
	if p.ScheduleType != "interval" && p.ScheduleType != "daily" {
		return modelDetectionInvalid("检测周期仅支持每隔指定分钟或每天")
	}
	if p.IntervalMinutes == 0 {
		p.IntervalMinutes = 1440
	}
	if p.IntervalMinutes < 1 || p.IntervalMinutes > 10080 {
		return modelDetectionInvalid("检测间隔必须为 1～10080 分钟")
	}
	if p.DailyTime == "" {
		p.DailyTime = "00:00"
	}
	if _, err := time.Parse("15:04", p.DailyTime); err != nil || len(p.DailyTime) != 5 {
		return modelDetectionInvalid("每日检测时间必须为 HH:mm")
	}
	if p.Timezone == "" {
		p.Timezone = "Asia/Shanghai"
	}
	if len(p.Timezone) > 100 {
		return modelDetectionInvalid("请选择有效时区")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return modelDetectionInvalid("请选择有效的 IANA 时区")
	}
	if p.DropThreshold == 0 {
		p.DropThreshold = 20
	}
	if math.IsNaN(p.DropThreshold) || math.IsInf(p.DropThreshold, 0) || p.DropThreshold <= 0 || p.DropThreshold > 100 {
		return modelDetectionInvalid("能力下降阈值必须大于 0 且不超过 100 分")
	}
	if p.MaxResults == 0 {
		p.MaxResults = 100
	}
	if p.MaxResults < 100 || p.MaxResults > 1000 {
		return modelDetectionInvalid("每个计划须保留 100～1000 条历史记录")
	}
	if p.ReferenceModel != "" {
		found := false
		for _, m := range modeltrace.Models() {
			if m.ID == p.ReferenceModel {
				found = true
				break
			}
		}
		if !found {
			return modelDetectionInvalid("所选参考模型不在本地指纹库中")
		}
	}
	account, err := s.accounts.GetByID(ctx, p.AccountID)
	if err != nil {
		return err
	}
	if account == nil {
		return ErrAccountNotFound
	}
	p.AccountName = account.Name
	return nil
}

func (s *ModelDetectionService) SavePlan(ctx context.Context, p *ModelDetectionPlan) (*ModelDetectionPlan, error) {
	if err := s.validatePlan(ctx, p); err != nil {
		return nil, err
	}
	return s.repo.SavePlan(ctx, p)
}
func (s *ModelDetectionService) ListPlans(ctx context.Context, accountID int64) ([]*ModelDetectionPlan, error) {
	return s.repo.ListPlans(ctx, accountID)
}
func (s *ModelDetectionService) ResetBaseline(ctx context.Context, id int64) (*ModelDetectionPlan, error) {
	return s.repo.ResetBaseline(ctx, id)
}
func (s *ModelDetectionService) RunPlan(ctx context.Context, id int64) (*ModelDetectionRun, error) {
	return s.repo.Enqueue(ctx, id, "manual")
}
func (s *ModelDetectionService) RunAccount(ctx context.Context, accountID int64, modelID, referenceModel string) (*ModelDetectionRun, error) {
	p := &ModelDetectionPlan{AccountID: accountID, ModelID: modelID, ReferenceModel: referenceModel}
	if err := s.validatePlan(ctx, p); err != nil {
		return nil, err
	}
	return s.repo.RunAccount(ctx, p)
}
func (s *ModelDetectionService) Run(ctx context.Context, id int64) (*ModelDetectionRun, error) {
	return s.repo.GetRun(ctx, id)
}
func (s *ModelDetectionService) History(ctx context.Context, accountID int64, limit int, before int64) (*ModelDetectionHistory, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	items, err := s.repo.History(ctx, accountID, limit+1, before)
	if err != nil {
		return nil, err
	}
	result := &ModelDetectionHistory{Items: items}
	if len(items) > limit {
		result.Items = items[:limit]
		id := result.Items[limit-1].ID
		result.NextBeforeID = &id
	}
	return result, nil
}
func (s *ModelDetectionService) Summaries(ctx context.Context, ids []int64) ([]ModelDetectionSummary, error) {
	if len(ids) > 200 {
		return nil, modelDetectionInvalid("一次最多查询 200 个账号")
	}
	return s.repo.Summaries(ctx, ids)
}
func (s *ModelDetectionService) Overview(ctx context.Context) (*ModelDetectionOverview, error) {
	return s.repo.Overview(ctx)
}
