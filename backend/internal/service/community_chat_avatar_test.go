package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type communityAvatarTestRepository struct {
	*communityTestRepository
	cached    *CommunityChatFile
	cacheBot  int64
	cacheUser int64
	saves     int
	personErr error
}

func (r *communityAvatarTestRepository) GetChatPerson(_ context.Context, group, id int64) (*CommunityChatPerson, error) {
	if r.personErr != nil {
		return nil, r.personErr
	}
	if group != -100 || id != 42 {
		return nil, ErrCommunityNotFound
	}
	return &CommunityChatPerson{TelegramUserID: id}, nil
}

func (r *communityAvatarTestRepository) GetChatAvatar(_ context.Context, bot, id int64) (*CommunityChatFile, error) {
	if r.cached == nil || r.cacheBot != bot || r.cacheUser != id {
		return nil, ErrCommunityNotFound
	}
	return r.cached, nil
}

func (r *communityAvatarTestRepository) SaveChatAvatar(_ context.Context, bot, id int64, file *CommunityChatFile) error {
	r.cached, r.cacheBot, r.cacheUser = file, bot, id
	r.saves++
	return nil
}

func TestCommunityChatAvatarDownloadsSmallPhotoAndUsesScopedCache(t *testing.T) {
	s, base, _ := communityTestService(t)
	repo := &communityAvatarTestRepository{communityTestRepository: base}
	s.repo = repo
	photo, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jVioAAAAASUVORK5CYII=")
	require.NoError(t, err)
	profileCalls, fileCalls, downloadCalls := 0, 0, 0
	s.delivery.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "api.telegram.org", req.URL.Host)
		switch {
		case strings.HasSuffix(req.URL.Path, "/getMe"):
			return supportWebhookResponseForTest(200, `{"ok":true,"result":{"id":77,"is_bot":true}}`), nil
		case strings.HasSuffix(req.URL.Path, "/getUserProfilePhotos"):
			profileCalls++
			var input map[string]int64
			require.NoError(t, json.NewDecoder(req.Body).Decode(&input))
			require.EqualValues(t, 42, input["user_id"])
			require.EqualValues(t, 1, input["limit"])
			return supportWebhookResponseForTest(200, `{"ok":true,"result":{"photos":[[{"file_id":"small","width":160,"height":160},{"file_id":"large","width":640,"height":640}]]}}`), nil
		case strings.HasSuffix(req.URL.Path, "/getFile"):
			fileCalls++
			var input supportTelegramFileRequest
			require.NoError(t, json.NewDecoder(req.Body).Decode(&input))
			require.Equal(t, "small", input.FileID)
			return supportWebhookResponseForTest(200, `{"ok":true,"result":{"file_path":"photos/avatar.png","file_size":68}}`), nil
		default:
			downloadCalls++
			require.Equal(t, http.MethodGet, req.Method)
			require.True(t, strings.HasSuffix(req.URL.Path, "/photos/avatar.png"))
			return &http.Response{StatusCode: 200, ContentLength: int64(len(photo)), Body: io.NopCloser(bytes.NewReader(photo))}, nil
		}
	})
	actor := SupportTicketActor{UserID: 1, IsAdmin: true}
	first, err := s.ChatAvatar(context.Background(), actor, 42)
	require.NoError(t, err)
	require.Equal(t, photo, first.Data)
	require.Equal(t, "image/png", first.MimeType)
	require.Equal(t, "avatar.jpg", first.FileName)
	second, err := s.ChatAvatar(context.Background(), actor, 42)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, 1, repo.saves)
	require.EqualValues(t, 77, repo.cacheBot)
	require.EqualValues(t, 42, repo.cacheUser)
	require.Equal(t, 1, profileCalls)
	require.Equal(t, 1, fileCalls)
	require.Equal(t, 1, downloadCalls)
	// 即使已有头像缓存，也必须先校验此身份属于当前群资料范围。
	repo.personErr = ErrCommunityNotFound
	_, err = s.ChatAvatar(context.Background(), actor, 42)
	require.ErrorIs(t, err, ErrCommunityNotFound)
	require.Equal(t, 1, profileCalls)
}

func TestCommunityChatAvatarCachesMissingPhoto(t *testing.T) {
	s, base, _ := communityTestService(t)
	repo := &communityAvatarTestRepository{communityTestRepository: base}
	s.repo = repo
	profileCalls := 0
	s.delivery.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/getMe") {
			return supportWebhookResponseForTest(200, `{"ok":true,"result":{"id":77,"is_bot":true}}`), nil
		}
		require.True(t, strings.HasSuffix(req.URL.Path, "/getUserProfilePhotos"))
		profileCalls++
		return supportWebhookResponseForTest(200, `{"ok":true,"result":{"photos":[]}}`), nil
	})
	for range 2 {
		_, err := s.ChatAvatar(context.Background(), SupportTicketActor{UserID: 1, IsAdmin: true}, 42)
		require.ErrorIs(t, err, ErrCommunityNotFound)
	}
	require.Equal(t, 1, profileCalls)
	require.Equal(t, 1, repo.saves)
	require.Empty(t, repo.cached.Data)
}

func TestCommunityChatAvatarRejectsOversizedOrNonImageDownloadWithoutCaching(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "超过实际下载限制", body: strings.Repeat("a", 1024*1024+1)},
		{name: "伪装成头像的网页", body: "<html><script>alert(1)</script></html>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, base, _ := communityTestService(t)
			repo := &communityAvatarTestRepository{communityTestRepository: base}
			s.repo = repo
			s.delivery.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(req.URL.Path, "/getMe"):
					return supportWebhookResponseForTest(200, `{"ok":true,"result":{"id":77,"is_bot":true}}`), nil
				case strings.HasSuffix(req.URL.Path, "/getUserProfilePhotos"):
					return supportWebhookResponseForTest(200, `{"ok":true,"result":{"photos":[[{"file_id":"small"}]]}}`), nil
				case strings.HasSuffix(req.URL.Path, "/getFile"):
					return supportWebhookResponseForTest(200, `{"ok":true,"result":{"file_path":"photos/avatar.jpg","file_size":0}}`), nil
				default:
					// 不提供长度，验证下载端自身的流量上限与内容类型校验。
					return &http.Response{StatusCode: 200, ContentLength: -1, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				}
			})
			_, err := s.ChatAvatar(context.Background(), SupportTicketActor{UserID: 1, IsAdmin: true}, 42)
			require.Error(t, err)
			require.Zero(t, repo.saves)
			require.Nil(t, repo.cached)
		})
	}
}
