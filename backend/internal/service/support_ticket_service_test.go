package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
)

// 仅覆写测试涉及的方法，未预期的仓储调用会立即使测试失败。
type supportTicketTestRepository struct {
	SupportTicketRepository
	ticket     *SupportTicket
	ticketErr  error
	attachment *SupportTicketAttachment
	existing   *SupportTicketMessage
	mappingID  int64
	mappingErr error
	addErr     error
	added      *SupportTicketMessage
	loaded     bool
}

func (r *supportTicketTestRepository) GetTicket(context.Context, int64) (*SupportTicket, error) {
	return r.ticket, r.ticketErr
}
func (r *supportTicketTestRepository) GetAttachment(context.Context, int64) (*SupportTicketAttachment, error) {
	return r.attachment, nil
}
func (r *supportTicketTestRepository) ListMessages(context.Context, int64, int64, int) ([]SupportTicketMessage, error) {
	r.loaded = true
	return []SupportTicketMessage{}, nil
}
func (r *supportTicketTestRepository) FindTelegramTicket(context.Context, int64, int64) (int64, error) {
	return r.mappingID, r.mappingErr
}
func (r *supportTicketTestRepository) GetMessageByExternalID(context.Context, string) (*SupportTicketMessage, error) {
	if r.existing != nil {
		return r.existing, nil
	}
	return nil, ErrSupportTicketNotFound
}
func (r *supportTicketTestRepository) AddMessage(_ context.Context, message *SupportTicketMessage, _ []int64) error {
	r.added = message
	message.ID = 9
	return r.addErr
}
func (r *supportTicketTestRepository) GetNotificationMessage(context.Context, int64) (*SupportTicket, *SupportTicketMessage, error) {
	return r.ticket, r.added, nil
}

func TestSupportTicketRejectsOtherUsersBeforeLoadingMessages(t *testing.T) {
	repo := &supportTicketTestRepository{ticket: &SupportTicket{ID: 5, UserID: 2}}
	_, err := NewSupportTicketService(repo).Get(context.Background(), SupportTicketActor{UserID: 1}, 5, 0, 50)
	require.ErrorIs(t, err, ErrSupportTicketNotFound)
	require.False(t, repo.loaded)
}

func TestSupportTicketAttachmentAuthorization(t *testing.T) {
	tests := []struct {
		name       string
		actor      SupportTicketActor
		attachment SupportTicketAttachment
		allowed    bool
	}{
		{"草稿所属用户", SupportTicketActor{UserID: 2}, SupportTicketAttachment{OwnerID: 2}, true},
		{"管理员不能读取他人草稿", SupportTicketActor{UserID: 1, IsAdmin: true}, SupportTicketAttachment{OwnerID: 2}, false},
		{"用户不能读取他人工单图片", SupportTicketActor{UserID: 3}, SupportTicketAttachment{OwnerID: 1, TicketID: 5, MessageID: 7}, false},
		{"工单用户可以读取管理员回复图片", SupportTicketActor{UserID: 2}, SupportTicketAttachment{OwnerID: 1, TicketID: 5, MessageID: 7}, true},
		{"管理员可以读取已发送图片", SupportTicketActor{UserID: 1, IsAdmin: true}, SupportTicketAttachment{OwnerID: 2, TicketID: 5, MessageID: 7}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &supportTicketTestRepository{ticket: &SupportTicket{ID: 5, UserID: 2}, attachment: &tt.attachment}
			_, err := NewSupportTicketService(repo).GetAttachment(context.Background(), tt.actor, 7)
			if tt.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrSupportAttachmentNotFound)
			}
		})
	}
}

func TestSupportTicketReplyRejectsClosedTicket(t *testing.T) {
	repo := &supportTicketTestRepository{ticket: &SupportTicket{ID: 5, UserID: 2, Status: SupportTicketStatusClosed}}
	_, err := NewSupportTicketService(repo).Reply(context.Background(), SupportTicketActor{UserID: 2}, 5, SupportTicketReplyInput{Content: "回复"})
	require.ErrorIs(t, err, ErrSupportTicketClosed)
	require.Nil(t, repo.added)
}

