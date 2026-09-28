//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type modelDetectionAccountRepo struct {
	AccountRepository
	account  *Account
	setError bool
}

func (r *modelDetectionAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *modelDetectionAccountRepo) SetError(context.Context, int64, string) error {
	r.setError = true
	return nil
}

type modelDetectionTestUpstream struct {
	response string
	status   int
	requests []*http.Request
	bodies   []string
}

func (u *modelDetectionTestUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.requests = append(u.requests, req)
	u.bodies = append(u.bodies, string(body))
	status := u.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(u.response))}, nil
}
func (u *modelDetectionTestUpstream) DoWithTLS(req *http.Request, proxy string, id int64, limit int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, limit)
}

func TestModelDetectionPromptUsesRealSelectedAccountPayload(t *testing.T) {
	claude := "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"answer\"}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
	responses := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"answer\"}\n\ndata: {\"type\":\"response.completed\"}\n\n"
	chat := "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	for _, test := range []struct{ name, platform, model, protocol, stream, path string }{
		{"OpenAI Responses", PlatformOpenAI, "gpt-5.4", "responses", responses, "input.0.content.0.text"},
		{"OpenAI Chat", PlatformOpenAI, "gpt-5.4", "chat_completions", chat, "messages.0.content"},
		{"Claude", PlatformAnthropic, "claude-sonnet-4-6", "", claude, "messages.0.content"},
		{"Gemini", PlatformGemini, "gemini-2.5-flash", "", "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"answer\"}]},\"finishReason\":\"STOP\"}]}\n\n", "contents.0.parts.0.text"},
		{"CN adaptive只调用一次", PlatformDeepseek, "deepseek-chat", "adaptive", chat, "messages.0.content"},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := &Account{ID: 1, Platform: test.platform, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://upstream.example"}, Extra: map[string]any{}}
			if test.protocol != "" {
				account.Extra["api_protocol"] = test.protocol
				if test.protocol == "chat_completions" {
					account.Extra["openai_responses_mode"] = "force_chat_completions"
				}
			}
			repo := &modelDetectionAccountRepo{account: account}
			upstream := &modelDetectionTestUpstream{response: test.stream}
			s := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: &config.Config{}}
			text, err := s.RunModelDetectionPrompt(context.Background(), 1, test.model, "this is the full benchmark question")
			require.NoError(t, err)
			require.Equal(t, "answer", text)
			require.Len(t, upstream.bodies, 1)
			require.Equal(t, "this is the full benchmark question", gjson.Get(upstream.bodies[0], test.path).String())
			require.False(t, repo.setError)
		})
	}
}

func TestModelDetectionRejectsTruncatedStreamAndRedactsErrors(t *testing.T) {
	for _, sample := range []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: [DONE]\n\n",
	} {
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-key"}, Extra: map[string]any{"openai_responses_mode": "force_chat_completions"}}
		upstream := &modelDetectionTestUpstream{response: sample}
		s := &AccountTestService{accountRepo: &modelDetectionAccountRepo{account: account}, httpUpstream: upstream, cfg: &config.Config{}}
		text, err := s.RunModelDetectionPrompt(context.Background(), 1, "gpt-5.4", "question")
		require.Error(t, err)
		require.Empty(t, text)
	}
	err := safeModelDetectionRequestError(io.ErrUnexpectedEOF)
	require.NotContains(t, err.Error(), io.ErrUnexpectedEOF.Error())
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-key"}, Extra: map[string]any{"openai_responses_mode": "force_chat_completions"}}
	repo := &modelDetectionAccountRepo{account: account}
	s := &AccountTestService{accountRepo: repo, httpUpstream: &modelDetectionTestUpstream{status: 401, response: "Authorization: Bearer highly-secret-token"}, cfg: &config.Config{}}
	_, err = s.RunModelDetectionPrompt(context.Background(), 1, "gpt-5.4", "question")
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
	require.NotContains(t, err.Error(), "highly-secret-token")
	require.False(t, repo.setError, "质量检测不能自动停用账号")
}

