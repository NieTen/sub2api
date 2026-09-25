package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type communitySingleUseTelegram struct {
	*communityTestTelegram
	revokeStatus        int
	revokeErr           error
	revoked             []communityRevokeInviteRequest
	membershipAtRevoke  []string
	inviteStateAtRevoke []string
}

func (r *communitySingleUseTelegram) RoundTrip(request *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(request.URL.Path, "/revokeChatInviteLink") {
		return r.communityTestTelegram.RoundTrip(request)
	}
	r.methods = append(r.methods, "revokeChatInviteLink")
	var input communityRevokeInviteRequest
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		return nil, err
	}
	r.revoked = append(r.revoked, input)
	r.membershipAtRevoke = append(r.membershipAtRevoke, r.repo.member.Status)
	r.inviteStateAtRevoke = append(r.inviteStateAtRevoke, r.repo.invite.Status)
	if r.revokeErr != nil {
		return nil, r.revokeErr
	}
	status := r.revokeStatus
	if status == 0 {
		status = http.StatusOK
	}
	raw, _ := json.Marshal(map[string]any{"ok": status == http.StatusOK, "result": true})
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
}

type communitySingleUseRepository struct {
	*communityTestRepository
}

func (r *communitySingleUseRepository) ClaimRevocations(context.Context, int, time.Duration) ([]CommunityInvite, error) {
	items := r.revocations
	r.revocations = nil
	return items, nil
}

func TestCommunityEverySuccessfulJoinImmediatelyRevokesItsOwnInvite(t *testing.T) {
	for _, path := range []string{"申请自动批准", "成员事件恢复", "成员事件重试", "刷新等待状态", "确认身份恢复", "已授权身份确认"} {
		for _, outcome := range []string{"撤销成功", "链接已失效", "Telegram服务失败", "连接失败"} {
			t.Run(path+"/"+outcome, func(t *testing.T) {
				s, repo, baseTelegram := communityTestService(t)
				telegram := &communitySingleUseTelegram{communityTestTelegram: baseTelegram}
				s.delivery.client.Transport = telegram
				telegram.present = true
				switch outcome {
				case "链接已失效":
					telegram.revokeStatus = http.StatusBadRequest
				case "Telegram服务失败":
					telegram.revokeStatus = http.StatusServiceUnavailable
				case "连接失败":
					telegram.revokeErr = errors.New("模拟 Telegram 连接失败")
				}
				ctx := context.Background()
				c, shared, err := s.checkedConfiguration(ctx)
				require.NoError(t, err)
				repo.invite = &CommunityInvite{ID: 10, UserID: 1, BotID: 77, GroupChatID: -100, URL: "https://t.me/+personal_link", URLHash: communityHash("https://t.me/+personal_link"), Status: "active", ExpiresAt: time.Now().Add(time.Minute)}
				if path == "成员事件重试" || path == "刷新等待状态" || path == "已授权身份确认" {
					repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", AuthorizedInviteID: 10, LastEventDate: 200, LastUpdateID: 25}
					repo.invite.TelegramUserID = 42
				}
				switch path {
				case "申请自动批准":
					telegram.present = false
					err = s.processJoinRequest(ctx, c, shared, 25, &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: 200, InviteLink: &communityTelegramInvite{InviteLink: repo.invite.URL}})
				case "成员事件恢复", "成员事件重试":
					err = s.processMemberUpdate(ctx, c, shared, 25, &communityTelegramMemberUpdate{Chat: communityTelegramChat{ID: -100}, Date: 200, NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"}, InviteLink: &communityTelegramInvite{InviteLink: repo.invite.URL}})
				case "刷新等待状态":
					_, err = s.Get(ctx, 1)
				case "确认身份恢复":
					repo.challenge = &CommunityChallenge{ID: "confirmed-identity", UserID: 1, BotID: 77, TelegramUserID: 42, Status: "claimed", ExpiresAt: time.Now().Add(time.Minute)}
					_, err = s.CreateInvite(ctx, 1, CommunityInviteInput{ChallengeID: repo.challenge.ID, TelegramUserID: 42})
				case "已授权身份确认":
					err = s.reconcileConfirmedMembership(ctx, c, shared, repo.member, repo.invite)
				}
				require.NoError(t, err)
				require.Equal(t, []communityRevokeInviteRequest{{ChatID: "-100", InviteLink: "https://t.me/+personal_link"}}, telegram.revoked)
				require.Equal(t, []string{"joined"}, telegram.membershipAtRevoke)
				require.Equal(t, []string{"revoke_pending"}, telegram.inviteStateAtRevoke)
				require.Equal(t, "revoke_pending", repo.invite.Status)
				require.Zero(t, repo.completedRevocations)
				// 远端撤销失败不能把已提交的成功入群变成 API 错误，也不能再返回可复用链接。
				state, err := s.Get(ctx, 1)
				require.NoError(t, err)
				require.Equal(t, "joined", state.Membership.Status)
				require.Equal(t, int64(42), state.Membership.TelegramUserID)
				require.Nil(t, state.Invite)
			})
		}
	}
}

func TestCommunityFailedImmediateRevocationStillRetriesInWorker(t *testing.T) {
	s, repo, baseTelegram := communityTestService(t)
	telegram := &communitySingleUseTelegram{communityTestTelegram: baseTelegram, revokeStatus: http.StatusServiceUnavailable}
	s.delivery.client.Transport = telegram
	s.repo = &communitySingleUseRepository{communityTestRepository: repo}
	ctx := context.Background()
	state, err := s.CreateInvite(ctx, 1, CommunityInviteInput{})
	require.NoError(t, err)
	c, shared, err := s.checkedConfiguration(ctx)
	require.NoError(t, err)
	require.NoError(t, s.processJoinRequest(ctx, c, shared, 25, &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: 200, InviteLink: &communityTelegramInvite{InviteLink: state.Invite.URL}}))
	require.Equal(t, "joined", repo.member.Status)
	require.Equal(t, "revoke_pending", repo.invite.Status)
	require.Len(t, telegram.revoked, 1)
	telegram.revokeStatus = http.StatusOK
	repo.revocations = []CommunityInvite{*repo.invite}
	s.process(ctx)
	require.Len(t, telegram.revoked, 2)
	require.Equal(t, 1, repo.completedRevocations)
	require.Zero(t, repo.failedRevocations)
	require.Equal(t, "joined", repo.member.Status)
}

