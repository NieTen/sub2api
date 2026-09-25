package service

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCommunityMemberEventRecoversOriginalInviteWhenAdministratorApprovesFirst(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	ctx := context.Background()
	state, err := s.CreateInvite(ctx, 1, CommunityInviteInput{})
	require.NoError(t, err)
	c, shared, err := s.checkedConfiguration(ctx)
	require.NoError(t, err)
	telegram.present = true
	update := communityTelegramUpdate{UpdateID: 90, ChatMember: &communityTelegramMemberUpdate{
		Chat: communityTelegramChat{ID: -100}, Date: time.Now().Unix(),
		OldChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "left"},
		NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42, Username: "member", FirstName: "网站用户"}, Status: "member"},
		InviteLink:    &communityTelegramInvite{InviteLink: state.Invite.URL, CreatesJoinRequest: true},
	}}
	raw, err := json.Marshal(update)
	require.NoError(t, err)
	require.NoError(t, s.processWebhook(ctx, c, shared, CommunityWebhookEvent{BotID: c.BotID, Payload: raw}))
	require.Equal(t, int64(1), repo.member.UserID)
	require.Equal(t, int64(42), repo.member.TelegramUserID)
	require.Equal(t, int64(10), repo.member.AuthorizedInviteID)
	require.Equal(t, "joined", repo.member.Status)
	require.NotContains(t, telegram.methods, "banChatMember")
	require.NotContains(t, telegram.methods, "approveChatJoinRequest")
	// 先到的成员事件已经完成绑定，迟到的同一加入申请不应再次审批或撤销身份。
	require.NoError(t, s.processJoinRequest(ctx, c, shared, 89, &communityTelegramJoinRequest{
		Chat: update.ChatMember.Chat, From: update.ChatMember.NewChatMember.User,
		Date: update.ChatMember.Date, InviteLink: update.ChatMember.InviteLink,
	}))
	require.Equal(t, []string{"joined"}, repo.marked)
	require.NotContains(t, telegram.methods, "declineChatJoinRequest")
}

func TestCommunityMemberEventCannotBypassOriginalInviteOrEligibility(t *testing.T) {
	for _, scenario := range []string{"其他链接", "过期链接", "其他机器人", "其他群组", "身份已占用", "无充值资格", "账号停用"} {
		t.Run(scenario, func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			ctx := context.Background()
			state, err := s.CreateInvite(ctx, 1, CommunityInviteInput{})
			require.NoError(t, err)
			link := state.Invite.URL
			switch scenario {
			case "其他链接":
				link = "https://t.me/+unrelated"
			case "过期链接":
				repo.invite.ExpiresAt = time.Now().Add(-time.Minute)
			case "其他机器人":
				repo.invite.BotID++
			case "其他群组":
				repo.invite.GroupChatID--
			case "身份已占用":
				repo.invite.TelegramUserID = 99
			case "无充值资格":
				communityVIPSettings(t, s, true, true)
			case "账号停用":
				repo.active = false
			}
			c, shared, err := s.checkedConfiguration(ctx)
			require.NoError(t, err)
			telegram.present = true
			require.NoError(t, s.processMemberUpdate(ctx, c, shared, 90, &communityTelegramMemberUpdate{
				Chat: communityTelegramChat{ID: -100}, Date: time.Now().Unix(),
				NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"},
				InviteLink:    &communityTelegramInvite{InviteLink: link},
			}))
			require.Nil(t, repo.member)
			require.Empty(t, repo.marked)
			require.Contains(t, telegram.methods, "banChatMember")
		})
	}
}

func TestCommunityRecoveredMemberEventCannotUndoUnboundLeaveEvent(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	ctx := context.Background()
	state, err := s.CreateInvite(ctx, 1, CommunityInviteInput{})
	require.NoError(t, err)
	c, shared, err := s.checkedConfiguration(ctx)
	require.NoError(t, err)
	// 尚无身份时先收到较新的离群事件，数据库没有可更新的绑定。
	require.NoError(t, s.processMemberUpdate(ctx, c, shared, 26, &communityTelegramMemberUpdate{
		Chat: communityTelegramChat{ID: -100}, Date: 201,
		NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "left"},
	}))
	require.Nil(t, repo.member)
	// 较早的加入事件随后到达，虽然携带有效专属邀请，也必须尊重 Telegram 当前已离群的事实。
	require.NoError(t, s.processMemberUpdate(ctx, c, shared, 25, &communityTelegramMemberUpdate{
		Chat: communityTelegramChat{ID: -100}, Date: 200,
		NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"},
		InviteLink:    &communityTelegramInvite{InviteLink: state.Invite.URL},
	}))
	require.Contains(t, telegram.methods, "getChatMember")
	require.NotContains(t, telegram.methods, "banChatMember")
	require.Equal(t, []string{"left"}, repo.marked)
	require.Nil(t, repo.member)
	require.Equal(t, "revoke_pending", repo.invite.Status)
}

