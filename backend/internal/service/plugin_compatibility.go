package service

import (
	"fmt"
	"strings"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"golang.org/x/mod/semver"
)

type PluginHostInfo struct {
	Version   string
	BuildType string
}

func EvaluatePluginCompatibility(manifest PluginManifest, host PluginHostInfo) PluginCompatibility {
	result := PluginCompatibility{
		CurrentSub2API:     host.Version,
		RequiredSub2API:    manifest.Requires.Sub2API,
		RecommendedSub2API: manifest.Requires.RecommendedSub2APIVersion,
		PluginProtocol:     manifest.Requires.PluginProtocol,
		TransportAPI:       manifest.Requires.TransportAPI,
		UIBridge:           manifest.Requires.UIBridge,
	}
	if manifest.Requires.PluginProtocol != pluginv1.ProtocolVersion ||
		manifest.Requires.TransportAPI != pluginv1.TransportAPIVersion ||
		manifest.Requires.UIBridge != pluginv1.UIBridgeVersion {
		result.Status = "incompatible"
		result.Message = "插件协议版本与当前 Sub2API 不兼容"
		return result
	}
	if !matchesSemverRange(host.Version, manifest.Requires.Sub2API) {
		result.Status = "incompatible"
		result.Message = fmt.Sprintf("当前 Sub2API %s 不满足插件要求 %s", host.Version, manifest.Requires.Sub2API)
		return result
	}
	result.Compatible = true
	// 插件清单仍使用严格的三段 SemVer；宿主的第四段是 fork 子版本，
	// 测试声明按母版本判断，避免每个子版本都要求插件重新声明一次。
	hostCompatibilityVersion := normalizeHostCompatibilitySemver(host.Version)
	if hostCompatibilityVersion != "" {
		for _, tested := range manifest.Requires.TestedSub2APIVersions {
			if normalizeSemver(tested) == hostCompatibilityVersion {
				result.Tested = true
				break
			}
		}
	}
	if result.Tested {
		result.Status = "compatible"
		result.Message = "当前 Sub2API 版本已由插件声明测试"
	} else {
		result.Status = "untested"
		result.Message = "版本范围兼容，但插件未声明已测试当前 Sub2API 版本"
	}
	return result
}

func normalizeSemver(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return ""
	}
	return v
}

// normalizeHostCompatibilitySemver 将宿主版本转换为插件兼容性使用的 SemVer。
// 宿主允许 A.B.C.D，其中 D 是 fork 子版本；插件只按母版本 A.B.C 做判断。
// 三段版本保持原义，五段及其他非数字段数一律视为非法。
func normalizeHostCompatibilitySemver(version string) string {
	raw := strings.TrimSpace(version)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "v") {
		raw = strings.TrimPrefix(raw, "v")
	}

	coreEnd := len(raw)
	if idx := strings.IndexAny(raw, "-+"); idx >= 0 {
		coreEnd = idx
	}
	core := raw[:coreEnd]
	parts := strings.Split(core, ".")
	if len(parts) != 3 && len(parts) != 4 {
		return ""
	}
	for _, part := range parts {
		if !isNumericVersionComponent(part) {
			return ""
		}
	}
	if len(parts) == 4 {
		parts = parts[:3]
	}

	normalized := "v" + strings.Join(parts, ".") + raw[coreEnd:]
	if !semver.IsValid(normalized) {
		return ""
	}
	return normalized
}

func isNumericVersionComponent(component string) bool {
	if component == "" || (len(component) > 1 && component[0] == '0') {
		return false
	}
	for _, char := range component {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func matchesSemverRange(version, expression string) bool {
	// 兼容性范围的左值是宿主版本，允许四段并折算到母版本；
	// 范围边界仍走严格 SemVer 校验，插件清单格式不会被放宽。
	v := normalizeHostCompatibilitySemver(version)
	if v == "" {
		return false
	}
	tokens := strings.Fields(strings.ReplaceAll(expression, ",", " "))
	if len(tokens) == 0 {
		return false
	}
	for _, token := range tokens {
		op := "="
		raw := token
		for _, candidate := range []string{">=", "<=", ">", "<", "="} {
			if strings.HasPrefix(token, candidate) {
				op = candidate
				raw = strings.TrimSpace(strings.TrimPrefix(token, candidate))
				break
			}
		}
		bound := normalizeSemver(raw)
		if bound == "" {
			return false
		}
		comparison := semver.Compare(v, bound)
		matched := map[string]bool{
			">=": comparison >= 0,
			"<=": comparison <= 0,
			">":  comparison > 0,
			"<":  comparison < 0,
			"=":  comparison == 0,
		}[op]
		if !matched {
			return false
		}
	}
	return true
}
