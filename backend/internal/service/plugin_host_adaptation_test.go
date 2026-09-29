package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type hostAdaptationTestRepository struct {
	pluginTokenRepository
	updates     int
	updateError error
}

func (r *hostAdaptationTestRepository) UpdateHostAdaptation(_ context.Context, id int64, enabled bool, hash string, previous bool) error {
	r.updates++
	if r.updateError != nil {
		return r.updateError
	}
	if r.installation.ID != id || r.installation.BinarySHA256 != hash || r.installation.HostAdaptationEnabled != previous {
		return ErrPluginStateChanged
	}
	r.installation.HostAdaptationEnabled = enabled
	return nil
}

type hostAdaptationActionClient struct {
	pluginv1.TransportPluginClient
	calls    int
	raw      []byte
	err      error
	deadline time.Time
}

func (c *hostAdaptationActionClient) RunAction(ctx context.Context, request *pluginv1.RunActionRequest, _ ...grpc.CallOption) (*pluginv1.RunActionResponse, error) {
	c.calls++
	c.raw = append([]byte(nil), request.ActionJson...)
	c.deadline, _ = ctx.Deadline()
	return &pluginv1.RunActionResponse{Accepted: true, Message: "已提交"}, c.err
}

func hostAdaptationTestInstallation() *PluginInstallation {
	return &PluginInstallation{ID: 17, PluginKey: stateKitPluginKey, BinarySHA256: "verified-binary", HostAdaptationEnabled: true,
		Manifest: PluginManifest{Capabilities: []PluginCapability{{ID: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth}}},
		Bindings: []PluginBinding{{Capability: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth, Enabled: true}},
	}
}

func TestPluginHostAdaptationSwitchPreservesStoppedPlugin(t *testing.T) {
	installation := hostAdaptationTestInstallation()
	installation.HostAdaptationEnabled = false
	installation.Bindings[0].Enabled = false
	installation.State = PluginStateDisabled
	installation.ConfigEncrypted = "unchanged-config"
	repo := &hostAdaptationTestRepository{pluginTokenRepository: pluginTokenRepository{installation: installation}}
	manager := &PluginManager{repo: repo}
	result, err := manager.SetHostAdaptation(context.Background(), installation.ID, true)
	require.NoError(t, err)
	require.True(t, result.HostAdaptationEnabled)
	require.Equal(t, "unchanged-config", result.ConfigEncrypted)
	require.Equal(t, PluginStateDisabled, result.State)
	require.False(t, result.Bindings[0].Enabled)
	require.Empty(t, manager.runtimes)
	_, err = manager.SetHostAdaptation(context.Background(), installation.ID, true)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updates)
	repo.updateError = ErrPluginStateChanged
	_, err = manager.SetHostAdaptation(context.Background(), installation.ID, false)
	require.ErrorIs(t, err, ErrPluginStateChanged)
	require.True(t, installation.HostAdaptationEnabled)
}

