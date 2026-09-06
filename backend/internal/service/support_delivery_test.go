package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
)

type supportDeliveryTestTransport struct {
	respond func(*http.Request) (*http.Response, error)
}

type supportDeliveryReceiptTestRepository struct {
	*supportTicketTestRepository
	receipts map[string]bool
	mappings []int64
}

func (r *supportDeliveryReceiptTestRepository) HasNotificationReceipt(_ context.Context, _ int64, key string) (bool, error) {
	return r.receipts[key], nil
}
func (r *supportDeliveryReceiptTestRepository) SaveNotificationReceipt(_ context.Context, _ int64, key string) error {
	r.receipts[key] = true
	return nil
}
func (r *supportDeliveryReceiptTestRepository) SaveTelegramMapping(_ context.Context, _ int64, messageID, _ int64) error {
	r.mappings = append(r.mappings, messageID)
	return nil
}

func (r supportDeliveryTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return r.respond(request)
}

func supportDeliveryTestSettings(t *testing.T) *notificationEmailMemorySettingRepo {
	t.Helper()
	r := newNotificationEmailMemorySettingRepo()
	raw, err := json.Marshal(SupportDeliverySettings{Enabled: true, AdminEmails: []string{"admin@example.com"}, TelegramBotToken: "123456:abcdefghijklmnopqrstuvwxyz123456", TelegramChatID: "-10012345", TelegramAllowedUserIDs: []int64{42}, TelegramWebhookSecret: "secret_12345678901234567890"})
	require.NoError(t, err)
	require.NoError(t, r.Set(context.Background(), SettingKeySupportDelivery, string(raw)))
	return r
}

func TestSupportDeliverySettingsPreserveAndRedactSecrets(t *testing.T) {
	r := supportDeliveryTestSettings(t)
	s := NewSupportDeliveryService(r, nil, nil, nil)
	view, err := s.GetSettings(context.Background())
	require.NoError(t, err)
	require.Empty(t, view.TelegramBotToken)
	require.Empty(t, view.TelegramWebhookSecret)
	require.True(t, view.TelegramBotTokenConfigured)
	require.True(t, view.TelegramWebhookSecretConfigured)
	view.AdminEmails = []string{"ADMIN@example.com", "admin@example.com"}
	saved, err := s.UpdateSettings(context.Background(), *view)
	require.NoError(t, err)
	require.Equal(t, []string{"admin@example.com"}, saved.AdminEmails)
	stored, err := s.loadSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "123456:abcdefghijklmnopqrstuvwxyz123456", stored.TelegramBotToken)
	require.Equal(t, "secret_12345678901234567890", stored.TelegramWebhookSecret)
	saved.TelegramAllowedUserIDs = nil
	_, err = s.UpdateSettings(context.Background(), *saved)
	require.ErrorIs(t, err, ErrSupportDeliveryInvalid)
}

func TestSupportDeliveryWebhookAuthorizationBeforeRepositoryAccess(t *testing.T) {
	s := NewSupportDeliveryService(supportDeliveryTestSettings(t), nil, nil, nil)
	valid := `{"update_id":12,"message":{"from":{"id":42},"chat":{"id":-10012345},"text":"回复","reply_to_message":{"message_id":7}}}`
	require.ErrorIs(t, s.HandleTelegramWebhook(context.Background(), "bad", []byte(valid)), ErrSupportTelegramUnauthorized)
	for _, payload := range []string{strings.Replace(valid, `"id":42`, `"id":43`, 1), strings.Replace(valid, `-10012345`, `-10099999`, 1), `{"update_id":12,"message":{"from":{"id":42},"chat":{"id":-10012345},"text":"普通聊天"}}`} {
		require.NoError(t, s.HandleTelegramWebhook(context.Background(), "secret_12345678901234567890", []byte(payload)))
	}
}

func TestSupportDeliveryWebhookRoutesAuthorizedReplyAndDeduplicates(t *testing.T) {
	repo := &supportTicketTestRepository{ticket: &SupportTicket{ID: 5, UserID: 2, Status: SupportTicketStatusOpen}, mappingID: 5}
	s := NewSupportDeliveryService(supportDeliveryTestSettings(t), nil, repo, NewSupportTicketService(repo))
	payload := []byte(`{"update_id":12,"message":{"from":{"id":42},"chat":{"id":-10012345},"text":"机器人回复","reply_to_message":{"message_id":7}}}`)
	require.NoError(t, s.HandleTelegramWebhook(context.Background(), "secret_12345678901234567890", payload))
	require.NotNil(t, repo.added)
	require.Equal(t, "telegram:12", repo.added.ExternalID)
	require.Equal(t, "机器人回复", repo.added.Content)
	require.Equal(t, "admin", repo.added.SenderRole)
	repo.existing = repo.added
	repo.added = nil
	require.NoError(t, s.HandleTelegramWebhook(context.Background(), "secret_12345678901234567890", payload))
	require.Nil(t, repo.added)
}

func TestSupportDeliveryTelegramFilePathCannotEscapeOfficialHost(t *testing.T) {
	for _, filePath := range []string{"https://127.0.0.1/private", "../secret", "/photos/private", "photos/../../private", "photos/%2e%2e/private"} {
		t.Run(filePath, func(t *testing.T) {
			s := NewSupportDeliveryService(nil, nil, nil, nil)
			calls := 0
			s.client.Transport = supportDeliveryTestTransport{respond: func(request *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "api.telegram.org", request.URL.Host)
				raw, _ := json.Marshal(map[string]any{"ok": true, "result": map[string]any{"file_path": filePath}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw)), Header: make(http.Header)}, nil
			}}
			_, err := s.downloadTelegramImage(context.Background(), "123:token", "file-id", "image.jpg")
			require.ErrorIs(t, err, ErrSupportAttachmentInvalid)
			require.Equal(t, 1, calls)
		})
	}
}

