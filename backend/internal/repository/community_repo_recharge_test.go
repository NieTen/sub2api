package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCommunityPaidBalanceRechargeUsesVerifiedPaymentHistory(t *testing.T) {
	// 精确断言实际支付、余额订单和用户范围，赠送余额与未支付订单不能作为资格。
	query := `SELECT EXISTS (SELECT 1 FROM payment_orders po WHERE po.user_id=$1 AND po.order_type='balance' AND po.paid_at IS NOT NULL AND po.pay_amount>0)`
	for _, paid := range []bool{true, false} {
		t.Run(map[bool]string{true: "存在成功支付的余额充值", false: "不存在符合条件的充值"}[paid], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(paid))
			result, err := (&communityRepository{db: db}).HasPaidBalanceRecharge(context.Background(), 42)
			require.NoError(t, err)
			require.Equal(t, paid, result)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCommunityPaidBalanceRechargeDoesNotHideDatabaseErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbErr := errors.New("数据库暂时不可用")
	mock.ExpectQuery(`SELECT EXISTS .*po.user_id=\$1`).WithArgs(int64(42)).WillReturnError(dbErr)
	paid, err := (&communityRepository{db: db}).HasPaidBalanceRecharge(context.Background(), 42)
	require.ErrorIs(t, err, dbErr)
	require.False(t, paid)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommunityVIPJoinChecksPaymentBeforeReservingIdentity(t *testing.T) {
	dbErr := errors.New("支付历史查询失败")
	for _, tc := range []struct {
		name     string
		required bool
		paid     bool
		queryErr error
		wantErr  error
	}{
		{name: "VIP支付成功允许预留", required: true, paid: true},
		{name: "VIP未成功支付不能占用身份", required: true, wantErr: service.ErrCommunityVIPRequired},
		{name: "VIP查询异常保留重试机会", required: true, queryErr: dbErr, wantErr: dbErr},
		{name: "普通群不查询支付历史", required: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, mock := communityTestRepo(t)
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT user_id FROM community_invites WHERE url_hash=\$1 AND group_chat_id=\$2 AND bot_id=\$3`).WithArgs("invite-hash", int64(-100), int64(88)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
			communityExpectActiveLock(mock)
			if tc.required {
				query := mock.ExpectQuery(regexp.QuoteMeta(communityPaidBalanceRechargeQuery)).WithArgs(int64(2))
				if tc.queryErr != nil {
					query.WillReturnError(tc.queryErr)
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tc.paid))
				}
			}
			if tc.wantErr != nil {
				// 不应读取或修改会员及邀请身份，直接回滚整个授权事务。
				mock.ExpectRollback()
			} else {
				mock.ExpectQuery(`SELECT .* FROM community_memberships WHERE user_id=\$1 FOR UPDATE`).WithArgs(int64(2)).WillReturnError(sql.ErrNoRows)
				mock.ExpectQuery(`SELECT .* FROM community_invites .*FOR UPDATE`).WithArgs("invite-hash", int64(2), int64(99), int64(-100), int64(88), int64(0), int64(1000), int64(0), int64(25), int64(-1)).WillReturnRows(communityDirectInvite(0))
				mock.ExpectQuery(`INSERT INTO community_memberships.*authorized_invite_id,last_event_date,last_update_id.*RETURNING`).WithArgs(int64(2), int64(99), "alice", "用户", int64(-100), int64(7), int64(1000), int64(25)).WillReturnRows(communityTestMember("pending", 7, 1000, 25))
				mock.ExpectExec(`UPDATE community_invites SET telegram_user_id=\$2 WHERE id=\$1`).WithArgs(int64(7), int64(99)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			member, invite, err := r.AuthorizeJoin(context.Background(), "invite-hash", service.CommunityTelegramIdentity{ID: 99, Username: "alice", Name: "用户"}, -100, 1000, 25, 88, tc.required)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, member)
				require.Nil(t, invite)
			} else {
				require.NoError(t, err)
				require.Equal(t, "pending", member.Status)
				require.Equal(t, int64(99), invite.TelegramUserID)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
