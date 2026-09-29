package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const PluginActionMaxBytes = 32768
const stateKitPluginKey = "io.github.wangyunjeff.sub2api-state-kit"

type PluginActionResult struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message"`
}

func pluginSupportsHostAdaptation(installation *PluginInstallation) bool {
	if installation == nil {
		return false
	}
	for _, capability := range installation.Manifest.Capabilities {
		if capability.ID == PluginCapabilityOpenAIOAuthOutbound && capability.Platform == PlatformOpenAI && capability.AccountType == AccountTypeOAuth {
			return true
		}
	}
	return false
}

func pluginHostAdaptationAllowed(installation *PluginInstallation) bool {
	return installation != nil && installation.HostAdaptationEnabled && pluginSupportsHostAdaptation(installation)
}

// 参考插件严格拒绝第二版握手；仅该插件的适配开关改变握手版本，原第二版字段仍然保留。
func pluginHostServiceVersion(installation *PluginInstallation) uint32 {
	if installation != nil && installation.HostAdaptationEnabled && installation.PluginKey == stateKitPluginKey {
		return 1
	}
	return pluginv1.HostServiceAPIVersion
}

func (s *pluginHostServiceServer) requirePluginHostAdaptation(ctx context.Context) error {
	if s == nil || s.adaptationAllowed == nil || !s.adaptationAllowed(ctx) {
		return status.Error(codes.PermissionDenied, "请先开启插件宿主适配")
	}
	return nil
}

// SetResourceDirectory 只在启动装配阶段注入，与账号目录共用授权范围。
func (m *PluginManager) SetResourceDirectory(directory PluginResourceDirectory) {
	m.mu.Lock()
	m.resourceDirectory = directory
	m.mu.Unlock()
}

func (m *PluginManager) HostAdaptationEnabled(ctx context.Context, id int64) (bool, error) {
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}
	return pluginHostAdaptationAllowed(installation), nil
}

func (m *PluginManager) SetHostAdaptation(ctx context.Context, id int64, enabled bool) (*PluginInstallation, error) {
	m.operationMu.Lock()
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		m.operationMu.Unlock()
		return nil, err
	}
	if !pluginSupportsHostAdaptation(installation) {
		m.operationMu.Unlock()
		return nil, errors.New("插件未声明宿主适配所需的账号能力")
	}
	changed := installation.HostAdaptationEnabled != enabled
	if changed {
		err = m.repo.UpdateHostAdaptation(ctx, id, enabled, installation.BinarySHA256, installation.HostAdaptationEnabled)
	}
	m.operationMu.Unlock()
	if err != nil {
		return nil, err
	}
	// 停用插件只保存开关；运行中的插件由现有协调流程重新协商宿主服务。
	if changed && hasEnabledOpenAIBinding(installation.Bindings) {
		if err := m.reconcileOnce(ctx); err != nil {
			result, getErr := m.Get(ctx, id)
			if getErr != nil {
				result = installation
				result.HostAdaptationEnabled = enabled
				result.Compatibility = EvaluatePluginCompatibility(result.Manifest, m.hostInfo)
			}
			result.RuntimeHealthy = false
			result.RuntimeMessage = "宿主适配设置已保存，插件重新连接失败，系统将自动重试"
			return result, nil
		}
	}
	result, err := m.Get(ctx, id)
	if err != nil && changed {
		installation.HostAdaptationEnabled = enabled
		installation.Compatibility = EvaluatePluginCompatibility(installation.Manifest, m.hostInfo)
		installation.RuntimeHealthy = false
		installation.RuntimeMessage = "宿主适配设置已保存，运行状态暂时无法读取，请刷新重试"
		return installation, nil
	}
	return result, err
}

func (m *PluginManager) ListResources(ctx context.Context, id int64) (*PluginResources, error) {
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !pluginHostAdaptationAllowed(installation) {
		return nil, errors.New("请先开启插件宿主适配")
	}
	m.mu.Lock()
	directory := m.resourceDirectory
	m.mu.Unlock()
	if directory == nil {
		return nil, errPluginResourcesUnavailable
	}
	// 仅枚举目录，允许停用插件配置资源，不启动进程也不请求上游。
	if adminDirectory, ok := directory.(PluginAdminResourceDirectory); ok {
		return adminDirectory.ListPluginAdminResources(ctx, pluginAccountScopeFromManifest(installation.Manifest))
	}
	return directory.ListPluginResources(ctx, pluginAccountScopeFromManifest(installation.Manifest))
}

func (m *PluginManager) RunAction(ctx context.Context, id int64, raw json.RawMessage) (*PluginActionResult, error) {
	if len(raw) == 0 || len(raw) > PluginActionMaxBytes || !json.Valid(raw) {
		return nil, errors.New("插件动作必须是有效且不超过 32768 字节的 JSON 对象")
	}
	var action map[string]json.RawMessage
	if json.Unmarshal(raw, &action) != nil || action == nil {
		return nil, errors.New("插件动作必须是 JSON 对象")
	}
	var requestID string
	if json.Unmarshal(action["request_id"], &requestID) != nil || strings.TrimSpace(requestID) == "" || len(requestID) > 128 {
		return nil, errors.New("插件动作 request_id 必须为 1～128 字节的字符串")
	}
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !pluginHostAdaptationAllowed(installation) {
		return nil, errors.New("请先开启插件宿主适配")
	}
	m.mu.Lock()
	runtime := m.runtimes[id]
	m.mu.Unlock()
	if !hasEnabledOpenAIBinding(installation.Bindings) || runtime == nil || runtime.api == nil || runtime.installation == nil ||
		runtime.installation.BinarySHA256 != installation.BinarySHA256 || !runtime.installation.HostAdaptationEnabled ||
		runtime.installation.ConfigEncrypted != installation.ConfigEncrypted || !runtime.beginRequest() {
		return nil, errors.New("插件未运行或正在重新连接，请启用插件并等待运行就绪")
	}
	defer runtime.finishRequest()
	actionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := runtime.api.RunAction(actionCtx, &pluginv1.RunActionRequest{ActionJson: raw})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, errors.New("当前插件版本不支持手动动作")
		}
		if status.Code(err) == codes.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("插件动作提交超时，请查询状态确认结果")
		}
		return nil, errors.New("插件动作提交失败，请检查插件运行状态")
	}
	if result == nil {
		return nil, errors.New("插件未返回动作结果")
	}
	return &PluginActionResult{Accepted: result.Accepted, Message: result.Message}, nil
}
