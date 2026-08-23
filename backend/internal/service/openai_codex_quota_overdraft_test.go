//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCodexQuotaOverdraftInjection(t *testing.T) {
	t.Cleanup(func() { SetCodexQuotaOverdraftEnabled(false) })
	SetCodexQuotaOverdraftEnabled(true)
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{CodexQuotaOverdraftEnabled: true}}}
	oauth := &Account{
		ID:       77,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 95,
			"codex_5h_reset_at":     time.Now().Add(time.Hour).Format(time.RFC3339),
		},
	}
	body := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)

	updated := svc.prepareCodexQuotaOverdraftBody(ctx, oauth, false, body)
	require.NotEqual(t, string(body), string(updated))
	require.True(t, codexQuotaOverdraftWasInjected(ctx, oauth.ID))

	var document codexQuotaOverdraftDocument
	require.NoError(t, json.Unmarshal(updated, &document))
	require.Len(t, document.Input, 3)
	var call, output codexQuotaOverdraftInputItem
	require.NoError(t, json.Unmarshal(document.Input[1], &call))
	require.NoError(t, json.Unmarshal(document.Input[2], &output))
	require.Equal(t, "custom_tool_call", call.Type)
	require.Equal(t, "custom_tool_call_output", output.Type)
	require.True(t, strings.HasPrefix(call.CallID, codexQuotaOverdraftCallIDPrefix))
	require.Equal(t, call.CallID, output.CallID)

	again := svc.prepareCodexQuotaOverdraftBody(ctx, oauth, false, updated)
	require.Equal(t, string(updated), string(again), "重复处理不能再次注入")
}

func TestCodexQuotaOverdraftInjectionGuards(t *testing.T) {
	t.Cleanup(func() { SetCodexQuotaOverdraftEnabled(false) })
	body := []byte(`{"input":[{"type":"message","role":"user"}]}`)
	oauth := &Account{
		ID:       78,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 95,
			"codex_5h_reset_at":     time.Now().Add(time.Hour).Format(time.RFC3339),
		},
	}
	enabledCfg := &config.Config{Gateway: config.GatewayConfig{CodexQuotaOverdraftEnabled: true}}
	svc := &OpenAIGatewayService{cfg: enabledCfg}

	SetCodexQuotaOverdraftEnabled(false)
	require.Equal(t, string(body), string(svc.prepareCodexQuotaOverdraftBody(WithCodexQuotaOverdraftScheduling(context.Background()), oauth, false, body)))

	SetCodexQuotaOverdraftEnabled(true)
	underPrearm := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_5h_used_percent": 94}}
	require.Equal(t, string(body), string(svc.prepareCodexQuotaOverdraftBody(WithCodexQuotaOverdraftScheduling(context.Background()), underPrearm, false, body)), "95% 以下不能注入")
	require.Equal(t, string(body), string(svc.prepareCodexQuotaOverdraftBody(context.Background(), oauth, false, body)), "未标记的端点不能注入")
	require.Equal(t, string(body), string(svc.prepareCodexQuotaOverdraftBody(WithCodexQuotaOverdraftScheduling(context.Background()), oauth, true, body)), "compact 不能注入")

	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	for _, account := range []*Account{
		{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: int64PtrForCodexQuotaOverdraftTest(1)},
	} {
		require.Equal(t, string(body), string(svc.prepareCodexQuotaOverdraftBody(ctx, account, false, body)))
	}
	agentIdentity := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{openAIAuthModeCredentialKey: OpenAIAuthModeAgentIdentity},
		Extra: map[string]any{
			"codex_7d_used_percent": 95,
			"codex_7d_reset_at":     time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		},
	}
	require.NotEqual(t, string(body), string(svc.prepareCodexQuotaOverdraftBody(ctx, agentIdentity, false, body)), "Agent Identity 必须支持透支请求注入")

	notUserLast := []byte(`{"input":[{"type":"message","role":"assistant"}]}`)
	require.Equal(t, string(notUserLast), string(svc.prepareCodexQuotaOverdraftBody(ctx, oauth, false, notUserLast)))
	invalid := []byte(`{"input":`)
	require.Equal(t, string(invalid), string(svc.prepareCodexQuotaOverdraftBody(ctx, oauth, false, invalid)))
	oversized := make([]byte, codexQuotaOverdraftMaxBodyBytes+1)
	require.Equal(t, oversized, svc.prepareCodexQuotaOverdraftBody(ctx, oauth, false, oversized))
}

