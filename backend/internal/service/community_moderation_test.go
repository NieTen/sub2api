package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func communityBoundMember() *CommunityMembership {
	boundAt := time.Now().Add(-time.Hour)
	return &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "joined", JoinedAt: &boundAt, BoundAt: &boundAt, AuthorizedInviteID: 10, LastEventDate: 100, LastUpdateID: 10}
}

func TestCommunityRemovalSeparatesSelfLeaveAdministratorKickAndServiceCleanup(t *testing.T) {
	for _, tc := range []struct {
		name       string
		actorID    int64
		status     string
		wantStatus string
	}{
		{name: "本人自行退出", actorID: 42, status: "left", wantStatus: "left"},
		{name: "管理员永久移出", actorID: 99, status: "kicked", wantStatus: "banned"},
		{name: "管理员移出但未永久封禁", actorID: 99, status: "left", wantStatus: "banned"},
		{name: "服务自身临时清理", actorID: 77, status: "kicked", wantStatus: "left"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo, _ := communityTestService(t)
			repo.member = communityBoundMember()
			c, shared, err := s.checkedConfiguration(context.Background())
			require.NoError(t, err)
			update := communityTelegramUpdate{UpdateID: 20, ChatMember: &communityTelegramMemberUpdate{
				Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: tc.actorID}, Date: 200,
				OldChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"},
				NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: tc.status},
			}}
			raw, err := json.Marshal(update)
			require.NoError(t, err)
			require.NoError(t, s.processWebhook(context.Background(), c, shared, CommunityWebhookEvent{BotID: c.BotID, Payload: raw}))
			require.NotNil(t, repo.member)
			require.Equal(t, tc.wantStatus, repo.member.Status)
			require.Equal(t, int64(42), repo.member.TelegramUserID)
			require.NotNil(t, repo.member.BoundAt)
			state, err := s.Get(context.Background(), 1)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus == "banned", state.Banned)
			if tc.wantStatus == "banned" {
				require.Nil(t, state.Invite)
				require.False(t, state.ShowJoinPrompt)
				_, err = s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
				require.ErrorIs(t, err, ErrCommunityBanned)
			}
		})
	}
}

func TestCommunitySelfLeaveOnlyAllowsOriginalBoundIdentityToRejoin(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	repo.member = communityBoundMember()
	c, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	require.NoError(t, s.processMemberUpdate(context.Background(), c, shared, 20, &communityTelegramMemberUpdate{
		Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: 200,
		OldChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"},
		NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "left"},
	}))
	state, err := s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.NoError(t, err)
	require.NotNil(t, state.Invite)
	require.Equal(t, int64(42), repo.invite.TelegramUserID)
	request := &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 43}, Date: 201, InviteLink: &communityTelegramInvite{InviteLink: state.Invite.URL}}
	require.NoError(t, s.processJoinRequest(context.Background(), c, shared, 21, request))
	require.NotContains(t, telegram.methods, "approveChatJoinRequest")
	require.Equal(t, int64(42), repo.member.TelegramUserID)
	request.From.ID = 42
	require.NoError(t, s.processJoinRequest(context.Background(), c, shared, 22, request))
	require.Equal(t, "joined", repo.member.Status)
	require.Equal(t, int64(42), repo.member.TelegramUserID)
	_, err = s.StartVerification(context.Background(), 1)
	require.ErrorIs(t, err, ErrCommunityConflict)
}

func TestCommunityConfirmedBindingSurvivesExpiredInviteAndUserCannotReplaceIt(t *testing.T) {
	s, repo, _ := communityTestService(t)
	boundAt := time.Now().Add(-time.Hour)
	repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", BoundAt: &boundAt}
	repo.invite = &CommunityInvite{ID: 1, UserID: 1, BotID: 77, TelegramUserID: 42, GroupChatID: -100, Status: "active", ExpiresAt: time.Now().Add(-time.Minute)}
	state, err := s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.NoError(t, err)
	require.NotNil(t, state.Membership)
	require.Equal(t, int64(42), state.Membership.TelegramUserID)
	require.Equal(t, int64(42), repo.invite.TelegramUserID)
	require.Empty(t, repo.marked)
	_, err = s.StartVerification(context.Background(), 1)
	require.ErrorIs(t, err, ErrCommunityConflict)
}

