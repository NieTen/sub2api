package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func communityVIPSettings(t *testing.T, s *CommunityService, vip, prompt bool) *CommunitySettings {
	t.Helper()
	c, err := s.GetSettings(context.Background())
	require.NoError(t, err)
	c.RequirePaidRecharge, c.LoginPromptEnabled = vip, prompt
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	require.NoError(t, s.settings.Set(context.Background(), SettingKeyCommunity, string(raw)))
	return c
}

func TestCommunityVIPHidesAllGroupDataAndRejectsUnpaidInvitations(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	communityVIPSettings(t, s, true, true)
	repo.member = &CommunityMembership{TelegramUserID: 42, GroupChatID: -100, Status: "joined"}
	repo.invite = &CommunityInvite{URL: "https://t.me/+private", Status: "active", ExpiresAt: time.Now().Add(time.Hour)}
	state, err := s.Get(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, state.RequirePaidRecharge)
	require.False(t, state.Eligible)
	require.False(t, state.Enabled)
	require.False(t, state.ShowJoinPrompt)
	require.Empty(t, state.GroupName)
	require.Empty(t, state.BotUsername)
	require.Empty(t, state.PromptKey)
	require.Nil(t, state.Invite)
	require.Nil(t, state.Challenge)
	require.Nil(t, state.Membership)
	require.Equal(t, "https://example.com/contact", state.ContactURL)
	require.Zero(t, repo.stateReads)
	_, err = s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.ErrorIs(t, err, ErrCommunityVIPRequired)
	_, err = s.StartVerification(context.Background(), 1)
	require.ErrorIs(t, err, ErrCommunityVIPRequired)
	require.Empty(t, telegram.methods)
}

func TestCommunityVIPPromptDependsOnPaidHistorySettingsAndCurrentMembership(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		vip, prompt, joined, wantPrompt bool
	}{
		{name: "成功充值未加入提示", vip: true, prompt: true, wantPrompt: true},
		{name: "已加入不提示", vip: true, prompt: true, joined: true},
		{name: "关闭提示", vip: true},
		{name: "普通群不弹VIP", prompt: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			c := communityVIPSettings(t, s, tc.vip, tc.prompt)
			repo.paid = true
			if tc.joined {
				repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "joined"}
			}
			state, err := s.Get(context.Background(), 1)
			require.NoError(t, err)
			require.True(t, state.Eligible)
			require.True(t, state.Enabled)
			require.Equal(t, tc.wantPrompt, state.ShowJoinPrompt)
			if tc.wantPrompt {
				require.Equal(t, communityHash("77:"+c.GroupChatID), state.PromptKey)
			}
			require.Empty(t, telegram.methods)
			if !tc.vip {
				require.Zero(t, repo.paidChecks)
			}
		})
	}
}

func TestCommunityVIPPaidUserCanClaimAndJoinWithoutPreVerification(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	communityVIPSettings(t, s, true, true)
	repo.paid = true
	state, err := s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.NoError(t, err)
	require.NotNil(t, state.Invite)
	require.Nil(t, state.Membership)
	require.True(t, state.ShowJoinPrompt)
	c, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	request := &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: time.Now().Unix(), InviteLink: &communityTelegramInvite{InviteLink: state.Invite.URL}}
	require.NoError(t, s.processJoinRequest(context.Background(), c, shared, 20, request))
	require.Contains(t, telegram.methods, "approveChatJoinRequest")
	require.Equal(t, "joined", repo.member.Status)
	state, err = s.Get(context.Background(), 1)
	require.NoError(t, err)
	require.False(t, state.ShowJoinPrompt)
}

func TestCommunityVIPLatestPolicyRejectsOldInvitationBeforeClaim(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	state, err := s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.NoError(t, err)
	oldConfig, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	communityVIPSettings(t, s, true, true)
	update := communityTelegramUpdate{UpdateID: 25, ChatJoinRequest: &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: time.Now().Unix(), InviteLink: &communityTelegramInvite{InviteLink: state.Invite.URL}}}
	raw, err := json.Marshal(update)
	require.NoError(t, err)
	require.NoError(t, s.processWebhook(context.Background(), oldConfig, shared, CommunityWebhookEvent{BotID: 77, Payload: raw}))
	require.Nil(t, repo.member)
	require.Contains(t, telegram.methods, "declineChatJoinRequest")
	require.NotContains(t, telegram.methods, "approveChatJoinRequest")
	require.NotContains(t, telegram.methods, "banChatMember")
}