func TestSupportDeliveryTelegramDownloadValidatesActualImage(t *testing.T) {
	for _, data := range [][]byte{[]byte("<svg onload=alert(1)>"), bytes.Repeat([]byte{1}, SupportTicketMaxImageBytes+1), supportTestPNG(t)} {
		s := NewSupportDeliveryService(nil, nil, nil, nil)
		s.client.Transport = supportDeliveryTestTransport{respond: func(request *http.Request) (*http.Response, error) {
			body := data
			if strings.HasSuffix(request.URL.Path, "/getFile") {
				body = []byte(`{"ok":true,"result":{"file_path":"photos/picture.png"}}`)
			}
			return &http.Response{StatusCode: 200, ContentLength: int64(len(body)), Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
		}}
		image, err := s.downloadTelegramImage(context.Background(), "123:token", "file-id", "image.png")
		if bytes.HasPrefix(data, []byte{137, 'P', 'N', 'G'}) {
			require.NoError(t, err)
			require.Equal(t, data, image.Data)
		} else {
			require.ErrorIs(t, err, ErrSupportAttachmentInvalid)
		}
	}
}

func TestSupportDeliveryTelegramChunksRespectUTF16Limits(t *testing.T) {
	text := strings.Repeat("🙂中文", 2000)
	chunks := supportTelegramChunks(text, 3500)
	require.Equal(t, text, strings.Join(chunks, ""))
	for _, chunk := range chunks {
		require.LessOrEqual(t, len(utf16.Encode([]rune(chunk))), 3500)
	}
}

func TestSupportDeliveryInlineImagesMIMEAndEscaping(t *testing.T) {
	images := []EmailInlineImage{{FileName: "图片.png", MimeType: "image/png", Data: supportTestPNG(t)}}
	body := supportEmailHTML("<script>alert(1)</script>", images)
	require.Contains(t, body, "&lt;script&gt;")
	require.Contains(t, body, "cid:image-0")
	message, err := buildSMTPMessageWithImages(&SMTPConfig{Host: "smtp.example.com", From: "from@example.com", FromName: "测试"}, "to@example.com", "主题", body, images)
	require.NoError(t, err)
	parsed, err := mail.ReadMessage(bytes.NewReader(message.data))
	require.NoError(t, err)
	mediaType, parameters, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/related", mediaType)
	reader := multipart.NewReader(parsed.Body, parameters["boundary"])
	textPart, err := reader.NextPart()
	require.NoError(t, err)
	textBody, err := io.ReadAll(textPart)
	require.NoError(t, err)
	require.Contains(t, string(textBody), "cid:image-0")
	imagePart, err := reader.NextPart()
	require.NoError(t, err)
	require.Equal(t, "<image-0>", imagePart.Header.Get("Content-ID"))
	require.Equal(t, "base64", imagePart.Header.Get("Content-Transfer-Encoding"))
	require.Equal(t, "image/png", imagePart.Header.Get("Content-Type"))
	_, err = buildSMTPMessageWithImages(&SMTPConfig{From: "from@example.com"}, "to@example.com\r\nBcc:evil@example.com", "title", body, images)
	require.Error(t, err)
}

func TestSupportDeliveryMIMEHeaderLiteralDoesNotTruncateHeaders(t *testing.T) {
	images := []EmailInlineImage{{FileName: "a.png", MimeType: "image/png", Data: supportTestPNG(t)}}
	message, err := buildSMTPMessageWithImages(&SMTPConfig{Host: "smtp.example.com", From: "from@example.com", FromName: "MIME-Version:"}, "to@example.com", "MIME-Version: ordinary subject", "body", images)
	require.NoError(t, err)
	parsed, err := mail.ReadMessage(bytes.NewReader(message.data))
	require.NoError(t, err)
	require.Contains(t, parsed.Header.Get("Subject"), "ordinary subject")
	require.Contains(t, parsed.Header.Get("To"), "to@example.com")
	require.Contains(t, parsed.Header.Get("Content-Type"), "multipart/related")
}

func TestSupportDeliveryTelegramRetryResumesAfterSuccessfulParts(t *testing.T) {
	repo := &supportDeliveryReceiptTestRepository{supportTicketTestRepository: &supportTicketTestRepository{attachment: &SupportTicketAttachment{FileName: "image.png", MimeType: "image/png", Data: supportTestPNG(t)}}, receipts: map[string]bool{}}
	s := NewSupportDeliveryService(nil, nil, repo, nil)
	methods := []string{}
	s.client.Transport = supportDeliveryTestTransport{respond: func(request *http.Request) (*http.Response, error) {
		method := request.URL.Path[strings.LastIndex(request.URL.Path, "/")+1:]
		methods = append(methods, method)
		if method == "sendMessage" {
			var payload supportTelegramTextRequest
			require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
			require.Contains(t, payload.Text, "工单用户：user@example.com\n发送方：管理员")
		}
		if len(methods) == 2 {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":55}}`)), Header: make(http.Header)}, nil
	}}
	c := &SupportDeliverySettings{TelegramBotToken: "123:token", TelegramChatID: "-100"}
	ticket := &SupportTicket{ID: 9, Subject: "问题", UserEmail: "user@example.com"}
	message := &SupportTicketMessage{SenderRole: "admin", Content: "回复", Attachments: []SupportTicketAttachment{{ID: 1}}}
	require.Error(t, s.sendTicketTelegram(context.Background(), 1, c, ticket, message))
	require.NoError(t, s.sendTicketTelegram(context.Background(), 1, c, ticket, message))
	require.Equal(t, []string{"sendMessage", "sendDocument", "sendDocument"}, methods)
	require.Len(t, repo.receipts, 2)
}
