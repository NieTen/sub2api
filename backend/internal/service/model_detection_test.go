package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/modelquality"
	"github.com/Wei-Shaw/sub2api/internal/pkg/modeltrace"
	"github.com/stretchr/testify/require"
)

type detectionTestAccounts struct{ AccountRepository }

func (*detectionTestAccounts) GetByID(context.Context, int64) (*Account, error) {
	return &Account{ID: 1, Name: "测试账号"}, nil
}

type detectionTestRepo struct {
	ModelDetectionRepository
	mu          sync.Mutex
	complete    *ModelDetectionRun
	progressErr error
	claimed     bool
	queue       []*ModelDetectionRun
}

func (r *detectionTestRepo) Complete(_ context.Context, run *ModelDetectionRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *run
	r.complete = &copy
	return nil
}
func (r *detectionTestRepo) Progress(context.Context, *ModelDetectionRun) error { return r.progressErr }
func (r *detectionTestRepo) Claim(context.Context, time.Duration) (*ModelDetectionRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimed || len(r.queue) == 0 {
		return nil, nil
	}
	r.claimed = true
	return r.queue[0], nil
}
func (*detectionTestRepo) ExpireLeases(context.Context) error    { return nil }
func (*detectionTestRepo) EnqueueDue(context.Context, int) error { return nil }

type detectionTestProbe struct {
	call  func(context.Context, int, string) (string, error)
	calls int
}

func (p *detectionTestProbe) RunModelDetectionPrompt(ctx context.Context, _ int64, _ string, prompt string) (string, error) {
	p.calls++
	return p.call(ctx, p.calls, prompt)
}