func TestCommunityAdministratorKickSurvivesUnbanAndOlderEventDelivery(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	repo.member = communityBoundMember()
	c, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	// 后到的管理员封禁记录仍需生效，即使先处理过更晚的 Telegram 解封。
	unban := &communityTelegramMemberUpdate{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 99}, Date: 201,
		OldChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "kicked"},
		NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "left"}}
	require.NoError(t, s.processMemberUpdate(context.Background(), c, shared, 21, unban))
	kick := &communityTelegramMemberUpdate{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 99}, Date: 200,
		OldChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"},
		NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "kicked"}}
	require.NoError(t, s.processMemberUpdate(context.Background(), c, shared, 20, kick))
	require.Equal(t, "banned", repo.member.Status)
	require.Equal(t, int64(201), repo.member.LastEventDate)
	unban.Date = 202
	require.NoError(t, s.processMemberUpdate(context.Background(), c, shared, 22, unban))
	require.Equal(t, "banned", repo.member.Status)
	telegram.present = true
	require.NoError(t, s.processMemberUpdate(context.Background(), c, shared, 23, &communityTelegramMemberUpdate{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 99}, Date: 203,
		NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"}}))
	require.Equal(t, "banned", repo.member.Status)
	require.Len(t, telegram.bans, 1)
	require.Zero(t, telegram.bans[0].UntilDate)
}

func TestCommunityKickBeforeWebsiteBindingStillBlocksTelegramIdentity(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	c, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	require.NoError(t, s.processMemberUpdate(context.Background(), c, shared, 20, &communityTelegramMemberUpdate{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 99}, Date: 200,
		OldChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"},
		NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "kicked"}}))
	require.Nil(t, repo.member)
	state, err := s.CreateInvite(context.Background(), 2, CommunityInviteInput{})
	require.NoError(t, err)
	require.NoError(t, s.processJoinRequest(context.Background(), c, shared, 21, &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: 201, InviteLink: &communityTelegramInvite{InviteLink: state.Invite.URL}}))
	require.Nil(t, repo.member)
	require.NotContains(t, telegram.methods, "approveChatJoinRequest")
	require.Contains(t, telegram.methods, "declineChatJoinRequest")
}

func TestCommunityUnbindRequiresAdministratorAndOwnersUnusedTicket(t *testing.T) {
	s, repo, _ := communityTestService(t)
	repo.member = communityBoundMember()
	repo.ticketOwners = map[int64]int64{10: 2, 11: 1}
	for _, actor := range []SupportTicketActor{{UserID: 1}, {IsAdmin: true}} {
		_, err := s.Unbind(context.Background(), actor, 1, 11)
		require.ErrorIs(t, err, ErrCommunityForbidden)
	}
	require.Zero(t, repo.unbindCalls)
	_, err := s.Unbind(context.Background(), SupportTicketActor{UserID: 99, IsAdmin: true}, 1, 0)
	require.ErrorIs(t, err, ErrCommunityUnbindTicket)
	_, err = s.Unbind(context.Background(), SupportTicketActor{UserID: 99, IsAdmin: true}, 1, 10)
	require.ErrorIs(t, err, ErrCommunityUnbindTicket)
	require.NotNil(t, repo.member)
	result, err := s.Unbind(context.Background(), SupportTicketActor{UserID: 99, IsAdmin: true}, 1, 11)
	require.NoError(t, err)
	require.True(t, result.Unbound)
	require.Equal(t, int64(42), result.TelegramUserID)
	require.Nil(t, repo.member)
	repo.member = communityBoundMember()
	_, err = s.Unbind(context.Background(), SupportTicketActor{UserID: 99, IsAdmin: true}, 1, 11)
	require.ErrorIs(t, err, ErrCommunityUnbindTicket)
	require.NotNil(t, repo.member)
}

