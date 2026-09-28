package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

type accountModelDetectionContextKey struct{}

// 检测题目只通过本轮上下文传递，普通连通性测试继续使用原来的请求内容。
func accountModelDetectionPrompt(ctx context.Context) string {
	prompt, _ := ctx.Value(accountModelDetectionContextKey{}).(string)
	return prompt
}

func isAccountModelDetection(c *gin.Context) bool {
	return c != nil && c.Request != nil && accountModelDetectionPrompt(c.Request.Context()) != ""
}

const modelDetectionOutputLimit = 256 * 1024

type modelDetectionCapture struct {
	buffer   bytes.Buffer
	header   http.Header
	cancel   context.CancelFunc
	overflow bool
}

func (w *modelDetectionCapture) Header() http.Header { return w.header }
func (w *modelDetectionCapture) WriteHeader(int)     {}
func (w *modelDetectionCapture) Flush()              {}
func (w *modelDetectionCapture) Write(p []byte) (int, error) {
	if w.buffer.Len()+len(p) > modelDetectionOutputLimit {
		w.overflow = true
		w.cancel()
		return 0, io.ErrShortBuffer
	}
	return w.buffer.Write(p)
}

// RunModelDetectionPrompt 复用账号原有鉴权、代理和协议链，只替换本轮文本题目。
func (s *AccountTestService) RunModelDetectionPrompt(ctx context.Context, accountID int64, modelID, prompt string) (string, error) {
	if s == nil || s.accountRepo == nil {
		return "", errors.New("账号检测服务尚未就绪")
	}
	modelID, prompt = strings.TrimSpace(modelID), strings.TrimSpace(prompt)
	if modelID == "" || len(modelID) > 200 || prompt == "" || len(prompt) > 32768 {
		return "", errors.New("检测模型或题目无效")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return "", errors.New("检测账号不存在或不可访问")
	}
	if account.IsSyntheticUITest() {
		return "", fmt.Errorf("%w：演示账号不能用于真实模型能力检测", ErrModelDetectionUnsupported)
	}
	if !modelDetectionTextModel(modelID) || !modelDetectionTextModel(account.GetMappedModel(modelID)) {
		return "", fmt.Errorf("%w：请选择对话或推理模型", ErrModelDetectionUnsupported)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = context.WithValue(ctx, accountModelDetectionContextKey{}, prompt)
	w := &modelDetectionCapture{header: make(http.Header), cancel: cancel}
	c, _ := gin.CreateTestContext(w)
	c.Request = (&http.Request{}).WithContext(ctx)
	err = s.TestAccountConnection(c, accountID, modelID, prompt, AccountTestModeDefault)
	// 无论成功与否都先提取已经收到的文本；不完整内容仅供排查，不交给评分器。
	text, eventError := parseTestSSEOutput(w.buffer.String())
	if w.overflow {
		return text, &modelDetectionRequestError{message: modelDetectionOversizedMessage, cause: ErrModelDetectionInvalid}
	}
	if ctx.Err() != nil {
		return text, safeModelDetectionRequestError(ctx.Err())
	}
	if err != nil || eventError != "" {
		if err == nil {
			err = errors.New(eventError)
		}
		return text, safeModelDetectionRequestError(err)
	}
	complete := false
	for _, line := range strings.Split(w.buffer.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event TestEvent
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) == nil && event.Type == "test_complete" && event.Success {
			complete = true
		}
	}
	if !complete || strings.TrimSpace(text) == "" || text == "(empty response)" {
		return text, &modelDetectionRequestError{message: modelDetectionIncompleteMessage}
	}
	return text, nil
}

func modelDetectionTextModel(model string) bool {
	model = strings.ToLower(model)
	for _, media := range []string{"image", "imagine", "video", "audio", "embedding", "rerank", "whisper", "realtime", "dall-e", "tts", "stt"} {
		if strings.Contains(model, media) {
			return false
		}
	}
	return true
}

var modelDetectionHTTPStatus = regexp.MustCompile(`(?i)(?:returned|status|http|api)\s*[:=]?\s*([45][0-9]{2})\b`)

const modelDetectionTimeoutMessage = "模型响应超时，本次结果不计入能力评分"
const modelDetectionCanceledMessage = "检测已取消，未自动重发请求"
const modelDetectionIncompleteMessage = "模型未返回完整文本，本次结果不计入能力评分"
const modelDetectionTruncatedMessage = "模型输出被截断或拦截，本次结果不计入能力评分"
const modelDetectionOversizedMessage = "模型输出超过检测限制，本次结果不计入能力评分"
const modelDetectionUnknownErrorMessage = "模型未返回完整有效响应，请检查该账号的模型配置与上游连接；本次结果不计入能力评分"

// 安全错误只携带固定诊断文本和分类标记，不保留可能包含凭据的上游原始错误。
type modelDetectionRequestError struct {
	message string
	cause   error
}

func (e *modelDetectionRequestError) Error() string { return e.message }
func (e *modelDetectionRequestError) Unwrap() error { return e.cause }

