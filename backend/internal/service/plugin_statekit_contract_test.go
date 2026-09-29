package service

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protowire"
)

// 以下字段号和方法路径取自 STATE Kit 0.3.4 的旧协议；测试不编译、导入或启动该插件。
// 客户端故意不用当前生成消息，防止同一次错误修改同时改变两端而掩盖兼容问题。
type stateKitRawProtoCodec struct{}

func (stateKitRawProtoCodec) Name() string { return "proto" }

func (stateKitRawProtoCodec) Marshal(value any) ([]byte, error) {
	switch data := value.(type) {
	case []byte:
		return data, nil
	case *[]byte:
		return *data, nil
	default:
		return nil, fmt.Errorf("原始协议测试不接受 %T", value)
	}
}

func (stateKitRawProtoCodec) Unmarshal(data []byte, value any) error {
	target, ok := value.(*[]byte)
	if !ok {
		return fmt.Errorf("原始协议测试不接受 %T", value)
	}
	*target = append((*target)[:0], data...)
	return nil
}

type stateKitRawField struct {
	number protowire.Number
	kind   protowire.Type
	value  uint64
	bytes  []byte
}

func stateKitDecodeWire(t *testing.T, data []byte) []stateKitRawField {
	t.Helper()
	var fields []stateKitRawField
	for len(data) > 0 {
		number, kind, size := protowire.ConsumeTag(data)
		require.Greater(t, size, 0, "字段标签必须有效")
		data = data[size:]
		field := stateKitRawField{number: number, kind: kind}
		switch kind {
		case protowire.VarintType:
			field.value, size = protowire.ConsumeVarint(data)
		case protowire.BytesType:
			field.bytes, size = protowire.ConsumeBytes(data)
		default:
			t.Fatalf("旧协议不预期字段 %d 使用类型 %d", number, kind)
		}
		require.GreaterOrEqual(t, size, 0, "字段内容必须有效")
		data = data[size:]
		fields = append(fields, field)
	}
	return fields
}

func stateKitWireString(data []byte, number protowire.Number, value string) []byte {
	return protowire.AppendString(protowire.AppendTag(data, number, protowire.BytesType), value)
}

func stateKitWireInteger(data []byte, number protowire.Number, value uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(data, number, protowire.VarintType), value)
}

func stateKitWireConnection(t *testing.T, server *grpc.Server) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	connection, err := grpc.NewClient("passthrough:///state-kit-wire-test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func stateKitInvokeWire(t *testing.T, connection *grpc.ClientConn, method string, request []byte) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var response []byte
	err := connection.Invoke(ctx, method, request, &response, grpc.ForceCodec(stateKitRawProtoCodec{}))
	return response, err
}

type stateKitWireDirectory struct {
	proxyID  atomic.Int64
	requests atomic.Int64
}

func (d *stateKitWireDirectory) ListPluginResources(_ context.Context, scope PluginAccountScope) (*PluginResources, error) {
	if !scope.Contains("openai", "oauth") || scope.Contains("anthropic", "oauth") {
		return nil, fmt.Errorf("未保留宿主账号授权范围")
	}
	d.requests.Add(1)
	return &PluginResources{
		Accounts: []PluginResourceAccount{{ID: 5939067819, Name: "测试账号", GroupIDs: []int64{44}}},
		Groups:   []PluginResourceGroup{{ID: 44, Name: "测试分组"}},
		Proxies:  []PluginResourceProxy{{ID: 5939067820, Name: "测试代理", Protocol: "socks5", Host: "proxy.invalid", Port: 1080}},
	}, nil
}

func (d *stateKitWireDirectory) ResolvePluginProxy(_ context.Context, scope PluginAccountScope, id int64) (string, error) {
	if !scope.Contains("openai", "oauth") || scope.Contains("anthropic", "oauth") {
		return "", fmt.Errorf("未保留宿主账号授权范围")
	}
	d.proxyID.Store(id)
	d.requests.Add(1)
	return "socks5://test-user:test-password@proxy.invalid:1080", nil
}

