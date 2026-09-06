package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type communityRepository struct{ db *sql.DB }
type communityScanner interface{ Scan(...any) error }

func NewCommunityRepository(db *sql.DB) service.CommunityRepository {
	return &communityRepository{db: db}
}

const communityMemberColumns = `user_id,telegram_user_id,telegram_username,telegram_name,group_chat_id,status,joined_at,COALESCE(authorized_invite_id,0),last_event_date,last_update_id`
const communityChallengeColumns = `id,user_id,bot_id,token_hash,COALESCE(telegram_user_id,0),telegram_username,telegram_name,status,expires_at`
const communityInviteColumns = `id,user_id,bot_id,COALESCE(telegram_user_id,0),group_chat_id,url,url_hash,expires_at,status,COALESCE(lease_token,''),attempts`
const communityPaidBalanceRechargeQuery = `SELECT EXISTS (SELECT 1 FROM payment_orders po WHERE po.user_id=$1 AND po.order_type='balance' AND po.paid_at IS NOT NULL AND po.pay_amount>0)`

func scanCommunityMember(row communityScanner) (*service.CommunityMembership, error) {
	m := &service.CommunityMembership{}
	err := row.Scan(&m.UserID, &m.TelegramUserID, &m.TelegramUsername, &m.TelegramName, &m.GroupChatID, &m.Status, &m.JoinedAt, &m.AuthorizedInviteID, &m.LastEventDate, &m.LastUpdateID)
	return m, communityError(err)
}
func scanCommunityChallenge(row communityScanner) (*service.CommunityChallenge, error) {
	c := &service.CommunityChallenge{}
	err := row.Scan(&c.ID, &c.UserID, &c.BotID, &c.TokenHash, &c.TelegramUserID, &c.TelegramUsername, &c.TelegramName, &c.Status, &c.ExpiresAt)
	return c, communityError(err)
}
func scanCommunityInvite(row communityScanner) (*service.CommunityInvite, error) {
	i := &service.CommunityInvite{}
	err := row.Scan(&i.ID, &i.UserID, &i.BotID, &i.TelegramUserID, &i.GroupChatID, &i.URL, &i.URLHash, &i.ExpiresAt, &i.Status, &i.LeaseToken, &i.Attempts)
	return i, communityError(err)
}
func communityError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrCommunityNotFound
	}
	var pg *pq.Error
	if errors.As(err, &pg) && pg.Code == "23505" {
		return service.ErrCommunityConflict
	}
	return err
}
func communityChanged(result sql.Result, err error) error {
	if err != nil {
		return communityError(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return service.ErrCommunityConflict
	}
	return nil
}
func (r *communityRepository) transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return communityError(err)
	}
	return communityError(tx.Commit())
}
func communityLockUser(ctx context.Context, tx *sql.Tx, userID int64, active bool) error {
	query := `SELECT id FROM users WHERE id=$1`
	if active {
		query += ` AND status='active' AND deleted_at IS NULL`
	}
	var id int64
	return communityError(tx.QueryRowContext(ctx, query+` FOR NO KEY UPDATE`, userID).Scan(&id))
}
func (r *communityRepository) EnsureActiveUser(ctx context.Context, userID int64) error {
	var id int64
	return communityError(r.db.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL`, userID).Scan(&id))
}

// HasPaidBalanceRecharge 以已核验的实际支付历史判断资格，后续退款不抹除曾充值的事实。
func (r *communityRepository) HasPaidBalanceRecharge(ctx context.Context, userID int64) (bool, error) {
	var paid bool
	err := r.db.QueryRowContext(ctx, communityPaidBalanceRechargeQuery, userID).Scan(&paid)
	return paid, err
}

func (r *communityRepository) GetState(ctx context.Context, userID int64) (*service.CommunityMembership, *service.CommunityChallenge, *service.CommunityInvite, error) {
	if err := r.EnsureActiveUser(ctx, userID); err != nil {
		return nil, nil, nil, err
	}
	m, err := scanCommunityMember(r.db.QueryRowContext(ctx, `SELECT `+communityMemberColumns+` FROM community_memberships WHERE user_id=$1`, userID))
	if errors.Is(err, service.ErrCommunityNotFound) {
		m = nil
	} else if err != nil {
		return nil, nil, nil, err
	}
	c, err := scanCommunityChallenge(r.db.QueryRowContext(ctx, `SELECT `+communityChallengeColumns+` FROM community_challenges WHERE user_id=$1 AND status IN ('waiting','claimed','confirmed') AND expires_at>NOW() ORDER BY created_at DESC,id DESC LIMIT 1`, userID))
	if errors.Is(err, service.ErrCommunityNotFound) {
		c = nil
	} else if err != nil {
		return nil, nil, nil, err
	}
	i, err := scanCommunityInvite(r.db.QueryRowContext(ctx, `SELECT `+communityInviteColumns+` FROM community_invites WHERE user_id=$1 AND status='active' AND expires_at>NOW() ORDER BY id DESC LIMIT 1`, userID))
	if errors.Is(err, service.ErrCommunityNotFound) {
		i = nil
	} else if err != nil {
		return nil, nil, nil, err
	}
	return m, c, i, nil
}

func (r *communityRepository) CreateChallenge(ctx context.Context, c *service.CommunityChallenge) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		if err := communityLockUser(ctx, tx, c.UserID, true); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE community_challenges SET status='superseded',updated_at=NOW() WHERE user_id=$1 AND status IN ('waiting','claimed')`, c.UserID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM community_challenges WHERE user_id=$1 AND (status='superseded' OR expires_at<=NOW())`, c.UserID); err != nil {
			return err
		}
		err := communityChanged(tx.ExecContext(ctx, `INSERT INTO community_challenges(id,user_id,bot_id,token_hash,status,expires_at) SELECT $1,$2,$3,$4,'waiting',$5 WHERE $5>NOW()`, c.ID, c.UserID, c.BotID, c.TokenHash, c.ExpiresAt))
		if err == nil {
			c.Status = "waiting"
		}
		return err
	})
}

func (r *communityRepository) ClaimChallenge(ctx context.Context, tokenHash string, botID int64, identity service.CommunityTelegramIdentity) (*service.CommunityChallenge, error) {
	var result *service.CommunityChallenge
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		var userID int64
		if err := tx.QueryRowContext(ctx, `SELECT user_id FROM community_challenges WHERE token_hash=$1 AND bot_id=$2`, tokenHash, botID).Scan(&userID); err != nil {
			return err
		}
		if err := communityLockUser(ctx, tx, userID, true); err != nil {
			return err
		}
		c, err := scanCommunityChallenge(tx.QueryRowContext(ctx, `SELECT `+communityChallengeColumns+` FROM community_challenges WHERE token_hash=$1 AND bot_id=$2 AND expires_at>NOW() AND status IN ('waiting','claimed','confirmed') FOR UPDATE`, tokenHash, botID))
		if err != nil {
			return err
		}
		if identity.ID <= 0 {
			return service.ErrCommunityInvalid
		}
		if c.TelegramUserID != 0 && c.TelegramUserID != identity.ID {
			return service.ErrCommunityConflict
		}
		if c.Status == "waiting" {
			if err := communityChanged(tx.ExecContext(ctx, `UPDATE community_challenges SET telegram_user_id=$2,telegram_username=$3,telegram_name=$4,status='claimed',updated_at=NOW() WHERE id=$1 AND status='waiting'`, c.ID, identity.ID, identity.Username, identity.Name)); err != nil {
				return err
			}
			c.TelegramUserID = identity.ID
			c.TelegramUsername = identity.Username
			c.TelegramName = identity.Name
			c.Status = "claimed"
		}
		result = c
		return nil
	})
	return result, err
}

func (r *communityRepository) ConfirmChallenge(ctx context.Context, userID int64, challengeID string, telegramID, groupID, botID int64) (*service.CommunityMembership, error) {
	var result *service.CommunityMembership
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		if err := communityLockUser(ctx, tx, userID, true); err != nil {
			return err
		}
		c, err := scanCommunityChallenge(tx.QueryRowContext(ctx, `SELECT `+communityChallengeColumns+` FROM community_challenges WHERE id=$1 AND user_id=$2 AND bot_id=$3 AND expires_at>NOW() AND status IN ('claimed','confirmed') FOR UPDATE`, challengeID, userID, botID))
		if err != nil {
			return err
		}
		if telegramID <= 0 || groupID == 0 || c.TelegramUserID != telegramID {
			return service.ErrCommunityConflict
		}
		m, err := scanCommunityMember(tx.QueryRowContext(ctx, `SELECT `+communityMemberColumns+` FROM community_memberships WHERE user_id=$1 FOR UPDATE`, userID))
		if errors.Is(err, service.ErrCommunityNotFound) {
			m, err = scanCommunityMember(tx.QueryRowContext(ctx, `INSERT INTO community_memberships(user_id,telegram_user_id,telegram_username,telegram_name,group_chat_id) VALUES($1,$2,$3,$4,$5) RETURNING `+communityMemberColumns, userID, telegramID, c.TelegramUsername, c.TelegramName, groupID))
		}
		if err != nil {
			return err
		}
		if m.TelegramUserID != telegramID {
			return service.ErrCommunityConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE community_challenges SET status='confirmed',updated_at=NOW() WHERE id=$1 AND user_id=$2`, challengeID, userID); err != nil {
			return err
		}
		result = m
		return nil
	})
	return result, err
}

