package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

type communityTelegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}
type communityTelegramChat struct {
	ID              int64    `json:"id"`
	Type            string   `json:"type"`
	Title           string   `json:"title"`
	Username        string   `json:"username"`
	ActiveUsernames []string `json:"active_usernames"`
}
type communityTelegramInvite struct {
	InviteLink         string `json:"invite_link"`
	CreatesJoinRequest bool   `json:"creates_join_request"`
	IsRevoked          bool   `json:"is_revoked"`
	ExpireDate         int64  `json:"expire_date"`
}
type communityTelegramMember struct {
	User               communityTelegramUser `json:"user"`
	Status             string                `json:"status"`
	IsMember           bool                  `json:"is_member"`
	CanInviteUsers     bool                  `json:"can_invite_users"`
	CanRestrictMembers bool                  `json:"can_restrict_members"`
}
type communityTelegramMessage struct {
	Chat           communityTelegramChat  `json:"chat"`
	From           *communityTelegramUser `json:"from"`
	Text           string                 `json:"text"`
	ReplyToMessage json.RawMessage        `json:"reply_to_message"`
}
type communityTelegramJoinRequest struct {
	Chat       communityTelegramChat    `json:"chat"`
	From       communityTelegramUser    `json:"from"`
	Date       int64                    `json:"date"`
	InviteLink *communityTelegramInvite `json:"invite_link"`
}
type communityTelegramMemberUpdate struct {
	Chat          communityTelegramChat    `json:"chat"`
	Date          int64                    `json:"date"`
	OldChatMember communityTelegramMember  `json:"old_chat_member"`
	NewChatMember communityTelegramMember  `json:"new_chat_member"`
	InviteLink    *communityTelegramInvite `json:"invite_link"`
}
type communityTelegramUpdate struct {
	UpdateID        int64                          `json:"update_id"`
	Message         *communityTelegramMessage      `json:"message"`
	ChatJoinRequest *communityTelegramJoinRequest  `json:"chat_join_request"`
	ChatMember      *communityTelegramMemberUpdate `json:"chat_member"`
}
type communityChatRequest struct {
	ChatID string `json:"chat_id"`
}
type communityChatMemberRequest struct {
	ChatID string `json:"chat_id"`
	UserID int64  `json:"user_id"`
}
type communityCreateInviteRequest struct {
	ChatID             string `json:"chat_id"`
	Name               string `json:"name"`
	ExpireDate         int64  `json:"expire_date"`
	CreatesJoinRequest bool   `json:"creates_join_request"`
}
type communityRevokeInviteRequest struct {
	ChatID     string `json:"chat_id"`
	InviteLink string `json:"invite_link"`
}
type communityBanMemberRequest struct {
	ChatID         string `json:"chat_id"`
	UserID         int64  `json:"user_id"`
	UntilDate      int64  `json:"until_date"`
	RevokeMessages bool   `json:"revoke_messages"`
}

func communityMemberPresent(m *communityTelegramMember) bool {
	return m != nil && (m.Status == "member" || m.Status == "administrator" || m.Status == "creator" || (m.Status == "restricted" && m.IsMember))
}
func communityMemberPrivileged(m *communityTelegramMember) bool {
	return m != nil && (m.Status == "administrator" || m.Status == "creator")
}
func communityPermanentError(err error) bool {
	return errors.Is(err, ErrCommunityNotFound) || errors.Is(err, ErrCommunityConflict) || errors.Is(err, ErrCommunityInvalid) || errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrCommunityVIPRequired)
}
func communityTelegramBadRequest(err error) bool {
	var api *SupportTelegramAPIError
	return errors.As(err, &api) && api.StatusCode == 400
}

func (s *CommunityService) getChatMember(ctx context.Context, token, chatID string, userID int64) (*communityTelegramMember, error) {
	var result communityTelegramMember
	if err := s.delivery.telegramJSON(ctx, token, "getChatMember", communityChatMemberRequest{ChatID: chatID, UserID: userID}, &result); err != nil {
		return nil, err
	}
	if result.User.ID != userID || result.Status == "" {
		return nil, errors.New("Telegram 成员状态响应无效")
	}
	return &result, nil
}

