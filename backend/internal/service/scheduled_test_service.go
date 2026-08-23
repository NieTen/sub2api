package service

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

var scheduledTestCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

const BatchScheduledTestPlanCreateConcurrency = 15

// ScheduledTestService provides CRUD operations for scheduled test plans and results.
type ScheduledTestService struct {
	planRepo   ScheduledTestPlanRepository
	resultRepo ScheduledTestResultRepository
}

// BatchCreateScheduledTestPlansInput 是批量新建定时测试计划的入参。
type BatchCreateScheduledTestPlansInput struct {
	AccountIDs     []int64
	ModelID        string
	CronExpression string
	Enabled        bool
	MaxResults     int
	AutoRecover    bool
	MaxConcurrency int
}

// BatchCreateScheduledTestPlanItem 表示单个账号的计划创建结果。
type BatchCreateScheduledTestPlanItem struct {
	AccountID    int64              `json:"account_id"`
	Success      bool               `json:"success"`
	Plan         *ScheduledTestPlan `json:"plan,omitempty"`
	ErrorMessage string             `json:"error_message,omitempty"`
}

// BatchCreateScheduledTestPlansResult 汇总批量新建计划结果。
type BatchCreateScheduledTestPlansResult struct {
	Total   int                                `json:"total"`
	Success int                                `json:"success"`
	Failed  int                                `json:"failed"`
	Results []BatchCreateScheduledTestPlanItem `json:"results"`
}

// NewScheduledTestService creates a new ScheduledTestService.
func NewScheduledTestService(
	planRepo ScheduledTestPlanRepository,
	resultRepo ScheduledTestResultRepository,
) *ScheduledTestService {
	return &ScheduledTestService{
		planRepo:   planRepo,
		resultRepo: resultRepo,
	}
}

// CreatePlan validates the cron expression, computes next_run_at, and persists the plan.
func (s *ScheduledTestService) CreatePlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun

	if plan.MaxResults <= 0 {
		plan.MaxResults = 50
	}

	return s.planRepo.Create(ctx, plan)
}

// CreatePlansBatch 为每个账号新建同一配置的定时测试计划，不覆盖任何已有计划。
func (s *ScheduledTestService) CreatePlansBatch(ctx context.Context, input BatchCreateScheduledTestPlansInput) (*BatchCreateScheduledTestPlansResult, error) {
	if s == nil || s.planRepo == nil {
		return nil, fmt.Errorf("scheduled test service is not configured")
	}
	if _, err := computeNextRun(input.CronExpression, time.Now()); err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	result := &BatchCreateScheduledTestPlansResult{
		Total:   len(input.AccountIDs),
		Results: make([]BatchCreateScheduledTestPlanItem, len(input.AccountIDs)),
	}
	if len(input.AccountIDs) == 0 {
		return result, nil
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(BatchScheduledTestPlanCreateConcurrency)

	for index, accountID := range input.AccountIDs {
		idx := index
		id := accountID
		result.Results[idx] = BatchCreateScheduledTestPlanItem{AccountID: id}

		g.Go(func() error {
			if id <= 0 {
				result.Results[idx].ErrorMessage = "invalid account id"
				return nil
			}

			created, err := s.CreatePlan(gctx, &ScheduledTestPlan{
				AccountID:      id,
				ModelID:        input.ModelID,
				CronExpression: input.CronExpression,
				Enabled:        input.Enabled,
				MaxResults:     input.MaxResults,
				AutoRecover:    input.AutoRecover,
			})
			if err != nil {
				result.Results[idx].ErrorMessage = err.Error()
				return nil
			}

			result.Results[idx].Success = true
			result.Results[idx].Plan = created
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	for _, item := range result.Results {
		if item.Success {
			result.Success++
		} else {
			result.Failed++
		}
	}
	return result, nil
}

// GetPlan retrieves a plan by ID.
func (s *ScheduledTestService) GetPlan(ctx context.Context, id int64) (*ScheduledTestPlan, error) {
	return s.planRepo.GetByID(ctx, id)
}

// ListPlansByAccount returns all plans for a given account.
func (s *ScheduledTestService) ListPlansByAccount(ctx context.Context, accountID int64) ([]*ScheduledTestPlan, error) {
	return s.planRepo.ListByAccountID(ctx, accountID)
}

// UpdatePlan validates cron and updates the plan.
func (s *ScheduledTestService) UpdatePlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun

	return s.planRepo.Update(ctx, plan)
}

// DeletePlan removes a plan and its results (via CASCADE).
func (s *ScheduledTestService) DeletePlan(ctx context.Context, id int64) error {
	return s.planRepo.Delete(ctx, id)
}

// ListResults returns the most recent results for a plan.
func (s *ScheduledTestService) ListResults(ctx context.Context, planID int64, limit int) ([]*ScheduledTestResult, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.resultRepo.ListByPlanID(ctx, planID, limit)
}

// SaveResult inserts a result and prunes old entries beyond maxResults.
func (s *ScheduledTestService) SaveResult(ctx context.Context, planID int64, maxResults int, result *ScheduledTestResult) error {
	result.PlanID = planID
	if _, err := s.resultRepo.Create(ctx, result); err != nil {
		return err
	}
	return s.resultRepo.PruneOldResults(ctx, planID, maxResults)
}

func computeNextRun(cronExpr string, from time.Time) (time.Time, error) {
	sched, err := scheduledTestCronParser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from), nil
}
