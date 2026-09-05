package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func codexQuotaOverdraftBypassesSchedulingThreshold(ctx context.Context, account *Account) bool {
	return codexQuotaOverdraftSchedulingEnabled(ctx) && isCodexQuotaOverdraftAccount(account) &&
		codexQuotaOverdraftSchedulingAllowed(account, time.Now().UTC())
}

func (s *RateLimitService) notifyCodexQuotaOverdraftAwareSchedulingBlock(
	account *Account,
	until time.Time,
) {
	s.notifyAccountSchedulingBlocked(account, until, AccountSchedulingThresholdReasonSource)
}

func (s *OpenAIGatewayService) listCodexQuotaOverdraftSchedulableAccounts(
	ctx context.Context,
	groupID *int64,
	platform string,
) ([]Account, bool, error) {
	if !CodexQuotaOverdraftSchedulingEnabled(ctx) || platform != PlatformOpenAI || s.accountRepo == nil {
		return nil, false, nil
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		accounts, err := s.accountRepo.ListSchedulableByPlatform(ctx, platform)
		if err != nil {
			return nil, true, fmt.Errorf("query overdraft accounts failed: %w", err)
		}
		return s.codexQuotaOverdraftNormalizeSchedulableAccounts(ctx, accounts), true, nil
	}
	if groupID == nil {
		accounts, err := s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, platform)
		if err != nil {
			return nil, true, fmt.Errorf("query overdraft accounts failed: %w", err)
		}
		return s.codexQuotaOverdraftNormalizeSchedulableAccounts(ctx, accounts), true, nil
	}

	return s.listCodexQuotaOverdraftSchedulableAccountsByGroupRelay(ctx, *groupID, platform)
}

func (s *OpenAIGatewayService) listCodexQuotaOverdraftSchedulableAccountsByGroupRelay(
	ctx context.Context,
	initialGroupID int64,
	platform string,
) ([]Account, bool, error) {
	if initialGroupID <= 0 {
		return nil, false, nil
	}
	if s.schedulerSnapshot == nil {
		accounts, err := s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, initialGroupID, platform)
		if err != nil {
			return nil, true, fmt.Errorf("query overdraft accounts failed: %w", err)
		}
		return s.codexQuotaOverdraftNormalizeSchedulableAccounts(ctx, accounts), true, nil
	}

	currentGroupID := initialGroupID
	visited := make(map[int64]struct{})
	for {
		if _, ok := visited[currentGroupID]; ok {
			return nil, true, nil
		}
		visited[currentGroupID] = struct{}{}

		group, err := s.schedulerSnapshot.GetGroupByIDLite(ctx, currentGroupID)
		if err != nil {
			return nil, true, fmt.Errorf("query overdraft group %d failed: %w", currentGroupID, err)
		}
		if group == nil || group.Platform != PlatformOpenAI || !group.CodexOverdraftEnabled {
			return nil, currentGroupID != initialGroupID, nil
		}

		accounts, err := s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, currentGroupID, platform)
		if err != nil {
			return nil, true, fmt.Errorf("query overdraft accounts failed: %w", err)
		}
		accounts = s.codexQuotaOverdraftNormalizeSchedulableAccounts(ctx, accounts)
		if len(accounts) > 0 {
			return accounts, true, nil
		}
		if group.CodexOverdraftNextGroupID == nil || *group.CodexOverdraftNextGroupID <= 0 {
			return accounts, true, nil
		}
		currentGroupID = *group.CodexOverdraftNextGroupID
	}
}

func (s *OpenAIGatewayService) codexQuotaOverdraftNormalizeSchedulableAccounts(
	ctx context.Context,
	accounts []Account,
) []Account {
	accounts = normalizeCodexQuotaOverdraftAccountsForScheduling(ctx, accounts)
	return s.filterOpenAIAccountsBySchedulingThreshold(ctx, accounts)
}

func (s *OpenAIGatewayService) handleCodexQuotaOverdraftUpstream429(
	ctx context.Context,
	account *Account,
	statusCode int,
	headers http.Header,
	responseBody []byte,
	canonicalModel []string,
) bool {
	if statusCode != http.StatusTooManyRequests || s.codexQuotaOverdraft == nil {
		return false
	}
	preferredModel := ""
	if len(canonicalModel) > 0 {
		preferredModel = canonicalModel[0]
	}
	return s.codexQuotaOverdraft.HandleQuota429(ctx, account, headers, responseBody, preferredModel)
}

func (s *OpenAIGatewayService) processCodexQuotaOverdraftUsageSnapshot(
	ctx context.Context,
	accountID int64,
	now time.Time,
	updates map[string]any,
) {
	enabled := CodexQuotaOverdraftEnabled()
	persistSnapshot := s.getCodexSnapshotThrottle().Allow(accountID, now)
	if enabled && codexQuotaOverdraftSnapshotPrearmReached(updates) {
		persistSnapshot = true
	}
	businessSuccess := enabled && codexQuotaOverdraftWasInjected(ctx, accountID)

	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if !enabled {
			if persistSnapshot {
				if err := s.accountRepo.UpdateExtra(updateCtx, accountID, updates); err == nil {
					notifyOpenAIAutoReset(accountID)
				}
			}
			return
		}
		var account *Account
		if !persistSnapshot && s.codexQuotaOverdraft != nil {
			current, err := s.accountRepo.GetByID(updateCtx, accountID)
			if err == nil && current != nil {
				account = current
				state, hasState := codexQuotaOverdraftStateFromAccount(current)
				_, wasExhausted := codexQuotaOverdraftSignalFromAccount(current, state, now)
				persistSnapshot = wasExhausted || hasState && state.Status != codexQuotaOverdraftProbeRecovered
			}
		}
		if persistSnapshot {
			if err := s.accountRepo.UpdateExtra(updateCtx, accountID, updates); err != nil {
				return
			}
			notifyOpenAIAutoReset(accountID)
		}
		if s.codexQuotaOverdraft == nil {
			return
		}
		if account == nil {
			current, err := s.accountRepo.GetByID(updateCtx, accountID)
			if err != nil || current == nil {
				return
			}
			account = current
		}
		mergeAccountExtra(account, updates)
		if businessSuccess {
			s.codexQuotaOverdraft.observeBusinessSuccess(account, "")
		} else {
			s.codexQuotaOverdraft.observeAccount(account, "")
		}
	}()
}

func (s *OpenAIGatewayService) observeCodexQuotaOverdraftScheduleSuccess(
	accountID int64,
	model string,
	requestCtx []context.Context,
) {
	if len(requestCtx) > 0 && s.codexQuotaOverdraft != nil && codexQuotaOverdraftWasInjected(requestCtx[0], accountID) {
		s.codexQuotaOverdraft.ObserveBusinessSuccessByID(accountID, model)
	}
}
