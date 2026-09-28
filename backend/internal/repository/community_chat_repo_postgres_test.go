package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func communityChatPostgresRepository(t *testing.T) (*communityRepository, context.Context) {
	t.Helper()
	r, ctx := communityPostgresRepository(t)
	migration, err := migrations.FS.ReadFile("243_community_group_chat.sql")
	require.NoError(t, err)
	_, err = r.db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	return r, ctx
}
func TestCommunityChatPostgresArchiveIdempotencyAndCursors(t *testing.T) {
	r, ctx := communityChatPostgresRepository(t)
	for update := int64(1); update <= 5; update++ {
		value := update
		m := &service.CommunityChatMessage{BotID: 77, GroupChatID: -100, UpdateID: &value, TelegramMessageID: update, TelegramUserID: 101, TelegramName: "群成员", MessageType: "text", Text: "保留原消息", CreatedAt: time.Now(), SenderKind: "user"}
		if update == 5 {
			edited := time.Now()
			m.TelegramMessageID = 1
			m.EditedAt = &edited
			m.Text = "修改后的消息"
		}
		require.NoError(t, r.SaveChatMessage(ctx, m))
		id := m.ID
		require.NoError(t, r.SaveChatMessage(ctx, m))
		require.Equal(t, id, m.ID)
	}
	page, err := r.ListChatMessages(ctx, -100, service.CommunityChatFilter{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.True(t, page.HasMore)
	require.NotNil(t, page.Items[1].EditedAt)
	older, err := r.ListChatMessages(ctx, -100, service.CommunityChatFilter{BeforeID: page.Items[0].ID, Limit: 2})
	require.NoError(t, err)
	require.Len(t, older.Items, 2)
	require.Less(t, older.LatestID, page.Items[0].ID)
	newer, err := r.ListChatMessages(ctx, -100, service.CommunityChatFilter{AfterID: older.Items[0].ID, Limit: 2})
	require.NoError(t, err)
	require.Len(t, newer.Items, 2)
	require.True(t, newer.HasMore)
	require.Equal(t, newer.Items[1].ID, newer.LatestID)
	other, err := r.ListChatMessages(ctx, -200, service.CommunityChatFilter{Limit: 20})
	require.NoError(t, err)
	require.Empty(t, other.Items)
	_, err = r.GetChatMessage(ctx, -200, page.Items[0].ID)
	require.ErrorIs(t, err, service.ErrCommunityNotFound)
	person, err := r.GetChatPerson(ctx, -100, 101)
	require.NoError(t, err)
	require.Equal(t, "群成员", person.TelegramName)
	require.Nil(t, person.Member)
}
func TestCommunityChatPostgresSendReservationAndNoReplay(t *testing.T) {
	r, ctx := communityChatPostgresRepository(t)
	request := func() *service.CommunityChatSend {
		return &service.CommunityChatSend{GroupChatID: -100, BotID: 77, AdminUserID: 1, ClientRequestID: "same-id-fixture", ContentHash: "hash"}
	}
	first := request()
	acquired, err := r.ReserveChatSend(ctx, first)
	require.NoError(t, err)
	require.True(t, acquired)
	retry := request()
	acquired, err = r.ReserveChatSend(ctx, retry)
	require.NoError(t, err)
	require.False(t, acquired)
	require.Equal(t, "sending", retry.Status)
	conflict := request()
	conflict.ContentHash = "changed"
	_, err = r.ReserveChatSend(ctx, conflict)
	require.ErrorIs(t, err, service.ErrCommunityConflict)
	first.Status = "sent"
	message := &service.CommunityChatMessage{BotID: 77, GroupChatID: -100, TelegramMessageID: 10, TelegramUserID: 77, SenderKind: "user", MessageType: "text", Text: "管理员发送", Outgoing: true, AdminUserID: 1, CreatedAt: time.Now()}
	require.NoError(t, r.FinishChatSend(ctx, first, message))
	require.Positive(t, message.ID)
	retry = request()
	acquired, err = r.ReserveChatSend(ctx, retry)
	require.NoError(t, err)
	require.False(t, acquired)
	require.Equal(t, "sent", retry.Status)
	require.Equal(t, message.ID, retry.MessageID)
	require.ErrorIs(t, r.FinishChatSend(ctx, first, message), service.ErrCommunityConflict)
	page, err := r.ListChatMessages(ctx, -100, service.CommunityChatFilter{Limit: 20})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	avatar := &service.CommunityChatFile{MimeType: "image/jpeg", Data: []byte{1, 2, 3}}
	require.NoError(t, r.SaveChatAvatar(ctx, 77, 101, avatar))
	cached, err := r.GetChatAvatar(ctx, 77, 101)
	require.NoError(t, err)
	require.Equal(t, avatar.Data, cached.Data)
	_, err = r.GetChatAvatar(ctx, 88, 101)
	require.ErrorIs(t, err, service.ErrCommunityNotFound)
}

// Telegram 身份和网站记录使用 64 位编号，避免 SQL 参数推断回退到 int32。
func TestCommunityChatPostgresLargeIDs(t *testing.T) {
	r, ctx := communityChatPostgresRepository(t)
	const largeID int64 = 5939067819
	_, err := r.db.ExecContext(ctx, `INSERT INTO users(id,status,role) VALUES($1,'active','admin')`, largeID)
	require.NoError(t, err)
	_, err = r.db.ExecContext(ctx, `ALTER SEQUENCE community_chat_messages_id_seq RESTART WITH 5939067819`)
	require.NoError(t, err)
	send := &service.CommunityChatSend{GroupChatID: -1005939067819, BotID: largeID, AdminUserID: largeID, ClientRequestID: "large-id-fixture", ContentHash: "hash"}
	acquired, err := r.ReserveChatSend(ctx, send)
	require.NoError(t, err)
	require.True(t, acquired)
	message := &service.CommunityChatMessage{BotID: largeID, GroupChatID: send.GroupChatID, TelegramMessageID: largeID, TelegramUserID: largeID, SenderKind: "user", MessageType: "text", Text: "大编号回归", Outgoing: true, AdminUserID: largeID, CreatedAt: time.Now()}
	send.Status = "sent"
	require.NoError(t, r.FinishChatSend(ctx, send, message))
	require.Equal(t, largeID, message.ID)
	require.Equal(t, largeID, send.MessageID)
	saved, err := r.GetChatMessage(ctx, send.GroupChatID, message.ID)
	require.NoError(t, err)
	require.Equal(t, largeID, saved.AdminUserID)
	require.Equal(t, largeID, saved.TelegramUserID)
}