func TestCommunityRecoveredMemberEventRechecksMembershipAfterLookupFailure(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	ctx := context.Background()
	state, err := s.CreateInvite(ctx, 1, CommunityInviteInput{})
	require.NoError(t, err)
	c, shared, err := s.checkedConfiguration(ctx)
	require.NoError(t, err)
	update := &communityTelegramMemberUpdate{
		Chat: communityTelegramChat{ID: -100}, Date: 200,
		NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"},
		InviteLink:    &communityTelegramInvite{InviteLink: state.Invite.URL},
	}
	telegram.badMember = true
	err = s.processMemberUpdate(ctx, c, shared, 25, update)
	require.Error(t, err)
	require.False(t, communityPermanentError(err))
	require.NotNil(t, repo.member)
	require.Equal(t, "pending", repo.member.Status)
	require.Equal(t, repo.invite.ID, repo.member.AuthorizedInviteID)
	require.Empty(t, repo.marked)
	// 已经留下授权，所以重试不会再走首次认领分支，但仍必须核对实时离群状态。
	telegram.badMember = false
	require.NoError(t, s.processMemberUpdate(ctx, c, shared, 25, update))
	checks := 0
	for _, method := range telegram.methods {
		if method == "getChatMember" {
			checks++
		}
	}
	require.Equal(t, 2, checks)
	require.Equal(t, []string{"left"}, repo.marked)
	require.Nil(t, repo.member)
	require.NotContains(t, telegram.methods, "banChatMember")
}

func TestCommunityRefreshRecoversAuthorizedPendingMember(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", AuthorizedInviteID: 10, LastEventDate: 200, LastUpdateID: 25}
	repo.invite = &CommunityInvite{ID: 10, UserID: 1, TelegramUserID: 42, GroupChatID: -100, BotID: 77, Status: "active", URL: "https://t.me/+original", ExpiresAt: time.Now().Add(time.Minute)}
	telegram.present = true
	communityVIPSettings(t, s, true, true)
	repo.paid = true
	state, err := s.Get(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "joined", state.Membership.Status)
	require.NotNil(t, state.Membership.JoinedAt)
	require.False(t, state.ShowJoinPrompt)
	require.Nil(t, state.Invite)
	require.Equal(t, int64(200), repo.member.LastEventDate)
	require.Equal(t, int64(25), repo.member.LastUpdateID)
	require.Equal(t, []string{"getMe", "getChatMember", "revokeChatInviteLink"}, telegram.methods)
}

func TestCommunityRefreshNeverInventsOrReassignsMembership(t *testing.T) {
	for _, scenario := range []string{"无身份", "未授权", "其他群组", "其他用户", "尚未加入", "查询失败", "机器人变更", "旧机器人授权", "其他邀请授权", "过期邀请", "其他用户邀请", "其他身份邀请", "社群停用", "无充值资格"} {
		t.Run(scenario, func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", AuthorizedInviteID: 10}
			repo.invite = &CommunityInvite{ID: 10, UserID: 1, TelegramUserID: 42, GroupChatID: -100, BotID: 77, Status: "active", ExpiresAt: time.Now().Add(time.Minute)}
			telegram.present = true
			switch scenario {
			case "无身份":
				repo.member = nil
			case "未授权":
				repo.member.AuthorizedInviteID = 0
			case "其他群组":
				repo.member.GroupChatID--
			case "其他用户":
				repo.member.UserID++
			case "尚未加入":
				telegram.present = false
			case "查询失败":
				telegram.badMember = true
			case "机器人变更":
				telegram.botID++
			case "旧机器人授权":
				repo.invite.BotID--
			case "其他邀请授权":
				repo.invite.ID++
			case "过期邀请":
				repo.invite.ExpiresAt = time.Now().Add(-time.Minute)
			case "其他用户邀请":
				repo.invite.UserID++
			case "其他身份邀请":
				repo.invite.TelegramUserID++
			case "社群停用":
				_, err := s.UpdateSettings(context.Background(), CommunitySettings{Enabled: false, GroupChatID: "-100"})
				require.NoError(t, err)
			case "无充值资格":
				communityVIPSettings(t, s, true, true)
			}
			_, err := s.Get(context.Background(), 1)
			require.NoError(t, err)
			require.Empty(t, repo.marked)
			require.Zero(t, repo.authorizedBot)
		})
	}
}

