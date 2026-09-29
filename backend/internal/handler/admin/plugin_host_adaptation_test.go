package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type pluginAdaptationHandlerRepository struct {
	service.PluginRepository
	installation *service.PluginInstallation
	reads        int
	updates      int
	failRead     int
}

func (r *pluginAdaptationHandlerRepository) GetByID(_ context.Context, id int64) (*service.PluginInstallation, error) {
	r.reads++
	if r.reads == r.failRead {
		return nil, errors.New("测试数据库暂时不可用")
	}
	if id != r.installation.ID {
		return nil, errors.New("测试插件不存在")
	}
	copy := *r.installation
	return &copy, nil
}

func (r *pluginAdaptationHandlerRepository) UpdateHostAdaptation(_ context.Context, id int64, enabled bool, digest string, expected bool) error {
	if id != r.installation.ID || digest != r.installation.BinarySHA256 || expected != r.installation.HostAdaptationEnabled {
		return errors.New("测试安装记录已经变化")
	}
	r.updates++
	r.installation.HostAdaptationEnabled = enabled
	return nil
}

type pluginAdaptationTestEncryptor struct{}

func (pluginAdaptationTestEncryptor) Encrypt(value string) (string, error) { return value, nil }
func (pluginAdaptationTestEncryptor) Decrypt(value string) (string, error) { return value, nil }

type pluginAdaptationHandlerDirectory struct{}

func (pluginAdaptationHandlerDirectory) ListPluginResources(_ context.Context, scope service.PluginAccountScope) (*service.PluginResources, error) {
	if !scope.Contains("openai", "oauth") || scope.Contains("anthropic", "oauth") {
		return nil, errors.New("测试账号范围错误")
	}
	return &service.PluginResources{
		Accounts: []service.PluginResourceAccount{{ID: 5939067819, Name: "测试账号", GroupIDs: []int64{9}}},
		Groups:   []service.PluginResourceGroup{{ID: 9, Name: "测试分组"}},
		Proxies:  []service.PluginResourceProxy{{ID: 3, Name: "测试代理", Protocol: "http", Host: "proxy.invalid", Port: 8080}},
	}, nil
}

func (pluginAdaptationHandlerDirectory) ResolvePluginProxy(context.Context, service.PluginAccountScope, int64) (string, error) {
	return "", errors.New("被动页面不应调用代理认证解析")
}

func newPluginAdaptationHandlerFixture(t *testing.T) (*PluginHandler, *pluginAdaptationHandlerRepository, string) {
	t.Helper()
	root := t.TempDir()
	installation := &service.PluginInstallation{
		ID: 17, PluginKey: "io.github.wangyunjeff.sub2api-state-kit", State: service.PluginStateDisabled,
		Manifest: service.PluginManifest{Capabilities: []service.PluginCapability{{
			ID: service.PluginCapabilityOpenAIOAuthOutbound, Platform: "openai", AccountType: "oauth",
		}}},
	}
	repository := &pluginAdaptationHandlerRepository{installation: installation}
	cfg := &config.Config{}
	cfg.Plugins.DataDir = root
	manager := service.NewPluginManager(repository, pluginAdaptationTestEncryptor{}, cfg, service.PluginHostInfo{}, nil)
	manager.SetResourceDirectory(pluginAdaptationHandlerDirectory{})
	return NewPluginHandler(manager), repository, root
}

