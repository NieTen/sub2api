package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errPluginResourcesUnavailable = errors.New("宿主资源目录暂时不可用")
var errPluginProxyUnavailable = errors.New("所选代理暂时不可用")

// PluginResourceDirectory 只提供受授权范围约束的资源摘要，认证代理地址仅交给插件后端。
type PluginResourceDirectory interface {
	ListPluginResources(context.Context, PluginAccountScope) (*PluginResources, error)
	ResolvePluginProxy(context.Context, PluginAccountScope, int64) (string, error)
}

// PluginAdminResourceDirectory 供管理员提前配置尚未启用的账号，只输出无凭据的选择器摘要。
// 该接口不注册到 HostService，不能借此扩大插件后端的原生账号访问范围。
type PluginAdminResourceDirectory interface {
	ListPluginAdminResources(context.Context, PluginAccountScope) (*PluginResources, error)
}

// PluginResources 是可交给管理页面和插件页面的白名单视图，不携带原始账号或代理对象。
type PluginResources struct {
	Accounts []PluginResourceAccount `json:"accounts"`
	Groups   []PluginResourceGroup   `json:"groups"`
	Proxies  []PluginResourceProxy   `json:"proxies"`
}

type PluginResourceAccount struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	GroupIDs []int64 `json:"group_ids"`
}

type PluginResourceGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type PluginResourceProxy struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
}

type pluginResourceDirectory struct {
	base     PluginAccountDirectory
	accounts AccountRepository
	proxies  ProxyRepository
	groups   GroupRepository
}

type pluginResourceAccountMetadata struct {
	GroupIDs []int64 `json:"GroupIDs"`
}

func NewPluginResourceDirectory(base PluginAccountDirectory, accounts AccountRepository, proxies ProxyRepository, groups GroupRepository) PluginResourceDirectory {
	return &pluginResourceDirectory{base: base, accounts: accounts, proxies: proxies, groups: groups}
}

func (d *pluginResourceDirectory) ListPluginResources(ctx context.Context, scope PluginAccountScope) (*PluginResources, error) {
	if d == nil || d.base == nil || scope.Empty() {
		return nil, errPluginResourcesUnavailable
	}
	accounts, err := d.base.ListPluginAccounts(ctx, scope, "", "")
	if err != nil {
		return nil, errPluginResourcesUnavailable
	}
	return d.resourceSummaries(ctx, scope, accounts, false)
}

func (d *pluginResourceDirectory) ListPluginAdminResources(ctx context.Context, scope PluginAccountScope) (*PluginResources, error) {
	if d == nil || d.accounts == nil || scope.Empty() {
		return nil, errPluginResourcesUnavailable
	}
	infos := make([]PluginAccountInfo, 0)
	for _, platform := range scope.Platforms() {
		accounts, err := d.accounts.ListAllWithFilters(ctx, platform, "", "", "", 0, "")
		if err != nil {
			return nil, errPluginResourcesUnavailable
		}
		for _, account := range accounts {
			if account.ID <= 0 || account.IsShadow() || account.Platform != platform || !scope.Contains(account.Platform, account.Type) {
				continue
			}
			// 只构造分组 ID 元数据，不能使用包含 Credentials、Extra、Proxy 的原始对象快照。
			metadata, err := json.Marshal(pluginResourceAccountMetadata{GroupIDs: account.GroupIDs})
			if err != nil {
				return nil, errPluginResourcesUnavailable
			}
			infos = append(infos, PluginAccountInfo{
				ID: account.ID, Name: account.Name, Platform: account.Platform, AccountType: account.Type,
				Status: account.Status, MetadataJSON: metadata,
			})
		}
	}
	return d.resourceSummaries(ctx, scope, infos, true)
}