func TestCommunityRefreshCannotOverwriteNewerLeaveEvent(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	joinedAt := time.Now().Add(-time.Hour)
	repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", AuthorizedInviteID: 10, LastEventDate: 200, LastUpdateID: 25, JoinedAt: &joinedAt}
	repo.invite = &CommunityInvite{ID: 10, UserID: 1, TelegramUserID: 42, GroupChatID: -100, BotID: 77, Status: "active", ExpiresAt: time.Now().Add(time.Minute)}
	telegram.present = true
	telegram.beforeMember = func() {
		// 使用独立记录模拟查询 Telegram 期间另一个回调已经提交了离群。
		left := *repo.member
		left.Status, left.AuthorizedInviteID, left.LastEventDate, left.LastUpdateID = "left", 0, 201, 26
		repo.member = &left
	}
	state, err := s.Get(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "left", state.Membership.Status)
	require.Empty(t, repo.marked)
}

func TestCommunityPrivateMessagesReceiveHelpWithoutBindingIdentity(t *testing.T) {
	for _, message := range []string{"/start", "/start@site_test_bot", "/help", "你好"} {
		t.Run(message, func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			c, shared, err := s.checkedConfiguration(context.Background())
			require.NoError(t, err)
			update := communityTelegramUpdate{UpdateID: 101, Message: &communityTelegramMessage{Chat: communityTelegramChat{ID: 42, Type: "private"}, From: &communityTelegramUser{ID: 42}, Text: message}}
			raw, err := json.Marshal(update)
			require.NoError(t, err)
			require.NoError(t, s.HandleTelegramWebhook(context.Background(), shared.TelegramWebhookSecret, raw))
			require.Equal(t, 1, repo.queued)
			require.NoError(t, s.processWebhook(context.Background(), c, shared, CommunityWebhookEvent{BotID: c.BotID, Payload: raw}))
			require.Len(t, telegram.messages, 1)
			require.Equal(t, "42", telegram.messages[0].ChatID)
			require.Contains(t, telegram.messages[0].Text, "专属入群链接")
			require.Nil(t, repo.member)
			require.Zero(t, repo.claimedBot)
			require.Zero(t, repo.authorizedBot)
		})
	}
}

func TestCommunityPrivateHelpPreservesTicketRepliesAndIgnoresInvalidSenders(t *testing.T) {
	for _, scenario := range []string{"回复工单", "群消息", "机器人", "缺少发送者", "发送者不匹配", "空消息"} {
		t.Run(scenario, func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			c, shared, err := s.checkedConfiguration(context.Background())
			require.NoError(t, err)
			message := &communityTelegramMessage{Chat: communityTelegramChat{ID: 42, Type: "private"}, From: &communityTelegramUser{ID: 42}, Text: "你好"}
			switch scenario {
			case "回复工单":
				message.ReplyToMessage = json.RawMessage(`{"message_id":8}`)
			case "群消息":
				message.Chat.Type = "supergroup"
			case "机器人":
				message.From.IsBot = true
			case "缺少发送者":
				message.From = nil
			case "发送者不匹配":
				message.From.ID++
			case "空消息":
				message.Text = " \n "
			}
			raw, err := json.Marshal(communityTelegramUpdate{UpdateID: 102, Message: message})
			require.NoError(t, err)
			require.NoError(t, s.HandleTelegramWebhook(context.Background(), shared.TelegramWebhookSecret, raw))
			require.NoError(t, s.processStart(context.Background(), c, shared, message))
			require.Zero(t, repo.queued)
			require.Empty(t, telegram.messages)
		})
	}
}

func TestCommunityPrivateHelpReportsExistingMembershipWithoutChangingIt(t *testing.T) {
	for _, status := range []string{"pending", "joined", "left"} {
		t.Run(status, func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: status}
			c, shared, err := s.checkedConfiguration(context.Background())
			require.NoError(t, err)
			require.NoError(t, s.processStart(context.Background(), c, shared, &communityTelegramMessage{Chat: communityTelegramChat{ID: 42, Type: "private"}, From: &communityTelegramUser{ID: 42}, Text: "/start"}))
			require.Len(t, telegram.messages, 1)
			require.Contains(t, telegram.messages[0].Text, "客服与社群")
			require.Equal(t, status, repo.member.Status)
			require.Empty(t, repo.marked)
			require.Zero(t, repo.authorizedBot)
		})
	}
}

