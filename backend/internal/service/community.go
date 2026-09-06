package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrCommunityInvalid     = infraerrors.BadRequest("COMMUNITY_INVALID", "社群配置或邀请信息无效")
	ErrCommunityDisabled    = infraerrors.Forbidden("COMMUNITY_DISABLED", "社群入群服务尚未开启")
	ErrCommunityNotFound    = infraerrors.NotFound("COMMUNITY_NOT_FOUND", "入群信息不存在或已失效")
	ErrCommunityConflict    = infraerrors.Conflict("COMMUNITY_CONFLICT", "入群身份已被占用或邀请状态已变化，请刷新后重试")
	ErrCommunityBusy        = infraerrors.Conflict("COMMUNITY_BUSY", "邀请链接正在生成，请稍后重试")
	ErrCommunityVIPRequired = infraerrors.Forbidden("COMMUNITY_VIP_REQUIRED", "仅成功支付过余额充值订单的用户可加入此群")
)

type CommunitySettings struct {
	RequirePaidRecharge bool   `json:"require_paid_recharge"`
	LoginPromptEnabled  bool   `json:"login_prompt_enabled"`
	Enabled             bool   `json:"enabled"`
	ContactURL          string `json:"contact_url"`
	GroupChatID         string `json:"group_chat_id"`
	GroupName           string `json:"group_name"`
	BotUsername         string `json:"bot_username"`
	BotID               int64  `json:"bot_id,omitempty"`
}
type CommunityInviteInput struct {
	ChallengeID    string `json:"challenge_id"`
	TelegramUserID int64  `json:"telegram_user_id"`
}
type CommunityTelegramIdentity struct {
	ID       int64
	Username string
	Name     string
}
type CommunityMembership struct {
	UserID             int64      `json:"-"`
	TelegramUserID     int64      `json:"telegram_user_id"`
	TelegramUsername   string     `json:"telegram_username"`
	TelegramName       string     `json:"telegram_name"`
	GroupChatID        int64      `json:"-"`
	Status             string     `json:"status"`
	JoinedAt           *time.Time `json:"joined_at,omitempty"`
	AuthorizedInviteID int64      `json:"-"`
	LastEventDate      int64      `json:"-"`
	LastUpdateID       int64      `json:"-"`
}
type CommunityChallenge struct {
	ID               string    `json:"id"`
	UserID           int64     `json:"-"`
	TokenHash        string    `json:"-"`
	BotID            int64     `json:"-"`
	TelegramUserID   int64     `json:"telegram_user_id,omitempty"`
	TelegramUsername string    `json:"telegram_username,omitempty"`
	TelegramName     string    `json:"telegram_name,omitempty"`
	Status           string    `json:"status"`
	ExpiresAt        time.Time `json:"expires_at"`
	BotURL           string    `json:"bot_url,omitempty"`
}
type CommunityInvite struct {
	ID             int64     `json:"-"`
	BotID          int64     `json:"-"`
	UserID         int64     `json:"-"`
	TelegramUserID int64     `json:"-"`
	GroupChatID    int64     `json:"-"`
	URL            string    `json:"url"`
	URLHash        string    `json:"-"`
	ExpiresAt      time.Time `json:"expires_at"`
	Status         string    `json:"-"`
	LeaseToken     string    `json:"-"`
	Attempts       int       `json:"-"`
}
type CommunityState struct {
	RequirePaidRecharge bool                 `json:"require_paid_recharge"`
	Eligible            bool                 `json:"eligible"`
	ShowJoinPrompt      bool                 `json:"show_join_prompt"`
	PromptKey           string               `json:"prompt_key,omitempty"`
	ContactURL          string               `json:"contact_url"`
	Enabled             bool                 `json:"enabled"`
	GroupName           string               `json:"group_name"`
	BotUsername         string               `json:"bot_username"`
	Membership          *CommunityMembership `json:"membership"`
	Challenge           *CommunityChallenge  `json:"challenge"`
	Invite              *CommunityInvite     `json:"invite"`
}
type CommunityWebhookEvent struct {
	ID         int64
	BotID      int64
	UpdateID   int64
	Payload    []byte
	LeaseToken string
	Attempts   int
}

