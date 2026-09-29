package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pluginResourceProxyRepository struct {
	ProxyRepository
	proxies []Proxy
	err     error
	gets    int
	lists   int
}

func (r *pluginResourceProxyRepository) ListActive(context.Context) ([]Proxy, error) {
	r.lists++
	return r.proxies, r.err
}

func (r *pluginResourceProxyRepository) GetByID(_ context.Context, id int64) (*Proxy, error) {
	r.gets++
	if r.err != nil {
		return nil, r.err
	}
	for i := range r.proxies {
		if r.proxies[i].ID == id {
			return &r.proxies[i], nil
		}
	}
	return nil, ErrProxyNotFound
}

type pluginResourceGroupRepository struct {
	GroupRepository
	groups []Group
	err    error
	pages  []int
}

type pluginResourceAccountRepository struct {
	AccountRepository
	accounts  []Account
	err       error
	platforms []string
	statuses  []string
}

func (r *pluginResourceAccountRepository) ListAllWithFilters(_ context.Context, platform, _, status, _ string, _ int64, _ string) ([]Account, error) {
	r.platforms = append(r.platforms, platform)
	r.statuses = append(r.statuses, status)
	return r.accounts, r.err
}

func (r *pluginResourceGroupRepository) List(_ context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	r.pages = append(r.pages, params.Page)
	if r.err != nil {
		return nil, nil, r.err
	}
	start := min(params.Offset(), len(r.groups))
	end := min(start+params.Limit(), len(r.groups))
	return r.groups[start:end], &pagination.PaginationResult{Total: int64(len(r.groups)), Page: params.Page, PageSize: params.Limit(), Pages: (len(r.groups) + params.Limit() - 1) / params.Limit()}, nil
}

func pluginResourceTestScope() PluginAccountScope {
	return newPluginAccountScope(pluginAccountScopeEntry{Platform: PlatformOpenAI, AccountType: AccountTypeOAuth})
}

func pluginResourceTestDirectory() (*pluginResourceDirectory, *fakeAccountDirectory, *pluginResourceProxyRepository, *pluginResourceGroupRepository) {
	base := &fakeAccountDirectory{infos: []PluginAccountInfo{
		{ID: 7, Name: "常用账号", Platform: PlatformOpenAI, AccountType: AccountTypeOAuth, Status: StatusActive,
			MetadataJSON: []byte(`{"GroupIDs":[3,2,3,-1],"Credentials":{"refresh_token":"REFRESH-SECRET"},"Extra":{"token":"EXTRA-SECRET"},"Proxy":{"Username":"PROXY-USER","Password":"PROXY-SECRET"}}`)},
	}}
	proxies := &pluginResourceProxyRepository{proxies: []Proxy{
		{ID: 11, Name: "前置代理", Protocol: "http", Host: "proxy.example", Port: 8080, Username: "PROXY-USER", Password: "PROXY-SECRET", Status: StatusActive},
	}}
	groups := &pluginResourceGroupRepository{groups: []Group{
		{ID: 2, Name: "公开分组", Platform: PlatformOpenAI, Status: StatusActive},
		{ID: 3, Name: "专用分组", Platform: PlatformOpenAI, Status: StatusActive},
	}}
	accounts := &pluginResourceAccountRepository{accounts: []Account{
		{ID: 7, Name: "常用账号", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, GroupIDs: []int64{2, 3}},
	}}
	return &pluginResourceDirectory{base: base, accounts: accounts, proxies: proxies, groups: groups}, base, proxies, groups
}

