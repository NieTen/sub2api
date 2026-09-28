package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type detectionBatchTestRepo struct {
	ModelDetectionRepository
	plans        []*ModelDetectionPlan
	createErrors map[string]error
	runErrors    map[string]error
}

func (r *detectionBatchTestRepo) SavePlan(_ context.Context, p *ModelDetectionPlan) (*ModelDetectionPlan, error) {
	if err := r.createErrors[p.ModelID]; err != nil {
		return nil, err
	}
	copy := *p
	copy.ID = int64(len(r.plans) + 1)
	r.plans = append(r.plans, &copy)
	return &copy, nil
}
func (r *detectionBatchTestRepo) RunAccount(_ context.Context, p *ModelDetectionPlan) (*ModelDetectionRun, error) {
	if err := r.runErrors[p.ModelID]; err != nil {
		return nil, err
	}
	copy := *p
	r.plans = append(r.plans, &copy)
	return &ModelDetectionRun{ID: int64(len(r.plans)), AccountID: p.AccountID, ModelID: p.ModelID, PlanSnapshot: copy}, nil
}

func TestModelDetectionBatchRunsDeduplicateAndPreservePartialSuccess(t *testing.T) {
	repo := &detectionBatchTestRepo{runErrors: map[string]error{"busy-model": ErrModelDetectionBusy}}
	s := NewModelDetectionService(repo, &detectionTestAccounts{}, nil)
	result, err := s.RunAccountModels(context.Background(), 1, []string{" first-model ", "busy-model", "first-model", "outside-bank-model"})
	require.NoError(t, err)
	require.Equal(t, 3, result.Total)
	require.Equal(t, 2, result.Success)
	require.Equal(t, 1, result.Failed)
	require.Len(t, repo.plans, 2)
	require.Equal(t, "first-model", result.Results[0].Run.PlanSnapshot.ReferenceModel)
	require.Equal(t, "MODEL_DETECTION_BUSY", result.Results[1].Reason)
	require.Nil(t, result.Results[1].Run)
	require.Equal(t, "outside-bank-model", result.Results[2].Run.PlanSnapshot.ReferenceModel)
	// 只重试失败模型，不再提交已经成功的两个模型。
	delete(repo.runErrors, "busy-model")
	retried, err := s.RunAccountModels(context.Background(), 1, []string{result.Results[1].ModelID})
	require.NoError(t, err)
	require.Equal(t, 1, retried.Success)
	require.Len(t, repo.plans, 3)
}

func TestModelDetectionBatchPlansValidateBeforeWriting(t *testing.T) {
	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("model-%d", i)
	}
	for _, tc := range []struct {
		name     string
		models   []string
		interval int
	}{
		{"未选择", nil, 60}, {"空模型", []string{"valid", " "}, 60}, {"控制字符", []string{"valid", "invalid\nmodel"}, 60}, {"超出二十", tooMany, 60}, {"非法周期", []string{"valid", "other"}, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &detectionBatchTestRepo{}
			s := NewModelDetectionService(repo, &detectionTestAccounts{}, nil)
			result, err := s.CreatePlansBatch(context.Background(), ModelDetectionBatchPlanInput{AccountID: 1, ModelIDs: tc.models, IntervalMinutes: tc.interval})
			require.ErrorIs(t, err, ErrModelDetectionInvalid)
			require.Nil(t, result)
			require.Empty(t, repo.plans)
		})
	}
}

func TestModelDetectionBatchPlansPartialDuplicateAndErrorRedaction(t *testing.T) {
	repo := &detectionBatchTestRepo{createErrors: map[string]error{"existing": ErrModelDetectionDuplicate, "db-error": errors.New("postgres://user:secret@database")}}
	s := NewModelDetectionService(repo, &detectionTestAccounts{}, nil)
	result, err := s.CreatePlansBatch(context.Background(), ModelDetectionBatchPlanInput{AccountID: 1, ModelIDs: []string{"new", "existing", "db-error", "new"}, Enabled: true, ScheduleType: "daily", DailyTime: "12:34", Timezone: "Asia/Shanghai"})
	require.NoError(t, err)
	require.Equal(t, 3, result.Total)
	require.Equal(t, 1, result.Success)
	require.Equal(t, 2, result.Failed)
	require.Equal(t, "daily", result.Results[0].Plan.ScheduleType)
	require.Equal(t, "12:34", result.Results[0].Plan.DailyTime)
	require.Equal(t, "new", result.Results[0].Plan.ReferenceModel)
	require.Equal(t, "MODEL_DETECTION_DUPLICATE", result.Results[1].Reason)
	require.Equal(t, "MODEL_DETECTION_FAILED", result.Results[2].Reason)
	require.NotContains(t, result.Results[2].Error, "secret")
	require.NotContains(t, result.Results[2].Error, "postgres")
}

func TestModelDetectionBatchLimitAppliesAfterDeduplication(t *testing.T) {
	input := make([]string, 40)
	for i := range input {
		input[i] = fmt.Sprintf("model-%d", i%20)
	}
	repo := &detectionBatchTestRepo{}
	s := NewModelDetectionService(repo, &detectionTestAccounts{}, nil)
	result, err := s.RunAccountModels(context.Background(), 1, input)
	require.NoError(t, err)
	require.Equal(t, 20, result.Total)
	require.Equal(t, 20, result.Success)
}