type CommunityMemberFilter struct {
	Page     int
	PageSize int
	Search   string
	Status   string
}
type CommunityMemberItem struct {
	UserID           int64      `json:"user_id"`
	Email            string     `json:"email"`
	Username         string     `json:"username"`
	UserStatus       string     `json:"user_status"`
	TelegramUserID   int64      `json:"telegram_user_id,omitempty"`
	TelegramUsername string     `json:"telegram_username"`
	TelegramName     string     `json:"telegram_name"`
	Status           string     `json:"status"`
	JoinedAt         *time.Time `json:"joined_at,omitempty"`
	InviteExpiresAt  *time.Time `json:"invite_expires_at,omitempty"`
}
type CommunityMemberSummary struct {
	Total     int64 `json:"total"`
	Joined    int64 `json:"joined"`
	NotJoined int64 `json:"not_joined"`
	Pending   int64 `json:"pending"`
	Left      int64 `json:"left"`
}
type CommunityMemberPage struct {
	Items    []CommunityMemberItem  `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	Summary  CommunityMemberSummary `json:"summary"`
}

// CommunityRepository 将身份确认、唯一归属、加入授权和事件顺序验证放在数据库事务中。
type CommunityRepository interface {
	Cleanup(ctx context.Context) error
	EnsureActiveUser(ctx context.Context, userID int64) error
	HasPaidBalanceRecharge(ctx context.Context, userID int64) (bool, error)
	GetState(ctx context.Context, userID int64) (*CommunityMembership, *CommunityChallenge, *CommunityInvite, error)
	ListMembers(ctx context.Context, groupID, botID int64, filter CommunityMemberFilter) (*CommunityMemberPage, error)
	CreateChallenge(ctx context.Context, challenge *CommunityChallenge) error
	ClaimChallenge(ctx context.Context, tokenHash string, botID int64, identity CommunityTelegramIdentity) (*CommunityChallenge, error)
	ConfirmChallenge(ctx context.Context, userID int64, challengeID string, telegramID, groupID, botID int64) (*CommunityMembership, error)
	GetMembershipByTelegram(ctx context.Context, telegramID int64) (*CommunityMembership, error)
	AcquireInviteLease(ctx context.Context, userID, groupID int64, token string, lease time.Duration) (bool, error)
	SaveInvite(ctx context.Context, invite *CommunityInvite, leaseToken string) error
	ReleaseInviteLease(ctx context.Context, userID int64, leaseToken string) error
	// AuthorizeJoin 首次原子认领有效专属邀请并预留 Telegram 身份，已有归属不能替换。
	AuthorizeJoin(ctx context.Context, inviteHash string, identity CommunityTelegramIdentity, groupID, eventDate, updateID, botID int64, requirePaidRecharge bool) (*CommunityMembership, *CommunityInvite, error)
	// MarkMembership 校验事件先后顺序；joined 必须已有加入授权，left 只更新现有绑定。
	MarkMembership(ctx context.Context, telegramID, groupID int64, status string, eventDate, updateID int64) error
	QueueInviteRevocation(ctx context.Context, userID, inviteID int64) error
	ClaimRevocations(ctx context.Context, limit int, lease time.Duration) ([]CommunityInvite, error)
	CompleteRevocation(ctx context.Context, inviteID int64, leaseToken string) error
	FailRevocation(ctx context.Context, inviteID int64, leaseToken string, retryAt time.Time) error
	EnqueueWebhook(ctx context.Context, botID, updateID int64, payload []byte) error
	ClaimWebhooks(ctx context.Context, limit int, lease time.Duration) ([]CommunityWebhookEvent, error)
	CompleteWebhook(ctx context.Context, eventID int64, leaseToken string) error
	FailWebhook(ctx context.Context, eventID int64, leaseToken string, retryAt time.Time) error
}