func TestSupportTicketMessageValidation(t *testing.T) {
	for _, input := range []SupportTicketReplyInput{{}, {Content: " ", AttachmentIDs: []int64{1, 1}}, {Content: "文本", AttachmentIDs: []int64{0}}, {AttachmentIDs: []int64{1, 2, 3, 4, 5}}, {Content: string([]byte{0xff})}} {
		_, err := validateSupportMessage(input.Content, input.AttachmentIDs)
		require.Error(t, err)
	}
	content, err := validateSupportMessage("  ", []int64{1})
	require.NoError(t, err)
	require.Empty(t, content)
}

func supportTestPNG(t *testing.T) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	require.NoError(t, png.Encode(&buffer, im))
	return buffer.Bytes()
}

func TestSupportTicketImageValidation(t *testing.T) {
	valid := supportTestPNG(t)
	a, err := ValidateSupportTicketImage("..\\目录\\截图\r\n.png", valid)
	require.NoError(t, err)
	require.Equal(t, "截图.png", a.FileName)
	require.Equal(t, "image/png", a.MimeType)
	for _, data := range [][]byte{nil, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), valid[:len(valid)/2], make([]byte, SupportTicketMaxImageBytes+1)} {
		_, err := ValidateSupportTicketImage("a.png", data)
		require.ErrorIs(t, err, ErrSupportAttachmentInvalid)
	}
	// 构造合法校验和的超大尺寸 PNG 头，验证完整解码前已拒绝像素炸弹。
	large := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(large[16:20], 5000)
	binary.BigEndian.PutUint32(large[20:24], 5000)
	binary.BigEndian.PutUint32(large[29:33], crc32.ChecksumIEEE(large[12:29]))
	_, err = ValidateSupportTicketImage("large.png", large)
	require.ErrorIs(t, err, ErrSupportAttachmentInvalid)
}

func TestSupportTicketTelegramRequiresTrustedMapping(t *testing.T) {
	repo := &supportTicketTestRepository{mappingErr: ErrSupportTelegramUnmapped}
	_, err := NewSupportTicketService(repo).ReplyTelegram(context.Background(), -100, 7, 101, "#5 请处理", nil)
	require.ErrorIs(t, err, ErrSupportTelegramUnmapped)
	require.Nil(t, repo.added)
}

func TestSupportTicketTelegramDuplicateReturnsOriginal(t *testing.T) {
	existing := &SupportTicketMessage{ID: 10, TicketID: 5, ExternalID: "telegram:101"}
	repo := &supportTicketTestRepository{mappingID: 5, existing: existing}
	message, err := NewSupportTicketService(repo).ReplyTelegram(context.Background(), -100, 7, 101, "重复消息", nil)
	require.NoError(t, err)
	require.Same(t, existing, message)
	require.Nil(t, repo.added)
}

func TestSupportTicketTelegramSetsTrustedSourceAndRole(t *testing.T) {
	repo := &supportTicketTestRepository{mappingID: 5, ticket: &SupportTicket{ID: 5, UserID: 2, Status: SupportTicketStatusOpen}}
	message, err := NewSupportTicketService(repo).ReplyTelegram(context.Background(), -100, 7, 101, "已处理", []SupportTicketImageInput{{FileName: "a.png", Data: supportTestPNG(t)}})
	require.NoError(t, err)
	require.Equal(t, "telegram", message.Source)
	require.Equal(t, "admin", message.SenderRole)
	require.Zero(t, message.SenderID)
	require.Equal(t, "telegram:101", message.ExternalID)
	require.Len(t, message.Attachments, 1)
	require.Equal(t, int64(2), message.Attachments[0].OwnerID)
}

func TestSupportTicketAttachmentPropagatesStorageErrors(t *testing.T) {
	storageErr := errors.New("数据库不可用")
	repo := &supportTicketTestRepository{ticketErr: storageErr, attachment: &SupportTicketAttachment{MessageID: 7, TicketID: 5}}
	_, err := NewSupportTicketService(repo).GetAttachment(context.Background(), SupportTicketActor{UserID: 2}, 8)
	require.ErrorIs(t, err, storageErr)
}
