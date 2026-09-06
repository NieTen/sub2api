package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"path"
	"strings"
	"unicode/utf8"

	_ "golang.org/x/image/webp"
)

type SupportTicketService struct{ repo SupportTicketRepository }

func NewSupportTicketService(repo SupportTicketRepository) *SupportTicketService {
	return &SupportTicketService{repo: repo}
}

func (s *SupportTicketService) Create(ctx context.Context, actor SupportTicketActor, input CreateSupportTicketInput) (*SupportTicketDetail, error) {
	subject := strings.TrimSpace(input.Subject)
	if actor.UserID <= 0 || subject == "" || strings.ContainsRune(subject, '\x00') || !utf8.ValidString(subject) || utf8.RuneCountInString(subject) > 200 {
		return nil, ErrSupportTicketInvalid
	}
	content, err := validateSupportMessage(input.Content, input.AttachmentIDs)
	if err != nil {
		return nil, err
	}
	ticket := &SupportTicket{UserID: actor.UserID, Subject: subject, Status: SupportTicketStatusOpen}
	message := &SupportTicketMessage{SenderID: actor.UserID, SenderRole: "user", Source: "web", Content: content}
	if err := s.repo.CreateTicket(ctx, ticket, message, input.AttachmentIDs); err != nil {
		return nil, err
	}
	return s.Get(ctx, actor, ticket.ID, 0, 50)
}

func (s *SupportTicketService) List(ctx context.Context, actor SupportTicketActor, page, pageSize int, status string) (*SupportTicketPage, error) {
	if actor.UserID <= 0 {
		return nil, ErrSupportTicketNotFound
	}
	if status != "" && status != SupportTicketStatusOpen && status != SupportTicketStatusClosed {
		return nil, ErrSupportTicketInvalid
	}
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		return nil, ErrSupportTicketInvalid
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	ownerID := actor.UserID
	if actor.IsAdmin {
		ownerID = 0
	}
	return s.repo.ListTickets(ctx, ownerID, status, page, pageSize)
}

func (s *SupportTicketService) authorizedTicket(ctx context.Context, actor SupportTicketActor, id int64) (*SupportTicket, error) {
	if actor.UserID <= 0 || id <= 0 {
		return nil, ErrSupportTicketNotFound
	}
	ticket, err := s.repo.GetTicket(ctx, id)
	if err != nil {
		return nil, err
	}
	if !actor.IsAdmin && ticket.UserID != actor.UserID {
		return nil, ErrSupportTicketNotFound
	}
	return ticket, nil
}

