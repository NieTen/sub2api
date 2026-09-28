package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type communityChatTestRepository struct {
	*communityTestRepository
	messages []*CommunityChatMessage
	send     *CommunityChatSend
}

func (r *communityChatTestRepository) SaveChatMessage(_ context.Context, m *CommunityChatMessage) error {
	m.ID = int64(len(r.messages) + 1)
	r.messages = append(r.messages, m)
	return nil
}
func (r *communityChatTestRepository) ReserveChatSend(_ context.Context, s *CommunityChatSend) (bool, error) {
	if r.send != nil {
		if r.send.ContentHash != s.ContentHash {
			return false, ErrCommunityConflict
		}
		*s = *r.send
		return false, nil
	}
	s.ID = 1
	s.Status = "sending"
	copy := *s
	r.send = &copy
	return true, nil
}
func (r *communityChatTestRepository) FinishChatSend(ctx context.Context, s *CommunityChatSend, m *CommunityChatMessage) error {
	if m != nil {
		if err := r.SaveChatMessage(ctx, m); err != nil {
			return err
		}
		s.MessageID = m.ID
	}
	copy := *s
	r.send = &copy
	return nil
}
func (r *communityChatTestRepository) GetChatMessage(_ context.Context, group, id int64) (*CommunityChatMessage, error) {
	for _, m := range r.messages {
		if m.ID == id && m.GroupChatID == group {
			return m, nil
		}
	}
	return nil, ErrCommunityNotFound
}

func TestCommunityChatArchivesMessagesEditsAndAnonymousSenders(t *testing.T) {
	s, base, _ := communityTestService(t)
	repo := &communityChatTestRepository{communityTestRepository: base}
	s.repo = repo
	c, err := s.GetSettings(context.Background())
	require.NoError(t, err)
	shared, err := s.delivery.loadSettings(context.Background())
	require.NoError(t, err)
	cases := []struct {
		raw, kind, text string
		sender          int64
		edited          bool
	}{
		{`{"update_id":11,"message":{"message_id":5,"date":1000,"chat":{"id":-100,"type":"supergroup"},"from":{"id":101,"first_name":"甲"},"text":"<script>仅文本</script>"}}`, "text", "<script>仅文本</script>", 101, false},
		{`{"update_id":12,"edited_message":{"message_id":5,"date":1000,"edit_date":1100,"chat":{"id":-100,"type":"supergroup"},"from":{"id":101},"text":"已修改"}}`, "text", "已修改", 101, true},
		{`{"update_id":13,"message":{"message_id":6,"date":1000,"chat":{"id":-100,"type":"supergroup"},"from":{"id":1087968824,"is_bot":true},"sender_chat":{"id":-100,"title":"匿名管理员"},"caption":"照片说明","photo":[{"file_id":"small","file_size":50},{"file_id":"large","file_size":100}]}}`, "photo", "照片说明", 0, false},
		{`{"update_id":14,"message":{"message_id":7,"date":1000,"chat":{"id":-100,"type":"supergroup"},"from":{"id":102,"is_bot":true},"voice":{"file_id":"voice","mime_type":"audio/ogg","file_size":100}}}`, "voice", "", 102, false},
		{`{"update_id":15,"message":{"message_id":8,"date":1000,"chat":{"id":-100,"type":"supergroup"},"poll":{"question":"投票","options":[]}}}`, "poll", `{"question":"投票","options":[]}`, 0, false},
		{`{"update_id":16,"message":{"message_id":9,"date":1000,"chat":{"id":-100,"type":"supergroup"},"future_feature":{"note":"新类型"}}}`, "other", `{"future_feature":{"note":"新类型"}}`, 0, false},
	}
	for _, tc := range cases {
		require.NoError(t, s.HandleTelegramWebhook(context.Background(), shared.TelegramWebhookSecret, []byte(tc.raw)))
		require.NoError(t, s.processWebhook(context.Background(), c, shared, CommunityWebhookEvent{BotID: 77, Payload: []byte(tc.raw)}))
		m := repo.messages[len(repo.messages)-1]
		require.Equal(t, tc.kind, m.MessageType)
		require.Equal(t, tc.text, m.Text)
		require.Equal(t, tc.sender, m.TelegramUserID)
		require.Equal(t, tc.edited, m.EditedAt != nil)
		if tc.kind == "photo" {
			require.Equal(t, "chat", m.SenderKind)
			require.Equal(t, "large", m.FileID)
		}
	}
	require.Equal(t, len(cases), base.queued)
	raw := `{"update_id":20,"message":{"message_id":5,"chat":{"id":-999,"type":"supergroup"},"text":"其他群"}}`
	require.NoError(t, s.HandleTelegramWebhook(context.Background(), shared.TelegramWebhookSecret, []byte(raw)))
	require.Equal(t, len(cases), base.queued)
	require.NoError(t, s.processWebhook(context.Background(), c, shared, CommunityWebhookEvent{BotID: 77, Payload: []byte(raw)}))
	require.Len(t, repo.messages, len(cases))
	encoded, err := json.Marshal(repo.messages[2])
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "file_id")
	require.NotContains(t, string(encoded), "bot_id")
}