func TestCodexQuotaOverdraftAccountExtraControlsEligibility(t *testing.T) {
	t.Cleanup(func() { SetCodexQuotaOverdraftEnabled(false) })
	SetCodexQuotaOverdraftEnabled(true)

	body := []byte(`{"input":[{"type":"message","role":"user"}]}`)
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{CodexQuotaOverdraftEnabled: true}}}
	reset := time.Now().Add(time.Hour).Format(time.RFC3339)

	disabled := &Account{
		ID:       901,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_quota_overdraft_enabled": false,
			"codex_5h_used_percent":         100,
			"codex_5h_reset_at":             reset,
		},
	}
	require.Equal(t, string(body), string(svc.prepareCodexQuotaOverdraftBody(ctx, disabled, false, body)), "账号级关闭后不能注入透支载荷")

	quotaCtx := withOpenAIQuotaAutoPauseSettings(ctx, OpsOpenAIAccountQuotaAutoPauseSettings{DefaultThreshold5h: 0.8})
	paused, _ := shouldAutoPauseOpenAIAccountByQuota(quotaCtx, disabled)
	require.True(t, paused, "账号级关闭后不能绕过调度阈值")

	oauthBlockedByType := &Account{
		ID:       902,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_quota_overdraft_account_types": []any{AccountTypeSetupToken},
			"codex_5h_used_percent":               100,
			"codex_5h_reset_at":                   reset,
		},
	}
	require.Equal(t, string(body), string(svc.prepareCodexQuotaOverdraftBody(ctx, oauthBlockedByType, false, body)), "账号类型白名单不含 oauth 时不能注入")
	paused, _ = shouldAutoPauseOpenAIAccountByQuota(quotaCtx, oauthBlockedByType)
	require.True(t, paused, "账号类型白名单不含 oauth 时不能绕过调度阈值")

	setupTokenAllowed := &Account{
		ID:       903,
		Platform: PlatformOpenAI,
		Type:     AccountTypeSetupToken,
		Extra: map[string]any{
			"codex_quota_overdraft_account_types": []any{AccountTypeSetupToken},
			"codex_5h_used_percent":               100,
			"codex_5h_reset_at":                   reset,
		},
	}
	require.NotEqual(t, string(body), string(svc.prepareCodexQuotaOverdraftBody(ctx, setupTokenAllowed, false, body)), "白名单允许 setup-token 时应支持注入")
	paused, _ = shouldAutoPauseOpenAIAccountByQuota(quotaCtx, setupTokenAllowed)
	require.False(t, paused, "白名单允许 setup-token 时应支持透支绕过阈值")
}

func TestCodexQuotaOverdraftGroupRelayUsesNextGroupWhenCurrentEmpty(t *testing.T) {
	t.Cleanup(func() { SetCodexQuotaOverdraftEnabled(false) })
	SetCodexQuotaOverdraftEnabled(true)

	groupA := int64(9101)
	groupB := int64(9102)
	repo := &codexQuotaOverdraftRelayAccountRepo{
		byGroup: map[int64][]Account{
			groupA: {},
			groupB: {{
				ID:          9201,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
				Extra: map[string]any{
					"codex_7d_used_percent": 100,
					"codex_7d_reset_at":     time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				},
				GroupIDs: []int64{groupB},
			}},
		},
	}
	groupRepo := &codexQuotaOverdraftRelayGroupRepo{groups: map[int64]*Group{
		groupA: {ID: groupA, Platform: PlatformOpenAI, CodexOverdraftEnabled: true, CodexOverdraftNextGroupID: &groupB},
		groupB: {ID: groupB, Platform: PlatformOpenAI, CodexOverdraftEnabled: true},
	}}
	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cfg:         &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{
			groupRepo: groupRepo,
		},
	}

	accounts, handled, err := svc.listCodexQuotaOverdraftSchedulableAccounts(WithCodexQuotaOverdraftScheduling(context.Background()), &groupA, PlatformOpenAI)

	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, []int64{groupA, groupB}, repo.groupCalls)
	require.Len(t, accounts, 1)
	require.Equal(t, int64(9201), accounts[0].ID)
}