func (s *SupportTicketService) Get(ctx context.Context, actor SupportTicketActor, ticketID, afterMessageID int64, limit int) (*SupportTicketDetail, error) {
	ticket, err := s.authorizedTicket(ctx, actor, ticketID)
	if err != nil {
		return nil, err
	}
	if afterMessageID < 0 {
		return nil, ErrSupportTicketInvalid
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	messages, err := s.repo.ListMessages(ctx, ticketID, afterMessageID, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	return &SupportTicketDetail{Ticket: ticket, Messages: messages, HasMore: hasMore}, nil
}

func (s *SupportTicketService) Reply(ctx context.Context, actor SupportTicketActor, ticketID int64, input SupportTicketReplyInput) (*SupportTicketMessage, error) {
	ticket, err := s.authorizedTicket(ctx, actor, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.Status == SupportTicketStatusClosed {
		return nil, ErrSupportTicketClosed
	}
	content, err := validateSupportMessage(input.Content, input.AttachmentIDs)
	if err != nil {
		return nil, err
	}
	role := "user"
	if actor.IsAdmin {
		role = "admin"
	}
	message := &SupportTicketMessage{TicketID: ticketID, SenderID: actor.UserID, SenderRole: role, Source: "web", Content: content}
	if err := s.repo.AddMessage(ctx, message, input.AttachmentIDs); err != nil {
		return nil, err
	}
	_, saved, err := s.repo.GetNotificationMessage(ctx, message.ID)
	return saved, err
}

func (s *SupportTicketService) SetStatus(ctx context.Context, actor SupportTicketActor, ticketID int64, status string) (*SupportTicket, error) {
	if _, err := s.authorizedTicket(ctx, actor, ticketID); err != nil {
		return nil, err
	}
	if status != SupportTicketStatusOpen && status != SupportTicketStatusClosed {
		return nil, ErrSupportTicketInvalid
	}
	if err := s.repo.SetStatus(ctx, ticketID, status); err != nil {
		return nil, err
	}
	return s.repo.GetTicket(ctx, ticketID)
}

func validateSupportMessage(content string, attachments []int64) (string, error) {
	content = strings.TrimSpace(content)
	if !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') || utf8.RuneCountInString(content) > 20000 || (content == "" && len(attachments) == 0) {
		return "", ErrSupportTicketInvalid
	}
	if len(attachments) > SupportTicketMaxAttachments {
		return "", ErrSupportAttachmentInvalid
	}
	seen := make(map[int64]bool, len(attachments))
	for _, id := range attachments {
		if id <= 0 || seen[id] {
			return "", ErrSupportAttachmentInvalid
		}
		seen[id] = true
	}
	return content, nil
}

// ValidateSupportTicketImage 先检查尺寸再完整解码，避免仅伪造文件头或通过超大像素图片耗尽内存。
func ValidateSupportTicketImage(filename string, data []byte) (*SupportTicketAttachment, error) {
	if len(data) == 0 || len(data) > SupportTicketMaxImageBytes {
		return nil, ErrSupportAttachmentInvalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 10000 || config.Height > 10000 || int64(config.Width)*int64(config.Height) > 16000000 {
		return nil, ErrSupportAttachmentInvalid
	}
	mimeTypes := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp"}
	mimeType, ok := mimeTypes[format]
	if !ok {
		return nil, ErrSupportAttachmentInvalid
	}
	decoded, actualFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || actualFormat != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return nil, ErrSupportAttachmentInvalid
	}
	filename = path.Base(strings.ReplaceAll(filename, "\\", "/"))
	filename = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, filename)
	if !utf8.ValidString(filename) || filename == "" || filename == "." || filename == "/" {
		filename = "image." + format
	}
	if utf8.RuneCountInString(filename) > 150 {
		filename = string([]rune(filename)[:150])
	}
	return &SupportTicketAttachment{FileName: filename, MimeType: mimeType, Size: len(data), Data: data}, nil
}

func (s *SupportTicketService) UploadAttachment(ctx context.Context, actor SupportTicketActor, filename string, data []byte) (*SupportTicketAttachment, error) {
	if actor.UserID <= 0 {
		return nil, ErrSupportAttachmentNotFound
	}
	a, err := ValidateSupportTicketImage(filename, data)
	if err != nil {
		return nil, err
	}
	a.OwnerID = actor.UserID
	if err := s.repo.CreateAttachment(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *SupportTicketService) GetAttachment(ctx context.Context, actor SupportTicketActor, id int64) (*SupportTicketAttachment, error) {
	if actor.UserID <= 0 || id <= 0 {
		return nil, ErrSupportAttachmentNotFound
	}
	a, err := s.repo.GetAttachment(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.MessageID == 0 {
		if a.OwnerID != actor.UserID {
			return nil, ErrSupportAttachmentNotFound
		}
	} else if _, err := s.authorizedTicket(ctx, actor, a.TicketID); err != nil {
		if errors.Is(err, ErrSupportTicketNotFound) {
			return nil, ErrSupportAttachmentNotFound
		}
		return nil, err
	}
	return a, nil
}

// DeleteAttachment 只清理当前用户未发送的草稿，不允许删除已进入工单的历史图片。
func (s *SupportTicketService) DeleteAttachment(ctx context.Context, actor SupportTicketActor, id int64) error {
	if actor.UserID <= 0 || id <= 0 {
		return ErrSupportAttachmentNotFound
	}
	return s.repo.DeleteDraftAttachment(ctx, actor.UserID, id)
}

// ReplyTelegram 仅使用已持久化的机器人消息映射定位工单，禁止凭文本中的数字工单号回复。
// 调用方负责校验 webhook 密钥、管理员聊天和发送者白名单。
func (s *SupportTicketService) ReplyTelegram(ctx context.Context, chatID, replyToMessageID, updateID int64, content string, images []SupportTicketImageInput) (*SupportTicketMessage, error) {
	if chatID == 0 || replyToMessageID <= 0 || updateID < 0 {
		return nil, ErrSupportTelegramUnmapped
	}
	ticketID, err := s.repo.FindTelegramTicket(ctx, chatID, replyToMessageID)
	if err != nil {
		return nil, err
	}
	externalID := fmt.Sprintf("telegram:%d", updateID)
	if existing, err := s.repo.GetMessageByExternalID(ctx, externalID); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrSupportTicketNotFound) {
		return nil, err
	}
	if len(images) > SupportTicketMaxAttachments {
		return nil, ErrSupportAttachmentInvalid
	}
	ids := make([]int64, len(images))
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	content, err = validateSupportMessage(content, ids)
	if err != nil {
		return nil, err
	}
	ticket, err := s.repo.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.Status == SupportTicketStatusClosed {
		return nil, ErrSupportTicketClosed
	}
	message := &SupportTicketMessage{TicketID: ticketID, SenderRole: "admin", Source: "telegram", Content: content, ExternalID: externalID, Attachments: make([]SupportTicketAttachment, 0, len(images))}
	for _, input := range images {
		a, err := ValidateSupportTicketImage(input.FileName, input.Data)
		if err != nil {
			return nil, err
		}
		a.OwnerID = ticket.UserID
		message.Attachments = append(message.Attachments, *a)
	}
	if err := s.repo.AddMessage(ctx, message, nil); err != nil {
		if errors.Is(err, ErrSupportDuplicateMessage) {
			return s.repo.GetMessageByExternalID(ctx, externalID)
		}
		return nil, err
	}
	_, saved, err := s.repo.GetNotificationMessage(ctx, message.ID)
	return saved, err
}
