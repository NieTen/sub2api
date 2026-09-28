package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// 真实 SQL 只使用显式测试连接及随机 schema，不读取或修改业务数据库。
func detectionPostgres(t *testing.T) (*modelDetectionRepository, context.Context) {
	t.Helper()
	dsn := os.Getenv("MODEL_DETECTION_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 MODEL_DETECTION_TEST_DSN，跳过真实 PostgreSQL 检测回归")
	}
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Equal(t, "postgres", u.Scheme)
	require.NotEmpty(t, u.Host)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	schema := "detection_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		_, err := admin.ExecContext(clean, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE")
		require.NoError(t, err)
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(10)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var actual string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&actual))
	require.Equal(t, schema, actual)
	_, err = db.ExecContext(ctx, `CREATE TABLE accounts(id BIGINT PRIMARY KEY,name VARCHAR(255) NOT NULL,deleted_at TIMESTAMPTZ); INSERT INTO accounts(id,name) VALUES(1,'测试账号一'),(2,'测试账号二'),(3,'测试账号三')`)
	require.NoError(t, err)
	ddl, err := migrations.FS.ReadFile("241_model_detection.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = db.ExecContext(ctx, string(ddl))
		require.NoError(t, err, "迁移应可重复执行")
	}
	return &modelDetectionRepository{db: db}, ctx
}
func detectionPlanFixture(accountID int64, model string) *service.ModelDetectionPlan {
	return &service.ModelDetectionPlan{AccountID: accountID, ModelID: model, Enabled: true, ScheduleType: "interval", IntervalMinutes: 60, DailyTime: "00:00", Timezone: "Asia/Shanghai", DropThreshold: 20, MaxResults: 100}
}
func detectionCreate(t *testing.T, ctx context.Context, r *modelDetectionRepository, accountID int64, model string) *service.ModelDetectionPlan {
	t.Helper()
	p, err := r.SavePlan(ctx, detectionPlanFixture(accountID, model))
	require.NoError(t, err)
	return p
}
func detectionClaim(t *testing.T, ctx context.Context, r *modelDetectionRepository, planID int64) *service.ModelDetectionRun {
	t.Helper()
	_, err := r.Enqueue(ctx, planID, "manual")
	require.NoError(t, err)
	run, err := r.Claim(ctx, 7*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, run)
	return run
}
func detectionFinish(t *testing.T, ctx context.Context, r *modelDetectionRepository, run *service.ModelDetectionRun, score float64) {
	t.Helper()
	run.Status = "completed"
	run.Score = &score
	run.Progress = 4
	run.Details = []service.ModelDetectionDetail{{Kind: "quality", Prompt: "题目", Response: "回答"}}
	require.NoError(t, r.Complete(ctx, run))
}

func TestModelDetectionPostgresBaselineHistoryAndSummaries(t *testing.T) {
	r, ctx := detectionPostgres(t)
	p := detectionCreate(t, ctx, r, 1, "text-model")
	first := detectionClaim(t, ctx, r, p.ID)
	detectionFinish(t, ctx, r, first, 100)
	stored, err := r.GetRun(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, "baseline", stored.Verdict)
	require.Equal(t, 100.0, *stored.BaselineScore)
	second := detectionClaim(t, ctx, r, p.ID)
	detectionFinish(t, ctx, r, second, 65)
	stored, err = r.GetRun(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, "suspected_drop", stored.Verdict)
	require.Equal(t, 35.0, *stored.DropPoints)
	require.Len(t, stored.Details, 1)
	plan, err := r.GetPlan(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, 100.0, *plan.BaselineScore)
	require.Equal(t, first.ID, *plan.BaselineRunID)
	require.NotNil(t, plan.NextRunAt)
	third := detectionClaim(t, ctx, r, p.ID)
	third.Status = "error"
	third.ErrorMessage = "请求超时"
	require.NoError(t, r.Complete(ctx, third))
	plan, err = r.GetPlan(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, 100.0, *plan.BaselineScore)
	history, err := r.History(ctx, 1, 2, 0)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, third.ID, history[0].ID)
	older, err := r.History(ctx, 1, 2, history[1].ID)
	require.NoError(t, err)
	require.Len(t, older, 1)
	require.Equal(t, first.ID, older[0].ID)
	summary, err := r.Summaries(ctx, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, summary, 2)
	require.Equal(t, 1, summary[0].PlanCount)
	require.Equal(t, third.ID, summary[0].LatestRun.ID)
	require.Empty(t, summary[0].LatestRun.Details)
	require.Nil(t, summary[1].LatestRun)
	o, err := r.Overview(ctx)
	require.NoError(t, err)
	require.Len(t, o.Recent, 3)
	require.Equal(t, 3, o.Stats.Total)
	require.Equal(t, 1, o.Stats.SuspectedDrop)
	require.Equal(t, 1, o.Stats.Error)
	require.Equal(t, 82.5, *o.Stats.AverageScore)
}