func TestCommunityUnbindDoesNotClearAccountOrTelegramBan(t *testing.T) {
	s, repo, _ := communityTestService(t)
	repo.member = communityBoundMember()
	repo.ticketOwners = map[int64]int64{11: 1}
	require.NoError(t, repo.RecordMemberRemoval(context.Background(), 42, -100, 200, 20, 99, true))
	result, err := s.Unbind(context.Background(), SupportTicketActor{UserID: 99, IsAdmin: true}, 1, 11)
	require.NoError(t, err)
	require.True(t, result.Banned)
	require.Nil(t, repo.member)
	require.ErrorIs(t, repo.CheckAdmission(context.Background(), 0, 42, -100), ErrCommunityBanned)
	state, err := s.Get(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, state.Banned)
	require.Nil(t, state.Invite)
	require.Nil(t, state.Membership)
	_, err = s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.ErrorIs(t, err, ErrCommunityBanned)
	_, err = s.StartVerification(context.Background(), 1)
	require.ErrorIs(t, err, ErrCommunityBanned)
}

func TestCommunityVIPIneligibleUserStillSeesBanBeforeAndAfterUnbinding(t *testing.T) {
	for _, unbound := range []bool{false, true} {
		t.Run(map[bool]string{false: "仍然绑定", true: "管理员已解除绑定"}[unbound], func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			communityVIPSettings(t, s, true, true)
			repo.member = communityBoundMember()
			repo.ticketOwners = map[int64]int64{11: 1}
			require.NoError(t, repo.RecordMemberRemoval(context.Background(), 42, -100, 200, 20, 99, true))
			if unbound {
				_, err := s.Unbind(context.Background(), SupportTicketActor{UserID: 99, IsAdmin: true}, 1, 11)
				require.NoError(t, err)
			}
			// 禁入提示独立于充值资格，且不能因此暴露无资格用户的 VIP 群资料。
			state, err := s.Get(context.Background(), 1)
			require.NoError(t, err)
			require.True(t, state.Banned)
			require.True(t, state.RequirePaidRecharge)
			require.False(t, state.Eligible)
			require.False(t, state.Enabled)
			require.False(t, state.ShowJoinPrompt)
			require.Empty(t, state.GroupName)
			require.Empty(t, state.BotUsername)
			require.Nil(t, state.Invite)
			require.Nil(t, state.Challenge)
			require.Nil(t, state.Membership)
			_, err = s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
			require.ErrorIs(t, err, ErrCommunityBanned)
			_, err = s.StartVerification(context.Background(), 1)
			require.ErrorIs(t, err, ErrCommunityBanned)
			require.Empty(t, telegram.methods)
		})
	}
}

func TestCommunityPrivateHelpReportsBanBeforeAndAfterUnbinding(t *testing.T) {
	for _, unbound := range []bool{false, true} {
		t.Run(map[bool]string{false: "仍然绑定", true: "管理员已解除绑定"}[unbound], func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			repo.member = communityBoundMember()
			repo.ticketOwners = map[int64]int64{11: 1}
			require.NoError(t, repo.RecordMemberRemoval(context.Background(), 42, -100, 200, 20, 99, true))
			if unbound {
				_, err := s.Unbind(context.Background(), SupportTicketActor{UserID: 99, IsAdmin: true}, 1, 11)
				require.NoError(t, err)
			}
			c, shared, err := s.checkedConfiguration(context.Background())
			require.NoError(t, err)
			require.NoError(t, s.processStart(context.Background(), c, shared, &communityTelegramMessage{Chat: communityTelegramChat{ID: 42, Type: "private"}, From: &communityTelegramUser{ID: 42}, Text: "/start"}))
			require.Len(t, telegram.messages, 1)
			require.Contains(t, telegram.messages[0].Text, "禁止重新加入")
			require.Contains(t, telegram.messages[0].Text, "解除网站绑定不会取消禁入")
			require.NotContains(t, telegram.messages[0].Text, "领取")
		})
	}
}