func (d *pluginResourceDirectory) resourceSummaries(ctx context.Context, scope PluginAccountScope, accounts []PluginAccountInfo, includeInactiveAccounts bool) (*PluginResources, error) {
	if d == nil || d.proxies == nil || d.groups == nil || scope.Empty() {
		return nil, errPluginResourcesUnavailable
	}
	out := &PluginResources{
		Accounts: make([]PluginResourceAccount, 0, len(accounts)),
		Groups:   make([]PluginResourceGroup, 0),
		Proxies:  make([]PluginResourceProxy, 0),
	}
	seenAccounts := make(map[int64]struct{}, len(accounts))
	referencedGroups := make(map[int64]struct{})
	for _, account := range accounts {
		if account.ID <= 0 || account.IsShadow || (!includeInactiveAccounts && account.Status != StatusActive) || !scope.Contains(account.Platform, account.AccountType) {
			continue
		}
		if _, exists := seenAccounts[account.ID]; exists {
			continue
		}
		seenAccounts[account.ID] = struct{}{}
		// 只解码分组 ID，绝不能将包含 Extra、代理认证信息的完整元数据传给页面。
		metadata := pluginResourceAccountMetadata{}
		if len(account.MetadataJSON) > 0 {
			if err := json.Unmarshal(account.MetadataJSON, &metadata); err != nil {
				return nil, errPluginResourcesUnavailable
			}
		}
		groupIDs := make([]int64, 0, len(metadata.GroupIDs))
		seenGroups := make(map[int64]struct{}, len(metadata.GroupIDs))
		for _, id := range metadata.GroupIDs {
			if id <= 0 {
				continue
			}
			if _, exists := seenGroups[id]; !exists {
				groupIDs = append(groupIDs, id)
				seenGroups[id] = struct{}{}
				referencedGroups[id] = struct{}{}
			}
		}
		sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
		out.Accounts = append(out.Accounts, PluginResourceAccount{ID: account.ID, Name: account.Name, GroupIDs: groupIDs})
	}
	seenGroups := make(map[int64]struct{})
	allowedPlatforms := make(map[string]struct{})
	for _, platform := range scope.Platforms() {
		allowedPlatforms[platform] = struct{}{}
	}
	// 已停用分组和混合平台分组仍可能绑定范围内账号，完整分页后按平台或实际关联筛选。
	for page := 1; ; page++ {
		groups, result, err := d.groups.List(ctx, pagination.PaginationParams{Page: page, PageSize: 1000, SortBy: "id", SortOrder: pagination.SortOrderAsc})
		if err != nil {
			return nil, errPluginResourcesUnavailable
		}
		for _, group := range groups {
			_, platformAllowed := allowedPlatforms[group.Platform]
			_, referenced := referencedGroups[group.ID]
			if group.ID <= 0 || (!platformAllowed && !referenced) {
				continue
			}
			if _, exists := seenGroups[group.ID]; !exists {
				out.Groups = append(out.Groups, PluginResourceGroup{ID: group.ID, Name: group.Name})
				seenGroups[group.ID] = struct{}{}
			}
		}
		if len(groups) == 0 || (result != nil && page >= result.Pages) || (result == nil && len(groups) < 1000) {
			break
		}
	}
	proxies, err := d.proxies.ListActive(ctx)
	if err != nil {
		return nil, errPluginResourcesUnavailable
	}
	now := time.Now()
	seenProxies := make(map[int64]struct{}, len(proxies))
	for _, proxy := range proxies {
		if proxy.ID <= 0 || !proxy.IsActive() || proxy.IsExpired(now) {
			continue
		}
		if _, exists := seenProxies[proxy.ID]; exists {
			continue
		}
		if _, err := pluginResourceProxyURL(&proxy); err != nil {
			continue
		}
		seenProxies[proxy.ID] = struct{}{}
		out.Proxies = append(out.Proxies, PluginResourceProxy{
			ID: proxy.ID, Name: proxy.Name, Protocol: proxy.Protocol, Host: proxy.Host, Port: proxy.Port,
		})
	}
	sort.Slice(out.Accounts, func(i, j int) bool { return out.Accounts[i].ID < out.Accounts[j].ID })
	sort.Slice(out.Groups, func(i, j int) bool { return out.Groups[i].ID < out.Groups[j].ID })
	sort.Slice(out.Proxies, func(i, j int) bool { return out.Proxies[i].ID < out.Proxies[j].ID })
	return out, nil
}