func TestPluginAdminResourcesIncludesAllStatusesWithoutExpandingBackendAccess(t *testing.T) {
	dir, base, _, groups := pluginResourceTestDirectory()
	repo := dir.accounts.(*pluginResourceAccountRepository)
	parentID := int64(7)
	repo.accounts = []Account{
		{ID: 7, Name: "启用账号", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, GroupIDs: []int64{2}},
		{ID: 8, Name: "停用账号", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusDisabled, GroupIDs: []int64{3, 3}, Credentials: map[string]any{"refresh_token": "REFRESH-SECRET"}},
		{ID: 9, Name: "报错账号", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError, GroupIDs: []int64{4}, Extra: map[string]any{"token": "EXTRA-SECRET"}},
		{ID: 10, Name: "过期账号", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusExpired, Proxy: &Proxy{Username: "PROXY-USER", Password: "PROXY-SECRET"}},
		{ID: 11, Name: "其他平台", Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive},
		{ID: 12, Name: "API Key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive},
		{ID: 13, Name: "影子账号", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, ParentAccountID: &parentID},
	}
	groups.groups = append(groups.groups, Group{ID: 4, Name: "停用账号关联的混合分组", Platform: "composite", Status: StatusDisabled})
	resources, err := dir.ListPluginAdminResources(context.Background(), pluginResourceTestScope())
	require.NoError(t, err)
	require.Len(t, resources.Accounts, 4)
	assert.Equal(t, []int64{3}, resources.Accounts[1].GroupIDs)
	assert.Equal(t, int64(10), resources.Accounts[3].ID)
	require.Len(t, resources.Groups, 3)
	assert.Equal(t, []string{PlatformOpenAI}, repo.platforms)
	assert.Equal(t, []string{""}, repo.statuses)
	encoded, err := json.Marshal(resources)
	require.NoError(t, err)
	for _, secret := range []string{"REFRESH-SECRET", "EXTRA-SECRET", "PROXY-USER", "PROXY-SECRET", "Credentials", "Metadata"} {
		assert.NotContains(t, string(encoded), secret)
	}
	// 新增管理员摘要不能放宽原 HostService 目录，更不能解析任何出站身份。
	backendResources, err := dir.ListPluginResources(context.Background(), pluginResourceTestScope())
	require.NoError(t, err)
	require.Len(t, backendResources.Accounts, 1)
	assert.Equal(t, int64(7), backendResources.Accounts[0].ID)
	assert.Zero(t, base.lastReq)
}

func TestPluginAdminResourcesScopeAndRepositoryFailure(t *testing.T) {
	dir, _, proxies, groups := pluginResourceTestDirectory()
	repo := dir.accounts.(*pluginResourceAccountRepository)
	_, err := dir.ListPluginAdminResources(context.Background(), PluginAccountScope{})
	require.Error(t, err)
	assert.Empty(t, repo.platforms)
	repo.err = errors.New("DATABASE-SECRET")
	_, err = dir.ListPluginAdminResources(context.Background(), pluginResourceTestScope())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "DATABASE-SECRET")
	assert.Zero(t, proxies.lists)
	assert.Empty(t, groups.pages)
}

func TestPluginResourcesWhitelistScopeAndPausedAccounts(t *testing.T) {
	dir, base, proxies, groups := pluginResourceTestDirectory()
	base.infos = append(base.infos,
		PluginAccountInfo{ID: 8, Name: "暂停账号", Platform: PlatformOpenAI, AccountType: AccountTypeOAuth, Status: StatusActive, Schedulable: false},
		PluginAccountInfo{ID: 9, Name: "其他平台", Platform: PlatformAnthropic, AccountType: AccountTypeOAuth, Status: StatusActive},
		PluginAccountInfo{ID: 10, Name: "API Key", Platform: PlatformOpenAI, AccountType: AccountTypeAPIKey, Status: StatusActive},
		PluginAccountInfo{ID: 12, Name: "影子账号", Platform: PlatformOpenAI, AccountType: AccountTypeOAuth, Status: StatusActive, IsShadow: true},
		PluginAccountInfo{ID: 13, Name: "禁用账号", Platform: PlatformOpenAI, AccountType: AccountTypeOAuth, Status: StatusDisabled},
	)
	groups.groups = append(groups.groups, Group{ID: 4, Name: "跨平台分组", Platform: PlatformAnthropic, Status: StatusActive})
	expired := time.Now().Add(-time.Minute)
	proxies.proxies = append(proxies.proxies,
		Proxy{ID: 12, Protocol: "http", Host: "disabled.example", Port: 80, Status: StatusDisabled},
		Proxy{ID: 13, Protocol: "http", Host: "expired.example", Port: 80, Status: StatusActive, ExpiresAt: &expired},
		Proxy{ID: 14, Protocol: "file", Host: "invalid.example", Port: 80, Status: StatusActive},
	)
	resources, err := dir.ListPluginResources(context.Background(), pluginResourceTestScope())
	require.NoError(t, err)
	require.Len(t, resources.Accounts, 2)
	assert.Equal(t, []int64{2, 3}, resources.Accounts[0].GroupIDs)
	assert.Equal(t, int64(8), resources.Accounts[1].ID)
	assert.NotNil(t, resources.Accounts[1].GroupIDs)
	require.Len(t, resources.Groups, 2)
	require.Len(t, resources.Proxies, 1)
	assert.Equal(t, []int{1}, groups.pages)
	assert.True(t, base.lastScope.Contains(PlatformOpenAI, AccountTypeOAuth))
	assert.False(t, base.lastScope.Contains(PlatformOpenAI, AccountTypeAPIKey))
	encoded, err := json.Marshal(resources)
	require.NoError(t, err)
	for _, secret := range []string{"REFRESH-SECRET", "EXTRA-SECRET", "PROXY-USER", "PROXY-SECRET", "Credentials", "Metadata", "proxy_url"} {
		assert.NotContains(t, string(encoded), secret)
	}
	assert.Contains(t, string(encoded), `"group_ids":[2,3]`)
	assert.Contains(t, string(encoded), `"host":"proxy.example"`)
}