func TestCommunityConfirmedIdentityRecoversMemberWhoseOriginalCallbacksWereMissing(t *testing.T) {
	for _, scenario := range []string{"原邀请仍有效", "原邀请已过期", "没有原邀请", "尚未实际入群"} {
		t.Run(scenario, func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			ctx := context.Background()
			communityVIPSettings(t, s, true, true)
			repo.paid = true
			if scenario != "没有原邀请" {
				_, err := s.CreateInvite(ctx, 1, CommunityInviteInput{})
				require.NoError(t, err)
				if scenario == "原邀请已过期" {
					repo.invite.ExpiresAt = time.Now().Add(-time.Minute)
				}
			}
			telegram.present = scenario != "尚未实际入群"
			verification, err := s.StartVerification(ctx, 1)
			require.NoError(t, err)
			botURL, err := url.Parse(verification.Challenge.BotURL)
			require.NoError(t, err)
			c, shared, err := s.checkedConfiguration(ctx)
			require.NoError(t, err)
			require.NoError(t, s.processStart(ctx, c, shared, &communityTelegramMessage{
				Chat: communityTelegramChat{ID: 42, Type: "private"}, From: &communityTelegramUser{ID: 42, Username: "my_account"},
				Text: "/start " + botURL.Query().Get("start"),
			}))
			// Telegram 读取身份后仍不创建会员，必须回到已登录网站明确确认。
			require.Nil(t, repo.member)
			require.Zero(t, repo.authorizedBot)
			claimed, err := s.Get(ctx, 1)
			require.NoError(t, err)
			require.Equal(t, "claimed", claimed.Challenge.Status)
			require.Equal(t, int64(42), claimed.Challenge.TelegramUserID)
			_, err = s.CreateInvite(ctx, 1, CommunityInviteInput{ChallengeID: claimed.Challenge.ID, TelegramUserID: 99})
			require.ErrorIs(t, err, ErrCommunityConflict)
			require.Nil(t, repo.member)
			state, err := s.CreateInvite(ctx, 1, CommunityInviteInput{ChallengeID: claimed.Challenge.ID, TelegramUserID: 42})
			require.NoError(t, err)
			require.Equal(t, int64(1), state.Membership.UserID)
			require.Equal(t, int64(42), state.Membership.TelegramUserID)
			if telegram.present {
				require.Equal(t, "joined", state.Membership.Status)
				require.NotNil(t, state.Membership.JoinedAt)
				require.Nil(t, state.Invite)
				require.False(t, state.ShowJoinPrompt)
			} else {
				require.Equal(t, "pending", state.Membership.Status)
				require.Nil(t, state.Membership.JoinedAt)
				require.NotNil(t, state.Invite)
				require.Zero(t, repo.authorizedBot)
			}
			require.NotContains(t, telegram.methods, "approveChatJoinRequest")
			require.NotContains(t, telegram.methods, "banChatMember")
		})
	}
}

func TestCommunityConfirmedIdentityCannotRecoverAfterEligibilityChanges(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	ctx := context.Background()
	communityVIPSettings(t, s, true, true)
	repo.paid = true
	verification, err := s.StartVerification(ctx, 1)
	require.NoError(t, err)
	repo.challenge.TelegramUserID, repo.challenge.Status = 42, "claimed"
	telegram.present = true
	telegram.beforeMember = func() { repo.paid = false }
	_, err = s.CreateInvite(ctx, 1, CommunityInviteInput{ChallengeID: verification.Challenge.ID, TelegramUserID: 42})
	require.ErrorIs(t, err, ErrCommunityVIPRequired)
	require.Equal(t, "pending", repo.member.Status)
	require.Empty(t, repo.marked)
	require.Zero(t, repo.authorizedBot)
}