func (s *CommunityService) HandleTelegramWebhook(ctx context.Context, secret string, raw []byte) error {
	shared, err := s.delivery.loadSettings(ctx)
	if err != nil {
		return err
	}
	if shared.TelegramWebhookSecret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(shared.TelegramWebhookSecret)) != 1 {
		return ErrSupportTelegramUnauthorized
	}
	c, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	if !c.Enabled || c.BotID <= 0 {
		return nil
	}
	if len(raw) > 1024*1024 {
		return ErrCommunityInvalid
	}
	var update communityTelegramUpdate
	if err = json.Unmarshal(raw, &update); err != nil {
		return ErrCommunityInvalid
	}
	if update.UpdateID < 0 {
		return ErrCommunityInvalid
	}
	groupID, _ := strconv.ParseInt(c.GroupChatID, 10, 64)
	relevant := communityPrivateMessage(update.Message)
	relevant = relevant || (update.ChatJoinRequest != nil && update.ChatJoinRequest.Chat.ID == groupID) || (update.ChatMember != nil && update.ChatMember.Chat.ID == groupID)
	if !relevant {
		return nil
	}
	return s.repo.EnqueueWebhook(ctx, c.BotID, update.UpdateID, raw)
}

func (s *CommunityService) processWebhook(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, event CommunityWebhookEvent) error {
	if event.BotID != c.BotID {
		return nil
	}
	// 每条排队事件使用最新群策略，避免开启 VIP 后仍按本轮工作循环的旧配置批准。
	latest, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	if !latest.Enabled || latest.BotID != event.BotID {
		return nil
	}
	c = latest
	var update communityTelegramUpdate
	if err := json.Unmarshal(event.Payload, &update); err != nil {
		return nil
	}
	if update.Message != nil {
		return s.processStart(ctx, c, shared, update.Message)
	}
	groupID, _ := strconv.ParseInt(c.GroupChatID, 10, 64)
	if update.ChatJoinRequest != nil && update.ChatJoinRequest.Chat.ID == groupID {
		return s.processJoinRequest(ctx, c, shared, update.UpdateID, update.ChatJoinRequest)
	}
	if update.ChatMember != nil && update.ChatMember.Chat.ID == groupID {
		return s.processMemberUpdate(ctx, c, shared, update.UpdateID, update.ChatMember)
	}
	return nil
}

func (s *CommunityService) processStart(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, message *communityTelegramMessage) error {
	if !communityPrivateMessage(message) {
		return nil
	}
	parts := strings.Fields(message.Text)
	if len(parts) != 2 || (parts[0] != "/start" && parts[0] != "/start@"+c.BotUsername) || !strings.HasPrefix(parts[1], "join_") {
		return s.replyCommunityHelp(ctx, c, shared, message)
	}
	token := strings.TrimPrefix(parts[1], "join_")
	if len(token) != 43 {
		return s.replyCommunityText(ctx, shared, message.Chat.ID, "该入群验证已失效，请回到网站重新生成验证或领取专属入群链接。")
	}
	challenge, err := s.repo.ClaimChallenge(ctx, communityHash(token), c.BotID, CommunityTelegramIdentity{ID: message.From.ID, Username: message.From.Username, Name: strings.TrimSpace(message.From.FirstName + " " + message.From.LastName)})
	if err == nil {
		err = s.requireCommunityAccess(ctx, c, challenge.UserID)
	}
	text := "已读取你的 Telegram 身份，请回到网站核对账号并确认，再领取专属入群链接。"
	if err != nil {
		if !communityPermanentError(err) {
			return err
		}
		text = "该入群验证已失效或已被认领，请回到网站重新生成验证。"
	}
	return s.replyCommunityText(ctx, shared, message.Chat.ID, text)
}

// 私聊工单回复仍由工单处理，社群帮助不会认领身份或干扰回复消息。
func communityPrivateMessage(message *communityTelegramMessage) bool {
	return message != nil && message.Chat.Type == "private" && message.From != nil && !message.From.IsBot && message.From.ID > 0 && message.From.ID == message.Chat.ID && strings.TrimSpace(message.Text) != "" && (len(message.ReplyToMessage) == 0 || string(message.ReplyToMessage) == "null")
}

