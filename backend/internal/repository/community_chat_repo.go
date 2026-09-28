package repository

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const communityChatColumns = `id,bot_id,group_chat_id,update_id,telegram_message_id,telegram_user_id,telegram_username,telegram_name,sender_kind,sender_chat_id,is_bot,message_type,text,file_id,file_name,mime_type,file_size,reply_to_message_id,outgoing,COALESCE(admin_user_id,0),created_at,edited_at`

func scanCommunityChat(row communityScanner) (*service.CommunityChatMessage, error) {
	m := &service.CommunityChatMessage{}
	err := row.Scan(&m.ID, &m.BotID, &m.GroupChatID, &m.UpdateID, &m.TelegramMessageID, &m.TelegramUserID, &m.TelegramUsername, &m.TelegramName, &m.SenderKind, &m.SenderChatID, &m.IsBot, &m.MessageType, &m.Text, &m.FileID, &m.FileName, &m.MimeType, &m.FileSize, &m.ReplyToMessageID, &m.Outgoing, &m.AdminUserID, &m.CreatedAt, &m.EditedAt)
	m.MediaAvailable = m.FileID != "" && m.FileSize <= service.CommunityChatMediaLimit
	return m, communityError(err)
}

type communityChatQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func saveCommunityChat(ctx context.Context, db communityChatQuery, m *service.CommunityChatMessage) error {
	// 同一回调可反复投递，但只能生成一条存档。编辑是独立 update_id，保留原文。
	err := db.QueryRowContext(ctx, `INSERT INTO community_chat_messages(bot_id,group_chat_id,update_id,telegram_message_id,telegram_user_id,telegram_username,telegram_name,sender_kind,sender_chat_id,is_bot,message_type,text,file_id,file_name,mime_type,file_size,reply_to_message_id,outgoing,admin_user_id,created_at,edited_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,NULLIF($19::bigint,0),$20,$21) ON CONFLICT(bot_id,update_id) DO NOTHING RETURNING id`, m.BotID, m.GroupChatID, m.UpdateID, m.TelegramMessageID, m.TelegramUserID, m.TelegramUsername, m.TelegramName, m.SenderKind, m.SenderChatID, m.IsBot, m.MessageType, m.Text, m.FileID, m.FileName, m.MimeType, m.FileSize, m.ReplyToMessageID, m.Outgoing, m.AdminUserID, m.CreatedAt, m.EditedAt).Scan(&m.ID)
	if errors.Is(err, sql.ErrNoRows) && m.UpdateID != nil {
		err = db.QueryRowContext(ctx, `SELECT id FROM community_chat_messages WHERE bot_id=$1 AND update_id=$2`, m.BotID, *m.UpdateID).Scan(&m.ID)
	}
	m.MediaAvailable = m.FileID != "" && m.FileSize <= service.CommunityChatMediaLimit
	return err
}
func (r *communityRepository) SaveChatMessage(ctx context.Context, m *service.CommunityChatMessage) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		// 同群按提交顺序分配消息编号，增量游标不会跳过并发事务中尚未提交的消息。
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, m.GroupChatID); err != nil {
			return err
		}
		return saveCommunityChat(ctx, tx, m)
	})
}
func (r *communityRepository) GetChatMessage(ctx context.Context, group, id int64) (*service.CommunityChatMessage, error) {
	return scanCommunityChat(r.db.QueryRowContext(ctx, `SELECT `+communityChatColumns+` FROM community_chat_messages WHERE group_chat_id=$1 AND id=$2`, group, id))
}
func (r *communityRepository) ListChatMessages(ctx context.Context, group int64, f service.CommunityChatFilter) (*service.CommunityChatPage, error) {
	limit := communityLimit(f.Limit)
	query := `SELECT ` + communityChatColumns + ` FROM community_chat_messages WHERE group_chat_id=$1`
	args := []any{group}
	switch {
	case f.AfterID > 0:
		query += ` AND id>$2`
		args = append(args, f.AfterID)
	case f.BeforeID > 0:
		query += ` AND id<$2`
		args = append(args, f.BeforeID)
	}
	if f.AfterID > 0 {
		query += ` ORDER BY id ASC`
	} else {
		query += ` ORDER BY id DESC`
	}
	if len(args) == 1 {
		query += ` LIMIT $2`
	} else {
		query += ` LIMIT $3`
	}
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &service.CommunityChatPage{Items: []service.CommunityChatMessage{}, LatestID: f.AfterID}
	for rows.Next() {
		m, err := scanCommunityChat(rows)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, *m)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(page.Items) > limit {
		page.HasMore = true
		page.Items = page.Items[:limit]
	}
	if f.AfterID == 0 {
		slices.Reverse(page.Items)
	}
	if len(page.Items) > 0 {
		page.LatestID = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}
func (r *communityRepository) GetChatPerson(ctx context.Context, group, id int64) (*service.CommunityChatPerson, error) {
	p := &service.CommunityChatPerson{TelegramUserID: id}
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM community_bans WHERE group_chat_id=$1 AND telegram_user_id=$2)`, group, id).Scan(&p.Banned); err != nil {
		return nil, err
	}
	err := r.db.QueryRowContext(ctx, `SELECT telegram_username,telegram_name,is_bot FROM community_chat_messages WHERE group_chat_id=$1 AND telegram_user_id=$2 ORDER BY id DESC LIMIT 1`, group, id).Scan(&p.TelegramUsername, &p.TelegramName, &p.IsBot)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	seen := err == nil
	m := &service.CommunityMemberItem{}
	err = r.db.QueryRowContext(ctx, `SELECT m.user_id,u.email,u.username,u.status,m.telegram_user_id,m.telegram_username,m.telegram_name,m.status,m.joined_at FROM community_memberships m JOIN users u ON u.id=m.user_id AND u.deleted_at IS NULL WHERE m.group_chat_id=$1 AND m.telegram_user_id=$2`, group, id).Scan(&m.UserID, &m.Email, &m.Username, &m.UserStatus, &m.TelegramUserID, &m.TelegramUsername, &m.TelegramName, &m.Status, &m.JoinedAt)
	if err == nil {
		p.Member = m
		if !seen {
			p.TelegramUsername, p.TelegramName = m.TelegramUsername, m.TelegramName
		}
		return p, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if !seen {
		return nil, service.ErrCommunityNotFound
	}
	return p, nil
}
func (r *communityRepository) ReserveChatSend(ctx context.Context, s *service.CommunityChatSend) (bool, error) {
	acquired := false
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		// 群内发送串行预留，重试先查幂等记录，再应用群级分钟限流。
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, s.GroupChatID); err != nil {
			return err
		}
		var hash string
		err := tx.QueryRowContext(ctx, `SELECT id,content_hash,status,COALESCE(message_id,0),error_message FROM community_chat_sends WHERE group_chat_id=$1 AND admin_user_id=$2 AND client_request_id=$3`, s.GroupChatID, s.AdminUserID, s.ClientRequestID).Scan(&s.ID, &hash, &s.Status, &s.MessageID, &s.ErrorMessage)
		if err == nil {
			if hash != s.ContentHash {
				return service.ErrCommunityConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM community_chat_sends WHERE group_chat_id=$1 AND created_at>NOW()-INTERVAL '1 minute'`, s.GroupChatID).Scan(&count); err != nil {
			return err
		}
		if count >= 20 {
			return infraerrors.New(429, "COMMUNITY_CHAT_RATE_LIMIT", "群消息发送过于频繁，请一分钟后重试")
		}
		s.Status = "sending"
		err = tx.QueryRowContext(ctx, `INSERT INTO community_chat_sends(group_chat_id,bot_id,admin_user_id,client_request_id,content_hash) VALUES($1,$2,$3,$4,$5) RETURNING id`, s.GroupChatID, s.BotID, s.AdminUserID, s.ClientRequestID, s.ContentHash).Scan(&s.ID)
		acquired = err == nil
		return err
	})
	return acquired, err
}
func (r *communityRepository) FinishChatSend(ctx context.Context, s *service.CommunityChatSend, m *service.CommunityChatMessage) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM community_chat_sends WHERE id=$1 AND group_chat_id=$2 AND admin_user_id=$3 FOR UPDATE`, s.ID, s.GroupChatID, s.AdminUserID).Scan(&status); err != nil {
			return err
		}
		if status != "sending" {
			return service.ErrCommunityConflict
		}
		if m != nil {
			if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, m.GroupChatID); err != nil {
				return err
			}
			if err := saveCommunityChat(ctx, tx, m); err != nil {
				return err
			}
			s.MessageID = m.ID
		}
		return communityChanged(tx.ExecContext(ctx, `UPDATE community_chat_sends SET status=$2,message_id=NULLIF($3::bigint,0),error_message=$4 WHERE id=$1 AND status='sending'`, s.ID, s.Status, s.MessageID, s.ErrorMessage))
	})
}
func (r *communityRepository) GetChatAvatar(ctx context.Context, bot, id int64) (*service.CommunityChatFile, error) {
	f := &service.CommunityChatFile{FileName: "avatar.jpg"}
	err := r.db.QueryRowContext(ctx, `SELECT mime_type,data FROM community_chat_avatars WHERE bot_id=$1 AND telegram_user_id=$2 AND expires_at>NOW()`, bot, id).Scan(&f.MimeType, &f.Data)
	return f, communityError(err)
}
func (r *communityRepository) SaveChatAvatar(ctx context.Context, bot, id int64, f *service.CommunityChatFile) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO community_chat_avatars(bot_id,telegram_user_id,mime_type,data,expires_at) VALUES($1,$2,$3,$4,NOW()+INTERVAL '1 hour') ON CONFLICT(bot_id,telegram_user_id) DO UPDATE SET mime_type=EXCLUDED.mime_type,data=EXCLUDED.data,expires_at=EXCLUDED.expires_at WHERE community_chat_avatars.bot_id=EXCLUDED.bot_id AND community_chat_avatars.telegram_user_id=EXCLUDED.telegram_user_id`, bot, id, f.MimeType, f.Data)
	return err
}