func TestModelDetectionStrictCompletionDoesNotChangeRegularProbes(t *testing.T) {
	for _, kind := range []string{"Claude", "Gemini"} {
		for _, strict := range []bool{true, false} {
			t.Run(kind+map[bool]string{true: "检测", false: "连通性"}[strict], func(t *testing.T) {
				c, _ := newTestContext()
				if strict {
					c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), accountModelDetectionContextKey{}, "question"))
				}
				s := &AccountTestService{}
				var err error
				if kind == "Claude" {
					err = s.processClaudeStream(c, strings.NewReader("data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"partial\"}}\n\n"))
				} else {
					err = s.processGeminiStream(c, strings.NewReader("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}\n\n"))
				}
				if strict {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}

func TestModelDetectionCaptureHasBoundedMemory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &modelDetectionCapture{header: make(http.Header), cancel: cancel}
	_, err := w.Write([]byte(strings.Repeat("x", modelDetectionOutputLimit+1)))
	require.Error(t, err)
	require.True(t, w.overflow)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Zero(t, w.buffer.Len())
}

func TestModelDetectionStrictClaudeNeedsNormalStopReason(t *testing.T) {
	for _, suffix := range []string{"data: [DONE]\n\n", "data: {\"type\":\"message_stop\"}\n\n", "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"}}\n\n"} {
		c, _ := newTestContext()
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), accountModelDetectionContextKey{}, "question"))
		err := (&AccountTestService{}).processClaudeStream(c, strings.NewReader("data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"partial\"}}\n\n"+suffix))
		require.Error(t, err)
	}
}

func TestModelDetectionAntigravityExcludesThoughtsOnlyForDetection(t *testing.T) {
	body := []byte(`data: {"response":{"candidates":[{"content":{"parts":[{"text":"thinking", "thought":true},{"text":"answer"}]},"finishReason":"STOP"}]}}`)
	require.Equal(t, "answer", extractTextFromSSEResponse(body, true))
	require.Equal(t, "thinkinganswer", extractTextFromSSEResponse(body))
}

func TestModelDetectionAntigravityDoesNotRetryOrChangeScheduling(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "fixture-token", "project_id": "fixture-project", "model_mapping": map[string]any{"gemini-2.5-flash": "gemini-2.5-flash"}}}
	ctx := context.WithValue(context.Background(), accountModelDetectionContextKey{}, "benchmark question")
	upstream := &modelDetectionTestUpstream{status: http.StatusServiceUnavailable, response: `{"error":{"message":"fixture failure"}}`}
	s := &AntigravityGatewayService{tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream, accountRepo: &modelDetectionAccountRepo{account: account}}
	_, err := s.TestConnection(ctx, account, "gemini-2.5-flash")
	require.Error(t, err)
	require.Len(t, upstream.bodies, 1)
	require.Contains(t, upstream.bodies[0], "benchmark question")
	// 未实现的状态写入会触发 mock panic，确保检测错误没有走生产调度惩罚路径。
	probe := &AccountTestService{accountRepo: &modelDetectionAccountRepo{account: account}}
	probe.observeGrokTestResponse(ctx, account, &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("invalid key"))})
	probe.reconcileOpenAI429State(ctx, account, http.Header{}, []byte(`{"error":{"type":"usage_limit_reached","resets_at":1893456000}}`))
}

func TestModelDetectionAntigravityCarriesPromptAndOutputBudget(t *testing.T) {
	s := &AntigravityGatewayService{}
	for _, kind := range []string{"gemini", "claude"} {
		var raw []byte
		var err error
		if kind == "gemini" {
			raw, err = s.buildGeminiTestRequest("fixture-project", "gemini-2.5-flash", "benchmark question")
		} else {
			raw, err = s.buildClaudeTestRequest("fixture-project", "claude-sonnet-4-6", "benchmark question")
		}
		require.NoError(t, err)
		require.Contains(t, string(raw), "benchmark question")
		require.Contains(t, string(raw), "4096")
		var payload map[string]any
		require.NoError(t, json.Unmarshal(raw, &payload))
	}
	require.True(t, modelDetectionGeminiComplete([]byte("data: {\"response\":{\"candidates\":[{\"finishReason\":\"STOP\"}]}}\n\n")))
	require.False(t, modelDetectionGeminiComplete([]byte("data: {\"response\":{\"candidates\":[{\"finishReason\":\"MAX_TOKENS\"}]}}\n\n")))
}