func (s *CommunityService) replyCommunityHelp(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, message *communityTelegramMessage) error {
	text := "你好！请登录网站，在“客服与社群”页面领取专属入群链接。通过该链接提交申请后，机器人会自动核验资格并批准入群。请勿转发你的专属链接。"
	membership, err := s.repo.GetMembershipByTelegram(ctx, message.From.ID)
	if err != nil && !communityPermanentError(err) {
		return err
	}
	groupID, _ := strconv.ParseInt(c.GroupChatID, 10, 64)
	if membership != nil && membership.GroupChatID == groupID {
		switch membership.Status {
		case "joined":
			text = "你的 Telegram 账号已完成网站社群绑定。请返回网站“客服与社群”页面查看入群状态；如需客服帮助，请使用网站的联系客服或工单入口。"
		case "pending":
			text = "已收到你的入群身份信息。请使用网站领取的专属链接提交入群申请，机器人会自动核验资格并批准。若已入群，请返回网站“客服与社群”页面刷新状态。"
		case "left":
			text = "你的 Telegram 账号已绑定，但当前未显示入群。请登录网站，在“客服与社群”页面重新领取专属邀请。"
		}
	}
	return s.replyCommunityText(ctx, shared, message.Chat.ID, text)
}

func (s *CommunityService) replyCommunityText(ctx context.Context, shared *SupportDeliverySettings, chatID int64, text string) error {
	err := s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "sendMessage", supportTelegramTextRequest{ChatID: strconv.FormatInt(chatID, 10), Text: text}, nil)
	var apiError *SupportTelegramAPIError
	if communityTelegramBadRequest(err) || (errors.As(err, &apiError) && apiError.StatusCode == 403) {
		return nil
	}
	return err
}

func (s *CommunityService) processJoinRequest(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, updateID int64, request *communityTelegramJoinRequest) error {
	if request.From.ID <= 0 || request.From.ID == c.BotID {
		return nil
	}
	// 迟到的加入申请不能撤销已经合法加入的成员，更不能触发踢人。
	existing, err := s.repo.GetMembershipByTelegram(ctx, request.From.ID)
	if err != nil && !communityPermanentError(err) {
		return err
	}
	if existing != nil && existing.Status == "joined" && existing.GroupChatID == request.Chat.ID {
		return nil
	}
	if request.From.IsBot || request.InviteLink == nil || !communityValidInviteURL(request.InviteLink.InviteLink) {
		return s.declineJoin(ctx, shared.TelegramBotToken, c.GroupChatID, request.From.ID)
	}
	identity := CommunityTelegramIdentity{ID: request.From.ID, Username: request.From.Username, Name: strings.TrimSpace(request.From.FirstName + " " + request.From.LastName)}
	membership, invite, err := s.repo.AuthorizeJoin(ctx, communityHash(request.InviteLink.InviteLink), identity, request.Chat.ID, request.Date, updateID, c.BotID, c.RequirePaidRecharge)
	if err != nil {
		if errors.Is(err, ErrCommunityVIPRequired) {
			return s.rejectIneligibleJoin(ctx, c, shared, updateID, request, existing)
		}
		if communityPermanentError(err) {
			return s.declineJoin(ctx, shared.TelegramBotToken, c.GroupChatID, request.From.ID)
		}
		return err
	}
	current, err := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, request.From.ID)
	if err != nil && !communityTelegramBadRequest(err) {
		return err
	}
	if !communityMemberPresent(current) {
		if err = s.currentCommunityAccess(ctx, c, membership.UserID); err != nil {
			if communityPermanentError(err) {
				return s.rejectIneligibleJoin(ctx, c, shared, updateID, request, membership)
			}
			return err
		}
		err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "approveChatJoinRequest", communityChatMemberRequest{ChatID: c.GroupChatID, UserID: request.From.ID}, nil)
		approvalRejected := communityTelegramBadRequest(err)
		if err != nil && !communityTelegramBadRequest(err) {
			return err
		}
		current, err = s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, request.From.ID)
		if err != nil {
			return err
		}
		if approvalRejected && !communityMemberPresent(current) {
			if err = s.repo.MarkMembership(ctx, request.From.ID, request.Chat.ID, "left", request.Date, updateID); err != nil && !communityPermanentError(err) {
				return err
			}
			return nil
		}
	}
	if !communityMemberPresent(current) {
		return ErrCommunityBusy
	}
	if err = s.currentCommunityAccess(ctx, c, membership.UserID); err != nil {
		if communityPermanentError(err) {
			return s.rejectIneligibleJoin(ctx, c, shared, updateID, request, membership)
		}
		return err
	}
	if err = s.completeCommunityJoin(ctx, c, shared, membership, invite, request.Date, updateID); err != nil {
		if communityPermanentError(err) {
			return nil
		}
		return err
	}
	return nil
}

