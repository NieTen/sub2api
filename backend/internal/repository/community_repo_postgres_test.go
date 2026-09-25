package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// 真实数据库测试仅使用显式指定的专用连接，每个测试独占随机 schema，不接触已有业务表。
func communityPostgresRepository(t *testing.T) (*communityRepository, context.Context) {
	t.Helper()
	dsn := os.Getenv("SUB2API_COMMUNITY_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 SUB2API_COMMUNITY_TEST_DSN，跳过真实 PostgreSQL 社群回归")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" || parsed.Host == "" {
		t.Fatal("SUB2API_COMMUNITY_TEST_DSN 必须为有效的 postgres:// 测试数据库连接")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	schema := "community_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		// schema 名由本测试生成并已成功创建，清理范围严格限定为该 schema。
		_, cleanupErr := admin.ExecContext(cleanup, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE")
		require.NoError(t, cleanupErr)
	})
	parameters := parsed.Query()
	parameters.Set("search_path", schema)
	parsed.RawQuery = parameters.Encode()
	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(4)
	var currentSchema string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&currentSchema))
	require.Equal(t, schema, currentSchema, "创建测试表前必须确认连接指向独占 schema")
	_, err = db.ExecContext(ctx, `CREATE TABLE users (id BIGINT PRIMARY KEY, status VARCHAR(16) NOT NULL, deleted_at TIMESTAMPTZ)`)
	require.NoError(t, err)
	for _, name := range []string{"230_telegram_community.sql", "231_community_direct_invites.sql"} {
		migration, readErr := migrations.FS.ReadFile(name)
		require.NoError(t, readErr)
		_, err = db.ExecContext(ctx, string(migration))
		require.NoError(t, err, "必须使用真实社群迁移建立测试结构")
	}
	_, err = db.ExecContext(ctx, `INSERT INTO users(id,status) VALUES (1,'active'),(2,'active')`)
	require.NoError(t, err)
	return &communityRepository{db: db}, ctx
}

func communityPostgresInvite(t *testing.T, ctx context.Context, repo *communityRepository, userID int64) *service.CommunityInvite {
	t.Helper()
	token := uuid.NewString()
	acquired, err := repo.AcquireInviteLease(ctx, userID, -100, token, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	inviteURL := "https://t.me/+" + uuid.NewString()
	hash := sha256.Sum256([]byte(inviteURL))
	invite := &service.CommunityInvite{
		UserID: userID, BotID: 77, GroupChatID: -100,
		URL: inviteURL, URLHash: hex.EncodeToString(hash[:]), ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, repo.SaveInvite(ctx, invite, token))
	require.Positive(t, invite.ID)
	return invite
}

type communityPostgresJoinResult struct {
	userID     int64
	telegramID int64
	membership *service.CommunityMembership
	err        error
}

// 同一专属链接只能由一个 Telegram 身份认领；真实 SQL 同时覆盖参数推断及最终状态查询。
func TestCommunityPostgresSharedInviteAndMembershipLifecycle(t *testing.T) {
	repo, ctx := communityPostgresRepository(t)
	invite := communityPostgresInvite(t, ctx, repo, 1)
	start := make(chan struct{})
	results := make(chan communityPostgresJoinResult, 2)
	for _, telegramID := range []int64{101, 102} {
		go func(id int64) {
			<-start
			member, _, err := repo.AuthorizeJoin(ctx, invite.URLHash, service.CommunityTelegramIdentity{ID: id}, -100, 1000, 25, 77, false)
			results <- communityPostgresJoinResult{userID: 1, telegramID: id, membership: member, err: err}
		}(telegramID)
	}
	close(start)
	var winner communityPostgresJoinResult
	succeeded := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			succeeded++
			winner = result
		} else {
			require.ErrorIs(t, result.err, service.ErrCommunityConflict)
			require.Nil(t, result.membership)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, invite.ID, winner.membership.AuthorizedInviteID)
	// 同一申请重复投递应保持原归属，可以安全继续完成入群。
	repeated, _, err := repo.AuthorizeJoin(ctx, invite.URLHash, service.CommunityTelegramIdentity{ID: winner.telegramID}, -100, 1000, 25, 77, false)
	require.NoError(t, err)
	require.Equal(t, winner.telegramID, repeated.TelegramUserID)
	require.NoError(t, repo.MarkMembership(ctx, winner.telegramID, -100, "joined", 1000, 25))
	joined, challenge, activeInvite, err := repo.GetState(ctx, 1)
	require.NoError(t, err, "加入后的状态查询不得因真实 SQL 错误失败")
	require.Nil(t, challenge)
	require.Nil(t, activeInvite, "成功入群后不再展示已消费的邀请")
	require.Equal(t, "joined", joined.Status)
	require.NotNil(t, joined.JoinedAt)
	require.Equal(t, winner.telegramID, joined.TelegramUserID)
	require.NoError(t, repo.MarkMembership(ctx, winner.telegramID, -100, "joined", 1000, 25))
	require.NoError(t, repo.MarkMembership(ctx, winner.telegramID, -100, "left", 1001, 26))
	left, _, activeInvite, err := repo.GetState(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "left", left.Status)
	require.Equal(t, joined.JoinedAt, left.JoinedAt, "离群后仍保留真实入群历史和身份归属")
	require.Zero(t, left.AuthorizedInviteID)
	require.Nil(t, activeInvite)
	require.ErrorIs(t, repo.MarkMembership(ctx, winner.telegramID, -100, "joined", 1000, 25), service.ErrCommunityConflict)
	_, _, err = repo.AuthorizeJoin(ctx, invite.URLHash, service.CommunityTelegramIdentity{ID: winner.telegramID}, -100, 1002, 27, 77, false)
	require.ErrorIs(t, err, service.ErrCommunityNotFound, "已经消费的原链接不得重新授权")
}

// 不同网站账号并发认领同一 Telegram 身份时，由真实唯一约束保证只有一个绑定成功。
func TestCommunityPostgresTelegramIdentityCannotBindTwoAccounts(t *testing.T) {
	repo, ctx := communityPostgresRepository(t)
	invites := []*service.CommunityInvite{communityPostgresInvite(t, ctx, repo, 1), communityPostgresInvite(t, ctx, repo, 2)}
	start := make(chan struct{})
	results := make(chan communityPostgresJoinResult, 2)
	for _, invite := range invites {
		go func(item *service.CommunityInvite) {
			<-start
			member, _, err := repo.AuthorizeJoin(ctx, item.URLHash, service.CommunityTelegramIdentity{ID: 101}, -100, 1000, 25, 77, false)
			results <- communityPostgresJoinResult{userID: item.UserID, telegramID: 101, membership: member, err: err}
		}(invite)
	}
	close(start)
	succeeded := 0
	for range 2 {
		result := <-results
		member, _, invite, err := repo.GetState(ctx, result.userID)
		require.NoError(t, err)
		if result.err == nil {
			succeeded++
			require.NotNil(t, member)
			require.Equal(t, int64(101), member.TelegramUserID)
			require.Equal(t, result.userID, member.UserID)
			require.Equal(t, "pending", member.Status)
			require.Equal(t, member.AuthorizedInviteID, invite.ID)
		} else {
			require.ErrorIs(t, result.err, service.ErrCommunityConflict, "数据库唯一冲突必须返回业务冲突，不能变成内部错误")
			require.Nil(t, member)
			require.Nil(t, result.membership)
			require.NotNil(t, invite)
			require.Zero(t, invite.TelegramUserID, "失败事务不得占用另一网站账号的邀请")
		}
	}
	require.Equal(t, 1, succeeded)
}
