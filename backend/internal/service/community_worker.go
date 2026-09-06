package service

import (
	"context"
	"log/slog"
	"strconv"
	"time"
)

func (s *CommunityService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.process(ctx)
			}
		}
	}()
}
func (s *CommunityService) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *CommunityService) process(ctx context.Context) {
	s.mu.Lock()
	needsCleanup := time.Since(s.lastCleanup) > time.Hour
	s.mu.Unlock()
	if needsCleanup {
		cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 10*time.Second)
		cleanupErr := s.repo.Cleanup(cleanupCtx)
		cleanupCancel()
		if cleanupErr == nil {
			s.mu.Lock()
			s.lastCleanup = time.Now()
			s.mu.Unlock()
		} else if ctx.Err() == nil {
			slog.Warn("社群过期记录清理失败")
		}
	}
	configCtx, configCancel := context.WithTimeout(ctx, 40*time.Second)
	c, shared, err := s.checkedConfiguration(configCtx)
	configCancel()
	if err != nil {
		return
	}
	// 逐条领取，既提高集中申请时的吞吐，也避免预领任务在等待中耗尽租约。
	for processed := 0; processed < 10; processed++ {
		claimCtx, claimCancel := context.WithTimeout(ctx, 10*time.Second)
		events, claimErr := s.repo.ClaimWebhooks(claimCtx, 1, 3*time.Minute)
		claimCancel()
		if claimErr != nil {
			if ctx.Err() == nil {
				slog.Warn("社群事件领取失败")
			}
			break
		}
		if len(events) == 0 {
			break
		}
		event := events[0]
		workCtx, workCancel := context.WithTimeout(ctx, 2*time.Minute)
		err := s.processWebhook(workCtx, c, shared, event)
		workCancel()
		retryNeeded := err != nil && !communityPermanentError(err)
		finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		if err == nil || communityPermanentError(err) {
			err = s.repo.CompleteWebhook(finishCtx, event.ID, event.LeaseToken)
		} else {
			err = s.repo.FailWebhook(finishCtx, event.ID, event.LeaseToken, time.Now().Add(time.Duration(1<<min(event.Attempts, 6))*time.Second))
		}
		if err != nil {
			slog.Warn("社群事件状态保存失败")
		}
		finishCancel()
		if retryNeeded || err != nil || ctx.Err() != nil {
			break
		}
	}
	for processed := 0; processed < 3; processed++ {
		claimCtx, claimCancel := context.WithTimeout(ctx, 10*time.Second)
		invites, claimErr := s.repo.ClaimRevocations(claimCtx, 1, 2*time.Minute)
		claimCancel()
		if claimErr != nil || len(invites) == 0 {
			break
		}
		invite := invites[0]
		workCtx, workCancel := context.WithTimeout(ctx, 45*time.Second)
		revokeErr := s.reconcileExpiringInvite(workCtx, c, shared, invite)
		revokeAttempted := false
		if revokeErr == nil && invite.BotID == c.BotID {
			revokeAttempted = true
			revokeErr = s.delivery.telegramJSON(workCtx, shared.TelegramBotToken, "revokeChatInviteLink", communityRevokeInviteRequest{ChatID: strconv.FormatInt(invite.GroupChatID, 10), InviteLink: invite.URL}, nil)
		}
		workCancel()
		finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		retryNeeded := revokeErr != nil && !(revokeAttempted && communityTelegramBadRequest(revokeErr))
		if !retryNeeded {
			err = s.repo.CompleteRevocation(finishCtx, invite.ID, invite.LeaseToken)
		} else {
			err = s.repo.FailRevocation(finishCtx, invite.ID, invite.LeaseToken, time.Now().Add(time.Duration(1<<min(invite.Attempts, 6))*time.Second))
		}
		if err != nil {
			slog.Warn("社群邀请撤销状态保存失败")
		}
		finishCancel()
		if retryNeeded || err != nil || ctx.Err() != nil {
			break
		}
	}
}

// 邀请到期时核对真实入群结果，防止丢失成员事件留下永久等待中的身份。
func (s *CommunityService) reconcileExpiringInvite(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, invite CommunityInvite) error {
	membership, err := s.repo.GetMembershipByTelegram(ctx, invite.TelegramUserID)
	if err != nil {
		if communityPermanentError(err) {
			return nil
		}
		return err
	}
	if membership == nil || membership.GroupChatID != invite.GroupChatID || membership.Status != "pending" || membership.AuthorizedInviteID != invite.ID {
		return nil
	}
	if invite.ExpiresAt.After(time.Now()) && invite.BotID == c.BotID {
		return nil
	}
	status := "left"
	if invite.BotID == c.BotID {
		member, err := s.getChatMember(ctx, shared.TelegramBotToken, strconv.FormatInt(invite.GroupChatID, 10), invite.TelegramUserID)
		if err != nil {
			return err
		}
		if communityMemberPresent(member) {
			// 旧群邀请不套用新群充值策略，当前群必须再次校验最新 VIP 资格。
			if strconv.FormatInt(invite.GroupChatID, 10) == c.GroupChatID {
				err = s.currentCommunityAccess(ctx, c, membership.UserID)
			} else {
				err = s.repo.EnsureActiveUser(ctx, membership.UserID)
			}
			if err != nil {
				if !communityPermanentError(err) {
					return err
				}
				if !communityMemberPrivileged(member) && member.User.ID != c.BotID {
					if err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "banChatMember", communityBanMemberRequest{ChatID: strconv.FormatInt(invite.GroupChatID, 10), UserID: invite.TelegramUserID, UntilDate: time.Now().Add(time.Minute).Unix(), RevokeMessages: false}, nil); err != nil {
						return err
					}
				}
			} else {
				status = "joined"
			}
		}
	}
	return s.repo.MarkMembership(ctx, invite.TelegramUserID, invite.GroupChatID, status, time.Now().Unix(), 0)
}