func TestStateKitLegacyWireResourceDirectory(t *testing.T) {
	directory := &stateKitWireDirectory{}
	scope := newPluginAccountScope(pluginAccountScopeEntry{Platform: "openai", AccountType: "oauth"})
	host := newPluginHostServiceServer("wire.test", newFakePluginKVStore(), nil, scope)
	host.resourceDirectory = directory
	host.adaptationAllowed = func(context.Context) bool { return true }
	server := grpc.NewServer()
	pluginv1.RegisterHostServiceServer(server, host)
	connection := stateKitWireConnection(t, server)

	data, err := stateKitInvokeWire(t, connection, "/sub2api.plugin.v1.HostService/ListResources", nil)
	require.NoError(t, err)
	fields := stateKitDecodeWire(t, data)
	require.Len(t, fields, 3)
	require.Equal(t, protowire.Number(1), fields[0].number)
	require.Equal(t, protowire.BytesType, fields[0].kind)
	require.Equal(t, []stateKitRawField{
		{number: 1, kind: protowire.VarintType, value: 5939067819},
		{number: 2, kind: protowire.BytesType, bytes: []byte("测试账号")},
	}, stateKitDecodeWire(t, fields[0].bytes))
	require.Equal(t, protowire.Number(2), fields[1].number)
	require.Equal(t, []stateKitRawField{
		{number: 1, kind: protowire.VarintType, value: 5939067820},
		{number: 2, kind: protowire.BytesType, bytes: []byte("测试代理")},
		{number: 3, kind: protowire.BytesType, bytes: []byte("socks5")},
		{number: 4, kind: protowire.BytesType, bytes: []byte("proxy.invalid")},
		{number: 5, kind: protowire.VarintType, value: 1080},
	}, stateKitDecodeWire(t, fields[1].bytes))
	require.Equal(t, stateKitRawField{number: 3, kind: protowire.VarintType, value: 1}, fields[2])
	require.NotContains(t, string(data), "test-password", "目录不应带代理认证信息")
	require.NotContains(t, string(data), "测试分组", "旧资源 RPC 不应擅自改变分组契约")
}

func TestStateKitLegacyWireRevocationKeepsNativeAccounts(t *testing.T) {
	directory := &stateKitWireDirectory{}
	accounts := &fakeAccountDirectory{infos: []PluginAccountInfo{{
		ID: 5939067819, Name: "兼容账号", Platform: "openai", AccountType: "oauth",
		Status: "active", Schedulable: true, MetadataJSON: []byte(`{"GroupIDs":[44]}`),
	}}}
	scope := newPluginAccountScope(pluginAccountScopeEntry{Platform: "openai", AccountType: "oauth"})
	var allowed atomic.Bool
	allowed.Store(true)
	host := newPluginHostServiceServer("wire.test", newFakePluginKVStore(), accounts, scope)
	host.resourceDirectory = directory
	host.adaptationAllowed = func(context.Context) bool { return allowed.Load() }
	server := grpc.NewServer()
	pluginv1.RegisterHostServiceServer(server, host)
	connection := stateKitWireConnection(t, server)
	request := stateKitWireInteger(nil, 1, 5939067820)
	data, err := stateKitInvokeWire(t, connection, "/sub2api.plugin.v1.HostService/ResolveProxy", request)
	require.NoError(t, err)
	require.Equal(t, int64(5939067820), directory.proxyID.Load(), "大于 32 位的代理 ID 不能截断")
	require.Equal(t, []stateKitRawField{
		{number: 1, kind: protowire.VarintType, value: 1},
		{number: 2, kind: protowire.BytesType, bytes: []byte("socks5://test-user:test-password@proxy.invalid:1080")},
	}, stateKitDecodeWire(t, data))

	// 不重建服务或连接，验证已运行的旧插件下一次调用即失去新增接口权限。
	allowed.Store(false)
	for _, method := range []string{"ResolveProxy", "ListResources"} {
		_, err = stateKitInvokeWire(t, connection, "/sub2api.plugin.v1.HostService/"+method, request)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	}
	require.Equal(t, int64(1), directory.requests.Load())

	request = stateKitWireString(nil, 1, "openai")
	request = stateKitWireString(request, 2, "oauth")
	data, err = stateKitInvokeWire(t, connection, "/sub2api.plugin.v1.HostService/ListAccounts", request)
	require.NoError(t, err, "关闭适配不能撤销原生账号目录")
	var legacyIDs []int64
	metadataPresent := false
	for _, field := range stateKitDecodeWire(t, data) {
		switch field.number {
		case 1:
			require.Equal(t, protowire.BytesType, field.kind)
			for packed := field.bytes; len(packed) > 0; {
				id, size := protowire.ConsumeVarint(packed)
				require.Greater(t, size, 0)
				legacyIDs = append(legacyIDs, int64(id))
				packed = packed[size:]
			}
		case 2:
			// 第一版插件忽略未知的第二字段；新版宿主仍必须保留它。
			metadataPresent = true
		}
	}
	require.Equal(t, []int64{5939067819}, legacyIDs)
	require.True(t, metadataPresent, "适配旧插件不能删除第二版账号元数据")
}

