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

func communityDirectMember(telegramID, groupID int64, status string, joinedAt *time.Time, authorized, date, update int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"user_id", "telegram_user_id", "telegram_username", "telegram_name", "group_chat_id", "status", "joined_at", "authorized_invite_id", "last_event_date", "last_update_id"}).AddRow(2, telegramID, "alice", "用户", groupID, status, joinedAt, authorized, date, update)
}

func communityDirectInvite(telegramID int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "user_id", "bot_id", "telegram_user_id", "group_chat_id", "url", "url_hash", "expires_at", "status", "lease_token", "attempts"}).AddRow(7, 2, 88, telegramID, -100, "https://t.me/+example", "invite-hash", time.Now().Add(time.Hour), "active", "", 0)
}

func TestCommunityFirstInviteLeaseDoesNotRequireMembership(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	communityExpectActiveLock(mock)
	mock.ExpectExec(`INSERT INTO community_invite_leases.*WHERE NOT EXISTS\(SELECT 1 FROM community_memberships.*ON CONFLICT\(user_id\).*WHERE community_invite_leases.lease_until<=NOW\(\)`).WithArgs(int64(2), int64(-100), "first", int64(120)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	acquired, err := r.AcquireInviteLease(context.Background(), 2, -100, "first", 2*time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunitySaveFirstInviteKeepsTelegramUnclaimed(t *testing.T) {
	r, mock := communityTestRepo(t)
	expires := time.Now().Add(time.Hour)
	mock.ExpectBegin()
	communityExpectActiveLock(mock)
	mock.ExpectExec(`DELETE FROM community_invite_leases l WHERE l.user_id=\$1 AND l.group_chat_id=\$2 AND l.lease_token=\$3 AND l.lease_until>NOW\(\).*m.authorized_invite_id IS NOT NULL`).WithArgs(int64(2), int64(-100), "lease").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`UPDATE community_invites SET status='revoke_pending'.*WHERE user_id=\$1 AND status='active'`).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO community_invites(user_id,bot_id,telegram_user_id,group_chat_id,url,url_hash,expires_at) SELECT $1,$2,NULLIF($3::bigint,0),$4,$5,$6,$7 WHERE $7>NOW() RETURNING id,status`)).WithArgs(int64(2), int64(88), int64(0), int64(-100), "https://t.me/+example", "invite-hash", expires).WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(7, "active"))
	mock.ExpectCommit()
	invite := &service.CommunityInvite{UserID: 2, BotID: 88, GroupChatID: -100, URL: "https://t.me/+example", URLHash: "invite-hash", ExpiresAt: expires}
	require.NoError(t, r.SaveInvite(context.Background(), invite, "lease"))
	require.Zero(t, invite.TelegramUserID)
	require.Equal(t, int64(7), invite.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityFirstJoinClaimAndGlobalIdentityReservationAreAtomic(t *testing.T) {
	for _, occupied := range []bool{false, true} {
		t.Run(map[bool]string{false: "首次认领", true: "同一Telegram被另一网站用户并发占用"}[occupied], func(t *testing.T) {
			r, mock := communityTestRepo(t)
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT user_id FROM community_invites WHERE url_hash=\$1 AND group_chat_id=\$2 AND bot_id=\$3`).WithArgs("invite-hash", int64(-100), int64(88)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
			communityExpectActiveLock(mock)
			mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery(`SELECT .* FROM community_invites .*telegram_user_id IS NULL OR telegram_user_id=\$3.*status='active' AND expires_at>NOW\(\).*FOR UPDATE`).WithArgs("invite-hash", int64(2), int64(99), int64(-100), int64(88), int64(0), int64(1000), int64(0), int64(25), int64(-1)).WillReturnRows(communityDirectInvite(0))
			insert := mock.ExpectQuery(`INSERT INTO community_memberships.*authorized_invite_id,last_event_date,last_update_id.*RETURNING`).WithArgs(int64(2), int64(99), "alice", "用户", int64(-100), int64(7), int64(1000), int64(25))
			if occupied {
				insert.WillReturnError(&pq.Error{Code: "23505", Constraint: "community_memberships_telegram_user_id_key"})
				mock.ExpectRollback()
			} else {
				insert.WillReturnRows(communityTestMember("pending", 7, 1000, 25))
				mock.ExpectExec(`UPDATE community_invites SET telegram_user_id=\$2 WHERE id=\$1 AND \(telegram_user_id IS NULL OR telegram_user_id=\$2\)`).WithArgs(int64(7), int64(99)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			member, invite, err := r.AuthorizeJoin(context.Background(), "invite-hash", service.CommunityTelegramIdentity{ID: 99, Username: "alice", Name: "用户"}, -100, 1000, 25, 88, false)
			if occupied {
				require.ErrorIs(t, err, service.ErrCommunityConflict)
				require.Nil(t, member)
				require.Nil(t, invite)
			} else {
				require.NoError(t, err)
				require.Equal(t, "pending", member.Status)
				require.Nil(t, member.JoinedAt)
				require.Equal(t, int64(99), invite.TelegramUserID)
				require.Equal(t, int64(7), member.AuthorizedInviteID)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCommunityDirectJoinRejectsInactiveOrDeletedUserBeforeClaim(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT user_id FROM community_invites`).WithArgs("invite-hash", int64(-100), int64(88)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	mock.ExpectQuery(`SELECT id FROM users WHERE id=\$1 AND status='active' AND deleted_at IS NULL FOR NO KEY UPDATE`).WithArgs(int64(2)).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	_, _, err := r.AuthorizeJoin(context.Background(), "invite-hash", service.CommunityTelegramIdentity{ID: 99}, -100, 1000, 25, 88, false)
	require.ErrorIs(t, err, service.ErrCommunityNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityTemporaryReservationReleasedOnlyWithoutRealJoin(t *testing.T) {
	joined := time.Now().Add(-time.Hour)
	for _, history := range []*time.Time{nil, &joined} {
		t.Run(map[bool]string{false: "释放未完成预留", true: "保留真实绑定"}[history != nil], func(t *testing.T) {
			r, mock := communityTestRepo(t)
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT user_id FROM community_memberships`).WithArgs(int64(99), int64(-100)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
			mock.ExpectQuery(`SELECT id FROM users WHERE id=\$1 FOR NO KEY UPDATE`).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
			mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnRows(communityDirectMember(99, -100, "pending", history, 7, 1000, 25))
			if history != nil {
				mock.ExpectExec(`UPDATE community_memberships SET status=\$2.*ELSE joined_at END.*WHERE user_id=\$1`).WithArgs(int64(2), "left", int64(1001), int64(26)).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectExec(`UPDATE community_invites SET status='revoke_pending'.*WHERE user_id=\$1 AND status='active'`).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
			if history == nil {
				mock.ExpectExec(`DELETE FROM community_memberships WHERE user_id=\$1 AND telegram_user_id=\$2 AND group_chat_id=\$3 AND joined_at IS NULL`).WithArgs(int64(2), int64(99), int64(-100)).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
			require.NoError(t, r.MarkMembership(context.Background(), 99, -100, "left", 1001, 26))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCommunityStaleMemberLookupCannotModifyReplacementReservation(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT user_id FROM community_memberships`).WithArgs(int64(99), int64(-100)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	mock.ExpectQuery(`SELECT id FROM users WHERE id=\$1 FOR NO KEY UPDATE`).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	// 等待用户锁期间，旧预留已经删除并被新 Telegram 身份重新领取。
	mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnRows(communityDirectMember(100, -100, "pending", nil, 8, 1000, 25))
	mock.ExpectRollback()
	require.ErrorIs(t, r.MarkMembership(context.Background(), 99, -100, "left", 1001, 26), service.ErrCommunityConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityGroupChangePreservesPreviouslyJoinedIdentity(t *testing.T) {
	r, mock := communityTestRepo(t)
	joined := time.Now().Add(-24 * time.Hour)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT user_id FROM community_invites`).WithArgs("invite-hash", int64(-100), int64(88)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	communityExpectActiveLock(mock)
	mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnRows(communityDirectMember(99, -200, "left", &joined, 0, 2000, 40))
	mock.ExpectQuery(`SELECT .* FROM community_invites.*FOR UPDATE`).WithArgs("invite-hash", int64(2), int64(99), int64(-100), int64(88), int64(0), int64(1000), int64(0), int64(25), int64(-1)).WillReturnRows(communityDirectInvite(99))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE community_memberships SET telegram_username=$2,telegram_name=$3,group_chat_id=$4,status='pending',authorized_invite_id=$5,last_event_date=$6,last_update_id=$7,updated_at=NOW() WHERE user_id=$1 AND telegram_user_id=$8`)).WithArgs(int64(2), "alice", "用户", int64(-100), int64(7), int64(1000), int64(25), int64(99)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE community_invites SET telegram_user_id`).WithArgs(int64(7), int64(99)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	member, _, err := r.AuthorizeJoin(context.Background(), "invite-hash", service.CommunityTelegramIdentity{ID: 99, Username: "alice", Name: "用户"}, -100, 1000, 25, 88, false)
	require.NoError(t, err)
	require.Equal(t, &joined, member.JoinedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityMembersIncludesUninvitedUsersAndSearchSummaryIgnoresStatus(t *testing.T) {
	r, mock := communityTestRepo(t)
	// 精确断言用户全集和当前群 LEFT JOIN；统计查询没有状态筛选参数。
	summarySQL := communityMemberListBase + `SELECT COUNT(*),COUNT(*) FILTER (WHERE status='joined'),COUNT(*) FILTER (WHERE status<>'joined'),COUNT(*) FILTER (WHERE status='pending'),COUNT(*) FILTER (WHERE status='left') FROM members`
	require.Contains(t, communityMemberListBase, "FROM users u")
	require.Contains(t, communityMemberListBase, "LEFT JOIN community_memberships m ON m.user_id=u.id AND m.group_chat_id=$1")
	require.Contains(t, communityMemberListBase, "WHERE u.deleted_at IS NULL")
	require.Contains(t, communityMemberListBase, "i.bot_id=$2 AND i.status='active' AND i.expires_at>NOW()")
	require.NotContains(t, communityMemberListBase, "u.status='active'")
	mock.ExpectQuery(regexp.QuoteMeta(summarySQL)).WithArgs(int64(-100), int64(88), "", "%%").WillReturnRows(sqlmock.NewRows([]string{"total", "joined", "not_joined", "pending", "left"}).AddRow(5, 2, 3, 1, 1))
	expires := time.Now().Add(time.Hour)
	mock.ExpectQuery(`WITH members AS .*SELECT user_id,email,username,user_status,telegram_user_id,telegram_username,telegram_name,status,joined_at,invite_expires_at FROM members WHERE \(\$5='all' OR \(\$5='not_joined' AND status<>'joined'\) OR status=\$5\) ORDER BY user_id DESC LIMIT \$6 OFFSET \$7`).WithArgs(int64(-100), int64(88), "", "%%", "not_joined", 20, int64(0)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "username", "user_status", "telegram_user_id", "telegram_username", "telegram_name", "status", "joined_at", "invite_expires_at"}).AddRow(5, "new@example.com", "新用户", "active", 0, "", "", "not_joined", nil, nil).AddRow(4, "waiting@example.com", "待入群", "active", 0, "", "", "pending", nil, expires).AddRow(3, "left@example.com", "离群", "disabled", 99, "alice", "用户", "left", nil, nil))
	page, err := r.ListMembers(context.Background(), -100, 88, service.CommunityMemberFilter{Status: "not_joined"})
	require.NoError(t, err)
	require.Equal(t, int64(3), page.Total)
	require.Equal(t, int64(5), page.Summary.Total)
	require.Equal(t, int64(2), page.Summary.Joined)
	require.Len(t, page.Items, 3)
	require.Equal(t, "not_joined", page.Items[0].Status)
	require.Zero(t, page.Items[0].TelegramUserID)
	require.Equal(t, "pending", page.Items[1].Status)
	require.Equal(t, &expires, page.Items[1].InviteExpiresAt)
	require.Equal(t, "disabled", page.Items[2].UserStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityMembersSearchEscapesWildcardsAndKeepsEmptyArray(t *testing.T) {
	r, mock := communityTestRepo(t)
	mock.ExpectQuery(`WITH members AS .*SELECT COUNT\(\*\).*FROM members`).WithArgs(int64(-100), int64(88), `a_%`, `%a\_\%%`).WillReturnRows(sqlmock.NewRows([]string{"total", "joined", "not_joined", "pending", "left"}).AddRow(0, 0, 0, 0, 0))
	mock.ExpectQuery(`WITH members AS .*SELECT user_id,email.*LIMIT \$6 OFFSET \$7`).WithArgs(int64(-100), int64(88), `a_%`, `%a\_\%%`, "joined", 10, int64(20)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "username", "user_status", "telegram_user_id", "telegram_username", "telegram_name", "status", "joined_at", "invite_expires_at"}))
	page, err := r.ListMembers(context.Background(), -100, 88, service.CommunityMemberFilter{Page: 3, PageSize: 10, Search: " a_% ", Status: "joined"})
	require.NoError(t, err)
	require.NotNil(t, page.Items)
	require.Empty(t, page.Items)
	require.Equal(t, 3, page.Page)
	require.NoError(t, mock.ExpectationsWereMet())
}