func (d *pluginResourceDirectory) ResolvePluginProxy(ctx context.Context, scope PluginAccountScope, id int64) (string, error) {
	if d == nil || d.proxies == nil || scope.Empty() || id <= 0 {
		return "", errPluginProxyUnavailable
	}
	// 每次解析都重查，禁用、删除、过期或认证变更必须影响下一轮采集。
	proxy, err := d.proxies.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrProxyNotFound) {
			return "", nil
		}
		return "", errPluginProxyUnavailable
	}
	if proxy == nil || proxy.ID != id || !proxy.IsActive() || proxy.IsExpired(time.Now()) {
		return "", nil
	}
	return pluginResourceProxyURL(proxy)
}

func pluginResourceProxyURL(proxy *Proxy) (string, error) {
	if proxy == nil || strings.TrimSpace(proxy.Host) == "" || strings.ContainsAny(proxy.Host, "@/?#{} \t\r\n") || proxy.Port <= 0 || proxy.Port > 65535 {
		return "", errPluginProxyUnavailable
	}
	// URL 构造会转义百分号，需提前拒绝非法域名；冒号或区域标记只允许出现在有效 IPv6 中。
	if strings.ContainsAny(proxy.Host, ":%") {
		if address, err := netip.ParseAddr(proxy.Host); err != nil || !address.Is6() {
			return "", errPluginProxyUnavailable
		}
	}
	if proxy.Password != "" && proxy.Username == "" {
		return "", errPluginProxyUnavailable
	}
	_, parsed, err := proxyurl.Parse(proxy.URL())
	if err != nil || parsed == nil {
		return "", errPluginProxyUnavailable
	}
	// 用户名存在而密码为空仍然是认证代理，不因通用 URL 辅助方法而丢失用户名。
	if proxy.Username != "" {
		parsed.User = url.UserPassword(proxy.Username, proxy.Password)
	}
	return parsed.String(), nil
}

func (s *pluginHostServiceServer) ListResources(ctx context.Context, req *pluginv1.ListResourcesRequest) (*pluginv1.ListResourcesResponse, error) {
	if err := s.requirePluginHostAdaptation(ctx); err != nil {
		return nil, err
	}
	if s.resourceDirectory == nil || s.scope.Empty() {
		return nil, status.Error(codes.PermissionDenied, "宿主资源目录不可用")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "请求为空")
	}
	resources, err := s.resourceDirectory.ListPluginResources(ctx, s.scope)
	if err != nil || resources == nil {
		return nil, status.Error(codes.Unavailable, "宿主资源目录暂时不可用")
	}
	out := &pluginv1.ListResourcesResponse{
		Accounts:         make([]*pluginv1.AccountSummary, 0, len(resources.Accounts)),
		Proxies:          make([]*pluginv1.ProxySummary, 0, len(resources.Proxies)),
		ActionsSupported: true,
	}
	for _, account := range resources.Accounts {
		out.Accounts = append(out.Accounts, &pluginv1.AccountSummary{Id: account.ID, Name: account.Name})
	}
	for _, proxy := range resources.Proxies {
		out.Proxies = append(out.Proxies, &pluginv1.ProxySummary{
			Id: proxy.ID, Name: proxy.Name, Protocol: proxy.Protocol, Host: proxy.Host, Port: int32(proxy.Port),
		})
	}
	return out, nil
}

func (s *pluginHostServiceServer) ResolveProxy(ctx context.Context, req *pluginv1.ResolveProxyRequest) (*pluginv1.ResolveProxyResponse, error) {
	if err := s.requirePluginHostAdaptation(ctx); err != nil {
		return nil, err
	}
	if s.resourceDirectory == nil || s.scope.Empty() {
		return nil, status.Error(codes.PermissionDenied, "代理目录不可用")
	}
	if req == nil || req.ProxyId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "代理 ID 无效")
	}
	proxyURL, err := s.resourceDirectory.ResolvePluginProxy(ctx, s.scope, req.ProxyId)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "所选代理暂时不可用")
	}
	return &pluginv1.ResolveProxyResponse{Found: proxyURL != "", ProxyUrl: proxyURL}, nil
}
