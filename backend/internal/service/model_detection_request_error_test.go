//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestModelDetectionSafeErrorKeepsClassification(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		cause   error
		message string
	}{
		{"原始截止时间", fmt.Errorf("fixture-secret: %w", context.DeadlineExceeded), context.DeadlineExceeded, modelDetectionTimeoutMessage},
		{"原始取消", fmt.Errorf("fixture-secret: %w", context.Canceled), context.Canceled, modelDetectionCanceledMessage},
		{"网络超时", &net.DNSError{Err: "fixture-secret", IsTimeout: true}, context.DeadlineExceeded, modelDetectionTimeoutMessage},
		{"响应头超时文本", errors.New("request failed: net/http: timeout awaiting response headers; fixture-secret"), context.DeadlineExceeded, modelDetectionTimeoutMessage},
		{"截止时间文本", errors.New("request failed: context deadline exceeded; fixture-secret"), context.DeadlineExceeded, modelDetectionTimeoutMessage},
		{"取消文本", errors.New("request failed: context canceled; fixture-secret"), context.Canceled, modelDetectionCanceledMessage},
		{"超限", ErrModelDetectionInvalid, ErrModelDetectionInvalid, modelDetectionOversizedMessage},
		{"未知错误", errors.New("Authorization: Bearer fixture-secret"), nil, modelDetectionUnknownErrorMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := safeModelDetectionRequestError(tc.err)
			require.Equal(t, tc.message, err.Error())
			require.NotContains(t, err.Error(), "fixture-secret")
			// 连通性服务经过 SSE 文本转发后仍能恢复固定分类，不会重新变成通用错误。
			forwarded := safeModelDetectionRequestError(errors.New(err.Error()))
			require.Equal(t, tc.message, forwarded.Error())
			if tc.cause != nil {
				require.ErrorIs(t, err, tc.cause)
				require.ErrorIs(t, forwarded, tc.cause)
			}
			wrapped := safeModelDetectionRequestError(fmt.Errorf("fixture-secret: %w", err))
			require.Equal(t, tc.message, wrapped.Error())
		})
	}
}

type modelDetectionRequestErrorUpstream struct {
	modelDetectionTestUpstream
	err error
}

func (u *modelDetectionRequestErrorUpstream) Do(req *http.Request, proxy string, accountID int64, limit int) (*http.Response, error) {
	if u.err != nil {
		return nil, u.err
	}
	return u.modelDetectionTestUpstream.Do(req, proxy, accountID, limit)
}

func (u *modelDetectionRequestErrorUpstream) DoWithTLS(req *http.Request, proxy string, accountID int64, limit int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, accountID, limit)
}

func TestModelDetectionWorkerKeepsSafeUpstreamDiagnostics(t *testing.T) {
	partial := "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"
	truncated := "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n"
	oversized := partial + "data: {\"choices\":[{\"delta\":{\"content\":\"" + strings.Repeat("x", modelDetectionOutputLimit) + "\"}}]}\n\n"
	for _, tc := range []struct {
		name       string
		httpStatus int
		body       string
		err        error
		want       string
		response   string
		status     string
	}{
		{name: "无权限", httpStatus: 401, body: "Authorization: Bearer fixture-secret", want: "HTTP 401", status: "error"},
		{name: "限流", httpStatus: 429, body: "Authorization: Bearer fixture-secret", want: "HTTP 429", status: "error"},
		{name: "上游不可用", httpStatus: 503, body: "Authorization: Bearer fixture-secret", want: "HTTP 503", status: "error"},
		{name: "未完整", body: partial + "data: [DONE]\n\n", want: modelDetectionIncompleteMessage, response: "partial", status: "error"},
		{name: "截断", body: truncated, want: modelDetectionTruncatedMessage, response: "partial", status: "error"},
		{name: "输出超限", body: oversized, want: modelDetectionOversizedMessage, response: "partial", status: "inconclusive"},
		{name: "传输独立超时", err: errors.New("net/http: timeout awaiting response headers; fixture-secret"), want: modelDetectionTimeoutMessage, status: "error"},
		{name: "传输取消", err: errors.New("context canceled; fixture-secret"), want: modelDetectionCanceledMessage, status: "error"},
		{name: "未知错误脱敏", err: errors.New("Authorization: Bearer fixture-secret"), want: modelDetectionUnknownErrorMessage, status: "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-key"}, Extra: map[string]any{"openai_responses_mode": "force_chat_completions"}}
			upstream := &modelDetectionRequestErrorUpstream{modelDetectionTestUpstream: modelDetectionTestUpstream{status: tc.httpStatus, response: tc.body}, err: tc.err}
			probe := &AccountTestService{accountRepo: &modelDetectionAccountRepo{account: account}, httpUpstream: upstream, cfg: &config.Config{}}
			repo := &detectionTestRepo{}
			s := NewModelDetectionService(repo, nil, probe)
			s.executeRun(context.Background(), &ModelDetectionRun{AccountID: 1, PlanID: 1, ModelID: "gpt-5.6-sol", Status: "running"})
			require.NotNil(t, repo.complete)
			require.Equal(t, tc.status, repo.complete.Status)
			require.Nil(t, repo.complete.Score)
			require.Empty(t, repo.complete.Verdict)
			require.Contains(t, repo.complete.ErrorMessage, tc.want)
			require.NotContains(t, repo.complete.ErrorMessage, "fixture-secret")
			require.Len(t, repo.complete.Details, 1)
			detail := repo.complete.Details[0]
			require.NotEmpty(t, detail.Prompt)
			require.Equal(t, "error", detail.Status)
			require.Equal(t, repo.complete.ErrorMessage, detail.ErrorMessage)
			require.Equal(t, tc.response, detail.Response)
		})
	}
}
