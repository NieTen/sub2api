package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

type communityTelegramMessageFields communityTelegramMessage
type communityTelegramReplyReference struct {
	MessageID int64 `json:"message_id"`
}
type communityTelegramProfilePhotos struct {
	Photos [][]SupportTelegramPhoto `json:"photos"`
}

// 保留 Telegram 新消息类型的字段，未识别类型仍然有可阅读的存档，不静默丢弃。
func (m *communityTelegramMessage) UnmarshalJSON(data []byte) error {
	var fields communityTelegramMessageFields
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*m = communityTelegramMessage(fields)
	return json.Unmarshal(data, &m.Raw)
}
func communityGroupMessage(m *communityTelegramMessage, group int64) bool {
	return m != nil && group < 0 && m.Chat.ID == group && m.MessageID > 0 && (m.Chat.Type == "group" || m.Chat.Type == "supergroup")
}

func communityArchiveMessage(bot int64, updateID *int64, m *communityTelegramMessage) *CommunityChatMessage {
	result := &CommunityChatMessage{BotID: bot, GroupChatID: m.Chat.ID, UpdateID: updateID, TelegramMessageID: m.MessageID, SenderKind: "unknown", MessageType: "other", CreatedAt: time.Unix(m.Date, 0).UTC(), Text: m.Text}
	if m.Date <= 0 {
		result.CreatedAt = time.Now().UTC()
	}
	if m.EditDate > 0 {
		edited := time.Unix(m.EditDate, 0).UTC()
		result.EditedAt = &edited
	}
	if m.From != nil {
		result.SenderKind = "user"
		result.TelegramUserID = m.From.ID
		result.TelegramUsername = m.From.Username
		result.TelegramName = strings.TrimSpace(m.From.FirstName + " " + m.From.LastName)
		result.IsBot = m.From.IsBot
	}
	// 匿名管理员和频道署名必须显示群身份，不能把 Telegram 的占位用户当作真人。
	if m.SenderChat != nil {
		result.SenderKind = "chat"
		result.SenderChatID = m.SenderChat.ID
		result.TelegramUserID = 0
		result.TelegramUsername = m.SenderChat.Username
		result.TelegramName = m.SenderChat.Title
		result.IsBot = false
	}
	if len(m.ReplyToMessage) > 0 {
		var reply communityTelegramReplyReference
		_ = json.Unmarshal(m.ReplyToMessage, &reply)
		result.ReplyToMessageID = reply.MessageID
	}
	if result.Text != "" {
		result.MessageType = "text"
	} else {
		result.Text = m.Caption
	}
	var file *SupportTelegramDocument
	switch {
	case len(m.Photo) > 0:
		p := m.Photo[len(m.Photo)-1]
		file = &SupportTelegramDocument{FileID: p.FileID, FileSize: p.FileSize, FileName: "photo.jpg", MimeType: "image/jpeg"}
		result.MessageType = "photo"
	case m.Animation != nil:
		file = m.Animation
		result.MessageType = "animation"
	case m.Video != nil:
		file = m.Video
		result.MessageType = "video"
	case m.Audio != nil:
		file = m.Audio
		result.MessageType = "audio"
	case m.Voice != nil:
		file = m.Voice
		result.MessageType = "voice"
	case m.VideoNote != nil:
		file = m.VideoNote
		result.MessageType = "video_note"
	case m.Sticker != nil:
		file = m.Sticker
		result.MessageType = "sticker"
	case m.Document != nil:
		file = m.Document
		result.MessageType = "document"
	}
	if file != nil {
		result.FileID = file.FileID
		result.FileName = file.FileName
		result.FileSize = file.FileSize
		result.MimeType = file.MimeType
		result.MediaAvailable = file.FileID != "" && file.FileSize <= CommunityChatMediaLimit
		if result.FileName == "" {
			result.FileName = result.MessageType
		}
	}
	if result.MessageType == "other" {
		for _, kind := range []string{"poll", "contact", "location", "venue", "dice", "new_chat_members", "left_chat_member", "new_chat_title", "new_chat_photo", "pinned_message", "group_chat_created", "supergroup_chat_created", "video_chat_started", "video_chat_ended", "video_chat_scheduled", "video_chat_participants_invited", "migrate_to_chat_id", "migrate_from_chat_id", "successful_payment"} {
			if raw, ok := m.Raw[kind]; ok {
				result.MessageType = kind
				result.Text = string(raw)
				break
			}
		}
		if result.MessageType == "other" && result.Text == "" {
			remaining := make(map[string]json.RawMessage)
			for key, value := range m.Raw {
				switch key {
				case "message_id", "date", "edit_date", "chat", "from", "sender_chat", "reply_to_message":
					continue
				}
				remaining[key] = value
			}
			data, _ := json.Marshal(remaining)
			result.Text = string(data)
		}
	}
	return result
}