func TestPluginResourcesDoesNotTruncateAccountDirectory(t *testing.T) {
	dir, base, _, groups := pluginResourceTestDirectory()
	base.infos = make([]PluginAccountInfo, 1501)
	for i := range base.infos {
		base.infos[i] = PluginAccountInfo{ID: int64(i + 1), Name: "账号" + strconv.Itoa(i), Platform: PlatformOpenAI, AccountType: AccountTypeOAuth, Status: StatusActive}
	}
	groups.groups = make([]Group, 1501)
	for i := range groups.groups {
		groups.groups[i] = Group{ID: int64(i + 1), Platform: PlatformOpenAI, Name: "分组" + strconv.Itoa(i), Status: StatusActive}
	}
	resources, err := dir.ListPluginResources(context.Background(), pluginResourceTestScope())
	require.NoError(t, err)
	require.Len(t, resources.Accounts, 1501)
	assert.Equal(t, int64(1501), resources.Accounts[1500].ID)
	require.Len(t, resources.Groups, 1501)
	assert.Equal(t, []int{1, 2}, groups.pages)
}

func TestPluginResourcesEmptyScopeNeverReadsRepositories(t *testing.T) {
	dir, _, proxies, groups := pluginResourceTestDirectory()
	_, err := dir.ListPluginResources(context.Background(), PluginAccountScope{})
	require.Error(t, err)
	_, err = dir.ResolvePluginProxy(context.Background(), PluginAccountScope{}, 11)
	require.Error(t, err)
	assert.Zero(t, proxies.lists)
	assert.Zero(t, proxies.gets)
	assert.Empty(t, groups.pages)
}

func TestPluginResourcesKeepsDisabledAndReferencedMixedGroups(t *testing.T) {
	dir, _, _, groups := pluginResourceTestDirectory()
	groups.groups = []Group{
		{ID: 2, Name: "混合分组", Platform: "composite", Status: StatusActive},
		{ID: 3, Name: "停用分组", Platform: PlatformOpenAI, Status: StatusDisabled},
		{ID: 4, Name: "其他平台分组", Platform: PlatformAnthropic, Status: StatusActive},
	}
	resources, err := dir.ListPluginResources(context.Background(), pluginResourceTestScope())
	require.NoError(t, err)
	assert.Equal(t, []PluginResourceGroup{{ID: 2, Name: "混合分组"}, {ID: 3, Name: "停用分组"}}, resources.Groups)
}

func TestPluginResourcesProxyIsResolvedFreshAndNeverFallsBack(t *testing.T) {
	dir, _, proxies, _ := pluginResourceTestDirectory()
	ctx := context.Background()
	scope := pluginResourceTestScope()
	resolved, err := dir.ResolvePluginProxy(ctx, scope, 11)
	require.NoError(t, err)
	assert.Contains(t, resolved, "PROXY-USER:PROXY-SECRET@proxy.example:8080")
	proxies.proxies[0].Password = "updated-password"
	resolved, err = dir.ResolvePluginProxy(ctx, scope, 11)
	require.NoError(t, err)
	assert.Contains(t, resolved, "updated-password")
	assert.NotContains(t, resolved, "PROXY-SECRET")
	proxies.proxies[0].Status = StatusDisabled
	proxies.proxies[0].FallbackMode = FallbackModeDirect
	resolved, err = dir.ResolvePluginProxy(ctx, scope, 11)
	require.NoError(t, err)
	assert.Empty(t, resolved)
	proxies.proxies[0].Status = StatusActive
	expired := time.Now().Add(-time.Minute)
	proxies.proxies[0].ExpiresAt = &expired
	resolved, err = dir.ResolvePluginProxy(ctx, scope, 11)
	require.NoError(t, err)
	assert.Empty(t, resolved)
	proxies.proxies = nil
	resolved, err = dir.ResolvePluginProxy(ctx, scope, 11)
	require.NoError(t, err)
	assert.Empty(t, resolved)
	assert.Equal(t, 5, proxies.gets)
}