func TestCommunityChatSendingDoesNotRepeatAfterLostResponse(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "成功重试返回原记录", true: "结果未知不重发"}[uncertain], func(t *testing.T) {
			s, base, _ := communityTestService(t)
			repo := &communityChatTestRepository{communityTestRepository: base}
			s.repo = repo
			calls := 0
			s.delivery.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/getMe") {
					return supportWebhookResponseForTest(200, `{"ok":true,"result":{"id":77,"is_bot":true}}`), nil
				}
				require.True(t, strings.HasSuffix(req.URL.Path, "/sendMessage"))
				calls++
				var input supportTelegramTextRequest
				require.NoError(t, json.NewDecoder(req.Body).Decode(&input))
				require.Equal(t, "-100", input.ChatID)
				require.Equal(t, "网页消息", input.Text)
				if uncertain {
					return nil, errors.New("含令牌的模拟原始网络错误")
				}
				return supportWebhookResponseForTest(200, `{"ok":true,"result":{"message_id":123,"date":1000,"chat":{"id":-100,"type":"supergroup"},"from":{"id":77,"is_bot":true},"text":"网页消息"}}`), nil
			})
			actor := SupportTicketActor{UserID: 1, IsAdmin: true}
			input := CommunityChatSendInput{Text: "网页消息", ClientRequestID: "request-fixture-123"}
			first, err := s.SendChatMessage(context.Background(), actor, input)
			if uncertain {
				require.ErrorIs(t, err, ErrCommunityChatUncertain)
				require.NotContains(t, err.Error(), "令牌")
			} else {
				require.NoError(t, err)
				require.True(t, first.Outgoing)
				require.EqualValues(t, 1, first.AdminUserID)
			}
			repeated, err := s.SendChatMessage(context.Background(), actor, input)
			if uncertain {
				require.ErrorIs(t, err, ErrCommunityChatUncertain)
				require.Empty(t, repo.messages)
			} else {
				require.NoError(t, err)
				require.Equal(t, first.ID, repeated.ID)
				require.Len(t, repo.messages, 1)
			}
			require.Equal(t, 1, calls)
			input.Text = "篡改同一幂等键"
			_, err = s.SendChatMessage(context.Background(), actor, input)
			require.ErrorIs(t, err, ErrCommunityConflict)
			require.Equal(t, 1, calls)
		})
	}
}

func TestCommunityChatPermissionsAndTextLength(t *testing.T) {
	s, _, _ := communityTestService(t)
	ctx := context.Background()
	user := SupportTicketActor{UserID: 1}
	_, err := s.ListChatMessages(ctx, user, CommunityChatFilter{})
	require.ErrorIs(t, err, ErrCommunityChatForbidden)
	_, err = s.SendChatMessage(ctx, user, CommunityChatSendInput{})
	require.ErrorIs(t, err, ErrCommunityChatForbidden)
	_, err = s.ChatPerson(ctx, user, 101)
	require.ErrorIs(t, err, ErrCommunityChatForbidden)
	_, err = s.ChatMedia(ctx, user, 1)
	require.ErrorIs(t, err, ErrCommunityChatForbidden)
	_, err = s.ChatAvatar(ctx, user, 101)
	require.ErrorIs(t, err, ErrCommunityChatForbidden)
	_, err = s.SendChatMessage(ctx, SupportTicketActor{UserID: 1, IsAdmin: true}, CommunityChatSendInput{Text: strings.Repeat("😀", 2049), ClientRequestID: "request-fixture-123"})
	require.ErrorIs(t, err, ErrCommunityInvalid)
}

func TestCommunityChatFileProxyRejectsUnsafePathsAndTypes(t *testing.T) {
	s, _, _ := communityTestService(t)
	requests := 0
	s.delivery.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		return supportWebhookResponseForTest(200, `{"ok":true,"result":{"file_path":"../secret","file_size":10}}`), nil
	})
	_, err := s.downloadCommunityFile(context.Background(), "fixture", "file", "unsafe.html", 1024)
	require.ErrorIs(t, err, ErrCommunityInvalid)
	require.Equal(t, 1, requests)
	s.delivery.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "api.telegram.org", req.URL.Host)
		if req.Method == http.MethodPost {
			return supportWebhookResponseForTest(200, `{"ok":true,"result":{"file_path":"documents/file.html","file_size":10}}`), nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("<html><script>alert(1)</script>"))}, nil
	})
	file, err := s.downloadCommunityFile(context.Background(), "fixture", "file", "unsafe.html", 1024)
	require.NoError(t, err)
	require.Equal(t, "application/octet-stream", file.MimeType)
}

func TestCommunityChatWebhookUpgradeSubscribesEditedMessagesOnce(t *testing.T) {
	settings := newNotificationEmailMemorySettingRepo()
	config := validSupportDeliverySettingsForTest()
	config.TelegramWebhookURL = "https://portal.example.com" + SupportTelegramWebhookPath
	config.TelegramWebhookRegistered = true
	data, err := json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, settings.Set(context.Background(), SettingKeySupportDelivery, string(data)))
	s := NewSupportDeliveryService(settings, nil, nil, nil)
	calls := 0
	s.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		var input supportTelegramWebhookRequest
		require.NoError(t, json.NewDecoder(req.Body).Decode(&input))
		require.Contains(t, input.AllowedUpdates, "edited_message")
		return supportWebhookResponseForTest(200, `{"ok":true,"result":true}`), nil
	})
	s.syncTelegramWebhook(context.Background())
	s.syncTelegramWebhook(context.Background())
	require.Equal(t, 1, calls)
	saved, err := s.loadSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, supportTelegramWebhookRevision, saved.TelegramWebhookRevision)
}