func TestCodexQuotaOverdraftGroupDisabledFallsBackToNormalScheduling(t *testing.T) {
	t.Cleanup(func() { SetCodexQuotaOverdraftEnabled(false) })
	SetCodexQuotaOverdraftEnabled(true)

	groupA := int64(9301)
	repo := &codexQuotaOverdraftRelayAccountRepo{
		byGroup: map[int64][]Account{
			groupA: {{
				ID:          9302,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
			}},
		},
	}
	groupRepo := &codexQuotaOverdraftRelayGroupRepo{groups: map[int64]*Group{
		groupA: {ID: groupA, Platform: PlatformOpenAI, CodexOverdraftEnabled: false},
	}}
	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cfg:         &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{
			groupRepo: groupRepo,
		},
	}

	accounts, handled, err := svc.listCodexQuotaOverdraftSchedulableAccounts(WithCodexQuotaOverdraftScheduling(context.Background()), &groupA, PlatformOpenAI)

	require.NoError(t, err)
	require.False(t, handled)
	require.Empty(t, accounts)
	require.Empty(t, repo.groupCalls, "分组关闭透支时不应进入透支专用查询")
}

func TestCodexQuotaOverdraftGroupRelayStopsOnCycle(t *testing.T) {
	t.Cleanup(func() { SetCodexQuotaOverdraftEnabled(false) })
	SetCodexQuotaOverdraftEnabled(true)

	groupA := int64(9401)
	groupB := int64(9402)
	repo := &codexQuotaOverdraftRelayAccountRepo{
		byGroup: map[int64][]Account{
			groupA: {},
			groupB: {},
		},
	}
	groupRepo := &codexQuotaOverdraftRelayGroupRepo{groups: map[int64]*Group{
		groupA: {ID: groupA, Platform: PlatformOpenAI, CodexOverdraftEnabled: true, CodexOverdraftNextGroupID: &groupB},
		groupB: {ID: groupB, Platform: PlatformOpenAI, CodexOverdraftEnabled: true, CodexOverdraftNextGroupID: &groupA},
	}}
	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cfg:         &config.Config{},
		schedulerSnapshot: &SchedulerSnapshotService{
			groupRepo: groupRepo,
		},
	}

	accounts, handled, err := svc.listCodexQuotaOverdraftSchedulableAccounts(WithCodexQuotaOverdraftScheduling(context.Background()), &groupA, PlatformOpenAI)

	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, []int64{groupA, groupB}, repo.groupCalls)
	require.Empty(t, accounts)
}

type codexQuotaOverdraftRelayAccountRepo struct {
	AccountRepository
	byGroup    map[int64][]Account
	groupCalls []int64
}

func (r *codexQuotaOverdraftRelayAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	r.groupCalls = append(r.groupCalls, groupID)
	accounts := r.byGroup[groupID]
	out := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account.Platform == platform {
			out = append(out, account)
		}
	}
	return out, nil
}

func (r *codexQuotaOverdraftRelayAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	out := make([]Account, 0)
	for _, accounts := range r.byGroup {
		for _, account := range accounts {
			if account.Platform == platform {
				out = append(out, account)
			}
		}
	}
	return out, nil
}

func (r *codexQuotaOverdraftRelayAccountRepo) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	return []Account{}, nil
}

type codexQuotaOverdraftRelayGroupRepo struct {
	GroupRepository
	groups map[int64]*Group
}

func (r *codexQuotaOverdraftRelayGroupRepo) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	group := r.groups[id]
	if group == nil {
		return nil, ErrGroupNotFound
	}
	return group, nil
}

func (r *codexQuotaOverdraftRelayGroupRepo) GetByID(ctx context.Context, id int64) (*Group, error) {
	return r.GetByIDLite(ctx, id)
}

func TestCodexQuotaOverdraftSchedulingDoesNotBypassThresholdBelowPrearm(t *testing.T) {
	t.Cleanup(func() { SetCodexQuotaOverdraftEnabled(false) })
	SetCodexQuotaOverdraftEnabled(true)
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 94,
		},
	}
	quotaCtx := withOpenAIQuotaAutoPauseSettings(ctx, OpsOpenAIAccountQuotaAutoPauseSettings{DefaultThreshold5h: 0.9})

	paused, _ := shouldAutoPauseOpenAIAccountByQuota(quotaCtx, account)
	require.True(t, paused)
}

