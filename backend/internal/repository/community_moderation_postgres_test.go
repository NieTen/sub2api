package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func communityPostgresConfirm(t *testing.T, ctx context.Context, repo *communityRepository, userID, telegramID int64) (*service.CommunityMembership, error) {
	t.Helper()
	challenge := &service.CommunityChallenge{ID: uuid.NewString(), UserID: userID, BotID: 77, TokenHash: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, repo.CreateChallenge(ctx, challenge))
	_, err := repo.ClaimChallenge(ctx, challenge.TokenHash, 77, service.CommunityTelegramIdentity{ID: telegramID})
	require.NoError(t, err)
	return repo.ConfirmChallenge(ctx, userID, challenge.ID, telegramID, -100, 77)
}

func TestCommunityPostgresConfirmedBindingSurvivesLeavingBeforeJoining(t *testing.T) {
	repo, ctx := communityPostgresRepository(t)
	member, err := communityPostgresConfirm(t, ctx, repo, 1, 101)
	require.NoError(t, err)
	require.NotNil(t, member.BoundAt)
	require.Nil(t, member.JoinedAt)
	require.NoError(t, repo.RecordMemberRemoval(ctx, 101, -100, 200, 10, 101, false))
	left, _, _, err := repo.GetState(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, left)
	require.Equal(t, "left", left.Status)
	require.Equal(t, member.BoundAt, left.BoundAt)
	_, err = communityPostgresConfirm(t, ctx, repo, 2, 101)
	require.ErrorIs(t, err, service.ErrCommunityConflict, "自行离群不能让其他网站账号抢绑")
	_, err = communityPostgresConfirm(t, ctx, repo, 1, 102)
	require.ErrorIs(t, err, service.ErrCommunityConflict, "已确认绑定不能直接换为另一 Telegram 账号")
}

func TestCommunityPostgresModeratorBanSurvivesUnbanAndOldEvents(t *testing.T) {
	repo, ctx := communityPostgresRepository(t)
	_, err := communityPostgresConfirm(t, ctx, repo, 1, 101)
	require.NoError(t, err)
	invite := communityPostgresInvite(t, ctx, repo, 1)
	_, _, err = repo.AuthorizeJoin(ctx, invite.URLHash, service.CommunityTelegramIdentity{ID: 101}, -100, 100, 1, 77, false)
	require.NoError(t, err)
	require.NoError(t, repo.MarkMembership(ctx, 101, -100, "joined", 100, 1))
	// 先到的新离群事件不应吃掉随后到达的管理员踢出证据。
	require.NoError(t, repo.RecordMemberRemoval(ctx, 101, -100, 300, 30, 101, false))
	require.NoError(t, repo.RecordMemberRemoval(ctx, 101, -100, 200, 20, 900, true))
	require.NoError(t, repo.RecordMemberRemoval(ctx, 101, -100, 400, 40, 900, false))
	banned, _, activeInvite, err := repo.GetState(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "banned", banned.Status)
	require.EqualValues(t, 300, banned.LastEventDate)
	require.EqualValues(t, 30, banned.LastUpdateID)
	require.Nil(t, activeInvite)
	require.ErrorIs(t, repo.CheckAdmission(ctx, 1, 0, -100), service.ErrCommunityBanned)
	require.ErrorIs(t, repo.CheckAdmission(ctx, 0, 101, -100), service.ErrCommunityBanned)
	require.NoError(t, repo.CheckAdmission(ctx, 1, 101, -200), "禁入限定原群")
	require.ErrorIs(t, repo.MarkMembership(ctx, 101, -100, "joined", 500, 50), service.ErrCommunityBanned)
	require.ErrorIs(t, repo.MarkMembership(ctx, 101, -100, "left", 500, 50), service.ErrCommunityBanned)
	_, err = repo.AcquireInviteLease(ctx, 1, -100, "blocked", time.Minute)
	require.ErrorIs(t, err, service.ErrCommunityBanned)
	require.ErrorIs(t, repo.SaveInvite(ctx, invite, "blocked"), service.ErrCommunityBanned)
	_, _, err = repo.AuthorizeJoin(ctx, invite.URLHash, service.CommunityTelegramIdentity{ID: 101}, -100, 500, 50, 77, false)
	require.ErrorIs(t, err, service.ErrCommunityBanned)
	_, err = communityPostgresConfirm(t, ctx, repo, 2, 101)
	require.ErrorIs(t, err, service.ErrCommunityBanned)
	page, err := repo.ListMembers(ctx, -100, 77, service.CommunityMemberFilter{Page: 1, PageSize: 20, Status: "banned"})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Summary.Banned)
	require.Len(t, page.Items, 1)
	require.EqualValues(t, 1, page.Items[0].UserID)
}

func TestCommunityPostgresUnknownTelegramBanBlocksLaterBinding(t *testing.T) {
	repo, ctx := communityPostgresRepository(t)
	require.NoError(t, repo.RecordMemberRemoval(ctx, 101, -100, 200, 20, 900, true))
	require.NoError(t, repo.RecordMemberRemoval(ctx, 101, -100, 201, 21, 900, false))
	require.ErrorIs(t, repo.CheckAdmission(ctx, 0, 101, -100), service.ErrCommunityBanned)
	_, err := communityPostgresConfirm(t, ctx, repo, 1, 101)
	require.ErrorIs(t, err, service.ErrCommunityBanned)
	state, _, _, err := repo.GetState(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, state)
	// 机器人短踢或用户主动离开没有永久禁入副作用。
	require.NoError(t, repo.RecordMemberRemoval(ctx, 102, -100, 300, 30, 77, false))
	require.NoError(t, repo.CheckAdmission(ctx, 0, 102, -100))
}