func (r *communityRepository) GetMembershipByTelegram(ctx context.Context, telegramID int64) (*service.CommunityMembership, error) {
	m, err := scanCommunityMember(r.db.QueryRowContext(ctx, `SELECT `+communityMemberColumns+` FROM community_memberships WHERE telegram_user_id=$1`, telegramID))
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *communityRepository) AcquireInviteLease(ctx context.Context, userID, groupID int64, token string, lease time.Duration) (bool, error) {
	acquired := false
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		if err := communityLockUser(ctx, tx, userID, true); err != nil {
			return err
		}
		// 无会员记录时同样可以领取；已有审批中的身份或有效邀请不能被新请求覆盖。
		result, err := tx.ExecContext(ctx, `INSERT INTO community_invite_leases(user_id,group_chat_id,lease_token,lease_until) SELECT $1,$2,$3,NOW()+($4*INTERVAL '1 second') WHERE NOT EXISTS(SELECT 1 FROM community_memberships m WHERE m.user_id=$1 AND m.group_chat_id=$2 AND (m.status='joined' OR m.authorized_invite_id IS NOT NULL)) AND NOT EXISTS(SELECT 1 FROM community_invites i WHERE i.user_id=$1 AND i.group_chat_id=$2 AND i.status='active' AND i.expires_at>NOW()) ON CONFLICT(user_id) DO UPDATE SET group_chat_id=EXCLUDED.group_chat_id,lease_token=EXCLUDED.lease_token,lease_until=EXCLUDED.lease_until WHERE community_invite_leases.lease_until<=NOW()`, userID, groupID, token, communityLeaseSeconds(lease))
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		acquired = n == 1
		return err
	})
	return acquired, err
}