func TestCodexQuotaOverdraftSchedulingOnlyBypassesQuotaThresholds(t *testing.T) {
	t.Cleanup(func() { SetCodexQuotaOverdraftEnabled(false) })
	SetCodexQuotaOverdraftEnabled(true)
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	now := time.Now().UTC()
	reset := now.Add(time.Hour)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"codex_5h_used_percent": 100,
			"codex_5h_reset_at":     reset.Format(time.RFC3339),
		},
	}
	quotaCtx := withOpenAIQuotaAutoPauseSettings(ctx, OpsOpenAIAccountQuotaAutoPauseSettings{DefaultThreshold5h: 0.8})
	paused, _ := shouldAutoPauseOpenAIAccountByQuota(quotaCtx, account)
	require.False(t, paused)

	account.RateLimitResetAt = &reset
	require.False(t, account.IsSchedulableForModelWithContext(quotaCtx, "gpt-5.4"), "真实 429 限流仍必须生效")
	account.RateLimitResetAt = nil
	account.TempUnschedulableUntil = &reset
	account.TempUnschedulableReason = BuildTempUnschedReasonPayload("oauth_401", "unauthorized")
	require.Same(t, account, normalizeCodexQuotaOverdraftAccountForScheduling(quotaCtx, account), "其他临时暂停不能绕过")

	account.TempUnschedulableReason = BuildAccountSchedulingThresholdReason("")
	normalized := normalizeCodexQuotaOverdraftAccountForScheduling(quotaCtx, account)
	require.NotSame(t, account, normalized)
	require.Nil(t, normalized.TempUnschedulableUntil)
	require.Empty(t, normalized.TempUnschedulableReason)
	require.NotNil(t, account.TempUnschedulableUntil, "不能修改缓存或数据库账号原对象")
}

func TestCodexQuotaOverdraftRuntimeThresholdBlockIsContextAware(t *testing.T) {
	t.Cleanup(func() { SetCodexQuotaOverdraftEnabled(false) })
	SetCodexQuotaOverdraftEnabled(true)
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})

	settingsRepo := newMockSettingRepo()
	settingsRepo.data[SettingKeyAccountSchedulingThresholds] = `{"openai":80}`
	accountRepo := &rateLimitAccountRepoStub{}
	runtimeBlocker := &OpenAIGatewayService{}
	rl := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	rl.SetSettingService(NewSettingService(settingsRepo, &config.Config{}))
	rl.SetAccountRuntimeBlocker(runtimeBlocker)
	reset := time.Now().UTC().Add(time.Hour)
	account := &Account{
		ID:          9001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"codex_7d_used_percent": 100,
			"codex_7d_reset_at":     reset.Format(time.RFC3339),
		},
	}

	require.True(t, rl.ApplyAccountSchedulingThreshold(context.Background(), account))
	require.Equal(t, 1, accountRepo.tempCalls, "普通端点仍应持久化阈值暂停")
	require.True(t, runtimeBlocker.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-5.4"), "非透支上下文仍应保留原有 runtime block")

	quotaCtx := WithCodexQuotaOverdraftScheduling(context.Background())
	require.False(t, runtimeBlocker.isOpenAIAccountRequestRuntimeBlockedForContext(quotaCtx, account, "gpt-5.4"), "透支上下文只绕过阈值来源的 runtime block")

	runtimeBlocker.BlockAccountScheduling(account, reset.Add(time.Hour), "429")
	require.True(t, runtimeBlocker.isOpenAIAccountRequestRuntimeBlockedForContext(quotaCtx, account, "gpt-5.4"), "真实 429 runtime block 不能被透支绕过")
}

func int64PtrForCodexQuotaOverdraftTest(value int64) *int64 {
	return &value
}

func TestCodexQuotaOverdraftCoordinatorOwnedByGateway(t *testing.T) {
	gateway := &OpenAIGatewayService{}

	first := gateway.codexQuotaOverdraftCoordinator(nil)
	second := gateway.codexQuotaOverdraftCoordinator(&TLSFingerprintProfileService{})

	require.NotNil(t, first)
	require.Same(t, first, second)
	require.Same(t, gateway, first.runtimeBlocker)
}
