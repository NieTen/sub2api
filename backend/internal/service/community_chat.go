package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const CommunityChatMediaLimit = 20 * 1024 * 1024

var ErrCommunityChatForbidden = infraerrors.Forbidden("COMMUNITY_CHAT_FORBIDDEN", "仅管理员可以查看群消息和操作机器人")
var ErrCommunityChatUncertain = infraerrors.Conflict("COMMUNITY_CHAT_SEND_UNCERTAIN", "发送结果尚未确认，请先检查 Telegram 群消息；本次不会自动重复发送")
var communityChatRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

type CommunityChatMessage struct {
	ID                int64      `json:"id"`
	BotID             int64      `json:"-"`
	GroupChatID       int64      `json:"-"`
	UpdateID          *int64     `json:"-"`
	TelegramMessageID int64      `json:"telegram_message_id"`
	TelegramUserID    int64      `json:"telegram_user_id"`
	TelegramUsername  string     `json:"telegram_username"`
	TelegramName      string     `json:"telegram_name"`
	SenderKind        string     `json:"sender_kind"`
	SenderChatID      int64      `json:"sender_chat_id,omitempty"`
	IsBot             bool       `json:"is_bot"`
	MessageType       string     `json:"message_type"`
	Text              string     `json:"text"`
	FileID            string     `json:"-"`
	FileName          string     `json:"file_name"`
	MimeType          string     `json:"mime_type"`
	FileSize          int64      `json:"file_size"`
	MediaAvailable    bool       `json:"media_available"`
	ReplyToMessageID  int64      `json:"reply_to_message_id,omitempty"`
	Outgoing          bool       `json:"outgoing"`
	AdminUserID       int64      `json:"admin_user_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	EditedAt          *time.Time `json:"edited_at,omitempty"`
}

type CommunityChatFilter struct {
	BeforeID, AfterID int64
	Limit             int
}
type CommunityChatPage struct {
	Items    []CommunityChatMessage `json:"items"`
	HasMore  bool                   `json:"has_more"`
	LatestID int64                  `json:"latest_id"`
}
type CommunityChatSendInput struct {
	Text            string `json:"text"`
	ClientRequestID string `json:"client_request_id"`
}
type CommunityChatSend struct {
	ID, GroupChatID, BotID, AdminUserID, MessageID     int64
	ClientRequestID, ContentHash, Status, ErrorMessage string
}
type CommunityChatPerson struct {
	Banned           bool                 `json:"banned"`
	TelegramUserID   int64                `json:"telegram_user_id"`
	TelegramUsername string               `json:"telegram_username"`
	TelegramName     string               `json:"telegram_name"`
	IsBot            bool                 `json:"is_bot"`
	Member           *CommunityMemberItem `json:"member"`
}
type CommunityChatFile struct {
	FileName, MimeType string
	Data               []byte
}

// 群消息扩展沿用社群仓储和已有数据库连接，不创建额外数据库访问通道。
type CommunityChatRepository interface {
	SaveChatMessage(context.Context, *CommunityChatMessage) error
	ListChatMessages(context.Context, int64, CommunityChatFilter) (*CommunityChatPage, error)
	GetChatMessage(context.Context, int64, int64) (*CommunityChatMessage, error)
	GetChatPerson(context.Context, int64, int64) (*CommunityChatPerson, error)
	ReserveChatSend(context.Context, *CommunityChatSend) (bool, error)
	FinishChatSend(context.Context, *CommunityChatSend, *CommunityChatMessage) error
	GetChatAvatar(context.Context, int64, int64) (*CommunityChatFile, error)
	SaveChatAvatar(context.Context, int64, int64, *CommunityChatFile) error
}

func communityChatAdmin(actor SupportTicketActor) error {
	if !actor.IsAdmin || actor.UserID <= 0 {
		return ErrCommunityChatForbidden
	}
	return nil
}
func (s *CommunityService) chatGroup(ctx context.Context, actor SupportTicketActor) (*CommunitySettings, int64, error) {
	if err := communityChatAdmin(actor); err != nil {
		return nil, 0, err
	}
	c, err := s.GetSettings(ctx)
	if err != nil {
		return nil, 0, err
	}
	group, parseErr := strconv.ParseInt(c.GroupChatID, 10, 64)
	if parseErr != nil || group >= 0 {
		return nil, 0, ErrCommunityDisabled
	}
	return c, group, nil
}
func (s *CommunityService) ListChatMessages(ctx context.Context, actor SupportTicketActor, filter CommunityChatFilter) (*CommunityChatPage, error) {
	_, group, err := s.chatGroup(ctx, actor)
	if err != nil {
		return nil, err
	}
	if filter.BeforeID < 0 || filter.AfterID < 0 || (filter.BeforeID > 0 && filter.AfterID > 0) || filter.Limit < 0 || filter.Limit > 100 {
		return nil, ErrCommunityInvalid
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	return s.repo.ListChatMessages(ctx, group, filter)
}
func (s *CommunityService) ChatPerson(ctx context.Context, actor SupportTicketActor, telegramID int64) (*CommunityChatPerson, error) {
	_, group, err := s.chatGroup(ctx, actor)
	if err != nil {
		return nil, err
	}
	if telegramID <= 0 {
		return nil, ErrCommunityInvalid
	}
	return s.repo.GetChatPerson(ctx, group, telegramID)
}

// 发送结果以幂等键保存。即使客户端断线，也会用独立短上下文保存 Telegram 返回结果。
func (s *CommunityService) SendChatMessage(ctx context.Context, actor SupportTicketActor, input CommunityChatSendInput) (*CommunityChatMessage, error) {
	if err := communityChatAdmin(actor); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(input.Text)
	if text == "" || strings.ContainsRune(text, 0) || len(utf16.Encode([]rune(text))) > 4096 || !communityChatRequestID.MatchString(input.ClientRequestID) {
		return nil, communitySettingsFieldError("text", "消息须为 1～4096 个字符，且必须提供有效的发送标识")
	}
	c, shared, err := s.checkedConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	group, _ := strconv.ParseInt(c.GroupChatID, 10, 64)
	send := &CommunityChatSend{GroupChatID: group, BotID: c.BotID, AdminUserID: actor.UserID, ClientRequestID: input.ClientRequestID, ContentHash: communityHash(text)}
	acquired, err := s.repo.ReserveChatSend(ctx, send)
	if err != nil {
		return nil, err
	}
	if !acquired {
		if send.Status == "sent" {
			return s.repo.GetChatMessage(ctx, group, send.MessageID)
		}
		if send.Status == "failed" {
			return nil, infraerrors.BadRequest("COMMUNITY_CHAT_SEND_FAILED", send.ErrorMessage)
		}
		return nil, ErrCommunityChatUncertain
	}
	requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	var response communityTelegramMessage
	err = s.delivery.telegramJSON(requestCtx, shared.TelegramBotToken, "sendMessage", supportTelegramTextRequest{ChatID: c.GroupChatID, Text: text}, &response)
	cancel()
	saveCtx, finish := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer finish()
	if err != nil || response.MessageID <= 0 || response.Chat.ID != group {
		send.Status, send.ErrorMessage = "uncertain", ErrCommunityChatUncertain.Message
		var api *SupportTelegramAPIError
		if errors.As(err, &api) && api.StatusCode >= 400 && api.StatusCode < 500 {
			send.Status, send.ErrorMessage = "failed", fmt.Sprintf("Telegram 拒绝发送（状态 %d），请检查机器人群权限或稍后重试", api.StatusCode)
		}
		if saveErr := s.repo.FinishChatSend(saveCtx, send, nil); saveErr != nil {
			return nil, ErrCommunityChatUncertain
		}
		if send.Status == "failed" {
			return nil, infraerrors.New(http.StatusBadGateway, "COMMUNITY_CHAT_SEND_FAILED", send.ErrorMessage)
		}
		return nil, ErrCommunityChatUncertain
	}
	message := communityArchiveMessage(c.BotID, nil, &response)
	message.Outgoing, message.AdminUserID = true, actor.UserID
	send.Status = "sent"
	if err = s.repo.FinishChatSend(saveCtx, send, message); err != nil {
		return nil, ErrCommunityChatUncertain
	}
	return message, nil
}
