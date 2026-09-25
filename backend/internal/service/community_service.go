package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const SettingKeyCommunity = "community_config"

type CommunityService struct {
	settings          SettingRepository
	repo              CommunityRepository
	delivery          *SupportDeliveryService
	mu                sync.Mutex
	configMu          sync.Mutex
	wg                sync.WaitGroup
	cancel            context.CancelFunc
	verifiedTokenHash string
	verifiedBotID     int64
	verifiedAt        time.Time
	lastCleanup       time.Time
}

func NewCommunityService(settings SettingRepository, repo CommunityRepository, delivery *SupportDeliveryService) *CommunityService {
	return &CommunityService{settings: settings, repo: repo, delivery: delivery}
}

func (s *CommunityService) GetSettings(ctx context.Context) (*CommunitySettings, error) {
	raw, err := s.settings.GetValue(ctx, SettingKeyCommunity)
	if errors.Is(err, ErrSettingNotFound) {
		return &CommunitySettings{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c CommunitySettings
	if err = json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

var communityBotUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{5,32}$`)

// 保存设置时保留原错误码，按字段返回具体原因，不修改共享错误或回显凭据。
func communitySettingsFieldError(field, message string) error {
	err := ErrCommunityInvalid.WithMetadata(map[string]string{"field": field})
	err.Message = message
	return err
}

// 仅公开校验阶段和安全状态码，Telegram 原始响应及含令牌的请求地址不进入提示。
func communitySettingsTelegramError(field, action string, err error) error {
	var api *SupportTelegramAPIError
	if errors.As(err, &api) {
		switch api.StatusCode {
		case http.StatusUnauthorized, http.StatusNotFound:
			return communitySettingsFieldError("telegram_bot_token", "Telegram 机器人身份验证失败，请在“机器人设置”中检查令牌是否有效")
		case http.StatusBadRequest, http.StatusForbidden:
			message := action + "失败，请检查群组 Chat ID，并确认机器人已加入该群且已设为管理员"
			if field == "telegram_bot_token" {
				message = "Telegram 机器人身份验证失败，请在“机器人设置”中检查完整令牌"
			}
			return communitySettingsFieldError(field, fmt.Sprintf("%s（Telegram 状态 %d）", message, api.StatusCode))
		case http.StatusTooManyRequests:
			return infraerrors.New(http.StatusBadGateway, "COMMUNITY_TELEGRAM_CHECK_FAILED", "Telegram 请求过于频繁，请稍后重新保存").WithMetadata(map[string]string{"field": field})
		}
		return infraerrors.New(http.StatusBadGateway, "COMMUNITY_TELEGRAM_CHECK_FAILED", fmt.Sprintf("%s失败（Telegram 状态 %d），请稍后重试", action, api.StatusCode)).WithMetadata(map[string]string{"field": field})
	}
	return infraerrors.New(http.StatusBadGateway, "COMMUNITY_TELEGRAM_CHECK_FAILED", action+"失败，请检查服务器与 Telegram 的连接后重试").WithMetadata(map[string]string{"field": field})
}

func (s *CommunityService) UpdateSettings(ctx context.Context, c CommunitySettings) (*CommunitySettings, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	s.configMu.Lock()
	defer s.configMu.Unlock()
	old, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	c.BotID = old.BotID
	c.ContactURL = strings.TrimSpace(c.ContactURL)
	c.GroupChatID = strings.TrimSpace(c.GroupChatID)
	c.GroupName = strings.TrimSpace(c.GroupName)
	c.BotUsername = strings.TrimPrefix(strings.TrimSpace(c.BotUsername), "@")
	if len(c.ContactURL) > 2048 {
		return nil, communitySettingsFieldError("contact_url", "客服页面地址长度不能超过 2048 字节")
	}
	if len([]rune(c.GroupName)) > 100 || strings.ContainsAny(c.GroupName, "\x00\r\n") {
		return nil, communitySettingsFieldError("group_name", "群组显示名称不能超过 100 个字符，且不能包含换行或空字符")
	}
	if c.ContactURL != "" {
		u, err := url.Parse(c.ContactURL)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || strings.ContainsAny(c.ContactURL, "\x00\r\n\t") {
			return nil, communitySettingsFieldError("contact_url", "客服页面地址无效，请填写不含账号密码的完整 HTTP 或 HTTPS 地址")
		}
	}
	if c.BotUsername != "" && !communityBotUsernamePattern.MatchString(c.BotUsername) {
		return nil, communitySettingsFieldError("bot_username", "机器人用户名必须为 5～32 位英文字母、数字或下划线，此处不能填写机器人令牌")
	}
	if c.GroupChatID != "" {
		id, err := strconv.ParseInt(c.GroupChatID, 10, 64)
		if err != nil || id >= 0 {
			return nil, communitySettingsFieldError("group_chat_id", "Telegram 群组 Chat ID 必须为负整数，请保留负号并填写完整的群组 ID")
		}
	}
	if c.Enabled {
		shared, err := s.delivery.loadSettings(ctx)
		if err != nil {
			return nil, err
		}
		if c.GroupChatID == "" {
			return nil, communitySettingsFieldError("group_chat_id", "启用 Telegram 社群时，请填写群组 Chat ID")
		}
		if shared.TelegramBotToken == "" {
			return nil, communitySettingsFieldError("telegram_bot_token", "请先在“机器人设置”中配置并保存 Telegram 机器人令牌")
		}
		if !supportTelegramTokenPattern.MatchString(shared.TelegramBotToken) {
			return nil, communitySettingsFieldError("telegram_bot_token", "Telegram 机器人令牌格式无效，请在“机器人设置”中填写 BotFather 提供的完整令牌")
		}
		if shared.TelegramWebhookSecret == "" {
			return nil, communitySettingsFieldError("telegram_webhook_secret", "请先在“机器人设置”中配置并保存 Webhook 验证密钥")
		}
		if !supportTelegramSecretPattern.MatchString(shared.TelegramWebhookSecret) {
			return nil, communitySettingsFieldError("telegram_webhook_secret", "Webhook 验证密钥必须为 16～256 位英文字母、数字、下划线或短横线，请在“机器人设置”中修改")
		}
		var bot communityTelegramUser
		if err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "getMe", map[string]any{}, &bot); err != nil {
			return nil, communitySettingsTelegramError("telegram_bot_token", "验证 Telegram 机器人身份", err)
		}
		if !bot.IsBot || bot.ID <= 0 || !communityBotUsernamePattern.MatchString(bot.Username) {
			return nil, communitySettingsFieldError("telegram_bot_token", "Telegram 未返回有效的机器人身份，请在“机器人设置”中检查令牌")
		}
		var group communityTelegramChat
		if err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "getChat", communityChatRequest{ChatID: c.GroupChatID}, &group); err != nil {
			return nil, communitySettingsTelegramError("group_chat_id", "通过机器人 @"+bot.Username+" 读取 Telegram 群组", err)
		}
		if group.Type != "group" && group.Type != "supergroup" {
			return nil, communitySettingsFieldError("group_chat_id", "Telegram Chat ID 对应的不是群组，请填写私密群组 ID，不能使用频道或个人会话 ID")
		}
		if group.Username != "" || len(group.ActiveUsernames) > 0 {
			return nil, communitySettingsFieldError("group_chat_id", "Telegram 群组必须为私密群，请在 Telegram 中关闭群组的公开用户名后重新保存")
		}
		if strconv.FormatInt(group.ID, 10) != c.GroupChatID {
			return nil, communitySettingsFieldError("group_chat_id", "Telegram 返回的群组 ID 与填写值不一致，请重新获取当前群组 Chat ID")
		}
		member, err := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, bot.ID)
		if err != nil {
			return nil, communitySettingsTelegramError("bot_permissions", "读取 Telegram 机器人 @"+bot.Username+" 的群权限", err)
		}
		if member.Status != "administrator" {
			return nil, communitySettingsFieldError("bot_permissions", "Telegram 机器人 @"+bot.Username+" 尚未设为群管理员，请将该机器人添加为管理员，并授予邀请用户和封禁用户权限")
		}
		var missingPermissions []string
		if !member.CanInviteUsers {
			missingPermissions = append(missingPermissions, "邀请用户")
		}
		if !member.CanRestrictMembers {
			missingPermissions = append(missingPermissions, "封禁用户")
		}
		if len(missingPermissions) > 0 {
			return nil, communitySettingsFieldError("bot_permissions", "Telegram 机器人 @"+bot.Username+" 缺少“"+strings.Join(missingPermissions, "、")+"”权限，请在群管理员设置中开启后重新保存")
		}
		c.BotID = bot.ID
		c.BotUsername = bot.Username
		if c.GroupName == "" {
			c.GroupName = group.Title
		}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	if err = s.settings.Set(ctx, SettingKeyCommunity, string(raw)); err != nil {
		return nil, err
	}
	return &c, nil
}

func communityHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
func communityRandomToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// checkedConfiguration 缓存机器人身份核实；令牌变更会立即使缓存失效。
func (s *CommunityService) checkedConfiguration(ctx context.Context) (*CommunitySettings, *SupportDeliverySettings, error) {
	c, err := s.GetSettings(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !c.Enabled {
		return nil, nil, ErrCommunityDisabled
	}
	shared, err := s.delivery.loadSettings(ctx)
	if err != nil {
		return nil, nil, err
	}
	if shared.TelegramBotToken == "" || shared.TelegramWebhookSecret == "" || c.BotID <= 0 {
		return nil, nil, ErrCommunityDisabled
	}
	hash := communityHash(shared.TelegramBotToken)
	s.mu.Lock()
	valid := s.verifiedTokenHash == hash && s.verifiedBotID == c.BotID && time.Since(s.verifiedAt) < time.Minute
	s.mu.Unlock()
	if !valid {
		var bot communityTelegramUser
		if err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "getMe", map[string]any{}, &bot); err != nil {
			return nil, nil, err
		}
		if bot.ID != c.BotID || !bot.IsBot {
			return nil, nil, ErrCommunityDisabled
		}
		s.mu.Lock()
		s.verifiedTokenHash = hash
		s.verifiedBotID = bot.ID
		s.verifiedAt = time.Now()
		s.mu.Unlock()
	}
	return c, shared, nil
}

// requireCommunityEligibility 只判断成功支付历史，查询失败必须保留原始错误供调用方重试。
func (s *CommunityService) requireCommunityEligibility(ctx context.Context, c *CommunitySettings, userID int64) error {
	if !c.RequirePaidRecharge {
		return nil
	}
	paid, err := s.repo.HasPaidBalanceRecharge(ctx, userID)
	if err != nil {
		return err
	}
	if !paid {
		return ErrCommunityVIPRequired
	}
	return nil
}

func (s *CommunityService) requireCommunityAccess(ctx context.Context, c *CommunitySettings, userID int64) error {
	if err := s.repo.EnsureActiveUser(ctx, userID); err != nil {
		return err
	}
	return s.requireCommunityEligibility(ctx, c, userID)
}

// currentCommunityAccess 在批准加入前重新读取策略，避免后台刚开启充值限制时沿用旧策略。
func (s *CommunityService) currentCommunityAccess(ctx context.Context, expected *CommunitySettings, userID int64) error {
	c, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	if !c.Enabled || c.BotID != expected.BotID || c.GroupChatID != expected.GroupChatID {
		return ErrCommunityDisabled
	}
	return s.requireCommunityAccess(ctx, c, userID)
}

func (s *CommunityService) Get(ctx context.Context, userID int64) (*CommunityState, error) {
	if err := s.repo.EnsureActiveUser(ctx, userID); err != nil {
		return nil, err
	}
	c, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.requireCommunityEligibility(ctx, c, userID); err != nil {
		if errors.Is(err, ErrCommunityVIPRequired) {
			return &CommunityState{ContactURL: c.ContactURL, RequirePaidRecharge: true}, nil
		}
		return nil, err
	}
	membership, challenge, invite, err := s.repo.GetState(ctx, userID)
	if err != nil {
		return nil, err
	}
	state := &CommunityState{ContactURL: c.ContactURL, Enabled: c.Enabled, RequirePaidRecharge: c.RequirePaidRecharge, Eligible: true, GroupName: c.GroupName, BotUsername: c.BotUsername, Membership: membership, Challenge: challenge, Invite: invite}
	if state.Enabled {
		shared, err := s.delivery.loadSettings(ctx)
		if err != nil {
			return nil, err
		}
		state.Enabled = c.BotID > 0 && shared.TelegramBotToken != "" && shared.TelegramWebhookSecret != ""
		s.mu.Lock()
		if s.verifiedTokenHash != "" && s.verifiedTokenHash != communityHash(shared.TelegramBotToken) {
			state.Enabled = false
		}
		s.mu.Unlock()
	}
	groupID, _ := strconv.ParseInt(c.GroupChatID, 10, 64)
	pendingMember := state.Enabled && membership != nil && membership.UserID == userID && membership.GroupChatID == groupID && membership.Status == "pending" && membership.AuthorizedInviteID > 0
	if pendingMember && invite != nil && invite.ID == membership.AuthorizedInviteID && invite.UserID == userID &&
		invite.TelegramUserID == membership.TelegramUserID && invite.GroupChatID == groupID && invite.BotID == c.BotID &&
		invite.Status == "active" && invite.ExpiresAt.After(time.Now()) {
		updated, reconcileErr := s.reconcilePendingMembership(ctx, c, membership)
		if reconcileErr != nil {
			return nil, reconcileErr
		}
		if updated {
			membership, challenge, invite, err = s.repo.GetState(ctx, userID)
			if err != nil {
				return nil, err
			}
			state.Membership, state.Challenge, state.Invite = membership, challenge, invite
		}
	}
	// 已确认但 Telegram 核对尚未完成时保留本人挑战，供网络恢复后继续确认，避免丢失重试入口。
	canRetryConfirmation := challenge != nil && membership != nil && membership.Status == "pending" &&
		membership.UserID == userID && membership.GroupChatID == groupID && challenge.UserID == userID &&
		challenge.TelegramUserID == membership.TelegramUserID
	if challenge != nil && (challenge.BotID != c.BotID || !challenge.ExpiresAt.After(time.Now()) || (challenge.Status == "confirmed" && !canRetryConfirmation)) {
		state.Challenge = nil
	}
	if invite != nil && (invite.BotID != c.BotID || invite.GroupChatID != groupID || !invite.ExpiresAt.After(time.Now()) || invite.Status != "active") {
		state.Invite = nil
	}
	if membership != nil && membership.GroupChatID != groupID {
		copy := *membership
		copy.Status = "left"
		copy.JoinedAt = nil
		state.Membership = &copy
	}
	if !state.Enabled {
		state.Invite = nil
		state.Challenge = nil
	}
	state.ShowJoinPrompt = state.Enabled && c.RequirePaidRecharge && c.LoginPromptEnabled && (state.Membership == nil || state.Membership.Status != "joined")
	if state.ShowJoinPrompt {
		state.PromptKey = communityHash(strconv.FormatInt(c.BotID, 10) + ":" + c.GroupChatID)
	}
	return state, nil
}

// 刷新页面时补偿已授权的等待状态；不会凭任意 Telegram ID 建立新绑定。
func (s *CommunityService) reconcilePendingMembership(ctx context.Context, expected *CommunitySettings, membership *CommunityMembership) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	c, shared, err := s.checkedConfiguration(ctx)
	if err != nil || c.BotID != expected.BotID || c.GroupChatID != expected.GroupChatID {
		return false, nil
	}
	member, err := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, membership.TelegramUserID)
	if err != nil || !communityMemberPresent(member) || member.User.IsBot {
		// Telegram 暂时不可用或仍未入群时保留等待状态，交给后续回调或刷新继续核对。
		return false, nil
	}
	if err = s.currentCommunityAccess(ctx, c, membership.UserID); err != nil {
		if communityPermanentError(err) || errors.Is(err, ErrCommunityDisabled) {
			return false, nil
		}
		return false, err
	}
	// 使用读取时的授权事件版本，防止覆盖核对期间已经发生的离群或重新申请。
	err = s.repo.MarkMembership(ctx, membership.TelegramUserID, membership.GroupChatID, "joined", membership.LastEventDate, membership.LastUpdateID)
	if err != nil && !communityPermanentError(err) {
		return false, err
	}
	return true, nil
}

func (s *CommunityService) StartVerification(ctx context.Context, userID int64) (*CommunityState, error) {
	if err := s.repo.EnsureActiveUser(ctx, userID); err != nil {
		return nil, err
	}
	c, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.requireCommunityEligibility(ctx, c, userID); err != nil {
		return nil, err
	}
	c, _, err = s.checkedConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.requireCommunityEligibility(ctx, c, userID); err != nil {
		return nil, err
	}
	membership, _, _, err := s.repo.GetState(ctx, userID)
	if err != nil {
		return nil, err
	}
	if membership != nil {
		return nil, ErrCommunityConflict
	}
	token, err := communityRandomToken(32)
	if err != nil {
		return nil, err
	}
	id, err := communityRandomToken(16)
	if err != nil {
		return nil, err
	}
	challenge := &CommunityChallenge{ID: id, UserID: userID, TokenHash: communityHash(token), BotID: c.BotID, Status: "waiting", ExpiresAt: time.Now().Add(15 * time.Minute)}
	if err = s.repo.CreateChallenge(ctx, challenge); err != nil {
		return nil, err
	}
	state, err := s.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !state.Eligible {
		return nil, ErrCommunityVIPRequired
	}
	if !state.Enabled {
		return nil, ErrCommunityDisabled
	}
	challenge.BotURL = "https://t.me/" + c.BotUsername + "?start=join_" + token
	state.Challenge = challenge
	return state, nil
}

func (s *CommunityService) CreateInvite(ctx context.Context, userID int64, input CommunityInviteInput) (*CommunityState, error) {
	if err := s.repo.EnsureActiveUser(ctx, userID); err != nil {
		return nil, err
	}
	c, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.requireCommunityEligibility(ctx, c, userID); err != nil {
		return nil, err
	}
	c, shared, err := s.checkedConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.requireCommunityEligibility(ctx, c, userID); err != nil {
		return nil, err
	}
	groupID, _ := strconv.ParseInt(c.GroupChatID, 10, 64)
	membership, _, invite, err := s.repo.GetState(ctx, userID)
	if err != nil {
		return nil, err
	}
	if input.ChallengeID != "" {
		if input.TelegramUserID <= 0 {
			return nil, ErrCommunityInvalid
		}
		membership, err = s.repo.ConfirmChallenge(ctx, userID, input.ChallengeID, input.TelegramUserID, groupID, c.BotID)
		if err != nil {
			return nil, err
		}
	} else if input.TelegramUserID != 0 {
		return nil, ErrCommunityInvalid
	}
	if membership != nil && membership.Status == "joined" && membership.GroupChatID == groupID {
		member, err := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, membership.TelegramUserID)
		if err != nil {
			return nil, err
		}
		if communityMemberPresent(member) {
			return s.Get(ctx, userID)
		}
		if err = s.repo.MarkMembership(ctx, membership.TelegramUserID, groupID, "left", time.Now().Unix(), 0); err != nil {
			return nil, err
		}
		invite = nil
	}
	// 兼容旧验证流程：从未真实加入、也没有在途批准的过期预留不应永久锁住账号。
	if input.ChallengeID == "" && membership != nil && membership.JoinedAt == nil && membership.AuthorizedInviteID == 0 && membership.Status != "joined" && (invite == nil || invite.Status != "active" || !invite.ExpiresAt.After(time.Now()) || invite.BotID != c.BotID || invite.GroupChatID != groupID) {
		if err = s.repo.MarkMembership(ctx, membership.TelegramUserID, membership.GroupChatID, "left", time.Now().Unix(), 0); err != nil {
			return nil, err
		}
		membership = nil
		invite = nil
	}
	if invite != nil && invite.BotID == c.BotID && invite.GroupChatID == groupID && invite.Status == "active" && invite.ExpiresAt.After(time.Now()) {
		if input.ChallengeID != "" {
			if err = s.reconcileConfirmedMembership(ctx, c, shared, membership, invite); err != nil {
				return nil, err
			}
		}
		return s.Get(ctx, userID)
	}
	if invite != nil && (invite.BotID != c.BotID || invite.GroupChatID != groupID) {
		if err = s.repo.QueueInviteRevocation(ctx, userID, invite.ID); err != nil {
			return nil, err
		}
	}
	leaseToken, err := communityRandomToken(16)
	if err != nil {
		return nil, err
	}
	acquired, err := s.repo.AcquireInviteLease(ctx, userID, groupID, leaseToken, 2*time.Minute)
	if err != nil {
		return nil, err
	}
	if !acquired {
		state, err := s.Get(ctx, userID)
		if err != nil {
			return nil, err
		}
		if state.Invite != nil {
			if input.ChallengeID != "" {
				if err = s.reconcileConfirmedMembership(ctx, c, shared, state.Membership, state.Invite); err != nil {
					return nil, err
				}
				return s.Get(ctx, userID)
			}
			return state, nil
		}
		return nil, ErrCommunityBusy
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.repo.ReleaseInviteLease(cleanup, userID, leaseToken)
	}()
	expires := time.Now().Add(15 * time.Minute)
	var result communityTelegramInvite
	if err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "createChatInviteLink", communityCreateInviteRequest{ChatID: c.GroupChatID, Name: "site-member", ExpireDate: expires.Unix(), CreatesJoinRequest: true}, &result); err != nil {
		return nil, err
	}
	if !communityValidInviteURL(result.InviteLink) || !result.CreatesJoinRequest || result.IsRevoked {
		return nil, ErrCommunityInvalid
	}
	if result.ExpireDate > 0 && result.ExpireDate < expires.Unix() {
		expires = time.Unix(result.ExpireDate, 0)
	}
	telegramID := int64(0)
	if membership != nil {
		telegramID = membership.TelegramUserID
	}
	invite = &CommunityInvite{UserID: userID, TelegramUserID: telegramID, GroupChatID: groupID, BotID: c.BotID, URL: result.InviteLink, URLHash: communityHash(result.InviteLink), ExpiresAt: expires, Status: "active"}
	if err = s.repo.SaveInvite(ctx, invite, leaseToken); err != nil {
		// 数据库写入失败的链接没有入群授权，尽力立即撤销；即使撤销失败也会拒绝它的加入请求。
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = s.delivery.telegramJSON(cleanup, shared.TelegramBotToken, "revokeChatInviteLink", communityRevokeInviteRequest{ChatID: c.GroupChatID, InviteLink: result.InviteLink}, nil)
		return nil, err
	}
	if input.ChallengeID != "" {
		if err = s.reconcileConfirmedMembership(ctx, c, shared, membership, invite); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, userID)
}

// 旧回调缺失且已经入群时，必须先由机器人核实身份、再由登录用户确认，才能恢复绑定。
func (s *CommunityService) reconcileConfirmedMembership(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, membership *CommunityMembership, invite *CommunityInvite) error {
	if membership == nil || invite == nil || membership.Status == "joined" {
		return nil
	}
	groupID, _ := strconv.ParseInt(c.GroupChatID, 10, 64)
	if membership.GroupChatID != groupID || invite.GroupChatID != groupID || invite.UserID != membership.UserID || invite.BotID != c.BotID {
		return ErrCommunityConflict
	}
	// 合成事件必须早于成员查询，查询期间收到的同秒真实事件可凭更高 updateID 保持优先。
	eventDate := time.Now().Unix()
	member, err := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, membership.TelegramUserID)
	if err != nil {
		return err
	}
	if !communityMemberPresent(member) || member.User.IsBot {
		return nil
	}
	if err = s.currentCommunityAccess(ctx, c, membership.UserID); err != nil {
		return err
	}
	if membership.AuthorizedInviteID == invite.ID {
		return s.repo.MarkMembership(ctx, membership.TelegramUserID, groupID, "joined", membership.LastEventDate, membership.LastUpdateID)
	}
	identity := CommunityTelegramIdentity{ID: membership.TelegramUserID, Username: member.User.Username, Name: strings.TrimSpace(member.User.FirstName + " " + member.User.LastName)}
	// 授权仍消费当前用户的有效专属邀请，数据库事务继续保证网站账号与 Telegram 身份双向唯一。
	if _, _, err = s.repo.AuthorizeJoin(ctx, invite.URLHash, identity, groupID, eventDate, 0, c.BotID, c.RequirePaidRecharge); err != nil {
		return err
	}
	if err = s.currentCommunityAccess(ctx, c, membership.UserID); err != nil {
		return err
	}
	return s.repo.MarkMembership(ctx, membership.TelegramUserID, groupID, "joined", eventDate, 0)
}

func communityValidInviteURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Host == "t.me" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && strings.HasPrefix(u.Path, "/+") && len(u.Path) > 3 && !strings.ContainsAny(value, "\x00\r\n\t ")
}

func (s *CommunityService) ListMembers(ctx context.Context, filter CommunityMemberFilter) (*CommunityMemberPage, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Page > 1000000 {
		return nil, ErrCommunityInvalid
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if len([]rune(filter.Search)) > 200 || strings.ContainsRune(filter.Search, '\x00') {
		return nil, ErrCommunityInvalid
	}
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	if filter.Status == "" {
		filter.Status = "all"
	}
	switch filter.Status {
	case "all", "joined", "not_joined", "pending", "left":
	default:
		return nil, ErrCommunityInvalid
	}
	c, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	groupID, _ := strconv.ParseInt(c.GroupChatID, 10, 64)
	return s.repo.ListMembers(ctx, groupID, c.BotID, filter)
}
