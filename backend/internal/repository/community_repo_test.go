package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func communityTestRepo(t *testing.T) (service.CommunityRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return NewCommunityRepository(db), mock
}
func communityTestMember(status string, authorized, eventDate, updateID int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"user_id", "telegram_user_id", "telegram_username", "telegram_name", "group_chat_id", "status", "joined_at", "authorized_invite_id", "last_event_date", "last_update_id"}).AddRow(2, 99, "alice", "用户", -100, status, nil, authorized, eventDate, updateID)
}
func communityTestChallenge(telegramID int64, status string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "user_id", "bot_id", "token_hash", "telegram_user_id", "telegram_username", "telegram_name", "status", "expires_at"}).AddRow("challenge", 2, 88, "hash", telegramID, "alice", "用户", status, time.Now().Add(time.Minute))
}
func communityTestInvite(status string, expires time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "user_id", "bot_id", "telegram_user_id", "group_chat_id", "url", "url_hash", "expires_at", "status", "lease_token", "attempts"}).AddRow(7, 2, 88, 99, -100, "https://t.me/+example", "invite-hash", expires, status, "lease", 1)
}
func communityExpectActiveLock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id FROM users WHERE id=\$1 AND status='active' AND deleted_at IS NULL FOR NO KEY UPDATE`).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
}

func TestCommunityChallengeIdentityCannotBeReplaced(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT user_id FROM community_challenges WHERE token_hash=\$1 AND bot_id=\$2`).WithArgs("hash", int64(88)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	communityExpectActiveLock(mock)
	mock.ExpectQuery(`SELECT .* FROM community_challenges WHERE token_hash=\$1 AND bot_id=\$2 AND expires_at>NOW\(\).*FOR UPDATE`).WithArgs("hash", int64(88)).WillReturnRows(communityTestChallenge(99, "claimed"))
	mock.ExpectRollback()
	_, err := r.ClaimChallenge(context.Background(), "hash", 88, service.CommunityTelegramIdentity{ID: 100})
	require.ErrorIs(t, err, service.ErrCommunityConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityChallengeReplacesOnlyCurrentUsersExpiredData(t *testing.T) {
	r, mock := communityTestRepo(t)
	expires := time.Now().Add(15 * time.Minute)
	mock.ExpectBegin()
	communityExpectActiveLock(mock)
	mock.ExpectExec(`UPDATE community_challenges SET status='superseded'.*WHERE user_id=\$1 AND status IN \('waiting','claimed'\)`).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM community_challenges WHERE user_id=\$1 AND \(status='superseded' OR expires_at<=NOW\(\)\)`).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO community_challenges.*WHERE \$5>NOW\(\)`).WithArgs("new", int64(2), int64(88), "hash", expires).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, r.CreateChallenge(context.Background(), &service.CommunityChallenge{ID: "new", UserID: 2, BotID: 88, TokenHash: "hash", ExpiresAt: expires}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityInactiveUserCannotConfirm(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM users .*status='active'.*FOR NO KEY UPDATE`).WithArgs(int64(2)).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	_, err := r.ConfirmChallenge(context.Background(), 2, "challenge", 99, -100, 88)
	require.ErrorIs(t, err, service.ErrCommunityNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityTelegramIdentityIsGloballyUnique(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	communityExpectActiveLock(mock)
	mock.ExpectQuery(`SELECT .* FROM community_challenges WHERE id=\$1 AND user_id=\$2 AND bot_id=\$3.*FOR UPDATE`).WithArgs("challenge", int64(2), int64(88)).WillReturnRows(communityTestChallenge(99, "claimed"))
	mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO community_memberships`).WithArgs(int64(2), int64(99), "alice", "用户", int64(-100)).WillReturnError(&pq.Error{Code: "23505", Constraint: "community_memberships_telegram_user_id_key"})
	mock.ExpectRollback()
	_, err := r.ConfirmChallenge(context.Background(), 2, "challenge", 99, -100, 88)
	require.ErrorIs(t, err, service.ErrCommunityConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityClaimedInvitationCannotAuthorizeOtherTelegram(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT user_id FROM community_invites WHERE url_hash=$1 AND group_chat_id=$2 AND bot_id=$3`)).WithArgs("invite-hash", int64(-100), int64(88)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	communityExpectActiveLock(mock)
	mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnRows(communityTestMember("pending", 7, 1000, 24))
	mock.ExpectRollback()
	_, _, err := r.AuthorizeJoin(context.Background(), "invite-hash", service.CommunityTelegramIdentity{ID: 100}, -100, 1000, 25, 88, false)
	require.ErrorIs(t, err, service.ErrCommunityConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityOldJoinRequestCannotOverrideLaterLeave(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT user_id FROM community_invites`).WithArgs("invite-hash", int64(-100), int64(88)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	communityExpectActiveLock(mock)
	mock.ExpectQuery(`SELECT .* FROM community_memberships.*FOR UPDATE`).WithArgs(int64(2)).WillReturnRows(communityTestMember("left", 0, 2000, 30))
	mock.ExpectRollback()
	_, _, err := r.AuthorizeJoin(context.Background(), "invite-hash", service.CommunityTelegramIdentity{ID: 99}, -100, 1000, 25, 88, false)
	require.ErrorIs(t, err, service.ErrCommunityConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityExpiredAuthorizationOnlyReplaysExactEvent(t *testing.T) {
	for _, tt := range []struct {
		name         string
		date, update int64
		allowed      bool
	}{{"同一已授权事件", 1000, 25, true}, {"不同事件不能复用过期授权", 1000, 26, false}} {
		t.Run(tt.name, func(t *testing.T) {
			r, mock := communityTestRepo(t)
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT user_id FROM community_invites`).WithArgs("invite-hash", int64(-100), int64(88)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
			communityExpectActiveLock(mock)
			mock.ExpectQuery(`SELECT .* FROM community_memberships.*FOR UPDATE`).WithArgs(int64(2)).WillReturnRows(communityTestMember("pending", 7, 1000, 25))
			query := mock.ExpectQuery(`SELECT .* FROM community_invites .*bot_id=\$5 AND \(\(status='active' AND expires_at>NOW\(\)\) OR \(id=\$6 AND \$7::bigint=\$8::bigint AND \$9::bigint=\$10::bigint\)\) FOR UPDATE`).WithArgs("invite-hash", int64(2), int64(99), int64(-100), int64(88), int64(7), tt.date, int64(1000), tt.update, int64(25))
			if tt.allowed {
				query.WillReturnRows(communityTestInvite("revoked", time.Now().Add(-time.Minute)))
				mock.ExpectExec(`UPDATE community_memberships SET telegram_username`).WithArgs(int64(2), "alice", "用户", int64(-100), int64(7), tt.date, tt.update, int64(99)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE community_invites SET telegram_user_id`).WithArgs(int64(7), int64(99)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			} else {
				query.WillReturnError(sql.ErrNoRows)
				mock.ExpectRollback()
			}
			_, _, err := r.AuthorizeJoin(context.Background(), "invite-hash", service.CommunityTelegramIdentity{ID: 99, Username: "alice", Name: "用户"}, -100, tt.date, tt.update, 88, false)
			if tt.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, service.ErrCommunityNotFound)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCommunityJoinRequiresSavedAuthorization(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT user_id FROM community_memberships`).WithArgs(int64(99), int64(-100)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	communityExpectActiveLock(mock)
	mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnRows(communityTestMember("pending", 0, 0, -1))
	mock.ExpectRollback()
	err := r.MarkMembership(context.Background(), 99, -100, "joined", 1000, 25)
	require.ErrorIs(t, err, service.ErrCommunityConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityLateMemberEventCannotUndoNewerLeaveInSameSecond(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT user_id FROM community_memberships`).WithArgs(int64(99), int64(-100)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	communityExpectActiveLock(mock)
	mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnRows(communityTestMember("left", 0, 1000, 26))
	mock.ExpectRollback()
	require.ErrorIs(t, r.MarkMembership(context.Background(), 99, -100, "joined", 1000, 25), service.ErrCommunityConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityJoinedStateAndRevocationCommitTogether(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT user_id FROM community_memberships`).WithArgs(int64(99), int64(-100)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	communityExpectActiveLock(mock)
	mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnRows(communityTestMember("pending", 7, 1000, 25))
	mock.ExpectExec(`UPDATE community_memberships SET status=\$2.*WHERE user_id=\$1`).WithArgs(int64(2), "joined", int64(1000), int64(25)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE community_invites SET status='revoke_pending'.*WHERE user_id=\$1 AND status='active'`).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, r.MarkMembership(context.Background(), 99, -100, "joined", 1000, 25))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunitySaveInviteDoesNotOverwriteConcurrentJoin(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	communityExpectActiveLock(mock)
	mock.ExpectExec(`DELETE FROM community_invite_leases l .*l.lease_token=\$3 AND l.lease_until>NOW\(\) AND NOT EXISTS.*m.status='joined' OR m.authorized_invite_id IS NOT NULL`).WithArgs(int64(2), int64(-100), "lease").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err := r.SaveInvite(context.Background(), &service.CommunityInvite{UserID: 2, TelegramUserID: 99, GroupChatID: -100}, "lease")
	require.ErrorIs(t, err, service.ErrCommunityConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityInviteLeaseProtectsApprovalAndExistingInvitation(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	communityExpectActiveLock(mock)
	mock.ExpectExec(`INSERT INTO community_invite_leases.*NOT EXISTS.*m.status='joined' OR m.authorized_invite_id IS NOT NULL.*NOT EXISTS\(SELECT 1 FROM community_invites.*ON CONFLICT\(user_id\).*WHERE community_invite_leases.lease_until<=NOW\(\)`).WithArgs(int64(2), int64(-100), "lease", int64(120)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	ok, err := r.AcquireInviteLease(context.Background(), 2, -100, "lease", 2*time.Minute)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityWebhookIdempotencyIncludesBotIdentity(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO community_webhook_events(bot_id,update_id,payload) VALUES($1,$2,$3::jsonb) ON CONFLICT(bot_id,update_id) DO NOTHING`)).WithArgs(int64(88), int64(25), `{"update_id":25}`).WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, r.EnqueueWebhook(context.Background(), 88, 25, []byte(`{"update_id":25}`)))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityWebhookCompletionRequiresCurrentLease(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectExec(`UPDATE community_webhook_events SET completed_at=NOW\(\),payload='\{\}'::jsonb.*WHERE id=\$1 AND lease_token=\$2 AND completed_at IS NULL`).WithArgs(int64(1), "stale").WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, r.CompleteWebhook(context.Background(), 1, "stale"), service.ErrCommunityConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityRevocationClaimsExpiredLinksAfterJoining(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectQuery(`WITH picked AS .*status='revoke_pending'.*status='active' AND expires_at<=NOW\(\).*FOR UPDATE SKIP LOCKED.*UPDATE community_invites`).WithArgs(1, sqlmock.AnyArg(), int64(60)).WillReturnRows(communityTestInvite("revoke_pending", time.Now().Add(-time.Minute)))
	items, err := r.ClaimRevocations(context.Background(), 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(88), items[0].BotID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityExplicitRevocationDoesNotTouchNewerInvitation(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectExec(`UPDATE community_invites SET status='revoke_pending',available_at=NOW\(\) WHERE user_id=\$1 AND id=\$2 AND status='active'`).WithArgs(int64(2), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, r.QueueInviteRevocation(context.Background(), 2, 7))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityCleanupIsBoundedAndPreservesReferencedInvites(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectExec(`WITH expired AS \(SELECT id FROM community_webhook_events WHERE completed_at < NOW\(\)-INTERVAL '7 days'.*LIMIT 500 FOR UPDATE SKIP LOCKED\) DELETE FROM community_webhook_events e USING expired x WHERE e.id=x.id AND e.completed_at`).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`WITH expired AS \(SELECT id FROM community_challenges WHERE expires_at < NOW\(\)-INTERVAL '1 day'.*LIMIT 500 FOR UPDATE SKIP LOCKED\) DELETE FROM community_challenges c USING expired x WHERE c.id=x.id AND c.expires_at`).WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectExec(`WITH expired AS \(SELECT i.id FROM community_invites i WHERE i.status='revoked'.*NOT EXISTS \(SELECT 1 FROM community_memberships m WHERE m.authorized_invite_id=i.id\).*LIMIT 500 FOR UPDATE OF i SKIP LOCKED\) DELETE FROM community_invites i USING expired x WHERE i.id=x.id AND i.status='revoked' AND NOT EXISTS`).WillReturnResult(sqlmock.NewResult(0, 2))
	require.NoError(t, r.Cleanup(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}
