package service

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrBulkEmailInvalid = infraerrors.BadRequest("BULK_EMAIL_INVALID", "批量邮件参数无效")
var ErrBulkEmailNotFound = infraerrors.NotFound("BULK_EMAIL_NOT_FOUND", "邮件批次不存在")
var ErrBulkEmailState = infraerrors.Conflict("BULK_EMAIL_STATE", "邮件批次状态不允许当前操作")
var ErrBulkEmailNoRecipients = infraerrors.BadRequest("BULK_EMAIL_NO_RECIPIENTS", "没有符合所选用户范围及余额、充值条件的有效收件人，请调整筛选条件")

type BulkEmailCreateInput struct {
	RecipientFilter BulkEmailRecipientFilter `json:"recipient_filter"`
	Subject         string                   `json:"subject"`
	Body            string                   `json:"body"`
	UserIDs         []int64                  `json:"user_ids"`
	AllActive       bool                     `json:"all_active"`
	AttachmentIDs   []int64                  `json:"attachment_ids"`
}

type BulkEmailBatch struct {
	RecipientFilter    BulkEmailRecipientFilter  `json:"recipient_filter"`
	ID                 int64                     `json:"id"`
	CreatedBy          int64                     `json:"created_by"`
	Subject            string                    `json:"subject"`
	Body               string                    `json:"body"`
	Status             string                    `json:"status"`
	TotalCount         int64                     `json:"total_count"`
	SentCount          int64                     `json:"sent_count"`
	FailedCount        int64                     `json:"failed_count"`
	Images             []EmailInlineImage        `json:"-"`
	ImageCount         int                       `json:"image_count"`
	Attachments        []SupportTicketAttachment `json:"attachments"`
	DraftAttachmentIDs []int64                   `json:"-"`
	CreatedAt          time.Time                 `json:"created_at"`
	UpdatedAt          time.Time                 `json:"updated_at"`
}

type BulkEmailRecipient struct {
	ID         int64      `json:"id"`
	BatchID    int64      `json:"batch_id"`
	UserID     int64      `json:"user_id"`
	Email      string     `json:"email"`
	Status     string     `json:"status"`
	Attempts   int        `json:"attempts"`
	LastError  string     `json:"last_error"`
	SentAt     *time.Time `json:"sent_at"`
	LeaseToken string     `json:"-"`
}

type BulkEmailRepository interface {
	CreateDraft(ctx context.Context, batch *BulkEmailBatch, userIDs []int64, allActive bool) error
	List(ctx context.Context, page, pageSize int) ([]BulkEmailBatch, int64, error)
	Get(ctx context.Context, id int64) (*BulkEmailBatch, error)
	Recipients(ctx context.Context, id int64, page, pageSize int) ([]BulkEmailRecipient, int64, error)
	StartBatch(ctx context.Context, id int64) error
	Retry(ctx context.Context, id int64) error
	Claim(ctx context.Context, limit int, lease time.Duration) ([]BulkEmailRecipient, error)
	Complete(ctx context.Context, item BulkEmailRecipient, sendErr string, retryAt time.Time) error
}

type BulkEmailService struct {
	repo        BulkEmailRepository
	email       *EmailService
	attachments SupportTicketRepository
	cancel      context.CancelFunc
	mu          sync.Mutex
	wg          sync.WaitGroup
}

func NewBulkEmailService(repo BulkEmailRepository, email *EmailService, attachments SupportTicketRepository) *BulkEmailService {
	return &BulkEmailService{repo: repo, email: email, attachments: attachments}
}

