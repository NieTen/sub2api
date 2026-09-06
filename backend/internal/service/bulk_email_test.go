package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type bulkEmailTestRepository struct {
	BulkEmailRepository
	batch  *BulkEmailBatch
	drafts int
}

func (r *bulkEmailTestRepository) CreateDraft(_ context.Context, batch *BulkEmailBatch, _ []int64, _ bool) error {
	r.drafts++
	batch.ID = 3
	batch.TotalCount = 2
	r.batch = batch
	return nil
}
func (r *bulkEmailTestRepository) Get(context.Context, int64) (*BulkEmailBatch, error) {
	return r.batch, nil
}

func TestBulkEmailCreateFreezesDraftAndAttachmentsWithoutSending(t *testing.T) {
	settings := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settings.Set(context.Background(), SettingKeySMTPHost, "smtp.example.com"))
	repo := &bulkEmailTestRepository{}
	attachments := &supportTicketTestRepository{attachment: &SupportTicketAttachment{OwnerID: 8, FileName: "image.png", MimeType: "image/png", Data: supportTestPNG(t)}}
	s := NewBulkEmailService(repo, NewEmailService(settings, nil), attachments)
	batch, err := s.Create(context.Background(), 8, BulkEmailCreateInput{Subject: "通知", Body: "内容", UserIDs: []int64{2, 3}, AttachmentIDs: []int64{1}})
	require.NoError(t, err)
	require.Equal(t, "draft", batch.Status)
	require.EqualValues(t, 2, batch.TotalCount)
	require.Equal(t, 1, repo.drafts)
	require.Len(t, batch.Images, 1)
	require.Len(t, batch.Attachments, 1)
	image, err := s.GetImage(context.Background(), 3, 0)
	require.NoError(t, err)
	require.Equal(t, attachments.attachment.Data, image.Data)
	_, err = s.GetImage(context.Background(), 3, 1)
	require.ErrorIs(t, err, ErrSupportAttachmentNotFound)
}

func TestBulkEmailRejectsOtherOwnersAndAmbiguousRecipients(t *testing.T) {
	settings := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settings.Set(context.Background(), SettingKeySMTPHost, "smtp.example.com"))
	repo := &bulkEmailTestRepository{}
	s := NewBulkEmailService(repo, NewEmailService(settings, nil), &supportTicketTestRepository{attachment: &SupportTicketAttachment{OwnerID: 9}})
	_, err := s.Create(context.Background(), 8, BulkEmailCreateInput{Subject: "通知", Body: "内容", UserIDs: []int64{2}, AttachmentIDs: []int64{1}})
	require.ErrorIs(t, err, ErrBulkEmailInvalid)
	require.Zero(t, repo.drafts)
	_, err = s.Create(context.Background(), 8, BulkEmailCreateInput{Subject: "通知", Body: "内容", UserIDs: []int64{2}, AllActive: true})
	require.ErrorIs(t, err, ErrBulkEmailInvalid)
	_, err = s.Create(context.Background(), 8, BulkEmailCreateInput{Subject: "通知\r\nBcc:evil@example.com", Body: "内容", AllActive: true})
	require.ErrorIs(t, err, ErrBulkEmailInvalid)
}
