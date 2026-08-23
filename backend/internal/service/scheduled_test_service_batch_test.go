package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type batchScheduledTestPlanRepo struct {
	ScheduledTestPlanRepository
	mu          sync.Mutex
	nextID      int64
	created     []*ScheduledTestPlan
	updateCalls int
	delay       time.Duration
	current     int32
	maxSeen     int32
}

func (r *batchScheduledTestPlanRepo) Create(_ context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	nowConcurrent := atomic.AddInt32(&r.current, 1)
	for {
		maxSeen := atomic.LoadInt32(&r.maxSeen)
		if nowConcurrent <= maxSeen || atomic.CompareAndSwapInt32(&r.maxSeen, maxSeen, nowConcurrent) {
			break
		}
	}
	defer atomic.AddInt32(&r.current, -1)

	if r.delay > 0 {
		time.Sleep(r.delay)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	created := *plan
	created.ID = r.nextID
	now := time.Now()
	created.CreatedAt = now
	created.UpdatedAt = now
	r.created = append(r.created, &created)
	return &created, nil
}

func (r *batchScheduledTestPlanRepo) Update(_ context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateCalls++
	return plan, nil
}

func TestScheduledTestServiceCreatePlansBatchAppendsWithoutOverwrite(t *testing.T) {
	repo := &batchScheduledTestPlanRepo{}
	svc := NewScheduledTestService(repo, nil)

	result, err := svc.CreatePlansBatch(context.Background(), BatchCreateScheduledTestPlansInput{
		AccountIDs:     []int64{3, 1, 2},
		ModelID:        "claude-sonnet-4-5",
		CronExpression: "*/30 * * * *",
		Enabled:        true,
		MaxResults:     100,
		AutoRecover:    true,
		MaxConcurrency: BatchScheduledTestPlanCreateConcurrency,
	})

	require.NoError(t, err)
	require.Equal(t, 3, result.Total)
	require.Equal(t, 3, result.Success)
	require.Equal(t, 0, result.Failed)
	require.Len(t, result.Results, 3)
	require.Len(t, repo.created, 3)
	require.Zero(t, repo.updateCalls)

	createdByAccountID := make(map[int64]*ScheduledTestPlan, len(repo.created))
	for _, plan := range repo.created {
		createdByAccountID[plan.AccountID] = plan
		require.Equal(t, "claude-sonnet-4-5", plan.ModelID)
		require.Equal(t, "*/30 * * * *", plan.CronExpression)
		require.True(t, plan.Enabled)
		require.Equal(t, 100, plan.MaxResults)
		require.True(t, plan.AutoRecover)
		require.NotNil(t, plan.NextRunAt)
	}

	for index, accountID := range []int64{3, 1, 2} {
		require.Contains(t, createdByAccountID, accountID)
		require.Equal(t, accountID, result.Results[index].AccountID)
		require.True(t, result.Results[index].Success)
		require.NotNil(t, result.Results[index].Plan)
	}
}

func TestScheduledTestServiceCreatePlansBatchUsesFifteenWorkers(t *testing.T) {
	accountIDs := make([]int64, 0, 32)
	for i := int64(1); i <= 32; i++ {
		accountIDs = append(accountIDs, i)
	}

	repo := &batchScheduledTestPlanRepo{
		delay: 25 * time.Millisecond,
	}
	svc := NewScheduledTestService(repo, nil)

	result, err := svc.CreatePlansBatch(context.Background(), BatchCreateScheduledTestPlansInput{
		AccountIDs:     accountIDs,
		ModelID:        "claude-sonnet-4-5",
		CronExpression: "*/30 * * * *",
		Enabled:        true,
		MaxResults:     100,
		AutoRecover:    true,
	})

	require.NoError(t, err)
	require.Equal(t, len(accountIDs), result.Total)
	require.Equal(t, len(accountIDs), result.Success)
	require.Equal(t, 0, result.Failed)
	require.Equal(t, int32(BatchScheduledTestPlanCreateConcurrency), atomic.LoadInt32(&repo.maxSeen))
	require.Len(t, repo.created, len(accountIDs))
}
