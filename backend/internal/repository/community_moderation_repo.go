package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const communityAdmissionQuery = `SELECT EXISTS (SELECT 1 FROM community_bans b WHERE b.group_chat_id=$3 AND (b.user_id=NULLIF($1::bigint,0) OR b.telegram_user_id=NULLIF($2::bigint,0) OR b.telegram_user_id=(SELECT m.telegram_user_id FROM community_memberships m WHERE m.user_id=$1)))`

// 群级事务锁保证未知身份的禁入记录与首次授权也能原子排序，所有写入先锁群再锁网站用户。
func communityLockAdmission(ctx context.Context, tx *sql.Tx, groupID int64) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('community-admission:' || $1::text,0))`, groupID)
	return err
}

func communityCheckAdmission(ctx context.Context, tx *sql.Tx, userID, telegramID, groupID int64) error {
	var banned bool
	if err := tx.QueryRowContext(ctx, communityAdmissionQuery, userID, telegramID, groupID).Scan(&banned); err != nil {
		return err
	}
	if banned {
		return service.ErrCommunityBanned
	}
	return nil
}

func (r *communityRepository) CheckAdmission(ctx context.Context, userID, telegramID, groupID int64) error {
	var banned bool
	if err := r.db.QueryRowContext(ctx, communityAdmissionQuery, userID, telegramID, groupID).Scan(&banned); err != nil {
		return err
	}
	if banned {
		return service.ErrCommunityBanned
	}
	return nil
}

func (r *communityRepository) RecordMemberRemoval(ctx context.Context, telegramID, groupID, eventDate, updateID, actorID int64, banned bool) error {
	if telegramID <= 0 || groupID >= 0 || eventDate <= 0 || updateID < 0 {
		return service.ErrCommunityInvalid
	}
	return r.transaction(ctx, func(tx *sql.Tx) error {
		if err := communityLockAdmission(ctx, tx, groupID); err != nil {
			return err
		}
		m, err := scanCommunityMember(tx.QueryRowContext(ctx, `SELECT `+communityMemberColumns+` FROM community_memberships WHERE telegram_user_id=$1`, telegramID))
		if errors.Is(err, service.ErrCommunityNotFound) {
			m = nil
		} else if err != nil {
			return err
		}
		var userID int64
		if m != nil {
			userID = m.UserID
			if err = communityLockUser(ctx, tx, userID, false); err != nil {
				return err
			}
			// 其他群的解绑也会竞争网站用户锁，拿到锁后重新核实归属，避免把旧快照写入禁入记录。
			current, readErr := scanCommunityMember(tx.QueryRowContext(ctx, `SELECT `+communityMemberColumns+` FROM community_memberships WHERE telegram_user_id=$1`, telegramID))
			if errors.Is(readErr, service.ErrCommunityNotFound) {
				m, userID = nil, 0
			} else if readErr != nil {
				return readErr
			} else if current.UserID != userID {
				return errors.New("社群绑定在处理成员变化时已变更，请重试")
			} else {
				m = current
			}
		}
		if banned {
			// 后到的管理员踢出证据也必须保留，较新的 unban/left 不能取消网站禁入策略。
			_, err = tx.ExecContext(ctx, `INSERT INTO community_bans(group_chat_id,telegram_user_id,user_id,actor_id,last_event_date,last_update_id) VALUES($1,$2,NULLIF($3::bigint,0),$4,$5,$6) ON CONFLICT(group_chat_id,telegram_user_id) DO UPDATE SET user_id=COALESCE(community_bans.user_id,EXCLUDED.user_id),actor_id=CASE WHEN (EXCLUDED.last_event_date,EXCLUDED.last_update_id)>(community_bans.last_event_date,community_bans.last_update_id) THEN EXCLUDED.actor_id ELSE community_bans.actor_id END,last_event_date=GREATEST(community_bans.last_event_date,EXCLUDED.last_event_date),last_update_id=CASE WHEN EXCLUDED.last_event_date>community_bans.last_event_date THEN EXCLUDED.last_update_id WHEN EXCLUDED.last_event_date=community_bans.last_event_date THEN GREATEST(community_bans.last_update_id,EXCLUDED.last_update_id) ELSE community_bans.last_update_id END`, groupID, telegramID, userID, actorID, eventDate, updateID)
			if err != nil || m == nil || m.GroupChatID != groupID {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE community_memberships SET status='banned',authorized_invite_id=NULL,bound_at=COALESCE(bound_at,NOW()),last_event_date=GREATEST(last_event_date,$3),last_update_id=CASE WHEN $3>last_event_date THEN $4 WHEN $3=last_event_date THEN GREATEST(last_update_id,$4) ELSE last_update_id END,updated_at=NOW() WHERE user_id=$1 AND telegram_user_id=$2`, userID, telegramID, eventDate, updateID); err != nil {
				return err
			}
		} else {
			if m == nil || m.GroupChatID != groupID || m.Status == "banned" {
				return nil
			}
			if err = communityCheckAdmission(ctx, tx, userID, telegramID, groupID); errors.Is(err, service.ErrCommunityBanned) {
				return nil
			} else if err != nil {
				return err
			}
			if eventDate < m.LastEventDate || (eventDate == m.LastEventDate && updateID < m.LastUpdateID) {
				return service.ErrCommunityConflict
			}
			if m.JoinedAt == nil && m.BoundAt == nil {
				_, err = tx.ExecContext(ctx, `DELETE FROM community_memberships WHERE user_id=$1 AND telegram_user_id=$2 AND bound_at IS NULL AND joined_at IS NULL`, userID, telegramID)
			} else {
				_, err = tx.ExecContext(ctx, `UPDATE community_memberships SET status='left',authorized_invite_id=NULL,last_event_date=$3,last_update_id=$4,updated_at=NOW() WHERE user_id=$1 AND telegram_user_id=$2`, userID, telegramID, eventDate, updateID)
			}
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE community_invites SET status='revoke_pending',available_at=NOW() WHERE user_id=$1 AND status='active'`, userID)
		return err
	})
}

func (r *communityRepository) Unbind(ctx context.Context, adminID, userID, ticketID int64) (*service.CommunityUnbindResult, error) {
	var result *service.CommunityUnbindResult
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		var validAdmin bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND role='admin' AND status='active' AND deleted_at IS NULL)`, adminID).Scan(&validAdmin); err != nil {
			return err
		}
		if !validAdmin {
			return service.ErrCommunityForbidden
		}
		var groupID int64
		if err := tx.QueryRowContext(ctx, `SELECT group_chat_id FROM community_memberships WHERE user_id=$1`, userID).Scan(&groupID); err != nil {
			return err
		}
		if err := communityLockAdmission(ctx, tx, groupID); err != nil {
			return err
		}
		if err := communityLockUser(ctx, tx, userID, false); err != nil {
			return err
		}
		m, err := scanCommunityMember(tx.QueryRowContext(ctx, `SELECT `+communityMemberColumns+` FROM community_memberships WHERE user_id=$1 FOR UPDATE`, userID))
		if err != nil {
			return err
		}
		if m.GroupChatID != groupID {
			return service.ErrCommunityConflict
		}
		var ticketCreated time.Time
		if err = tx.QueryRowContext(ctx, `SELECT created_at FROM support_tickets WHERE id=$1 AND user_id=$2 FOR UPDATE`, ticketID, userID).Scan(&ticketCreated); errors.Is(err, sql.ErrNoRows) {
			return service.ErrCommunityUnbindTicket
		} else if err != nil {
			return err
		}
		if m.BoundAt != nil && ticketCreated.Before(*m.BoundAt) {
			return service.ErrCommunityUnbindTicket
		}
		var used bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM community_unbind_audits WHERE ticket_id=$1)`, ticketID).Scan(&used); err != nil {
			return err
		}
		if used {
			return service.ErrCommunityUnbindTicket
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO community_unbind_audits(admin_id,user_id,telegram_user_id,group_chat_id,ticket_id) VALUES($1,$2,$3,$4,$5)`, adminID, userID, m.TelegramUserID, groupID, ticketID)
		if err != nil {
			return err
		}
		for _, query := range []string{
			`UPDATE community_invites SET status='revoke_pending',available_at=NOW() WHERE user_id=$1 AND status='active'`,
			`UPDATE community_challenges SET status='superseded',updated_at=NOW() WHERE user_id=$1 AND status IN ('waiting','claimed','confirmed')`,
			`DELETE FROM community_invite_leases WHERE user_id=$1`,
			`DELETE FROM community_memberships WHERE user_id=$1`,
		} {
			if _, err = tx.ExecContext(ctx, query, userID); err != nil {
				return err
			}
		}
		admission := communityCheckAdmission(ctx, tx, userID, m.TelegramUserID, groupID)
		if admission != nil && !errors.Is(admission, service.ErrCommunityBanned) {
			return admission
		}
		result = &service.CommunityUnbindResult{UserID: userID, TelegramUserID: m.TelegramUserID, TicketID: ticketID, Unbound: true, Banned: errors.Is(admission, service.ErrCommunityBanned)}
		return nil
	})
	return result, err
}