func TestCommunityVIPPaymentLookupErrorsNeverBecomeRejectionsOrKicks(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	state, err := s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.NoError(t, err)
	c := communityVIPSettings(t, s, true, true)
	_, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	dbErr := errors.New("支付记录查询暂时失败")
	repo.paidErr = dbErr
	_, err = s.Get(context.Background(), 1)
	require.ErrorIs(t, err, dbErr)
	_, err = s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.ErrorIs(t, err, dbErr)
	request := &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: time.Now().Unix(), InviteLink: &communityTelegramInvite{InviteLink: state.Invite.URL}}
	require.ErrorIs(t, s.processJoinRequest(context.Background(), c, shared, 25, request), dbErr)
	require.False(t, communityPermanentError(dbErr))
	require.Nil(t, repo.member)
	repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", AuthorizedInviteID: 10}
	telegram.present = true
	memberUpdate := &communityTelegramMemberUpdate{Chat: communityTelegramChat{ID: -100}, Date: time.Now().Unix(), NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"}}
	require.ErrorIs(t, s.processMemberUpdate(context.Background(), c, shared, 26, memberUpdate), dbErr)
	expired := CommunityInvite{ID: 10, BotID: 77, UserID: 1, TelegramUserID: 42, GroupChatID: -100, ExpiresAt: time.Now().Add(-time.Minute)}
	require.ErrorIs(t, s.reconcileExpiringInvite(context.Background(), c, shared, expired), dbErr)
	require.Empty(t, repo.marked)
	require.NotContains(t, telegram.methods, "approveChatJoinRequest")
	require.NotContains(t, telegram.methods, "declineChatJoinRequest")
	require.NotContains(t, telegram.methods, "banChatMember")
}

func TestCommunityVIPPendingReservationIsReleasedWhenOldPolicyChanges(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	state, err := s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.NoError(t, err)
	oldConfig, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	communityVIPSettings(t, s, true, true)
	request := &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: time.Now().Unix(), InviteLink: &communityTelegramInvite{InviteLink: state.Invite.URL}}
	// 模拟已经取出旧策略的在途任务；批准之前的复查必须释放刚建立的临时身份。
	require.NoError(t, s.processJoinRequest(context.Background(), oldConfig, shared, 25, request))
	require.Nil(t, repo.member)
	require.Contains(t, telegram.methods, "declineChatJoinRequest")
	require.NotContains(t, telegram.methods, "approveChatJoinRequest")
}

func TestCommunityVIPSettingsPersistWithoutChangingOrdinaryGroupDefaults(t *testing.T) {
	s, _, _ := communityTestService(t)
	c, err := s.GetSettings(context.Background())
	require.NoError(t, err)
	require.False(t, c.RequirePaidRecharge)
	require.False(t, c.LoginPromptEnabled)
	c.RequirePaidRecharge, c.LoginPromptEnabled = true, true
	_, err = s.UpdateSettings(context.Background(), *c)
	require.NoError(t, err)
	stored, err := s.GetSettings(context.Background())
	require.NoError(t, err)
	require.True(t, stored.RequirePaidRecharge)
	require.True(t, stored.LoginPromptEnabled)
}

func TestCommunityVIPMemberAndExpiryEventsCannotCompleteUnpaidJoin(t *testing.T) {
	for _, action := range []string{"成员事件", "过期补偿"} {
		t.Run(action, func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			c := communityVIPSettings(t, s, true, true)
			_, shared, err := s.checkedConfiguration(context.Background())
			require.NoError(t, err)
			repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", AuthorizedInviteID: 10}
			telegram.present = true
			if action == "成员事件" {
				update := &communityTelegramMemberUpdate{Chat: communityTelegramChat{ID: -100}, Date: time.Now().Unix(), NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"}}
				err = s.processMemberUpdate(context.Background(), c, shared, 30, update)
			} else {
				err = s.reconcileExpiringInvite(context.Background(), c, shared, CommunityInvite{ID: 10, BotID: 77, UserID: 1, TelegramUserID: 42, GroupChatID: -100, ExpiresAt: time.Now().Add(-time.Minute)})
			}
			require.NoError(t, err)
			require.Nil(t, repo.member)
			require.Equal(t, []string{"left"}, repo.marked)
			require.Contains(t, telegram.methods, "banChatMember")
		})
	}
}
