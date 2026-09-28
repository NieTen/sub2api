package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type detectionBatchPostgresAccounts struct{ service.AccountRepository }

func (*detectionBatchPostgresAccounts) GetByID(_ context.Context, id int64) (*service.Account, error) {
	return &service.Account{ID: id, Name: "测试账号"}, nil
}

func TestModelDetectionPostgresBatchPartialSuccessAndRetry(t *testing.T) {
	repo, ctx := detectionPostgres(t)
	s := service.NewModelDetectionService(repo, &detectionBatchPostgresAccounts{}, nil)
	p := detectionCreate(t, ctx, repo, 1, "busy-model")
	_, err := repo.Enqueue(ctx, p.ID, "manual")
	require.NoError(t, err)
	result, err := s.RunAccountModels(ctx, 1, []string{"busy-model", "new-model", " new-model ", "outside-bank-model"})
	require.NoError(t, err)
	require.Equal(t, 3, result.Total)
	require.Equal(t, 2, result.Success)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, "MODEL_DETECTION_BUSY", result.Results[0].Reason)
	plans, err := repo.ListPlans(ctx, 1)
	require.NoError(t, err)
	require.Len(t, plans, 3)
	for _, item := range result.Results[1:] {
		require.Equal(t, item.ModelID, item.Run.PlanSnapshot.ReferenceModel)
		require.Equal(t, item.ModelID, item.Run.Fingerprint.ReferenceModel)
	}
	run, err := repo.Claim(ctx, 42*time.Minute)
	require.NoError(t, err)
	require.Equal(t, p.ID, run.PlanID)
	detectionFinish(t, ctx, repo, run, 90)
	retry, err := s.RunAccountModels(ctx, 1, []string{result.Results[0].ModelID})
	require.NoError(t, err)
	require.Equal(t, 1, retry.Success)
	require.Equal(t, p.ID, retry.Results[0].Run.PlanID)
	plans, err = repo.ListPlans(ctx, 1)
	require.NoError(t, err)
	require.Len(t, plans, 3, "重试不得重复创建计划")
	created, err := s.CreatePlansBatch(ctx, service.ModelDetectionBatchPlanInput{AccountID: 1, ModelIDs: []string{"new-model", "another-model"}, Enabled: true, ScheduleType: "daily", DailyTime: "09:35", Timezone: "Asia/Shanghai"})
	require.NoError(t, err)
	require.Equal(t, 1, created.Success)
	require.Equal(t, 1, created.Failed)
	require.Equal(t, "MODEL_DETECTION_DUPLICATE", created.Results[0].Reason)
	require.Equal(t, "09:35", created.Results[1].Plan.DailyTime)
}

func TestModelDetectionPostgresAutoReferencePreservesLegacyBaseline(t *testing.T) {
	repo, ctx := detectionPostgres(t)
	p := detectionCreate(t, ctx, repo, 1, "selected-model")
	first := detectionClaim(t, ctx, repo, p.ID)
	detectionFinish(t, ctx, repo, first, 95)
	// 模拟旧版本留下的手工参考配置，升级后只修正新任务，不清空历史基线。
	_, err := repo.db.ExecContext(ctx, `UPDATE model_detection_plans SET reference_model='legacy-manual-reference' WHERE id=$1`, p.ID)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, `UPDATE model_detection_runs SET plan_snapshot=jsonb_set(plan_snapshot,'{reference_model}','"legacy-finished-reference"'::jsonb) WHERE id=$1`, first.ID)
	require.NoError(t, err)
	old, err := repo.GetPlan(ctx, p.ID)
	require.NoError(t, err)
	generation := old.BaselineGeneration
	require.Equal(t, "selected-model", old.ReferenceModel)
	old.ReferenceModel = "attempted-override"
	saved, err := repo.SavePlan(ctx, old)
	require.NoError(t, err)
	require.Equal(t, generation, saved.BaselineGeneration)
	require.Equal(t, 95.0, *saved.BaselineScore)
	queued, err := repo.Enqueue(ctx, p.ID, "scheduled")
	require.NoError(t, err)
	require.Equal(t, "selected-model", queued.PlanSnapshot.ReferenceModel)
	// 模拟升级前已经排队的任务；认领时事务修正快照，已结束历史保持原样。
	_, err = repo.db.ExecContext(ctx, `UPDATE model_detection_runs SET plan_snapshot=jsonb_set(plan_snapshot,'{reference_model}','"legacy-queued-reference"'::jsonb),fingerprint=jsonb_set(fingerprint,'{reference_model}','"legacy-queued-reference"'::jsonb) WHERE id=$1`, queued.ID)
	require.NoError(t, err)
	run, err := repo.Claim(ctx, 42*time.Minute)
	require.NoError(t, err)
	require.Equal(t, "selected-model", run.PlanSnapshot.ReferenceModel)
	require.Equal(t, "selected-model", run.Fingerprint.ReferenceModel)
	detectionFinish(t, ctx, repo, run, 70)
	stored, err := repo.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "suspected_drop", stored.Verdict)
	require.Equal(t, 95.0, *stored.BaselineScore)
	baseline, err := repo.GetRun(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, "baseline", baseline.Verdict)
	require.Equal(t, "legacy-finished-reference", baseline.PlanSnapshot.ReferenceModel, "已经结束的历史快照不得重写")
}