func TestCommunityReusedInviteCannotReplaceFirstMemberWhenRevocationFails(t *testing.T) {
	s, repo, baseTelegram := communityTestService(t)
	telegram := &communitySingleUseTelegram{communityTestTelegram: baseTelegram, revokeStatus: http.StatusServiceUnavailable}
	s.delivery.client.Transport = telegram
	ctx := context.Background()
	state, err := s.CreateInvite(ctx, 1, CommunityInviteInput{})
	require.NoError(t, err)
	c, shared, err := s.checkedConfiguration(ctx)
	require.NoError(t, err)
	request := &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: 200, InviteLink: &communityTelegramInvite{InviteLink: state.Invite.URL}}
	require.NoError(t, s.processJoinRequest(ctx, c, shared, 25, request))
	request.From.ID = 99
	require.NoError(t, s.processJoinRequest(ctx, c, shared, 26, request))
	require.Equal(t, int64(42), repo.member.TelegramUserID)
	require.Equal(t, int64(42), repo.invite.TelegramUserID)
	require.Equal(t, "joined", repo.member.Status)
	require.Equal(t, []string{"joined"}, repo.marked)
	require.Contains(t, telegram.methods, "declineChatJoinRequest")
	// 首个合法成员的迟到请求不重复批准，更不能被误踢。
	request.From.ID, request.Date, request.InviteLink = 42, 199, nil
	require.NoError(t, s.processJoinRequest(ctx, c, shared, 24, request))
	require.NotContains(t, telegram.methods, "banChatMember")
	require.Len(t, telegram.revoked, 1)
}

func TestCommunityImmediateRevocationNeverTargetsMismatchedInvite(t *testing.T) {
	for _, mismatch := range []string{"邀请ID", "网站用户", "Telegram身份", "群组", "机器人", "非法链接"} {
		t.Run(mismatch, func(t *testing.T) {
			s, repo, baseTelegram := communityTestService(t)
			telegram := &communitySingleUseTelegram{communityTestTelegram: baseTelegram}
			s.delivery.client.Transport = telegram
			c, shared, err := s.checkedConfiguration(context.Background())
			require.NoError(t, err)
			repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", AuthorizedInviteID: 10}
			repo.invite = &CommunityInvite{ID: 10, UserID: 1, TelegramUserID: 42, BotID: 77, GroupChatID: -100, URL: "https://t.me/+personal_link", Status: "active", ExpiresAt: time.Now().Add(time.Minute)}
			candidate := *repo.invite
			switch mismatch {
			case "邀请ID":
				candidate.ID++
			case "网站用户":
				candidate.UserID++
			case "Telegram身份":
				candidate.TelegramUserID++
			case "群组":
				candidate.GroupChatID--
			case "机器人":
				candidate.BotID++
			case "非法链接":
				candidate.URL = "https://example.com/other"
			}
			require.NoError(t, s.completeCommunityJoin(context.Background(), c, shared, repo.member, &candidate, 200, 25))
			require.Empty(t, telegram.revoked)
			require.Equal(t, "joined", repo.member.Status)
			require.Equal(t, "revoke_pending", repo.invite.Status)
		})
	}
}

func TestCommunityImmediateRevocationOnlyRunsAfterSuccessfulMembershipCommit(t *testing.T) {
	s, repo, baseTelegram := communityTestService(t)
	telegram := &communitySingleUseTelegram{communityTestTelegram: baseTelegram}
	s.delivery.client.Transport = telegram
	c, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "left", AuthorizedInviteID: 10, LastEventDate: 201, LastUpdateID: 26}
	repo.invite = &CommunityInvite{ID: 10, UserID: 1, TelegramUserID: 42, BotID: 77, GroupChatID: -100, URL: "https://t.me/+personal_link", Status: "active", ExpiresAt: time.Now().Add(time.Minute)}
	err = s.completeCommunityJoin(context.Background(), c, shared, repo.member, repo.invite, 200, 25)
	require.ErrorIs(t, err, ErrCommunityConflict)
	require.Empty(t, telegram.revoked)
	require.Equal(t, "left", repo.member.Status)
	require.Equal(t, "active", repo.invite.Status)
}
