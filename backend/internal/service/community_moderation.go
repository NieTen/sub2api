package service

import (
	"context"
	"errors"
	"strconv"
)

// 解除绑定必须由管理员审核用户本人提交的工单，工单归属和审计写入在仓储事务中完成。
func (s *CommunityService) Unbind(ctx context.Context, actor SupportTicketActor, userID, ticketID int64) (*CommunityUnbindResult, error) {
	if !actor.IsAdmin || actor.UserID <= 0 {
		return nil, ErrCommunityForbidden
	}
	if userID <= 0 || ticketID <= 0 {
		return nil, ErrCommunityUnbindTicket
	}
	return s.repo.Unbind(ctx, actor.UserID, userID, ticketID)
}

func (s *CommunityService) checkCommunityAdmission(ctx context.Context, c *CommunitySettings, userID, telegramID int64) error {
	groupID, err := strconv.ParseInt(c.GroupChatID, 10, 64)
	if err != nil || groupID == 0 {
		return nil
	}
	return s.repo.CheckAdmission(ctx, userID, telegramID, groupID)
}

// 本地禁入记录独立于 Telegram 的解封状态；真实管理员仍受保护，不通过机器人撤销群权限。
func (s *CommunityService) enforceCommunityBan(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, telegramID int64) error {
	current, err := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, telegramID)
	if err != nil {
		if communityTelegramBadRequest(err) {
			return nil
		}
		return err
	}
	if !communityMemberPresent(current) || communityMemberPrivileged(current) || current.User.ID == c.BotID {
		return nil
	}
	return s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "banChatMember", communityBanMemberRequest{ChatID: c.GroupChatID, UserID: telegramID, RevokeMessages: false}, nil)
}

func (s *CommunityService) rejectBannedCommunityJoin(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, telegramID int64) error {
	if err := s.declineJoin(ctx, shared.TelegramBotToken, c.GroupChatID, telegramID); err != nil {
		return err
	}
	return s.enforceCommunityBan(ctx, c, shared, telegramID)
}

// 禁入不是普通退群，任何补偿流程都不能将其改成可重新领取邀请的 left。
func communityRemovalError(err error) error {
	if communityPermanentError(err) || errors.Is(err, ErrCommunityBanned) {
		return nil
	}
	return err
}
