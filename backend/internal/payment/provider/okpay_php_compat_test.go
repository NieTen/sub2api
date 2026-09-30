package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type okpayPHPRequestFixture struct {
	Name              string          `json:"name"`
	ID                string          `json:"id"`
	Token             string          `json:"token"`
	Fields            json.RawMessage `json:"fields"`
	ExpectedSignature string          `json:"expected_signature"`
	PHPBody           string          `json:"php_body"`
}

type okpayPHPRequestCapture struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

type okpayPHPReferenceResult struct {
	Name             string `json:"name"`
	Signature        string `json:"signature"`
	Body             string `json:"body"`
	AcceptsGoRequest bool   `json:"accepts_go_request"`
}

func TestOKPayRequestsMatchIndependentPHPFixtures(t *testing.T) {
	fixturePath := filepath.Join("testdata", "okpay_php_requests.json")
	raw, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	var fixtureFile struct {
		Cases []okpayPHPRequestFixture `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixtureFile))
	require.NotEmpty(t, fixtureFile.Cases)
	captures := make([]okpayPHPRequestCapture, 0, len(fixtureFile.Cases))
	for _, fixture := range fixtureFile.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			var body string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				data, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				body = string(data)
				require.Equal(t, http.MethodPost, request.Method)
				require.Equal(t, "application/x-www-form-urlencoded", request.Header.Get("Content-Type"))
				fmt.Fprint(w, `{"status":"success","code":10000,"data":{"fixture":true}}`)
			}))
			defer server.Close()
			provider, err := NewOKPay("php-compat", map[string]string{"id": fixture.ID, "token": fixture.Token, "apiBase": server.URL, "signatureAlgorithm": OKPaySignatureLegacyMD5})
			require.NoError(t, err)
			provider.httpClient.Transport = server.Client().Transport
			fields, err := okpayDecodeJSON(string(fixture.Fields))
			require.NoError(t, err)
			_, err = provider.post(context.Background(), "/payLink", fields)
			require.NoError(t, err)
			received, err := url.ParseQuery(body)
			require.NoError(t, err)
			expected, err := url.ParseQuery(fixture.PHPBody)
			require.NoError(t, err)
			// 黄金值由附件 PHP 原算法独立生成，不能用当前 Go 签名实现重算期望值。
			require.Equal(t, fixture.ExpectedSignature, received.Get("sign"))
			require.Equal(t, expected, received)
			require.NotContains(t, received, "token")
			captures = append(captures, okpayPHPRequestCapture{Name: fixture.Name, Body: body})
			// 诊断专用 PHP 模式需要与原附件逐字节一致，普通支付仍保留当前传输方式。
			_, err = provider.postWithTransport(context.Background(), "/payLink", fields, okpayTransportPHPReference)
			require.NoError(t, err)
			require.Equal(t, fixture.PHPBody, body)
		})
	}
	t.Run("可选PHP原算法差分", func(t *testing.T) {
		php, err := exec.LookPath("php")
		if err != nil {
			t.Skip("本机没有 PHP，已执行固定黄金值校验；跳过可选运行时差分")
		}
		require.Len(t, captures, len(fixtureFile.Cases))
		input, err := json.Marshal(captures)
		require.NoError(t, err)
		// 只在本地运行签名算法，不执行附件的网络请求，也不要求生产环境安装 PHP。
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, php, "-n", filepath.Join("testdata", "okpay_php_reference.php"), fixturePath)
		command.Stdin = bytes.NewReader(input)
		output, err := command.Output()
		require.NoError(t, err)
		var results []okpayPHPReferenceResult
		require.NoError(t, json.Unmarshal(output, &results))
		require.Len(t, results, len(fixtureFile.Cases))
		for index, result := range results {
			fixture := fixtureFile.Cases[index]
			require.Equal(t, fixture.Name, result.Name)
			require.Equal(t, fixture.ExpectedSignature, result.Signature, fixture.Name)
			require.Equal(t, fixture.PHPBody, result.Body, fixture.Name)
			require.True(t, result.AcceptsGoRequest, fixture.Name)
		}
	})
}
