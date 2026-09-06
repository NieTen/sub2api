package service

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type SupportTelegramUser struct {
	ID    int64 `json:"id"`
	IsBot bool  `json:"is_bot"`
}
type SupportTelegramChat struct {
	ID int64 `json:"id"`
}
type SupportTelegramPhoto struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}
type SupportTelegramDocument struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}
type SupportTelegramMessage struct {
	MessageID      int64                    `json:"message_id"`
	From           *SupportTelegramUser     `json:"from"`
	Chat           SupportTelegramChat      `json:"chat"`
	Text           string                   `json:"text"`
	Caption        string                   `json:"caption"`
	ReplyToMessage *SupportTelegramMessage  `json:"reply_to_message"`
	Photo          []SupportTelegramPhoto   `json:"photo"`
	Document       *SupportTelegramDocument `json:"document"`
}
type SupportTelegramUpdate struct {
	UpdateID int64                   `json:"update_id"`
	Message  *SupportTelegramMessage `json:"message"`
}
type supportTelegramResponse struct {
	OK        bool            `json:"ok"`
	Result    json.RawMessage `json:"result"`
	ErrorCode int             `json:"error_code"`
}
type supportTelegramFile struct {
	FilePath string `json:"file_path"`
	FileSize int64  `json:"file_size"`
}

// SupportTelegramAPIError 只保留状态码，禁止将含令牌的 URL 或原始服务端正文写入错误。
type SupportTelegramAPIError struct{ StatusCode int }

func (e *SupportTelegramAPIError) Error() string {
	return fmt.Sprintf("Telegram 请求失败（状态 %d）", e.StatusCode)
}