func TestCommunityConfirmedIdentityCanRetryAfterTelegramLookupFails(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	ctx := context.Background()
	verification, err := s.StartVerification(ctx, 1)
	require.NoError(t, err)
	repo.challenge.TelegramUserID, repo.challenge.Status = 42, "claimed"
	telegram.present, telegram.badMember = true, true
	_, err = s.CreateInvite(ctx, 1, CommunityInviteInput{ChallengeID: verification.Challenge.ID, TelegramUserID: 42})
	require.Error(t, err)
	require.Empty(t, repo.marked)
	require.Equal(t, "pending", repo.member.Status)
	state, err := s.Get(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, state.Challenge)
	require.Equal(t, "confirmed", state.Challenge.Status)
	require.Equal(t, int64(42), state.Challenge.TelegramUserID)
	telegram.badMember = false
	state, err = s.CreateInvite(ctx, 1, CommunityInviteInput{ChallengeID: state.Challenge.ID, TelegramUserID: state.Challenge.TelegramUserID})
	require.NoError(t, err)
	require.Equal(t, "joined", state.Membership.Status)
	require.Nil(t, state.Challenge)
	require.Nil(t, state.Invite)
}

func TestCommunityConfirmedChallengeOnlyRemainsVisibleForItsPendingIdentity(t *testing.T) {
	for _, scenario := range []string{"无身份", "其他身份", "其他用户", "其他群组", "其他机器人", "已过期", "已加入"} {
		t.Run(scenario, func(t *testing.T) {
			s, repo, _ := communityTestService(t)
			repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending"}
			repo.challenge = &CommunityChallenge{ID: "retry", UserID: 1, TelegramUserID: 42, BotID: 77, Status: "confirmed", ExpiresAt: time.Now().Add(time.Minute)}
			switch scenario {
			case "无身份":
				repo.member = nil
			case "其他身份":
				repo.challenge.TelegramUserID++
			case "其他用户":
				repo.challenge.UserID++
			case "其他群组":
				repo.member.GroupChatID--
			case "其他机器人":
				repo.challenge.BotID++
			case "已过期":
				repo.challenge.ExpiresAt = time.Now().Add(-time.Minute)
			case "已加入":
				repo.member.Status = "joined"
			}
			state, err := s.Get(context.Background(), 1)
			require.NoError(t, err)
			require.Nil(t, state.Challenge)
		})
	}
}

type communityOrderedJoinRepository struct {
	*communityTestRepository
	eventDate int64
	updateID  int64
}

func (r *communityOrderedJoinRepository) AuthorizeJoin(ctx context.Context, hash string, identity CommunityTelegramIdentity, groupID, eventDate, updateID, botID int64, requirePaidRecharge bool) (*CommunityMembership, *CommunityInvite, error) {
	r.eventDate, r.updateID = eventDate, updateID
	// 与真实仓储一致：原子授权不能覆盖更新的成员事件。
	if r.member != nil && r.member.GroupChatID == groupID && (eventDate < r.member.LastEventDate || (eventDate == r.member.LastEventDate && updateID < r.member.LastUpdateID)) {
		return nil, nil, ErrCommunityConflict
	}
	return r.communityTestRepository.AuthorizeJoin(ctx, hash, identity, groupID, eventDate, updateID, botID, requirePaidRecharge)
}

func TestCommunityConfirmedIdentityCannotOverwriteLeaveDuringMemberLookup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, repo, telegram := communityTestService(t)
		ctx := context.Background()
		joinedAt := time.Now().Add(-time.Hour)
		repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", JoinedAt: &joinedAt}
		repo.invite = &CommunityInvite{ID: 10, UserID: 1, TelegramUserID: 42, GroupChatID: -100, BotID: 77, Status: "active", URLHash: communityHash("https://t.me/+original"), ExpiresAt: time.Now().Add(time.Minute)}
		ordered := &communityOrderedJoinRepository{communityTestRepository: repo}
		s.repo = ordered
		c, shared, err := s.checkedConfiguration(ctx)
		require.NoError(t, err)
		telegram.present = true
		var leaveDate int64
		telegram.beforeMember = func() {
			leaveDate = time.Now().Unix()
			left := *repo.member
			left.Status, left.LastEventDate, left.LastUpdateID = "left", leaveDate, 90
			repo.member = &left
			// 使用标准库虚拟时间模拟查询跨秒，避免真实等待或依赖机器运行速度。
			time.Sleep(time.Second)
		}
		err = s.reconcileConfirmedMembership(ctx, c, shared, repo.member, repo.invite)
		require.ErrorIs(t, err, ErrCommunityConflict)
		require.Equal(t, leaveDate, ordered.eventDate)
		require.Zero(t, ordered.updateID)
		require.Equal(t, "left", repo.member.Status)
		require.Zero(t, repo.member.AuthorizedInviteID)
		require.Empty(t, repo.marked)
	})
}