func (s *CommunityService) ChatMedia(ctx context.Context, actor SupportTicketActor, id int64) (*CommunityChatFile, error) {
	c, group, err := s.chatGroup(ctx, actor)
	if err != nil {
		return nil, err
	}
	message, err := s.repo.GetChatMessage(ctx, group, id)
	if err != nil {
		return nil, err
	}
	if message.FileID == "" {
		return nil, ErrCommunityNotFound
	}
	if message.FileSize > CommunityChatMediaLimit {
		return nil, communitySettingsFieldError("file", "该附件超过 Telegram 的 20 MB 下载限制，请在 Telegram 中查看")
	}
	if message.BotID != c.BotID {
		return nil, communitySettingsFieldError("file", "该消息由旧机器人接收，请在 Telegram 中查看原附件")
	}
	_, shared, err := s.checkedConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	return s.downloadCommunityFile(ctx, shared.TelegramBotToken, message.FileID, message.FileName, CommunityChatMediaLimit)
}
func (s *CommunityService) ChatAvatar(ctx context.Context, actor SupportTicketActor, id int64) (*CommunityChatFile, error) {
	c, group, err := s.chatGroup(ctx, actor)
	if err != nil {
		return nil, err
	}
	if _, err = s.repo.GetChatPerson(ctx, group, id); err != nil {
		return nil, err
	}
	cached, err := s.repo.GetChatAvatar(ctx, c.BotID, id)
	if err == nil {
		if len(cached.Data) == 0 {
			return nil, ErrCommunityNotFound
		}
		return cached, nil
	}
	if !errors.Is(err, ErrCommunityNotFound) {
		return nil, err
	}
	_, shared, err := s.checkedConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	var photos communityTelegramProfilePhotos
	if err = s.delivery.telegramJSON(ctx, shared.TelegramBotToken, "getUserProfilePhotos", map[string]any{"user_id": id, "limit": 1}, &photos); err != nil {
		return nil, communitySettingsTelegramError("avatar", "读取头像", err)
	}
	file := &CommunityChatFile{MimeType: "image/jpeg", FileName: "avatar.jpg", Data: []byte{}}
	if len(photos.Photos) > 0 && len(photos.Photos[0]) > 0 {
		// 小尺寸头像即可满足列表，限制实际下载大小，避免批量头像耗尽内存。
		photo := photos.Photos[0][0]
		file, err = s.downloadCommunityFile(ctx, shared.TelegramBotToken, photo.FileID, "avatar.jpg", 1024*1024)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(file.MimeType, "image/") {
			return nil, ErrCommunityNotFound
		}
	}
	if err = s.repo.SaveChatAvatar(ctx, c.BotID, id, file); err != nil {
		return nil, err
	}
	if len(file.Data) == 0 {
		return nil, ErrCommunityNotFound
	}
	return file, nil
}
func (s *CommunityService) downloadCommunityFile(ctx context.Context, token, fileID, fileName string, limit int64) (*CommunityChatFile, error) {
	if fileID == "" || len(fileID) > 1024 {
		return nil, ErrCommunityInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var file supportTelegramFile
	if err := s.delivery.telegramJSON(ctx, token, "getFile", supportTelegramFileRequest{FileID: fileID}, &file); err != nil {
		return nil, communitySettingsTelegramError("file", "读取附件", err)
	}
	if file.FileSize > limit {
		return nil, communitySettingsFieldError("file", "附件大小超过网页下载限制，请在 Telegram 中查看")
	}
	if !supportTelegramFilePathPattern.MatchString(file.FilePath) || strings.HasPrefix(file.FilePath, "/") || strings.Contains(file.FilePath, "..") || path.Clean(file.FilePath) != file.FilePath {
		return nil, ErrCommunityInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.telegram.org/file/bot"+token+"/"+file.FilePath, nil)
	if err != nil {
		return nil, ErrCommunityInvalid
	}
	response, err := s.delivery.client.Do(request)
	if err != nil {
		return nil, errors.New("Telegram 附件下载失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Telegram 附件读取失败（状态 %d）", response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, ErrCommunityInvalid
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("Telegram 附件读取失败或超过大小限制")
	}
	mime := http.DetectContentType(data)
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "video/mp4", "video/webm", "audio/mpeg", "audio/wave", "audio/ogg", "application/ogg":
	default:
		mime = "application/octet-stream"
	}
	fileName = path.Base(strings.ReplaceAll(fileName, `\`, "/"))
	if fileName == "." || fileName == "" {
		fileName = "attachment"
	}
	return &CommunityChatFile{FileName: fileName, MimeType: mime, Data: data}, nil
}