func (r *communityRepository) SaveInvite(ctx context.Context, i *service.CommunityInvite, leaseToken string) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		if err := communityLockUser(ctx, tx, i.UserID, true); err != nil {
			return err
		}
		// 事务中消费生成租约；无会员时不创建任何虚假的 Telegram 身份。
		if err := communityChanged(tx.ExecContext(ctx, `DELETE FROM community_invite_leases l WHERE l.user_id=$1 AND l.group_chat_id=$2 AND l.lease_token=$3 AND l.lease_until>NOW() AND NOT EXISTS(SELECT 1 FROM community_memberships m WHERE m.user_id=$1 AND m.group_chat_id=$2 AND (m.status='joined' OR m.authorized_invite_id IS NOT NULL))`, i.UserID, i.GroupChatID, leaseToken)); err != nil {
			return err
		}
		m, err := scanCommunityMember(tx.QueryRowContext(ctx, `SELECT `+communityMemberColumns+` FROM community_memberships WHERE user_id=$1 FOR UPDATE`, i.UserID))
		if errors.Is(err, service.ErrCommunityNotFound) {
			if i.TelegramUserID != 0 {
				return service.ErrCommunityConflict
			}
		} else if err != nil {
			return err
		} else {
			if i.TelegramUserID != 0 && i.TelegramUserID != m.TelegramUserID {
				return service.ErrCommunityConflict
			}
			i.TelegramUserID = m.TelegramUserID
		}
		if _, err := tx.ExecContext(ctx, `UPDATE community_invites SET status='revoke_pending',available_at=NOW() WHERE user_id=$1 AND status='active'`, i.UserID); err != nil {
			return err
		}
		return communityError(tx.QueryRowContext(ctx, `INSERT INTO community_invites(user_id,bot_id,telegram_user_id,group_chat_id,url,url_hash,expires_at) SELECT $1,$2,NULLIF($3::bigint,0),$4,$5,$6,$7 WHERE $7>NOW() RETURNING id,status`, i.UserID, i.BotID, i.TelegramUserID, i.GroupChatID, i.URL, i.URLHash, i.ExpiresAt).Scan(&i.ID, &i.Status))
	})
}
func (r *communityRepository) ReleaseInviteLease(ctx context.Context, userID int64, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM community_invite_leases WHERE user_id=$1 AND lease_token=$2`, userID, token)
	return err
}

func (r *communityRepository) AuthorizeJoin(ctx context.Context, inviteHash string, identity service.CommunityTelegramIdentity, groupID, eventDate, updateID, botID int64, requirePaidRecharge bool) (*service.CommunityMembership, *service.CommunityInvite, error) {
	if identity.ID <= 0 || eventDate <= 0 || updateID < 0 || botID <= 0 || groupID == 0 {
		return nil, nil, service.ErrCommunityInvalid
	}
	var member *service.CommunityMembership
	var invite *service.CommunityInvite
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		var userID int64
		if err := tx.QueryRowContext(ctx, `SELECT user_id FROM community_invites WHERE url_hash=$1 AND group_chat_id=$2 AND bot_id=$3`, inviteHash, groupID, botID).Scan(&userID); err != nil {
			return err
		}
		if err := communityLockUser(ctx, tx, userID, true); err != nil {
			return err
		}
		// 在认领身份前核对当前群要求，旧邀请也不能绕过新增的充值门槛。
		if requirePaidRecharge {
			var paid bool
			if err := tx.QueryRowContext(ctx, communityPaidBalanceRechargeQuery, userID).Scan(&paid); err != nil {
				return err
			}
			if !paid {
				return service.ErrCommunityVIPRequired
			}
		}
		m, err := scanCommunityMember(tx.QueryRowContext(ctx, `SELECT `+communityMemberColumns+` FROM community_memberships WHERE user_id=$1 FOR UPDATE`, userID))
		if errors.Is(err, service.ErrCommunityNotFound) {
			m = nil
		} else if err != nil {
			return err
		}
		var authorizedID, lastDate, lastUpdate int64
		lastUpdate = -1
		if m != nil {
			if m.TelegramUserID != identity.ID {
				return service.ErrCommunityConflict
			}
			if m.GroupChatID == groupID {
				if m.Status == "joined" || eventDate < m.LastEventDate || (eventDate == m.LastEventDate && updateID < m.LastUpdateID) {
					return service.ErrCommunityConflict
				}
				authorizedID, lastDate, lastUpdate = m.AuthorizedInviteID, m.LastEventDate, m.LastUpdateID
			}
		}
		// 同一已授权事件重试可以越过链接到期时间；新的申请必须使用当前有效链接。
		i, err := scanCommunityInvite(tx.QueryRowContext(ctx, `SELECT `+communityInviteColumns+` FROM community_invites WHERE url_hash=$1 AND user_id=$2 AND (telegram_user_id IS NULL OR telegram_user_id=$3) AND group_chat_id=$4 AND bot_id=$5 AND ((status='active' AND expires_at>NOW()) OR (id=$6 AND $7::bigint=$8::bigint AND $9::bigint=$10::bigint)) FOR UPDATE`, inviteHash, userID, identity.ID, groupID, botID, authorizedID, eventDate, lastDate, updateID, lastUpdate))
		if err != nil {
			return err
		}
		// 首次申请时预留唯一归属；同一链接的后续申请不能替换首次认领者。
		if m == nil {
			m, err = scanCommunityMember(tx.QueryRowContext(ctx, `INSERT INTO community_memberships(user_id,telegram_user_id,telegram_username,telegram_name,group_chat_id,authorized_invite_id,last_event_date,last_update_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+communityMemberColumns, userID, identity.ID, identity.Username, identity.Name, groupID, i.ID, eventDate, updateID))
			if err != nil {
				return err
			}
		} else if err = communityChanged(tx.ExecContext(ctx, `UPDATE community_memberships SET telegram_username=$2,telegram_name=$3,group_chat_id=$4,status='pending',authorized_invite_id=$5,last_event_date=$6,last_update_id=$7,updated_at=NOW() WHERE user_id=$1 AND telegram_user_id=$8`, userID, identity.Username, identity.Name, groupID, i.ID, eventDate, updateID, identity.ID)); err != nil {
			return err
		}
		if err = communityChanged(tx.ExecContext(ctx, `UPDATE community_invites SET telegram_user_id=$2 WHERE id=$1 AND (telegram_user_id IS NULL OR telegram_user_id=$2)`, i.ID, identity.ID)); err != nil {
			return err
		}
		m.TelegramUsername = identity.Username
		m.TelegramName = identity.Name
		m.GroupChatID = groupID
		m.Status = "pending"
		i.TelegramUserID = identity.ID
		m.AuthorizedInviteID = i.ID
		m.LastEventDate = eventDate
		m.LastUpdateID = updateID
		member = m
		invite = i
		return nil
	})
	return member, invite, err
}