func pluginAdaptationHandlerRequest(t *testing.T, method, path, body string, handle gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, "/plugins/:id", handle)
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestPluginHostAdaptationHandlerRequiresExplicitBoolean(t *testing.T) {
	handler, repository, _ := newPluginAdaptationHandlerFixture(t)
	for _, body := range []string{"", `{}`, `null`, `[]`, `{"enabled":null}`, `{"enabled":"true"}`, `{"enabled":1}`, `{"enabled":true} {}`, `{"enabled":true,"unknown":false}`, strings.Repeat(" ", 1025) + `{"enabled":true}`} {
		t.Run(body[:min(len(body), 50)], func(t *testing.T) {
			response := pluginAdaptationHandlerRequest(t, http.MethodPut, "/plugins/17", body, handler.SetHostAdaptation)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}
	require.Zero(t, repository.reads)
	require.Zero(t, repository.updates)
	for _, enabled := range []bool{true, false} {
		body, err := json.Marshal(map[string]bool{"enabled": enabled})
		require.NoError(t, err)
		response := pluginAdaptationHandlerRequest(t, http.MethodPut, "/plugins/17", string(body), handler.SetHostAdaptation)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Equal(t, enabled, repository.installation.HostAdaptationEnabled)
		require.Contains(t, response.Body.String(), `"host_adaptation_enabled":`+map[bool]string{true: "true", false: "false"}[enabled])
	}
	require.Equal(t, 2, repository.updates)
}

func TestPluginActionHandlerBoundsAndRejectsMalformedJSON(t *testing.T) {
	handler, repository, _ := newPluginAdaptationHandlerFixture(t)
	for _, body := range []string{"", `null`, `[]`, `true`, `{"request_id":"one"} {}`, `{}`, `{"request_id":7}`, strings.Repeat("x", service.PluginActionMaxBytes+1)} {
		response := pluginAdaptationHandlerRequest(t, http.MethodPost, "/plugins/17", body, handler.RunAction)
		require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	}
	require.Zero(t, repository.reads, "格式错误不能触达插件或数据库")
	response := pluginAdaptationHandlerRequest(t, http.MethodPost, "/plugins/17", `{"request_id":"valid-id","kind":"test"}`, handler.RunAction)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "开启插件宿主适配")
}

func TestPluginResourcesHandlerWorksWhilePluginStopped(t *testing.T) {
	handler, repository, _ := newPluginAdaptationHandlerFixture(t)
	repository.installation.HostAdaptationEnabled = true
	response := pluginAdaptationHandlerRequest(t, http.MethodGet, "/plugins/17", "", handler.Resources)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"group_ids":[9]`)
	require.Contains(t, response.Body.String(), `"id":5939067819`)
	require.NotContains(t, response.Body.String(), "password")
	require.Equal(t, service.PluginStateDisabled, repository.installation.State)
	require.Zero(t, repository.updates)

	repository.installation.HostAdaptationEnabled = false
	response = pluginAdaptationHandlerRequest(t, http.MethodGet, "/plugins/17", "", handler.Resources)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.NotContains(t, response.Body.String(), "测试账号")
}

func TestPluginHostAdaptationHandlersRejectInvalidID(t *testing.T) {
	handler := NewPluginHandler(nil)
	for _, endpoint := range []struct {
		method string
		handle gin.HandlerFunc
	}{
		{http.MethodPut, handler.SetHostAdaptation},
		{http.MethodGet, handler.Resources},
		{http.MethodPost, handler.RunAction},
	} {
		for _, id := range []string{"invalid", "0", "-1", "9223372036854775808"} {
			response := pluginAdaptationHandlerRequest(t, endpoint.method, "/plugins/"+id, `{"enabled":true}`, endpoint.handle)
			require.Equal(t, http.StatusBadRequest, response.Code)
		}
	}
}

func TestPluginUIAssetPreviewCSPFollowsCurrentAdaptation(t *testing.T) {
	handler, repository, root := newPluginAdaptationHandlerFixture(t)
	installation := repository.installation
	installation.InstallPath = filepath.Join(root, "fixture")
	require.NoError(t, os.MkdirAll(filepath.Join(installation.InstallPath, "ui"), 0o700))
	// 仅创建用于哈希校验的惰性字节文件，绝不启动插件进程。
	binary := []byte("不可执行的测试占位文件")
	html := []byte("<!doctype html><title>测试插件配置页</title>")
	require.NoError(t, os.WriteFile(filepath.Join(installation.InstallPath, "fixture.bin"), binary, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(installation.InstallPath, "ui", "index.html"), html, 0o600))
	digest := sha256.Sum256(binary)
	installation.BinarySHA256 = hex.EncodeToString(digest[:])
	digest = sha256.Sum256(html)
	installation.Manifest.Files = map[string]string{"ui/index.html": hex.EncodeToString(digest[:])}
	installation.Manifest.UI.Entrypoint = "ui/index.html"
	installation.Manifest.Runtimes = map[string]service.PluginRuntime{installation.Manifest.RuntimeKey(): {Path: "fixture.bin"}}
	token, _, err := handler.manager.CreateUIAssetToken(context.Background(), installation.ID, time.Minute)
	require.NoError(t, err)
	router := gin.New()
	router.GET("/plugin-ui/:token/*path", handler.ServeUIAsset)
	requestAsset := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/plugin-ui/"+token+"/index.html", nil))
		return response
	}
	for _, enabled := range []bool{false, true, false} {
		installation.HostAdaptationEnabled = enabled
		response := requestAsset()
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, string(html), response.Body.String())
		policy := response.Header().Get("Content-Security-Policy")
		require.Equal(t, enabled, strings.Contains(policy, "frame-src 'self' about:"))
		for _, restriction := range []string{"default-src 'none'", "connect-src 'none'", "form-action 'none'", "base-uri 'none'", "frame-ancestors 'self'", "navigate-to 'none'"} {
			require.Contains(t, policy, restriction)
		}
		require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
		require.Equal(t, "no-referrer", response.Header().Get("Referrer-Policy"))
		require.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
		require.Equal(t, "SAMEORIGIN", response.Header().Get("X-Frame-Options"))
		require.Equal(t, "cross-origin", response.Header().Get("Cross-Origin-Resource-Policy"))
	}
	installation.HostAdaptationEnabled = true
	installation.Manifest.Capabilities = nil
	response := requestAsset()
	require.Equal(t, http.StatusOK, response.Code)
	require.NotContains(t, response.Header().Get("Content-Security-Policy"), "frame-src")

	// 第一次查询用于读取静态资源，第二次查询检查最新开关；后者失败时不能发送页面。
	repository.failRead = repository.reads + 2
	response = requestAsset()
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Empty(t, response.Body.String())
	require.NotContains(t, response.Header().Get("Content-Security-Policy"), "frame-src")
}