// 先持久化入群与撤销任务，再尽力立即关闭专属链接；远端失败由原有任务重试，不回滚已完成的入群。
func (s *CommunityService) completeCommunityJoin(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, membership *CommunityMembership, invite *CommunityInvite, eventDate, updateID int64) error {
	if invite == nil && membership.Status != "joined" {
		// 成员事件重试可能已存在授权，仍从服务端读取原邀请，不能使用回调中任意链接执行撤销。
		_, _, storedInvite, readErr := s.repo.GetState(ctx, membership.UserID)
		if readErr == nil {
			invite = storedInvite
		}
	}
	if err := s.repo.MarkMembership(ctx, membership.TelegramUserID, membership.GroupChatID, "joined", eventDate, updateID); err != nil {
		return err
	}
	if invite == nil || invite.ID <= 0 || invite.ID != membership.AuthorizedInviteID || invite.UserID != membership.UserID ||
		invite.TelegramUserID != membership.TelegramUserID || invite.GroupChatID != membership.GroupChatID ||
		invite.BotID != c.BotID || strconv.FormatInt(invite.GroupChatID, 10) != c.GroupChatID || !communityValidInviteURL(invite.URL) {
		return nil
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	err := s.delivery.telegramJSON(cleanup, shared.TelegramBotToken, "revokeChatInviteLink", communityRevokeInviteRequest{ChatID: c.GroupChatID, InviteLink: invite.URL}, nil)
	if err != nil && !communityTelegramBadRequest(err) {
		slog.Warn("社群邀请即时撤销失败，将由后台任务重试")
	}
	return nil
}

// 申请在处理中失去资格时，核对实际成员后释放临时预留；数据库错误不会进入此路径。
func (s *CommunityService) rejectIneligibleJoin(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, updateID int64, request *communityTelegramJoinRequest, membership *CommunityMembership) error {
	if err := s.declineJoin(ctx, shared.TelegramBotToken, c.GroupChatID, request.From.ID); err != nil {
		return err
	}
	if membership == nil || membership.GroupChatID != request.Chat.ID || membership.Status == "joined" {
		return nil
	}
	if request.Date < membership.LastEventDate || (request.Date == membership.LastEventDate && updateID < membership.LastUpdateID) {
		return nil
	}
	current, err := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, request.From.ID)
	if err != nil {
		return err
	}
	if communityMemberPresent(current) && !communityMemberPrivileged(current) {
		if err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "banChatMember", communityBanMemberRequest{ChatID: c.GroupChatID, UserID: request.From.ID, UntilDate: time.Now().Add(time.Minute).Unix(), RevokeMessages: false}, nil); err != nil {
			return err
		}
	}
	err = s.repo.MarkMembership(ctx, request.From.ID, request.Chat.ID, "left", request.Date, updateID)
	if communityPermanentError(err) {
		return nil
	}
	return err
}

func (s *CommunityService) declineJoin(ctx context.Context, token, chatID string, userID int64) error {
	err := s.delivery.telegramJSON(ctx, token, "declineChatJoinRequest", communityChatMemberRequest{ChatID: chatID, UserID: userID}, nil)
	if communityTelegramBadRequest(err) {
		return nil
	}
	return err
}