func TestPluginHostAdaptationStoppedResourcesAndLiveRevocation(t *testing.T) {
	installation := hostAdaptationTestInstallation()
	installation.Bindings[0].Enabled = false
	directory, _, _, _ := pluginResourceTestDirectory()
	repo := &pluginTokenRepository{installation: installation}
	manager := &PluginManager{repo: repo, kvStore: newFakePluginKVStore(), resourceDirectory: directory}
	resources, err := manager.ListResources(context.Background(), installation.ID)
	require.NoError(t, err)
	require.Len(t, resources.Accounts, 1)
	require.Empty(t, manager.runtimes)
	// 使用与数据库记录独立的运行时快照，模拟另一副本关闭、换包或改变能力。
	snapshot := *installation
	server := manager.buildHostServices(&snapshot).(*pluginHostServiceServer)
	_, err = server.ListResources(context.Background(), &pluginv1.ListResourcesRequest{})
	require.NoError(t, err)
	installation.HostAdaptationEnabled = false
	_, err = server.ResolveProxy(context.Background(), &pluginv1.ResolveProxyRequest{ProxyId: 11})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = manager.ListResources(context.Background(), installation.ID)
	require.Error(t, err)
	installation.HostAdaptationEnabled = true
	installation.BinarySHA256 = "replacement"
	_, err = server.ListResources(context.Background(), &pluginv1.ListResourcesRequest{})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	installation.BinarySHA256 = snapshot.BinarySHA256
	installation.Manifest.Capabilities = nil
	_, err = server.ListResources(context.Background(), &pluginv1.ListResourcesRequest{})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestPluginHostAdaptationSavedSwitchSurvivesReconnectFailure(t *testing.T) {
	installation := hostAdaptationTestInstallation()
	installation.HostAdaptationEnabled = false
	repo := &hostAdaptationTestRepository{pluginTokenRepository: pluginTokenRepository{installation: installation, listErr: errors.New("数据库临时不可读")}}
	manager := &PluginManager{repo: repo}
	result, err := manager.SetHostAdaptation(context.Background(), installation.ID, true)
	require.NoError(t, err)
	require.True(t, result.HostAdaptationEnabled)
	require.False(t, result.RuntimeHealthy)
	require.Contains(t, result.RuntimeMessage, "已保存")
	require.Contains(t, result.RuntimeMessage, "自动重试")
	require.True(t, installation.HostAdaptationEnabled)
	require.True(t, installation.Bindings[0].Enabled)
}

func TestPluginHostActionValidationAndRuntimeBoundary(t *testing.T) {
	installation := hostAdaptationTestInstallation()
	snapshot := *installation
	client := &hostAdaptationActionClient{}
	runtime := &pluginRuntime{installation: &snapshot, api: client, done: make(chan struct{})}
	manager := &PluginManager{repo: &pluginTokenRepository{installation: installation}, runtimes: map[int64]*pluginRuntime{installation.ID: runtime}}
	for _, raw := range []string{"", "null", "[]", "{}", `{"request_id":42}`, `{"request_id":" "}`, `{"request_id":"x"} {}`, `{"request_id":"` + strings.Repeat("x", 129) + `"}`, `{"request_id":"x","payload":"` + strings.Repeat("x", PluginActionMaxBytes) + `"}`} {
		_, err := manager.RunAction(context.Background(), installation.ID, json.RawMessage(raw))
		require.Error(t, err)
	}
	require.Zero(t, client.calls)
	raw := json.RawMessage(`{"kind":"test","request_id":"original-id","account_id":5939067819}`)
	result, err := manager.RunAction(context.Background(), installation.ID, raw)
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.Equal(t, []byte(raw), client.raw)
	require.WithinDuration(t, time.Now().Add(10*time.Second), client.deadline, time.Second)
	require.Zero(t, runtime.inFlight.Load())
	for _, reject := range []func(){
		func() { installation.HostAdaptationEnabled = false },
		func() { installation.Bindings[0].Enabled = false },
		func() { snapshot.HostAdaptationEnabled = false },
		func() { snapshot.BinarySHA256 = "stale" },
		func() { snapshot.ConfigEncrypted = "stale" },
		func() { runtime.draining.Store(true) },
	} {
		reject()
		_, err = manager.RunAction(context.Background(), installation.ID, raw)
		require.Error(t, err)
		require.Equal(t, 1, client.calls)
		installation.HostAdaptationEnabled = true
		installation.Bindings[0].Enabled = true
		snapshot = *installation
		runtime.draining.Store(false)
	}
	delete(manager.runtimes, installation.ID)
	_, err = manager.RunAction(context.Background(), installation.ID, raw)
	require.Error(t, err)
	require.Empty(t, manager.runtimes)
}

func TestPluginHostActionErrorsDoNotExposeRuntimeCredentials(t *testing.T) {
	installation := hostAdaptationTestInstallation()
	client := &hostAdaptationActionClient{}
	runtime := &pluginRuntime{installation: installation, api: client, done: make(chan struct{})}
	manager := &PluginManager{repo: &pluginTokenRepository{installation: installation}, runtimes: map[int64]*pluginRuntime{installation.ID: runtime}}
	for _, failure := range []error{errors.New("proxy://USER:SECRET@example"), status.Error(codes.Unimplemented, "SECRET"), context.DeadlineExceeded} {
		client.err = failure
		_, err := manager.RunAction(context.Background(), installation.ID, json.RawMessage(`{"request_id":"r"}`))
		require.Error(t, err)
		require.NotContains(t, err.Error(), "SECRET")
		require.Zero(t, runtime.inFlight.Load())
	}
}
