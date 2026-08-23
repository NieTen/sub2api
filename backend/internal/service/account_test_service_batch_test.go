package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type batchConnectionTestAccountRepo struct {
	AccountRepository
	mu       sync.Mutex
	accounts map[int64]*Account
	delay    time.Duration
	current  int32
	maxSeen  int32
}

func (r *batchConnectionTestAccountRepo) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]*Account, 0, len(ids))
	for _, id := range ids {
		account := r.accounts[id]
		if account == nil {
			continue
		}
		out = append(out, cloneBatchConnectionAccount(account))
	}
	return out, nil
}

func (r *batchConnectionTestAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	now := atomic.AddInt32(&r.current, 1)
	for {
		maxSeen := atomic.LoadInt32(&r.maxSeen)
		if now <= maxSeen || atomic.CompareAndSwapInt32(&r.maxSeen, maxSeen, now) {
			break
		}
	}
	defer atomic.AddInt32(&r.current, -1)

	if r.delay > 0 {
		time.Sleep(r.delay)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil {
		return nil, ErrAccountNotFound
	}
	return cloneBatchConnectionAccount(account), nil
}

func cloneBatchConnectionAccount(account *Account) *Account {
	clone := *account
	clone.Credentials = mergeMap(nil, account.Credentials)
	clone.Extra = mergeMap(nil, account.Extra)
	return &clone
}

func TestAccountTestServiceBatchTestConnectionsUsesFifteenWorkers(t *testing.T) {
	accounts := make(map[int64]*Account)
	accountIDs := make([]int64, 0, 32)
	for i := int64(1); i <= 32; i++ {
		accountIDs = append(accountIDs, i)
		accounts[i] = &Account{
			ID:       i,
			Name:     fmt.Sprintf("account-%02d", i),
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra: map[string]any{
				"synthetic_ui_test": true,
			},
		}
	}
	repo := &batchConnectionTestAccountRepo{
		accounts: accounts,
		delay:    25 * time.Millisecond,
	}
	svc := &AccountTestService{accountRepo: repo}

	result, err := svc.BatchTestConnections(context.Background(), BatchAccountConnectionTestInput{
		AccountIDs: accountIDs,
	})

	require.NoError(t, err)
	require.Equal(t, len(accountIDs), result.Total)
	require.Equal(t, len(accountIDs), result.Success)
	require.Equal(t, 0, result.Failed)
	require.Equal(t, int32(BatchAccountConnectionTestConcurrency), atomic.LoadInt32(&repo.maxSeen))
	require.Len(t, result.Results, len(accountIDs))
	for index, item := range result.Results {
		require.Equal(t, accountIDs[index], item.AccountID)
		require.True(t, item.Responded)
		require.True(t, item.Success)
		require.NotZero(t, item.LatencyMs)
		require.NotEmpty(t, item.ResponseText)
	}
}

func TestAccountTestServiceBatchTestConnectionsKeepsMissingAccountAsFailedResult(t *testing.T) {
	repo := &batchConnectionTestAccountRepo{
		accounts: map[int64]*Account{
			1: {
				ID:       1,
				Name:     "healthy",
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Status:   StatusActive,
				Extra: map[string]any{
					"synthetic_ui_test": true,
				},
			},
		},
	}
	svc := &AccountTestService{accountRepo: repo}

	result, err := svc.BatchTestConnections(context.Background(), BatchAccountConnectionTestInput{
		AccountIDs: []int64{1, 2},
	})

	require.NoError(t, err)
	require.Equal(t, 2, result.Total)
	require.Equal(t, 1, result.Success)
	require.Equal(t, 1, result.Failed)
	require.Len(t, result.Results, 2)
	require.True(t, result.Results[0].Success)
	require.Equal(t, int64(2), result.Results[1].AccountID)
	require.False(t, result.Results[1].Success)
	require.False(t, result.Results[1].Responded)
	require.Equal(t, "account not found", result.Results[1].ErrorMessage)
}
