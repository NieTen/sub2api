//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAntigravityDetectionMappedAliasRequiresCompleteAnswer(t *testing.T) {
	t.Parallel()

	for _, finishReason := range []string{"STOP", "MAX_TOKENS", ""} {
		t.Run(finishReason, func(t *testing.T) {
			t.Parallel()

			account := &Account{
				ID: 1, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Concurrency: 1,
				Credentials: map[string]any{
					"access_token": "fixture-token",
					"project_id":   "fixture-project",
					"model_mapping": map[string]any{
						"custom-alias": "gemini-2.5-flash",
					},
				},
			}
			upstream := &modelDetectionTestUpstream{
				response: fmt.Sprintf(`data: {"response":{"candidates":[{"content":{"parts":[{"text":"thinking","thought":true},{"text":"answer"}]},"finishReason":%q}]}}`, finishReason),
			}
			svc := &AntigravityGatewayService{tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream}
			ctx := context.WithValue(context.Background(), accountModelDetectionContextKey{}, "benchmark question")

			result, err := svc.TestConnectionWithPrompt(ctx, account, "custom-alias", "regular probe prompt")
			require.Len(t, upstream.bodies, 1)
			body := upstream.bodies[0]
			require.Equal(t, "gemini-2.5-flash", gjson.Get(body, "model").String())
			require.Equal(t, "benchmark question", gjson.Get(body, "request.contents.0.parts.0.text").String())
			require.EqualValues(t, 4096, gjson.Get(body, "request.generationConfig.maxOutputTokens").Int())
			require.Equal(t, "VALIDATED", gjson.Get(body, "request.toolConfig.functionCallingConfig.mode").String())
			if finishReason == "STOP" {
				require.NoError(t, err)
				require.Equal(t, "answer", result.Text)
				require.Equal(t, "gemini-2.5-flash", result.MappedModel)
			} else {
				require.ErrorContains(t, err, "模型检测响应未正常结束")
				require.Nil(t, result)
			}
		})
	}
}

func TestAntigravityImageMappedAliasKeepsPromptAndInlineImage(t *testing.T) {
	t.Parallel()

	for _, prompt := range []string{"", "a cute cat"} {
		t.Run(prompt, func(t *testing.T) {
			t.Parallel()

			account := &Account{
				ID: 1, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Concurrency: 1,
				Credentials: map[string]any{
					"access_token": "fixture-token",
					"project_id":   "fixture-project",
					"model_mapping": map[string]any{
						"custom-image-alias": "gemini-3.1-flash-image",
					},
				},
			}
			upstream := &modelDetectionTestUpstream{
				response: `data: {"response":{"candidates":[{"content":{"parts":[{"text":"Here is your image"},{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}}]}}]}}`,
			}
			svc := &AntigravityGatewayService{tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream}

			result, err := svc.TestConnectionWithPrompt(context.Background(), account, "custom-image-alias", prompt)
			require.NoError(t, err)
			require.Len(t, upstream.bodies, 1)
			body := upstream.bodies[0]
			wantPrompt := prompt
			if wantPrompt == "" {
				wantPrompt = defaultGeminiImageTestPrompt
			}
			require.Equal(t, wantPrompt, gjson.Get(body, "request.contents.0.parts.0.text").String())
			require.Equal(t, "IMAGE", gjson.Get(body, "request.generationConfig.responseModalities.1").String())
			require.False(t, gjson.Get(body, "request.generationConfig.maxOutputTokens").Exists())
			require.Equal(t, "Here is your image", result.Text)
			require.Equal(t, "gemini-3.1-flash-image", result.MappedModel)
			require.Equal(t, []TestConnectionImage{{ImageURL: "data:image/png;base64,iVBORw0KGgo=", MimeType: "image/png"}}, result.Images)
		})
	}
}

func TestExtractContentFromSSEResponseDetectionExcludesThoughtImages(t *testing.T) {
	t.Parallel()

	body := []byte(`data: {"response":{"candidates":[{"content":{"parts":[{"text":"thinking","thought":true},{"thought":true,"inlineData":{"mimeType":"image/png","data":"aGlkZGVu"}},{"text":"answer"},{"inlineData":{"mimeType":"image/png","data":"dmlzaWJsZQ=="}}]}}]}}`)
	text, images := extractContentFromSSEResponse(body, true)
	require.Equal(t, "answer", text)
	require.Equal(t, []TestConnectionImage{{ImageURL: "data:image/png;base64,dmlzaWJsZQ==", MimeType: "image/png"}}, images)

	text, images = extractContentFromSSEResponse(body)
	require.Equal(t, "thinkinganswer", text)
	require.Len(t, images, 2)
}