func TestModelDetectionPlanValidation(t *testing.T) {
	s := NewModelDetectionService(nil, &detectionTestAccounts{}, nil)
	valid := func() *ModelDetectionPlan {
		return &ModelDetectionPlan{AccountID: 1, ModelID: "gpt-test", ScheduleType: "interval", IntervalMinutes: 30}
	}
	p := valid()
	require.NoError(t, s.validatePlan(context.Background(), p))
	require.Equal(t, 100, p.MaxResults)
	require.Equal(t, 20.0, p.DropThreshold)
	known := valid()
	known.ModelID = modeltrace.Models()[0].ID
	require.NoError(t, s.validatePlan(context.Background(), known))
	require.Equal(t, known.ModelID, known.ReferenceModel)
	unknown := valid()
	unknown.ReferenceModel = "不允许覆盖的其他模型"
	require.NoError(t, s.validatePlan(context.Background(), unknown))
	require.Equal(t, "gpt-test", unknown.ReferenceModel)
	for _, change := range []func(*ModelDetectionPlan){func(p *ModelDetectionPlan) { p.IntervalMinutes = -1 }, func(p *ModelDetectionPlan) { p.IntervalMinutes = 10081 }, func(p *ModelDetectionPlan) { p.Timezone = "invalid/local" }, func(p *ModelDetectionPlan) { p.DailyTime = "25:01" }, func(p *ModelDetectionPlan) { p.ModelID = "x\ny" }, func(p *ModelDetectionPlan) { p.MaxResults = 99 }, func(p *ModelDetectionPlan) { p.DropThreshold = 101 }} {
		p := valid()
		change(p)
		require.ErrorIs(t, s.validatePlan(context.Background(), p), ErrModelDetectionInvalid)
	}
}
func TestModelDetectionNextRun(t *testing.T) {
	from := time.Date(2026, 9, 28, 16, 30, 0, 0, time.UTC)
	next, err := ModelDetectionNextRun(&ModelDetectionPlan{Enabled: true, ScheduleType: "interval", IntervalMinutes: 90}, from)
	require.NoError(t, err)
	require.Equal(t, from.Add(90*time.Minute), *next)
	next, err = ModelDetectionNextRun(&ModelDetectionPlan{Enabled: true, ScheduleType: "daily", DailyTime: "01:00", Timezone: "Asia/Shanghai"}, from)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 9, 28, 17, 0, 0, 0, time.UTC), next.UTC())
	next, err = ModelDetectionNextRun(&ModelDetectionPlan{Enabled: false}, from)
	require.NoError(t, err)
	require.Nil(t, next)
	// 春季跳时中不存在的 02:30 不能倒退或排出已经过去的任务。
	from = time.Date(2026, 3, 8, 7, 15, 0, 0, time.UTC)
	next, err = ModelDetectionNextRun(&ModelDetectionPlan{Enabled: true, ScheduleType: "daily", DailyTime: "02:30", Timezone: "America/New_York"}, from)
	require.NoError(t, err)
	require.True(t, next.After(from))
	require.Less(t, next.Sub(from), 48*time.Hour)
}
func TestModelDetectionWorkerRequestFailuresNeverScore(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status string
	}{{"网络错误", errors.New("Authorization Bearer secret-key"), "error"}, {"超时", context.DeadlineExceeded, "error"}, {"不支持", ErrModelDetectionUnsupported, "inconclusive"}, {"取消", context.Canceled, "error"}} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &detectionTestRepo{}
			probe := &detectionTestProbe{call: func(context.Context, int, string) (string, error) { return "", tc.err }}
			s := NewModelDetectionService(repo, nil, probe)
			run := &ModelDetectionRun{PlanID: 1, Status: "running"}
			s.executeRun(context.Background(), run)
			require.Equal(t, tc.status, repo.complete.Status)
			require.Nil(t, repo.complete.Score)
			require.Empty(t, repo.complete.Verdict)
			require.NotContains(t, repo.complete.ErrorMessage, "secret-key")
			require.Equal(t, 1, probe.calls)
		})
	}
}
func TestModelDetectionWorkerSuccessAndInvalidQuality(t *testing.T) {
	expected := map[string]string{}
	for _, id := range []string{"q1", "q2", "q3", "q4", "q5", "q6", "q7", "q8", "q9", "q10"} {
		expected[id] = "42"
	}
	answer, _ := json.Marshal(expected)
	for _, tc := range []struct {
		name, answer, status string
		score                *float64
	}{{name: "完整答题", answer: string(answer), status: "completed", score: floatPointer(100)}, {name: "缺失答案", answer: `{"other":"42"}`, status: "inconclusive"}} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &detectionTestRepo{}
			probe := &detectionTestProbe{call: func(ctx context.Context, n int, _ string) (string, error) {
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), modelDetectionRequestTimeout)
				if n < 4 {
					return strings.Repeat("42 ", 300), nil
				}
				return tc.answer, nil
			}}
			s := NewModelDetectionService(repo, nil, probe)
			s.qualityChallenge = func() modelquality.Challenge {
				return modelquality.Challenge{Prompt: "计算测试题", Expected: expected, Version: modelquality.Version}
			}
			run := &ModelDetectionRun{PlanID: 1, Status: "running"}
			s.executeRun(context.Background(), run)
			require.Equal(t, tc.status, repo.complete.Status)
			require.Equal(t, tc.score, repo.complete.Score)
			require.Equal(t, 4, probe.calls)
			require.Len(t, repo.complete.Details, 4)
			require.Equal(t, "unsupported", repo.complete.Fingerprint.Status)
		})
	}
}
func floatPointer(v float64) *float64 { return &v }
func TestModelDetectionProgressLeaseFailureStopsRequests(t *testing.T) {
	repo := &detectionTestRepo{progressErr: ErrModelDetectionLease}
	probe := &detectionTestProbe{call: func(context.Context, int, string) (string, error) { return "1 2 3", nil }}
	s := NewModelDetectionService(repo, nil, probe)
	s.executeRun(context.Background(), &ModelDetectionRun{Status: "running"})
	require.Equal(t, 0, probe.calls, "题目保存失败时不能发起可能计费的请求")
	require.Nil(t, repo.complete.Score)
}
func TestModelDetectionFingerprintInvalidAndUnsupported(t *testing.T) {
	result := classifyDetectionFingerprint([]modeltrace.Output{{Text: "我无法回答", ExpectedCount: 300}}, "")
	require.Equal(t, "invalid", result.Status)
	result = classifyDetectionFingerprint([]modeltrace.Output{{Text: strings.Repeat("42 ", 300), ExpectedCount: 300}}, "")
	require.Equal(t, "unsupported", result.Status)
	require.NotEmpty(t, result.Analysis)
	result = classifyDetectionFingerprint([]modeltrace.Output{{Text: strings.Repeat("42 ", 300), ExpectedCount: 300}}, modeltrace.Models()[0].ID)
	require.Equal(t, "inconclusive", result.Status)
	result = classifyDetectionFingerprint([]modeltrace.Output{{Text: strings.Repeat("42 ", 300), ExpectedCount: 300}}, "已移出参考库的模型")
	require.Equal(t, "unsupported", result.Status)
}
func TestModelDetectionStopCancelsInFlight(t *testing.T) {
	started := make(chan struct{})
	repo := &detectionTestRepo{queue: []*ModelDetectionRun{{Status: "running"}}}
	probe := &detectionTestProbe{call: func(ctx context.Context, _ int, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}}
	s := NewModelDetectionService(repo, nil, probe)
	s.Start()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("任务未启动")
	}
	done := make(chan struct{})
	go func() { s.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("停止未取消上游请求")
	}
	require.Equal(t, "error", repo.complete.Status)
	require.Empty(t, repo.complete.Verdict)
}