type detectionConcurrentResult struct {
	run *service.ModelDetectionRun
	err error
}

func TestModelDetectionPostgresConcurrentManualAndPlanUniqueness(t *testing.T) {
	r, ctx := detectionPostgres(t)
	start := make(chan struct{})
	results := make(chan detectionConcurrentResult, 2)
	for range 2 {
		go func() {
			<-start
			p := detectionPlanFixture(1, "manual-model")
			p.Enabled = false
			run, err := r.RunAccount(ctx, p)
			results <- detectionConcurrentResult{run, err}
		}()
	}
	close(start)
	var winner *service.ModelDetectionRun
	success := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			success++
			winner = result.run
		} else {
			require.ErrorIs(t, result.err, service.ErrModelDetectionBusy)
		}
	}
	require.Equal(t, 1, success)
	plans, err := r.ListPlans(ctx, 1)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.False(t, plans[0].Enabled)
	_, err = r.SavePlan(ctx, detectionPlanFixture(1, "manual-model"))
	require.ErrorIs(t, err, service.ErrModelDetectionDuplicate)
	other := detectionCreate(t, ctx, r, 1, "other-model")
	other.ModelID = "manual-model"
	_, err = r.SavePlan(ctx, other)
	require.ErrorIs(t, err, service.ErrModelDetectionDuplicate)
	claimed, err := r.Claim(ctx, 7*time.Minute)
	require.NoError(t, err)
	require.Equal(t, winner.ID, claimed.ID)
	detectionFinish(t, ctx, r, claimed, 90)
	// 暂停计划仍允许再次手动运行，同时保留原来的周期参数。
	plan := plans[0]
	plan.IntervalMinutes = 17
	plan.ReferenceModel = "已保存的参考模型"
	plan, err = r.SavePlan(ctx, plan)
	require.NoError(t, err)
	request := detectionPlanFixture(1, "manual-model")
	request.Enabled = false
	request.ReferenceModel = "本轮选择的参考模型"
	manual, err := r.RunAccount(ctx, request)
	require.NoError(t, err)
	require.Equal(t, plan.ID, manual.PlanID)
	require.Equal(t, 17, manual.PlanSnapshot.IntervalMinutes)
	require.False(t, manual.PlanSnapshot.Enabled)
	require.Equal(t, "manual-model", manual.PlanSnapshot.ReferenceModel)
	saved, err := r.GetPlan(ctx, plan.ID)
	require.NoError(t, err)
	require.Equal(t, "manual-model", saved.ReferenceModel)
	require.NotNil(t, saved.BaselineScore, "仅修正参考模型不得清空已有能力基线")
	claimed, err = r.Claim(ctx, 7*time.Minute)
	require.NoError(t, err)
	detectionFinish(t, ctx, r, claimed, 85)
	request.ReferenceModel = ""
	manual, err = r.RunAccount(ctx, request)
	require.NoError(t, err)
	require.Equal(t, "manual-model", manual.PlanSnapshot.ReferenceModel, "空参考仍自动使用所选模型")
}