func (s *BulkEmailService) Create(ctx context.Context, adminID int64, input BulkEmailCreateInput) (*BulkEmailBatch, error) {
	filter, err := input.RecipientFilter.Normalize()
	if err != nil {
		return nil, err
	}
	input.Subject, input.Body = strings.TrimSpace(input.Subject), strings.TrimSpace(input.Body)
	if adminID <= 0 || input.Subject == "" || !utf8.ValidString(input.Subject) || !utf8.ValidString(input.Body) || len([]rune(input.Subject)) > 200 || strings.ContainsAny(input.Subject, "\r\n\x00") || strings.ContainsRune(input.Body, '\x00') || len(input.Body) > 100000 || (input.Body == "" && len(input.AttachmentIDs) == 0) || len(input.AttachmentIDs) > 4 || len(input.UserIDs) > 10000 || (input.AllActive && len(input.UserIDs) > 0) || (!input.AllActive && len(input.UserIDs) == 0) {
		return nil, ErrBulkEmailInvalid
	}
	if _, err := s.email.GetSMTPConfig(ctx); err != nil {
		return nil, err
	}
	batch := &BulkEmailBatch{CreatedBy: adminID, Subject: input.Subject, Body: input.Body, Status: "draft", Images: []EmailInlineImage{}, RecipientFilter: filter}
	seen := map[int64]bool{}
	for _, id := range input.AttachmentIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		a, err := s.attachments.GetAttachment(ctx, id)
		if err != nil {
			return nil, err
		}
		if a.OwnerID != adminID || a.TicketID != 0 {
			return nil, ErrBulkEmailInvalid
		}
		batch.Images = append(batch.Images, EmailInlineImage{FileName: a.FileName, MimeType: a.MimeType, Data: a.Data})
		batch.DraftAttachmentIDs = append(batch.DraftAttachmentIDs, id)
	}
	batch.ImageCount = len(batch.Images)
	if err := s.repo.CreateDraft(ctx, batch, input.UserIDs, input.AllActive); err != nil {
		return nil, err
	}
	decorateBulkEmailBatch(batch)
	return batch, nil
}

func (s *BulkEmailService) List(ctx context.Context, page, pageSize int) ([]BulkEmailBatch, int64, error) {
	return s.repo.List(ctx, page, pageSize)
}
func (s *BulkEmailService) Get(ctx context.Context, id int64, page, pageSize int) (*BulkEmailBatch, []BulkEmailRecipient, int64, error) {
	b, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, 0, err
	}
	decorateBulkEmailBatch(b)
	r, n, err := s.repo.Recipients(ctx, id, page, pageSize)
	return b, r, n, err
}
func decorateBulkEmailBatch(b *BulkEmailBatch) {
	b.Attachments = []SupportTicketAttachment{}
	for i, image := range b.Images {
		b.Attachments = append(b.Attachments, SupportTicketAttachment{ID: int64(i), FileName: image.FileName, MimeType: image.MimeType, Size: len(image.Data)})
	}
}
func (s *BulkEmailService) GetImage(ctx context.Context, batchID int64, index int) (*EmailInlineImage, error) {
	b, err := s.repo.Get(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(b.Images) {
		return nil, ErrSupportAttachmentNotFound
	}
	return &b.Images[index], nil
}
func (s *BulkEmailService) StartBatch(ctx context.Context, id int64) error {
	if _, err := s.email.GetSMTPConfig(ctx); err != nil {
		return err
	}
	return s.repo.StartBatch(ctx, id)
}
func (s *BulkEmailService) Retry(ctx context.Context, id int64) error { return s.repo.Retry(ctx, id) }

func (s *BulkEmailService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.process(ctx)
			}
		}
	}()
}
func (s *BulkEmailService) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *BulkEmailService) process(ctx context.Context) {
	claimCtx, claimCancel := context.WithTimeout(ctx, 10*time.Second)
	items, err := s.repo.Claim(claimCtx, 1, 3*time.Minute)
	claimCancel()
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("批量邮件领取失败", "error", err)
		}
		return
	}
	for _, item := range items {
		deliveryCtx, deliveryCancel := context.WithTimeout(ctx, 2*time.Minute)
		batch, err := s.repo.Get(deliveryCtx, item.BatchID)
		if err == nil {
			body := supportEmailHTML(batch.Body, batch.Images)
			err = s.email.SendEmailWithImages(deliveryCtx, item.Email, batch.Subject, body, batch.Images)
		}
		deliveryCancel()
		failure := ""
		if err != nil {
			failure = "邮件发送失败，请检查 SMTP 配置或收件地址"
		}
		completeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		if err := s.repo.Complete(completeCtx, item, failure, time.Now().Add(time.Duration(1<<min(item.Attempts, 6))*time.Minute)); err != nil {
			slog.Warn("批量邮件状态保存失败", "error", err)
		}
		cancel()
	}
}

func supportEmailHTML(body string, images []EmailInlineImage) string {
	var b strings.Builder
	b.WriteString("<div style=\"white-space:pre-wrap\">")
	b.WriteString(html.EscapeString(body))
	b.WriteString("</div>")
	for i := range images {
		fmt.Fprintf(&b, "<p><img src=\"cid:image-%d\" style=\"max-width:100%%\" alt=\"附件图片\"></p>", i)
	}
	return b.String()
}