// telegramRequest 仅向 Telegram 官方固定域名请求，错误不包含令牌、原始 URL 或服务端响应正文。
func (s *SupportDeliveryService) telegramRequest(ctx context.Context, token, method, contentType string, body io.Reader, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/"+method, body)
	if err != nil {
		return errors.New("Telegram 请求构造失败")
	}
	request.Header.Set("Content-Type", contentType)
	response, err := s.client.Do(request)
	if err != nil {
		return errors.New("Telegram 网络请求失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &SupportTelegramAPIError{StatusCode: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return errors.New("Telegram 响应过大或读取失败")
	}
	var envelope supportTelegramResponse
	if err = json.Unmarshal(data, &envelope); err != nil {
		return errors.New("Telegram 返回无效响应")
	}
	if !envelope.OK {
		return &SupportTelegramAPIError{StatusCode: envelope.ErrorCode}
	}
	if result != nil {
		if err = json.Unmarshal(envelope.Result, result); err != nil {
			return errors.New("Telegram 响应解析失败")
		}
	}
	return nil
}
func (s *SupportDeliveryService) telegramJSON(ctx context.Context, token, method string, payload any, result any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.telegramRequest(ctx, token, method, "application/json", bytes.NewReader(data), result)
}

type supportTelegramTextRequest struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

func (s *SupportDeliveryService) sendTicketTelegram(ctx context.Context, notificationID int64, c *SupportDeliverySettings, ticket *SupportTicket, message *SupportTicketMessage) error {
	if c.TelegramBotToken == "" || c.TelegramChatID == "" {
		return nil
	}
	chatID, err := strconv.ParseInt(c.TelegramChatID, 10, 64)
	if err != nil {
		return ErrSupportDeliveryInvalid
	}
	author := "用户"
	if message.SenderRole == "admin" {
		author = "管理员"
	}
	content := fmt.Sprintf("工单 #%d：%s\n工单用户：%s\n发送方：%s\n\n%s\n\n直接回复此消息可回复工单。", ticket.ID, ticket.Subject, ticket.UserEmail, author, message.Content)
	for i, chunk := range supportTelegramChunks(content, 3500) {
		receiptKey := fmt.Sprintf("telegram:%s:text:%d", c.TelegramChatID, i)
		done, err := s.repo.HasNotificationReceipt(ctx, notificationID, receiptKey)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		var result SupportTelegramMessage
		if err = s.telegramJSON(ctx, c.TelegramBotToken, "sendMessage", supportTelegramTextRequest{ChatID: c.TelegramChatID, Text: chunk}, &result); err != nil {
			return err
		}
		if result.MessageID <= 0 {
			return errors.New("Telegram 未返回消息编号")
		}
		if err = s.repo.SaveTelegramMapping(ctx, chatID, result.MessageID, ticket.ID); err != nil {
			return err
		}
		if err = s.repo.SaveNotificationReceipt(ctx, notificationID, receiptKey); err != nil {
			return err
		}
	}
	images, err := s.messageImages(ctx, message)
	if err != nil {
		return err
	}
	for i, image := range images {
		receiptKey := fmt.Sprintf("telegram:%s:image:%d", c.TelegramChatID, i)
		done, err := s.repo.HasNotificationReceipt(ctx, notificationID, receiptKey)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		var payload bytes.Buffer
		writer := multipart.NewWriter(&payload)
		if err = writer.WriteField("chat_id", c.TelegramChatID); err != nil {
			return err
		}
		if err = writer.WriteField("caption", fmt.Sprintf("工单 #%d 附件，直接回复此图片可回复工单。", ticket.ID)); err != nil {
			return err
		}
		// 以原图文件发送，保留清晰度并兼容 Telegram 照片接口不接受的长图、GIF 和 WebP。
		method, field := "sendDocument", "document"
		part, err := writer.CreateFormFile(field, image.FileName)
		if err != nil {
			return err
		}
		if _, err = part.Write(image.Data); err != nil {
			return err
		}
		if err = writer.Close(); err != nil {
			return err
		}
		var result SupportTelegramMessage
		if err = s.telegramRequest(ctx, c.TelegramBotToken, method, writer.FormDataContentType(), &payload, &result); err != nil {
			return err
		}
		if result.MessageID <= 0 {
			return errors.New("Telegram 未返回图片消息编号")
		}
		if err = s.repo.SaveTelegramMapping(ctx, chatID, result.MessageID, ticket.ID); err != nil {
			return err
		}
		if err = s.repo.SaveNotificationReceipt(ctx, notificationID, receiptKey); err != nil {
			return err
		}
	}
	return nil
}

// Telegram 长度按 UTF-16 单元计数，避免表情符号导致长消息被拒绝。
func supportTelegramChunks(value string, limit int) []string {
	chunks := []string{}
	start, units := 0, 0
	for i, r := range value {
		cost := 1
		if r > 0xffff {
			cost = 2
		}
		if units+cost > limit {
			chunks = append(chunks, value[start:i])
			start = i
			units = 0
		}
		units += cost
	}
	if start < len(value) {
		chunks = append(chunks, value[start:])
	}
	return chunks
}

func (s *SupportDeliveryService) HandleTelegramWebhook(ctx context.Context, secret string, raw []byte) error {
	c, err := s.loadSettings(ctx)
	if err != nil {
		return err
	}
	if c.TelegramWebhookSecret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(c.TelegramWebhookSecret)) != 1 {
		return ErrSupportTelegramUnauthorized
	}
	if !c.Enabled || c.TelegramBotToken == "" {
		return nil
	}
	if len(raw) > 1024*1024 {
		return ErrSupportTicketInvalid
	}
	var update SupportTelegramUpdate
	if err = json.Unmarshal(raw, &update); err != nil {
		return ErrSupportTicketInvalid
	}
	message := update.Message
	if update.UpdateID < 0 || message == nil || message.From == nil || message.From.IsBot || message.ReplyToMessage == nil {
		return nil
	}
	chatID, err := strconv.ParseInt(c.TelegramChatID, 10, 64)
	if err != nil {
		return ErrSupportDeliveryInvalid
	}
	if message.Chat.ID != chatID {
		return nil
	}
	authorized := false
	for _, id := range c.TelegramAllowedUserIDs {
		if id == message.From.ID {
			authorized = true
			break
		}
	}
	if !authorized {
		return nil
	}
	// 在图片下载之前检查映射与幂等记录，无关回复不会触发网络请求。
	if _, err = s.repo.FindTelegramTicket(ctx, chatID, message.ReplyToMessage.MessageID); err != nil {
		if errors.Is(err, ErrSupportTelegramUnmapped) || errors.Is(err, ErrSupportTicketNotFound) {
			return nil
		}
		return err
	}
	if _, err = s.repo.GetMessageByExternalID(ctx, fmt.Sprintf("telegram:%d", update.UpdateID)); err == nil {
		return nil
	} else if !errors.Is(err, ErrSupportTicketNotFound) {
		return err
	}
	content := message.Text
	if content == "" {
		content = message.Caption
	}
	images := []SupportTicketImageInput{}
	if len(message.Photo) > 0 {
		photo := message.Photo[0]
		for _, candidate := range message.Photo {
			if int64(candidate.Width)*int64(candidate.Height) > int64(photo.Width)*int64(photo.Height) {
				photo = candidate
			}
		}
		if photo.FileSize > SupportTicketMaxImageBytes || photo.Width > 10000 || photo.Height > 10000 || int64(photo.Width)*int64(photo.Height) > 25000000 {
			return nil
		}
		image, err := s.downloadTelegramImage(ctx, c.TelegramBotToken, photo.FileID, "photo.jpg")
		if err != nil {
			if errors.Is(err, ErrSupportAttachmentInvalid) {
				return nil
			}
			return err
		}
		images = append(images, image)
	} else if message.Document != nil {
		document := message.Document
		if document.FileSize > SupportTicketMaxImageBytes || !strings.HasPrefix(document.MimeType, "image/") {
			return nil
		}
		image, err := s.downloadTelegramImage(ctx, c.TelegramBotToken, document.FileID, document.FileName)
		if err != nil {
			if errors.Is(err, ErrSupportAttachmentInvalid) {
				return nil
			}
			return err
		}
		images = append(images, image)
	}
	if strings.TrimSpace(content) == "" && len(images) == 0 {
		return nil
	}
	_, err = s.tickets.ReplyTelegram(ctx, chatID, message.ReplyToMessage.MessageID, update.UpdateID, content, images)
	if errors.Is(err, ErrSupportDuplicateMessage) || errors.Is(err, ErrSupportTicketClosed) || errors.Is(err, ErrSupportAttachmentInvalid) || errors.Is(err, ErrSupportTicketInvalid) || errors.Is(err, ErrSupportTelegramUnmapped) || errors.Is(err, ErrSupportTicketNotFound) {
		return nil
	}
	return err
}