// 上游错误正文可能含请求头或密钥，检测历史只保存可操作的原因与状态码。
func safeModelDetectionRequestError(err error) error {
	var safe *modelDetectionRequestError
	if errors.As(err, &safe) {
		return safe
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &modelDetectionRequestError{message: modelDetectionTimeoutMessage, cause: context.DeadlineExceeded}
	}
	if errors.Is(err, context.Canceled) {
		return &modelDetectionRequestError{message: modelDetectionCanceledMessage, cause: context.Canceled}
	}
	if errors.Is(err, ErrModelDetectionInvalid) {
		return &modelDetectionRequestError{message: modelDetectionOversizedMessage, cause: ErrModelDetectionInvalid}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return &modelDetectionRequestError{message: modelDetectionTimeoutMessage, cause: context.DeadlineExceeded}
	}
	if match := modelDetectionHTTPStatus.FindStringSubmatch(err.Error()); len(match) > 1 {
		return &modelDetectionRequestError{message: fmt.Sprintf("模型请求失败（HTTP %s），请检查该账号的模型权限、凭据、额度或上游状态", match[1])}
	}
	// 原连通性服务通过 SSE 文本传递错误，只识别固定原因，绝不复制上游正文。
	message := err.Error()
	if strings.Contains(message, context.DeadlineExceeded.Error()) || strings.Contains(message, "timeout awaiting response headers") || strings.Contains(message, "i/o timeout") || strings.Contains(message, "Client.Timeout exceeded") {
		return &modelDetectionRequestError{message: modelDetectionTimeoutMessage, cause: context.DeadlineExceeded}
	}
	if strings.Contains(message, context.Canceled.Error()) {
		return &modelDetectionRequestError{message: modelDetectionCanceledMessage, cause: context.Canceled}
	}
	switch message {
	case modelDetectionTimeoutMessage:
		return &modelDetectionRequestError{message: modelDetectionTimeoutMessage, cause: context.DeadlineExceeded}
	case modelDetectionCanceledMessage:
		return &modelDetectionRequestError{message: modelDetectionCanceledMessage, cause: context.Canceled}
	case modelDetectionOversizedMessage:
		return &modelDetectionRequestError{message: modelDetectionOversizedMessage, cause: ErrModelDetectionInvalid}
	case modelDetectionIncompleteMessage, "模型检测响应在完成前中断", "模型检测响应缺少正常结束状态", "模型检测响应缺少正常结束原因", "模型检测响应未正常完成", "模型检测响应未正常结束", "Stream ended before response.completed", "Chat Completions stream from /v1/chat/completions ended before [DONE]":
		return &modelDetectionRequestError{message: modelDetectionIncompleteMessage}
	case modelDetectionTruncatedMessage, "模型检测输出被截断或拦截":
		return &modelDetectionRequestError{message: modelDetectionTruncatedMessage}
	case "账号检测服务尚未就绪", "检测模型或题目无效", "检测账号不存在或不可访问":
		return &modelDetectionRequestError{message: message}
	}
	return &modelDetectionRequestError{message: modelDetectionUnknownErrorMessage}
}

func applyAccountModelDetectionPrompt(ctx context.Context, payload map[string]any, protocol string) {
	prompt := accountModelDetectionPrompt(ctx)
	if prompt == "" {
		return
	}
	switch protocol {
	case "anthropic":
		payload["messages"] = []map[string]any{{"role": "user", "content": prompt}}
		payload["max_tokens"] = 4096
		delete(payload, "temperature")
	case "responses", "responses_oauth":
		payload["input"] = []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": prompt}}}}
		// OAuth 需要非空 instructions；检测不继承普通探针的编程代理身份和工具使用指南。
		payload["instructions"] = "Follow the user's task exactly. Return only the requested answer."
		if protocol == "responses" {
			payload["max_output_tokens"] = 4096
		}
	case "chat":
		payload["messages"] = []map[string]any{{"role": "user", "content": prompt}}
		model, _ := payload["model"].(string)
		if strings.HasPrefix(model, "gpt-5") || strings.HasPrefix(model, "gpt-6") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4") {
			payload["max_completion_tokens"] = 4096
		} else {
			payload["max_tokens"] = 4096
		}
	case "gemini":
		payload["contents"] = []map[string]any{{"role": "user", "parts": []map[string]any{{"text": prompt}}}}
		payload["generationConfig"] = map[string]any{"maxOutputTokens": 4096}
	}
}

func modelDetectionPayloadBytes(ctx context.Context, raw []byte, protocol string) []byte {
	if accountModelDetectionPrompt(ctx) == "" {
		return raw
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return raw
	}
	applyAccountModelDetectionPrompt(ctx, payload, protocol)
	result, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return result
}

// Antigravity 的测试使用完整 SSE 缓冲，质量检测必须看到正常结束原因才可评分。
func modelDetectionGeminiComplete(body []byte) bool {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var data map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &data) != nil {
			continue
		}
		if wrapped, ok := data["response"].(map[string]any); ok {
			data = wrapped
		}
		if candidates, ok := data["candidates"].([]any); ok {
			for _, entry := range candidates {
				candidate, _ := entry.(map[string]any)
				if reason, _ := candidate["finishReason"].(string); reason == "STOP" {
					return true
				}
			}
		}
	}
	return false
}