func (r *communityRepository) MarkMembership(ctx context.Context, telegramID, groupID int64, status string, eventDate, updateID int64) error {
	if status != "joined" && status != "left" {
		return service.ErrCommunityInvalid
	}
	return r.transaction(ctx, func(tx *sql.Tx) error {
		var userID int64
		if err := tx.QueryRowContext(ctx, `SELECT user_id FROM community_memberships WHERE telegram_user_id=$1 AND group_chat_id=$2`, telegramID, groupID).Scan(&userID); err != nil {
			return err
		}
		if err := communityLockUser(ctx, tx, userID, status == "joined"); err != nil {
			return err
		}
		m, err := scanCommunityMember(tx.QueryRowContext(ctx, `SELECT `+communityMemberColumns+` FROM community_memberships WHERE user_id=$1 FOR UPDATE`, userID))
		if err != nil {
			return err
		}
		if m.TelegramUserID != telegramID || m.GroupChatID != groupID || eventDate < m.LastEventDate || (eventDate == m.LastEventDate && updateID < m.LastUpdateID) {
			return service.ErrCommunityConflict
		}
		if status == "joined" && m.AuthorizedInviteID <= 0 {
			return service.ErrCommunityConflict
		}
		// 尚未真实入群的身份仅为临时预留；失败时撤销凭证并释放双向唯一占用。
		if status == "left" && m.JoinedAt == nil {
			if _, err = tx.ExecContext(ctx, `UPDATE community_invites SET status='revoke_pending',available_at=NOW() WHERE user_id=$1 AND status='active'`, userID); err != nil {
				return err
			}
			return communityChanged(tx.ExecContext(ctx, `DELETE FROM community_memberships WHERE user_id=$1 AND telegram_user_id=$2 AND group_chat_id=$3 AND joined_at IS NULL`, userID, telegramID, groupID))
		}
		if err = communityChanged(tx.ExecContext(ctx, `UPDATE community_memberships SET status=$2,last_event_date=$3,last_update_id=$4,joined_at=CASE WHEN $2='joined' THEN COALESCE(joined_at,NOW()) ELSE joined_at END,authorized_invite_id=CASE WHEN $2='left' THEN NULL ELSE authorized_invite_id END,updated_at=NOW() WHERE user_id=$1`, userID, status, eventDate, updateID)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE community_invites SET status='revoke_pending',available_at=NOW() WHERE user_id=$1 AND status='active'`, userID)
		return err
	})
}
func (r *communityRepository) QueueInviteRevocation(ctx context.Context, userID, inviteID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE community_invites SET status='revoke_pending',available_at=NOW() WHERE user_id=$1 AND id=$2 AND status='active'`, userID, inviteID)
	return err
}

// 从网站用户全集出发，未申请过邀请的用户也参与未加入统计。
const communityMemberListBase = `WITH members AS (
	SELECT u.id AS user_id,u.email,u.username,u.status AS user_status,
	COALESCE(m.telegram_user_id,0) AS telegram_user_id,
	COALESCE(m.telegram_username,'') AS telegram_username,COALESCE(m.telegram_name,'') AS telegram_name,
	CASE WHEN m.status='joined' THEN 'joined' WHEN inv.expires_at IS NOT NULL THEN 'pending' ELSE COALESCE(m.status,'not_joined') END AS status,
	m.joined_at,inv.expires_at AS invite_expires_at
	FROM users u
	LEFT JOIN community_memberships m ON m.user_id=u.id AND m.group_chat_id=$1
	LEFT JOIN LATERAL (SELECT i.expires_at FROM community_invites i WHERE i.user_id=u.id AND i.group_chat_id=$1 AND i.bot_id=$2 AND i.status='active' AND i.expires_at>NOW() ORDER BY i.id DESC LIMIT 1) inv ON TRUE
	WHERE u.deleted_at IS NULL AND ($3='' OR u.id::text=$3 OR u.email ILIKE $4 ESCAPE '\' OR u.username ILIKE $4 ESCAPE '\' OR m.telegram_user_id::text=$3)
) `

func (r *communityRepository) ListMembers(ctx context.Context, groupID, botID int64, filter service.CommunityMemberFilter) (*service.CommunityMemberPage, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	filter.PageSize = communityLimit(filter.PageSize)
	status := strings.TrimSpace(filter.Status)
	if status == "" {
		status = "all"
	}
	switch status {
	case "all", "joined", "not_joined", "pending", "left":
	default:
		return nil, service.ErrCommunityInvalid
	}
	search := strings.TrimSpace(filter.Search)
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search) + "%"
	args := []any{groupID, botID, search, pattern}
	page := &service.CommunityMemberPage{Items: []service.CommunityMemberItem{}, Page: filter.Page, PageSize: filter.PageSize}
	// 统计只受搜索条件影响，切换状态筛选不会改变统计卡片。
	err := r.db.QueryRowContext(ctx, communityMemberListBase+`SELECT COUNT(*),COUNT(*) FILTER (WHERE status='joined'),COUNT(*) FILTER (WHERE status<>'joined'),COUNT(*) FILTER (WHERE status='pending'),COUNT(*) FILTER (WHERE status='left') FROM members`, args...).Scan(&page.Summary.Total, &page.Summary.Joined, &page.Summary.NotJoined, &page.Summary.Pending, &page.Summary.Left)
	if err != nil {
		return nil, err
	}
	switch status {
	case "joined":
		page.Total = page.Summary.Joined
	case "not_joined":
		page.Total = page.Summary.NotJoined
	case "pending":
		page.Total = page.Summary.Pending
	case "left":
		page.Total = page.Summary.Left
	default:
		page.Total = page.Summary.Total
	}
	args = append(args, status, filter.PageSize, int64(filter.Page-1)*int64(filter.PageSize))
	rows, err := r.db.QueryContext(ctx, communityMemberListBase+`SELECT user_id,email,username,user_status,telegram_user_id,telegram_username,telegram_name,status,joined_at,invite_expires_at FROM members WHERE ($5='all' OR ($5='not_joined' AND status<>'joined') OR status=$5) ORDER BY user_id DESC LIMIT $6 OFFSET $7`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item service.CommunityMemberItem
		if err := rows.Scan(&item.UserID, &item.Email, &item.Username, &item.UserStatus, &item.TelegramUserID, &item.TelegramUsername, &item.TelegramName, &item.Status, &item.JoinedAt, &item.InviteExpiresAt); err != nil {
			return nil, err
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

func communityLeaseSeconds(lease time.Duration) int64 {
	if lease < time.Second {
		return 60
	}
	return int64(lease / time.Second)
}
func communityLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > 100 {
		return 100
	}
	return limit
}
func (r *communityRepository) ClaimRevocations(ctx context.Context, limit int, lease time.Duration) ([]service.CommunityInvite, error) {
	token := uuid.NewString()
	rows, err := r.db.QueryContext(ctx, `WITH picked AS (SELECT id FROM community_invites WHERE ((status='revoke_pending' AND available_at<=NOW()) OR (status='active' AND expires_at<=NOW())) AND (lease_until IS NULL OR lease_until<NOW()) ORDER BY available_at,id LIMIT $1 FOR UPDATE SKIP LOCKED) UPDATE community_invites i SET status='revoke_pending',lease_token=$2,lease_until=NOW()+($3*INTERVAL '1 second'),attempts=i.attempts+1 FROM picked p WHERE i.id=p.id RETURNING i.id,i.user_id,i.bot_id,COALESCE(i.telegram_user_id,0),i.group_chat_id,i.url,i.url_hash,i.expires_at,i.status,i.lease_token,i.attempts`, communityLimit(limit), token, communityLeaseSeconds(lease))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []service.CommunityInvite{}
	for rows.Next() {
		i, err := scanCommunityInvite(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *i)
	}
	return items, rows.Err()
}
func (r *communityRepository) CompleteRevocation(ctx context.Context, inviteID int64, token string) error {
	return communityChanged(r.db.ExecContext(ctx, `UPDATE community_invites SET status='revoked',revoked_at=NOW(),lease_token=NULL,lease_until=NULL WHERE id=$1 AND lease_token=$2 AND status='revoke_pending'`, inviteID, token))
}
func (r *communityRepository) FailRevocation(ctx context.Context, inviteID int64, token string, retryAt time.Time) error {
	return communityChanged(r.db.ExecContext(ctx, `UPDATE community_invites SET available_at=$3,lease_token=NULL,lease_until=NULL WHERE id=$1 AND lease_token=$2 AND status='revoke_pending'`, inviteID, token, retryAt))
}
func (r *communityRepository) EnqueueWebhook(ctx context.Context, botID, updateID int64, payload []byte) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO community_webhook_events(bot_id,update_id,payload) VALUES($1,$2,$3::jsonb) ON CONFLICT(bot_id,update_id) DO NOTHING`, botID, updateID, string(payload))
	return err
}
func (r *communityRepository) ClaimWebhooks(ctx context.Context, limit int, lease time.Duration) ([]service.CommunityWebhookEvent, error) {
	token := uuid.NewString()
	rows, err := r.db.QueryContext(ctx, `WITH picked AS (SELECT id FROM community_webhook_events WHERE completed_at IS NULL AND available_at<=NOW() AND (lease_until IS NULL OR lease_until<NOW()) ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED) UPDATE community_webhook_events e SET lease_token=$2,lease_until=NOW()+($3*INTERVAL '1 second'),attempts=e.attempts+1 FROM picked p WHERE e.id=p.id RETURNING e.id,e.bot_id,e.update_id,e.payload,e.lease_token,e.attempts`, communityLimit(limit), token, communityLeaseSeconds(lease))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []service.CommunityWebhookEvent{}
	for rows.Next() {
		var e service.CommunityWebhookEvent
		if err := rows.Scan(&e.ID, &e.BotID, &e.UpdateID, &e.Payload, &e.LeaseToken, &e.Attempts); err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}
func (r *communityRepository) CompleteWebhook(ctx context.Context, eventID int64, token string) error {
	return communityChanged(r.db.ExecContext(ctx, `UPDATE community_webhook_events SET completed_at=NOW(),payload='{}'::jsonb,lease_token=NULL,lease_until=NULL WHERE id=$1 AND lease_token=$2 AND completed_at IS NULL`, eventID, token))
}
func (r *communityRepository) FailWebhook(ctx context.Context, eventID int64, token string, retryAt time.Time) error {
	return communityChanged(r.db.ExecContext(ctx, `UPDATE community_webhook_events SET available_at=$3,lease_token=NULL,lease_until=NULL WHERE id=$1 AND lease_token=$2 AND completed_at IS NULL`, eventID, token, retryAt))
}

// Cleanup 按类型分批回收过期数据，跳过其它实例正在处理的行，并保留仍用于会员授权的邀请。
func (r *communityRepository) Cleanup(ctx context.Context) error {
	queries := []string{
		`WITH expired AS (SELECT id FROM community_webhook_events WHERE completed_at < NOW()-INTERVAL '7 days' ORDER BY completed_at,id LIMIT 500 FOR UPDATE SKIP LOCKED) DELETE FROM community_webhook_events e USING expired x WHERE e.id=x.id AND e.completed_at < NOW()-INTERVAL '7 days'`,
		`WITH expired AS (SELECT id FROM community_challenges WHERE expires_at < NOW()-INTERVAL '1 day' ORDER BY expires_at,id LIMIT 500 FOR UPDATE SKIP LOCKED) DELETE FROM community_challenges c USING expired x WHERE c.id=x.id AND c.expires_at < NOW()-INTERVAL '1 day'`,
		`WITH expired AS (SELECT i.id FROM community_invites i WHERE i.status='revoked' AND i.revoked_at < NOW()-INTERVAL '30 days' AND NOT EXISTS (SELECT 1 FROM community_memberships m WHERE m.authorized_invite_id=i.id) ORDER BY i.revoked_at,i.id LIMIT 500 FOR UPDATE OF i SKIP LOCKED) DELETE FROM community_invites i USING expired x WHERE i.id=x.id AND i.status='revoked' AND NOT EXISTS (SELECT 1 FROM community_memberships m WHERE m.authorized_invite_id=i.id)`,
	}
	for _, query := range queries {
		if _, err := r.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}