// 动作方向与资源目录相反：这里让当前宿主生成客户端调用只懂旧字段号的模拟插件。
type stateKitRawActionServer interface {
	runRawAction(context.Context, []byte) ([]byte, error)
}

type stateKitRawActionPlugin struct {
	actions chan string
}

func (p *stateKitRawActionPlugin) runRawAction(_ context.Context, data []byte) ([]byte, error) {
	number, kind, size := protowire.ConsumeTag(data)
	if size <= 0 || number != 1 || kind != protowire.BytesType {
		return nil, status.Error(codes.InvalidArgument, "旧插件要求第一字段为动作 JSON")
	}
	action, consumed := protowire.ConsumeBytes(data[size:])
	if consumed < 0 || size+consumed != len(data) {
		return nil, status.Error(codes.InvalidArgument, "动作 JSON 编码无效")
	}
	p.actions <- string(action)
	response := stateKitWireInteger(nil, 1, 1)
	return stateKitWireString(response, 2, "已接收，等待状态更新"), nil
}

func stateKitRawActionHandler(server any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	var request []byte
	if err := decode(&request); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return server.(stateKitRawActionServer).runRawAction(ctx, request)
	}
	info := &grpc.UnaryServerInfo{Server: server, FullMethod: "/sub2api.plugin.v1.TransportPlugin/RunAction"}
	return interceptor(ctx, request, info, func(ctx context.Context, request any) (any, error) {
		return server.(stateKitRawActionServer).runRawAction(ctx, request.([]byte))
	})
}

func TestStateKitLegacyWireRunAction(t *testing.T) {
	plugin := &stateKitRawActionPlugin{actions: make(chan string, 1)}
	server := grpc.NewServer(grpc.ForceServerCodec(stateKitRawProtoCodec{}))
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "sub2api.plugin.v1.TransportPlugin",
		HandlerType: (*stateKitRawActionServer)(nil),
		Methods:     []grpc.MethodDesc{{MethodName: "RunAction", Handler: stateKitRawActionHandler}},
	}, plugin)
	connection := stateKitWireConnection(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	action := `{"kind":"test","request_id":"wire-request-1","account_id":5939067819,"model":"gpt-test","prompt":"只回复 OK","use_state":false}`
	response, err := pluginv1.NewTransportPluginClient(connection).RunAction(ctx, &pluginv1.RunActionRequest{ActionJson: []byte(action)})
	require.NoError(t, err)
	require.True(t, response.Accepted)
	require.Equal(t, "已接收，等待状态更新", response.Message)
	require.Equal(t, action, <-plugin.actions)
}

type stateKitHandshakePlugin struct {
	brokerProbePlugin
	requiredVersion uint32
	offeredVersion  atomic.Uint32
}

func (p *stateKitHandshakePlugin) InitHostServices(ctx context.Context, request *pluginv1.InitHostServicesRequest) (*pluginv1.InitHostServicesResponse, error) {
	p.offeredVersion.Store(request.HostServiceApiVersion)
	// 模拟参考插件的严格等值校验，不能只验证宿主选版辅助函数。
	if request.HostServiceApiVersion != p.requiredVersion {
		return &pluginv1.InitHostServicesResponse{Message: "unsupported host service API"}, nil
	}
	return p.brokerProbePlugin.InitHostServices(ctx, request)
}

