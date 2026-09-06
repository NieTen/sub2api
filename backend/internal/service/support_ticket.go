package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SupportTicketStatusOpen     = "open"
	SupportTicketStatusClosed   = "closed"
	SupportTicketMaxImageBytes  = 5 * 1024 * 1024
	SupportTicketMaxAttachments = 4
)

var (
	ErrSupportTicketNotFound     = infraerrors.NotFound("SUPPORT_TICKET_NOT_FOUND", "工单不存在")
	ErrSupportTicketInvalid      = infraerrors.BadRequest("SUPPORT_TICKET_INVALID", "工单标题或消息内容不符合要求")
	ErrSupportTicketClosed       = infraerrors.BadRequest("SUPPORT_TICKET_CLOSED", "工单已关闭，请先重新打开")
	ErrSupportAttachmentInvalid  = infraerrors.BadRequest("SUPPORT_ATTACHMENT_INVALID", "图片无效、超过限制或不可用于当前消息")
	ErrSupportAttachmentNotFound = infraerrors.NotFound("SUPPORT_ATTACHMENT_NOT_FOUND", "图片不存在")
	ErrSupportDuplicateMessage   = infraerrors.Conflict("SUPPORT_DUPLICATE_MESSAGE", "消息已处理")
	ErrSupportTelegramUnmapped   = infraerrors.BadRequest("SUPPORT_TELEGRAM_UNMAPPED", "请直接回复机器人的工单通知消息")
)

type SupportTicketActor struct {
	UserID  int64
	IsAdmin bool
}

type SupportTicket struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	Subject       string    `json:"subject"`
	Status        string    `json:"status"`
	UserEmail     string    `json:"user_email"`
	Username      string    `json:"username"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	LastMessageAt time.Time `json:"last_message_at"`
}

type SupportTicketMessage struct {
	ID          int64                     `json:"id"`
	TicketID    int64                     `json:"ticket_id"`
	SenderID    int64                     `json:"sender_id"`
	SenderRole  string                    `json:"sender_role"`
	Source      string                    `json:"source"`
	Content     string                    `json:"content"`
	ExternalID  string                    `json:"-"`
	CreatedAt   time.Time                 `json:"created_at"`
	Attachments []SupportTicketAttachment `json:"attachments"`
}

type SupportTicketAttachment struct {
	ID        int64     `json:"id"`
	TicketID  int64     `json:"ticket_id,omitempty"`
	MessageID int64     `json:"message_id,omitempty"`
	OwnerID   int64     `json:"-"`
	FileName  string    `json:"file_name"`
	MimeType  string    `json:"mime_type"`
	Size      int       `json:"size"`
	Data      []byte    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateSupportTicketInput struct {
	Subject       string  `json:"subject"`
	Content       string  `json:"content"`
	AttachmentIDs []int64 `json:"attachment_ids"`
}

type SupportTicketReplyInput struct {
	Content       string  `json:"content"`
	AttachmentIDs []int64 `json:"attachment_ids"`
}

type SupportTicketImageInput struct {
	FileName string
	Data     []byte
}

type SupportTicketDetail struct {
	Ticket   *SupportTicket         `json:"ticket"`
	Messages []SupportTicketMessage `json:"messages"`
	HasMore  bool                   `json:"has_more"`
}

type SupportTicketPage struct {
	Items    []SupportTicket `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

type SupportTicketNotification struct {
	ID         int64
	TicketID   int64
	MessageID  int64
	Channel    string
	Attempts   int
	LeaseToken string
}

// SupportTicketRepository 将消息、附件绑定和通知写入同一事务。
type SupportTicketRepository interface {
	CreateTicket(context.Context, *SupportTicket, *SupportTicketMessage, []int64) error
	GetTicket(context.Context, int64) (*SupportTicket, error)
	ListTickets(ctx context.Context, ownerID int64, status string, page, pageSize int) (*SupportTicketPage, error)
	ListMessages(ctx context.Context, ticketID, afterID int64, limit int) ([]SupportTicketMessage, error)
	AddMessage(context.Context, *SupportTicketMessage, []int64) error
	SetStatus(ctx context.Context, ticketID int64, status string) error
	CreateAttachment(context.Context, *SupportTicketAttachment) error
	DeleteDraftAttachment(ctx context.Context, ownerID, id int64) error
	GetAttachment(context.Context, int64) (*SupportTicketAttachment, error)
	GetNotificationMessage(context.Context, int64) (*SupportTicket, *SupportTicketMessage, error)
	GetMessageByExternalID(context.Context, string) (*SupportTicketMessage, error)
	ClaimNotifications(ctx context.Context, limit int, lease time.Duration) ([]SupportTicketNotification, error)
	CompleteNotification(ctx context.Context, id int64, leaseToken string) error
	FailNotification(ctx context.Context, id int64, leaseToken, lastError string, retryAt time.Time) error
	HasNotificationReceipt(ctx context.Context, notificationID int64, key string) (bool, error)
	SaveNotificationReceipt(ctx context.Context, notificationID int64, key string) error
	SaveTelegramMapping(ctx context.Context, chatID, messageID, ticketID int64) error
	FindTelegramTicket(ctx context.Context, chatID, messageID int64) (int64, error)
}