func TestPluginResourcesInvalidProxyAndRepositoryErrorsAreRedacted(t *testing.T) {
	dir, _, proxies, groups := pluginResourceTestDirectory()
	ctx := context.Background()
	scope := pluginResourceTestScope()
	proxies.err = errors.New("connection failed http://PROXY-USER:PROXY-SECRET@internal.example")
	_, err := dir.ListPluginResources(ctx, scope)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "PROXY-")
	_, err = dir.ResolvePluginProxy(ctx, scope, 11)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "PROXY-")
	proxies.err = nil
	proxies.proxies[0].Host = "bad%host"
	_, err = dir.ResolvePluginProxy(ctx, scope, 11)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "PROXY-")
	groups.err = errors.New("DATABASE-SECRET")
	_, err = dir.ListPluginResources(ctx, scope)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "DATABASE-SECRET")
}

func TestPluginResourcesProxyURLPreservesEmptyPasswordAndRemoteDNS(t *testing.T) {
	proxy := &Proxy{ID: 1, Protocol: "socks5", Host: "proxy.example", Port: 1080, Username: "user", Status: StatusActive}
	resolved, err := pluginResourceProxyURL(proxy)
	require.NoError(t, err)
	assert.Equal(t, "socks5h://user:@proxy.example:1080", resolved)
}

func TestPluginHostResourcesGateIsCheckedOnEveryCall(t *testing.T) {
	dir, _, proxies, _ := pluginResourceTestDirectory()
	allowed := true
	server := &pluginHostServiceServer{
		pluginKey: "local.example.plugin", resourceDirectory: dir, scope: pluginResourceTestScope(),
		adaptationAllowed: func(context.Context) bool { return allowed },
	}
	ctx := context.Background()
	resources, err := server.ListResources(ctx, &pluginv1.ListResourcesRequest{})
	require.NoError(t, err)
	assert.True(t, resources.ActionsSupported)
	require.Len(t, resources.Accounts, 1)
	require.Len(t, resources.Proxies, 1)
	resolved, err := server.ResolveProxy(ctx, &pluginv1.ResolveProxyRequest{ProxyId: 11})
	require.NoError(t, err)
	assert.True(t, resolved.Found)
	allowed = false
	_, err = server.ListResources(ctx, &pluginv1.ListResourcesRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = server.ResolveProxy(ctx, &pluginv1.ResolveProxyRequest{ProxyId: 11})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, 1, proxies.lists)
	assert.Equal(t, 1, proxies.gets)
}

func TestPluginHostResourcesValidationAndScopeCannotBeBypassed(t *testing.T) {
	dir, _, proxies, _ := pluginResourceTestDirectory()
	ctx := context.Background()
	server := &pluginHostServiceServer{resourceDirectory: dir, scope: pluginResourceTestScope(), adaptationAllowed: func(context.Context) bool { return true }}
	_, err := server.ListResources(ctx, nil)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = server.ResolveProxy(ctx, nil)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = server.ResolveProxy(ctx, &pluginv1.ResolveProxyRequest{ProxyId: 0})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	server.scope = PluginAccountScope{}
	_, err = server.ListResources(ctx, &pluginv1.ListResourcesRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = server.ResolveProxy(ctx, &pluginv1.ResolveProxyRequest{ProxyId: 11})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Zero(t, proxies.gets)
	assert.Zero(t, proxies.lists)
}

func TestPluginHostResourcesRPCNeverIncludesCredentialInErrorsOrMetadata(t *testing.T) {
	dir, _, proxies, _ := pluginResourceTestDirectory()
	server := &pluginHostServiceServer{resourceDirectory: dir, scope: pluginResourceTestScope(), adaptationAllowed: func(context.Context) bool { return true }}
	ctx := context.Background()
	resources, err := server.ListResources(ctx, &pluginv1.ListResourcesRequest{})
	require.NoError(t, err)
	encoded, err := json.Marshal(resources)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "PROXY-SECRET")
	assert.NotContains(t, string(encoded), "REFRESH-SECRET")
	assert.NotContains(t, string(encoded), "GroupIDs")
	proxies.err = errors.New("PROXY-SECRET")
	_, err = server.ResolveProxy(ctx, &pluginv1.ResolveProxyRequest{ProxyId: 11})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.NotContains(t, err.Error(), "PROXY-SECRET")
}