func TestStateKitLegacyHandshakeIsScopedAndUsesRealBroker(t *testing.T) {
	for _, test := range []struct {
		name          string
		pluginKey     string
		adapted       bool
		pluginVersion uint32
		offered       uint32
		ready         bool
	}{
		{name: "已开启适配的 STATE Kit 使用第一版", pluginKey: "io.github.wangyunjeff.sub2api-state-kit", adapted: true, pluginVersion: 1, offered: 1, ready: true},
		{name: "未开启适配的 STATE Kit 不改变默认握手", pluginKey: "io.github.wangyunjeff.sub2api-state-kit", adapted: false, pluginVersion: 1, offered: 2, ready: false},
		{name: "其他已适配插件保留第二版", pluginKey: "local.other.plugin", adapted: true, pluginVersion: 2, offered: 2, ready: true},
		{name: "相似插件标识不能取得第一版兼容握手", pluginKey: "io.github.wangyunjeff.sub2api-state-kit.other", adapted: true, pluginVersion: 2, offered: 2, ready: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			installation := &PluginInstallation{PluginKey: test.pluginKey, HostAdaptationEnabled: test.adapted}
			require.Equal(t, test.offered, pluginHostServiceVersion(installation))
			probe := &stateKitHandshakePlugin{requiredVersion: test.pluginVersion}
			transport := dispenseTransportClient(t, probe)
			store := newFakePluginKVStore()
			host := newPluginHostServiceServer(test.pluginKey, store, nil, PluginAccountScope{})
			require.NoError(t, offerPluginHostServices(context.Background(), installation, transport.TransportPluginClient, transport.Broker, host, 5*time.Second))
			require.Equal(t, test.offered, probe.offeredVersion.Load())
			probe.mu.Lock()
			defer probe.mu.Unlock()
			require.Empty(t, probe.failure)
			require.Equal(t, test.ready, probe.ready)
			if test.ready {
				require.True(t, probe.getFound)
				require.Equal(t, []byte("hello"), probe.getValue)
			}
		})
	}
	require.Equal(t, uint32(2), pluginHostServiceVersion(nil))
}

func TestStateKitRequiredHandshakeFailsClosed(t *testing.T) {
	installation := &PluginInstallation{PluginKey: "io.github.wangyunjeff.sub2api-state-kit", HostAdaptationEnabled: true}
	host := newPluginHostServiceServer(installation.PluginKey, newFakePluginKVStore(), nil, PluginAccountScope{})
	t.Run("缺少宿主服务不能伪装适配完成", func(t *testing.T) {
		require.Error(t, offerPluginHostServices(context.Background(), installation, nil, nil, host, time.Second))
		transport := dispenseTransportClient(t, &noHostServicesPlugin{})
		require.Error(t, offerPluginHostServices(context.Background(), installation, transport.TransportPluginClient, transport.Broker, nil, time.Second))
	})
	t.Run("已适配插件不实现握手时阻止启动", func(t *testing.T) {
		transport := dispenseTransportClient(t, &noHostServicesPlugin{})
		require.Error(t, offerPluginHostServices(context.Background(), installation, transport.TransportPluginClient, transport.Broker, host, 5*time.Second))
	})
	t.Run("已适配插件明确拒绝服务时阻止启动", func(t *testing.T) {
		probe := &stateKitHandshakePlugin{requiredVersion: 2}
		transport := dispenseTransportClient(t, probe)
		require.Error(t, offerPluginHostServices(context.Background(), installation, transport.TransportPluginClient, transport.Broker, host, 5*time.Second))
		require.Equal(t, uint32(1), probe.offeredVersion.Load())
	})
	t.Run("其他插件仍可选择不实现宿主服务", func(t *testing.T) {
		transport := dispenseTransportClient(t, &noHostServicesPlugin{})
		ordinary := &PluginInstallation{PluginKey: "local.other.plugin", HostAdaptationEnabled: true}
		require.NoError(t, offerPluginHostServices(context.Background(), ordinary, transport.TransportPluginClient, transport.Broker, host, 5*time.Second))
	})
}