func communityPostgresTicket(t *testing.T, ctx context.Context, repo *communityRepository, userID int64, created time.Time) int64 {
	t.Helper()
	var id int64
	require.NoError(t, repo.db.QueryRowContext(ctx, `INSERT INTO support_tickets(user_id,subject,created_at) VALUES($1,'本人申请解除社群绑定',$2) RETURNING id`, userID, created).Scan(&id))
	return id
}

func TestCommunityPostgresUnbindRequiresAdminAndFreshOwnedUnusedTicket(t *testing.T) {
	repo, ctx := communityPostgresRepository(t)
	_, err := repo.db.ExecContext(ctx, `INSERT INTO users(id,status,role) VALUES(3,'active','admin')`)
	require.NoError(t, err)
	_, err = communityPostgresConfirm(t, ctx, repo, 1, 101)
	require.NoError(t, err)
	invite := communityPostgresInvite(t, ctx, repo, 1)
	wrong := communityPostgresTicket(t, ctx, repo, 2, time.Now())
	old := communityPostgresTicket(t, ctx, repo, 1, time.Now().Add(-time.Hour))
	valid := communityPostgresTicket(t, ctx, repo, 1, time.Now())
	_, err = repo.Unbind(ctx, 2, 1, valid)
	require.ErrorIs(t, err, service.ErrCommunityForbidden)
	_, err = repo.Unbind(ctx, 3, 1, wrong)
	require.ErrorIs(t, err, service.ErrCommunityUnbindTicket)
	_, err = repo.Unbind(ctx, 3, 1, old)
	require.ErrorIs(t, err, service.ErrCommunityUnbindTicket)
	result, err := repo.Unbind(ctx, 3, 1, valid)
	require.NoError(t, err)
	require.True(t, result.Unbound)
	require.False(t, result.Banned)
	require.EqualValues(t, 101, result.TelegramUserID)
	member, challenge, activeInvite, err := repo.GetState(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, member)
	require.Nil(t, challenge)
	require.Nil(t, activeInvite)
	var status string
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT status FROM community_invites WHERE id=$1`, invite.ID).Scan(&status))
	require.Equal(t, "revoke_pending", status)
	_, err = communityPostgresConfirm(t, ctx, repo, 2, 101)
	require.NoError(t, err, "管理员完成解绑后原 Telegram 可以重新绑定")
	_, err = communityPostgresConfirm(t, ctx, repo, 1, 102)
	require.NoError(t, err)
	// 将工单时间调到本次绑定后，仍必须被消费审计挡住，不能只靠时间检查。
	_, err = repo.db.ExecContext(ctx, `UPDATE support_tickets SET created_at=NOW()+INTERVAL '1 minute' WHERE id=$1`, valid)
	require.NoError(t, err)
	_, err = repo.Unbind(ctx, 3, 1, valid)
	require.ErrorIs(t, err, service.ErrCommunityUnbindTicket)
	var count int
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM community_unbind_audits WHERE ticket_id=$1`, valid).Scan(&count))
	require.Equal(t, 1, count)
}

func TestCommunityPostgresUnbindingDoesNotRemoveBan(t *testing.T) {
	repo, ctx := communityPostgresRepository(t)
	_, err := repo.db.ExecContext(ctx, `INSERT INTO users(id,status,role) VALUES(3,'active','admin')`)
	require.NoError(t, err)
	_, err = communityPostgresConfirm(t, ctx, repo, 1, 101)
	require.NoError(t, err)
	require.NoError(t, repo.RecordMemberRemoval(ctx, 101, -100, 100, 10, 900, true))
	ticket := communityPostgresTicket(t, ctx, repo, 1, time.Now())
	result, err := repo.Unbind(ctx, 3, 1, ticket)
	require.NoError(t, err)
	require.True(t, result.Banned)
	require.ErrorIs(t, repo.CheckAdmission(ctx, 1, 0, -100), service.ErrCommunityBanned)
	require.ErrorIs(t, repo.CheckAdmission(ctx, 0, 101, -100), service.ErrCommunityBanned)
	_, err = communityPostgresConfirm(t, ctx, repo, 2, 101)
	require.ErrorIs(t, err, service.ErrCommunityBanned)
	page, err := repo.ListMembers(ctx, -100, 77, service.CommunityMemberFilter{Page: 1, PageSize: 20, Status: "banned"})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Summary.Banned)
	require.Len(t, page.Items, 1, "解绑不应让禁入账号从管理员禁入列表消失")
}

func TestCommunityPostgresBanSerializesWithFirstAuthorization(t *testing.T) {
	repo, ctx := communityPostgresRepository(t)
	invite := communityPostgresInvite(t, ctx, repo, 1)
	start := make(chan struct{})
	authorization := make(chan error, 1)
	removal := make(chan error, 1)
	go func() {
		<-start
		_, _, err := repo.AuthorizeJoin(ctx, invite.URLHash, service.CommunityTelegramIdentity{ID: 101}, -100, 100, 1, 77, false)
		authorization <- err
	}()
	go func() {
		<-start
		removal <- repo.RecordMemberRemoval(ctx, 101, -100, 200, 2, 900, true)
	}()
	close(start)
	err := <-authorization
	require.True(t, err == nil || errors.Is(err, service.ErrCommunityBanned), "授权必须成功排在禁入之前，或被先提交的禁入拒绝")
	require.NoError(t, <-removal)
	require.ErrorIs(t, repo.CheckAdmission(ctx, 0, 101, -100), service.ErrCommunityBanned)
	member, _, _, err := repo.GetState(ctx, 1)
	require.NoError(t, err)
	if member != nil {
		require.Equal(t, "banned", member.Status)
		require.Zero(t, member.AuthorizedInviteID)
	}
}