func TestModelDetectionPostgresGlobalConcurrencyAndExpiredFencing(t *testing.T) {
	r, ctx := detectionPostgres(t)
	for i := 1; i <= 4; i++ {
		p := detectionCreate(t, ctx, r, 1, fmt.Sprintf("model-%d", i))
		_, err := r.Enqueue(ctx, p.ID, "manual")
		require.NoError(t, err)
	}
	start := make(chan struct{})
	results := make(chan detectionConcurrentResult, 6)
	for range 6 {
		go func() {
			<-start
			run, err := r.Claim(ctx, 7*time.Minute)
			results <- detectionConcurrentResult{run, err}
		}()
	}
	close(start)
	claimed := []*service.ModelDetectionRun{}
	for range 6 {
		result := <-results
		require.NoError(t, result.err)
		if result.run != nil {
			claimed = append(claimed, result.run)
		}
	}
	require.Len(t, claimed, 2)
	require.NotEqual(t, claimed[0].ID, claimed[1].ID)
	old := claimed[0]
	_, err := r.db.ExecContext(ctx, `UPDATE model_detection_runs SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, old.ID)
	require.NoError(t, err)
	require.ErrorIs(t, r.Progress(ctx, old), service.ErrModelDetectionLease)
	require.NoError(t, r.ExpireLeases(ctx))
	old.Status = "completed"
	score := 100.0
	old.Score = &score
	require.ErrorIs(t, r.Complete(ctx, old), service.ErrModelDetectionLease)
	stored, err := r.GetRun(ctx, old.ID)
	require.NoError(t, err)
	require.Equal(t, "error", stored.Status)
	require.Nil(t, stored.Score)
	p, err := r.GetPlan(ctx, old.PlanID)
	require.NoError(t, err)
	require.Nil(t, p.BaselineScore)
	next, err := r.Claim(ctx, 7*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, next)
	require.NotEqual(t, old.ID, next.ID)
}

func TestModelDetectionPostgresBaselineResetAndPlanEditDuringRun(t *testing.T) {
	r, ctx := detectionPostgres(t)
	p := detectionCreate(t, ctx, r, 1, "model-before")
	first := detectionClaim(t, ctx, r, p.ID)
	detectionFinish(t, ctx, r, first, 90)
	run := detectionClaim(t, ctx, r, p.ID)
	_, err := r.ResetBaseline(ctx, p.ID)
	require.NoError(t, err)
	detectionFinish(t, ctx, r, run, 20)
	stored, err := r.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "inconclusive", stored.Status)
	require.Empty(t, stored.Verdict)
	p, err = r.GetPlan(ctx, p.ID)
	require.NoError(t, err)
	require.Nil(t, p.BaselineScore)
	run = detectionClaim(t, ctx, r, p.ID)
	p.ModelID = "model-after"
	p.Enabled = false
	p, err = r.SavePlan(ctx, p)
	require.NoError(t, err)
	detectionFinish(t, ctx, r, run, 100)
	stored, err = r.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "model-before", stored.ModelID)
	require.Equal(t, "inconclusive", stored.Status)
	p, err = r.GetPlan(ctx, p.ID)
	require.NoError(t, err)
	require.Nil(t, p.BaselineScore)
	require.Nil(t, p.NextRunAt)
	run = detectionClaim(t, ctx, r, p.ID)
	detectionFinish(t, ctx, r, run, 80)
	p, err = r.GetPlan(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, 80.0, *p.BaselineScore)
	require.Nil(t, p.NextRunAt)
}

func TestModelDetectionPostgresDisableAndAccountDeletion(t *testing.T) {
	r, ctx := detectionPostgres(t)
	p := detectionCreate(t, ctx, r, 1, "due-model")
	_, err := r.db.ExecContext(ctx, `UPDATE model_detection_plans SET next_run_at=NOW()-INTERVAL '1 minute' WHERE id=$1`, p.ID)
	require.NoError(t, err)
	p.Enabled = false
	_, err = r.SavePlan(ctx, p)
	require.NoError(t, err)
	require.NoError(t, r.EnqueueDue(ctx, 20))
	runs, err := r.History(ctx, 1, 10, 0)
	require.NoError(t, err)
	require.Empty(t, runs)
	p.Enabled = true
	p, err = r.SavePlan(ctx, p)
	require.NoError(t, err)
	_, err = r.db.ExecContext(ctx, `UPDATE model_detection_plans SET next_run_at=NOW()-INTERVAL '1 minute' WHERE id=$1`, p.ID)
	require.NoError(t, err)
	require.NoError(t, r.EnqueueDue(ctx, 20))
	require.NoError(t, r.EnqueueDue(ctx, 20))
	runs, err = r.History(ctx, 1, 10, 0)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	p.Enabled = false
	p, err = r.SavePlan(ctx, p)
	require.NoError(t, err)
	cancelled, err := r.GetRun(ctx, runs[0].ID)
	require.NoError(t, err)
	require.Equal(t, "error", cancelled.Status)
	claimed, err := r.Claim(ctx, 7*time.Minute)
	require.NoError(t, err)
	require.Nil(t, claimed)
	// 暂停后的手动任务仍可入队，随后删除账号时由后台安全结束。
	manual, err := r.Enqueue(ctx, p.ID, "manual")
	require.NoError(t, err)
	_, err = r.db.ExecContext(ctx, `UPDATE accounts SET deleted_at=NOW() WHERE id=$1`, 1)
	require.NoError(t, err)
	require.NoError(t, r.ExpireLeases(ctx))
	stored, err := r.GetRun(ctx, manual.ID)
	require.NoError(t, err)
	require.Equal(t, "error", stored.Status)
}

func TestModelDetectionPostgresRecentOrderAndRetention(t *testing.T) {
	r, ctx := detectionPostgres(t)
	p := detectionCreate(t, ctx, r, 1, "history-model")
	first := detectionClaim(t, ctx, r, p.ID)
	detectionFinish(t, ctx, r, first, 100)
	// 保留至少 100 条历史，并额外保留作为基线的早期证据。
	_, err := r.db.ExecContext(ctx, `INSERT INTO model_detection_runs(plan_id,account_id,account_name,model_id,status,suite_version,trigger,plan_snapshot,finished_at) SELECT plan_id,account_id,account_name,model_id,'error',suite_version,'manual',plan_snapshot,NOW()-INTERVAL '1 day' FROM model_detection_runs CROSS JOIN generate_series(1,110) WHERE id=$1`, first.ID)
	require.NoError(t, err)
	last := detectionClaim(t, ctx, r, p.ID)
	detectionFinish(t, ctx, r, last, 95)
	var count int
	require.NoError(t, r.db.QueryRowContext(ctx, `SELECT COUNT(id) FROM model_detection_runs WHERE plan_id=$1`, p.ID).Scan(&count))
	require.Equal(t, 101, count)
	_, err = r.GetRun(ctx, first.ID)
	require.NoError(t, err)
	_, err = r.db.ExecContext(ctx, `UPDATE model_detection_runs SET finished_at=NOW()+INTERVAL '1 second' WHERE id=$1`, first.ID)
	require.NoError(t, err)
	o, err := r.Overview(ctx)
	require.NoError(t, err)
	require.Len(t, o.Recent, 10)
	require.Equal(t, first.ID, o.Recent[0].ID)
	require.Empty(t, o.Recent[0].Details)
	_, err = r.GetRun(ctx, 999999)
	require.True(t, errors.Is(err, service.ErrModelDetectionNotFound))
}

func TestModelDetectionPostgresRejectsInvalidCompletion(t *testing.T) {
	r, ctx := detectionPostgres(t)
	p := detectionCreate(t, ctx, r, 1, "validation-model")
	run := detectionClaim(t, ctx, r, p.ID)
	run.Status = "completed"
	invalid := math.NaN()
	run.Score = &invalid
	require.ErrorIs(t, r.Complete(ctx, run), service.ErrModelDetectionInvalid)
	run.Status = "running"
	require.ErrorIs(t, r.Complete(ctx, run), service.ErrModelDetectionInvalid)
	run.Status = "completed"
	score := 99.0
	run.Score = &score
	run.Verdict = "baseline"
	run.SuiteVersion = "old-suite"
	require.ErrorIs(t, r.Complete(ctx, run), service.ErrModelDetectionInvalid)
	run.SuiteVersion = service.ModelDetectionSuiteVersion
	_, err := r.ResetBaseline(ctx, p.ID)
	require.NoError(t, err)
	run.Verdict = "baseline"
	require.NoError(t, r.Complete(ctx, run))
	stored, err := r.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "inconclusive", stored.Status)
	require.Empty(t, stored.Verdict)
	require.Nil(t, stored.Score)
	saved, err := r.GetPlan(ctx, p.ID)
	require.NoError(t, err)
	require.Nil(t, saved.BaselineScore)
}
