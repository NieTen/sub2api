package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
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
	if len(c.ContactURL) > 2048 || len([]rune(c.GroupName)) > 100 || strings.ContainsAny(c.GroupName, "\x00\r\n") {
		return nil, ErrCommunityInvalid
	}
	if c.ContactURL != "" {
		u, err := url.Parse(c.ContactURL)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || strings.ContainsAny(c.ContactURL, "\x00\r\n\t") {
			return nil, ErrCommunityInvalid
		}
	}
	if c.BotUsername != "" && !communityBotUsernamePattern.MatchString(c.BotUsername) {
		return nil, ErrCommunityInvalid
	}
	if c.GroupChatID != "" {
		id, err := strconv.ParseInt(c.GroupChatID, 10, 64)
		if err != nil || id >= 0 {
			return nil, ErrCommunityInvalid
		}
	}
	if c.Enabled {
		shared, err := s.delivery.loadSettings(ctx)
		if err != nil {
			return nil, err
		}
		if shared.TelegramBotToken == "" || shared.TelegramWebhookSecret == "" || c.GroupChatID == "" {
			return nil, ErrCommunityInvalid
		}
		var bot communityTelegramUser
		if err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "getMe", map[string]any{}, &bot); err != nil {
			return nil, err
		}
		if !bot.IsBot || bot.ID <= 0 || !communityBotUsernamePattern.MatchString(bot.Username) {
			return nil, ErrCommunityInvalid
		}
		var group communityTelegramChat
		if err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "getChat", communityChatRequest{ChatID: c.GroupChatID}, &group); err != nil {
			return nil, err
		}
		if (group.Type != "group" && group.Type != "supergroup") || group.Username != "" || len(group.ActiveUsernames) > 0 || strconv.FormatInt(group.ID, 10) != c.GroupChatID {
			return nil, ErrCommunityInvalid
		}
		member, err := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, bot.ID)
		if err != nil {
			return nil, err
		}
		if member.Status != "administrator" || !member.CanInviteUsers || !member.CanRestrictMembers {
			return nil, ErrCommunityInvalid
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
	if challenge != nil && (challenge.BotID != c.BotID || !challenge.ExpiresAt.After(time.Now()) || challenge.Status == "confirmed") {
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
	return s.Get(ctx, userID)
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