type supportTelegramFileRequest struct {
	FileID string `json:"file_id"`
}

var supportTelegramFilePathPattern = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

func (s *SupportDeliveryService) downloadTelegramImage(ctx context.Context, token, fileID, fileName string) (SupportTicketImageInput, error) {
	var result SupportTicketImageInput
	if fileID == "" || len(fileID) > 1024 {
		return result, ErrSupportAttachmentInvalid
	}
	var file supportTelegramFile
	if err := s.telegramJSON(ctx, token, "getFile", supportTelegramFileRequest{FileID: fileID}, &file); err != nil {
		return result, err
	}
	if file.FileSize > SupportTicketMaxImageBytes || !supportTelegramFilePathPattern.MatchString(file.FilePath) || strings.HasPrefix(file.FilePath, "/") || strings.Contains(file.FilePath, "..") || path.Clean(file.FilePath) != file.FilePath {
		return result, ErrSupportAttachmentInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.telegram.org/file/bot"+token+"/"+file.FilePath, nil)
	if err != nil {
		return result, ErrSupportAttachmentInvalid
	}
	response, err := s.client.Do(request)
	if err != nil {
		return result, errors.New("Telegram 图片下载失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, errors.New("Telegram 图片下载失败")
	}
	if response.ContentLength > SupportTicketMaxImageBytes {
		return result, ErrSupportAttachmentInvalid
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, SupportTicketMaxImageBytes+1))
	if err != nil {
		return result, errors.New("Telegram 图片读取失败")
	}
	if len(data) > SupportTicketMaxImageBytes {
		return result, ErrSupportAttachmentInvalid
	}
	a, err := ValidateSupportTicketImage(fileName, data)
	if err != nil {
		return result, err
	}
	return SupportTicketImageInput{FileName: a.FileName, Data: a.Data}, nil
}