func (s *CommunityService) processMemberUpdate(ctx context.Context, c *CommunitySettings, shared *SupportDeliverySettings, updateID int64, update *communityTelegramMemberUpdate) error {
	member := update.NewChatMember
	id := member.User.ID
	if id <= 0 || id == c.BotID {
		return nil
	}
	membership, err := s.repo.GetMembershipByTelegram(ctx, id)
	if err != nil && !communityPermanentError(err) {
		return err
	}
	if !communityMemberPresent(&member) {
		if membership == nil {
			return nil
		}
		if err = s.repo.MarkMembership(ctx, id, update.Chat.ID, "left", update.Date, updateID); err != nil {
			if communityPermanentError(err) {
				return nil
			}
			return err
		}
		return nil
	}
	// 管理员先批准、或申请事件丢失时，仍须通过原专属邀请事务核验唯一归属和当前资格。
	var authorizedInvite *CommunityInvite
	needsAuthorization := membership == nil || membership.GroupChatID != update.Chat.ID || (membership.Status != "joined" && membership.AuthorizedInviteID == 0)
	if needsAuthorization && !member.User.IsBot && update.InviteLink != nil && communityValidInviteURL(update.InviteLink.InviteLink) {
		identity := CommunityTelegramIdentity{ID: id, Username: member.User.Username, Name: strings.TrimSpace(member.User.FirstName + " " + member.User.LastName)}
		authorized, invite, authorizeErr := s.repo.AuthorizeJoin(ctx, communityHash(update.InviteLink.InviteLink), identity, update.Chat.ID, update.Date, updateID, c.BotID, c.RequirePaidRecharge)
		if authorizeErr != nil && !communityPermanentError(authorizeErr) {
			return authorizeErr
		}
		if authorizeErr == nil {
			membership = authorized
			authorizedInvite = invite
		}
	}
	if membership != nil && membership.GroupChatID == update.Chat.ID {
		if update.Date < membership.LastEventDate || (update.Date == membership.LastEventDate && updateID < membership.LastUpdateID) {
			return nil
		}
		if membership.Status != "joined" && membership.AuthorizedInviteID > 0 && !member.User.IsBot {
			// 原申请回调缺失时，较新的离群事件可能因当时尚无绑定而被忽略；不能仅凭迟到的加入事件完成绑定。
			// 查询失败保留授权并重试，重试仍须再次核对，避免跳过首次恢复分支后误标已加入。
			current, checkErr := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, id)
			if checkErr != nil {
				return checkErr
			}
			if !communityMemberPresent(current) {
				err = s.repo.MarkMembership(ctx, id, update.Chat.ID, "left", update.Date, updateID)
				if communityPermanentError(err) {
					return nil
				}
				return err
			}
		}
		activeErr := s.currentCommunityAccess(ctx, c, membership.UserID)
		if activeErr != nil && !communityPermanentError(activeErr) {
			return activeErr
		}
		if activeErr == nil && (membership.Status == "joined" || membership.AuthorizedInviteID > 0) && !member.User.IsBot {
			if err = s.completeCommunityJoin(ctx, c, shared, membership, authorizedInvite, update.Date, updateID); err != nil {
				if communityPermanentError(err) {
					return nil
				}
				return err
			}
			return nil
		}
	}
	// 使用实时成员状态避免迟到事件误踢已离开成员或刚被提拔的管理员。
	current, err := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, id)
	if err != nil {
		if communityTelegramBadRequest(err) {
			return nil
		}
		return err
	}
	if !communityMemberPresent(current) || communityMemberPrivileged(current) || current.User.ID == c.BotID {
		return nil
	}
	if err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "banChatMember", communityBanMemberRequest{ChatID: c.GroupChatID, UserID: id, UntilDate: time.Now().Add(time.Minute).Unix(), RevokeMessages: false}, nil); err != nil {
		if communityTelegramBadRequest(err) {
			latest, readErr := s.getChatMember(ctx, shared.TelegramBotToken, c.GroupChatID, id)
			if readErr == nil && (!communityMemberPresent(latest) || communityMemberPrivileged(latest)) {
				return nil
			}
		}
		return err
	}
	if membership != nil {
		if err = s.repo.MarkMembership(ctx, id, update.Chat.ID, "left", update.Date, updateID); err != nil && !communityPermanentError(err) {
			return err
		}
		return nil
	}
	return nil
}
