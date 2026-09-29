package repository

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 插件适配测试只连接显式指定的专用数据库，每项测试均使用独立随机 schema。
func pluginAdaptationPostgresDatabase(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("SUB2API_PLUGIN_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 SUB2API_PLUGIN_TEST_DSN，跳过真实 PostgreSQL 插件适配回归")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Equal(t, "postgres", parsed.Scheme, "测试连接必须使用 postgres:// 格式")
	require.NotEmpty(t, parsed.Host)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	schema := "plugin_adaptation_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		// 仅清理本测试创建且已经确认存在的随机 schema。
		_, cleanupErr := admin.ExecContext(cleanup, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE")
		require.NoError(t, cleanupErr)
	})
	params := parsed.Query()
	params.Set("search_path", schema)
	parsed.RawQuery = params.Encode()
	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(4)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var actualSchema string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&actualSchema))
	require.Equal(t, schema, actualSchema)
	_, err = db.ExecContext(ctx, `CREATE TABLE users (id BIGINT PRIMARY KEY)`)
	require.NoError(t, err)
	for _, name := range []string{"229_plugins.sql", "230_plugin_artifacts.sql"} {
		pluginAdaptationApplyMigration(t, ctx, db, name)
	}
	return db, ctx
}

func pluginAdaptationApplyMigration(t *testing.T, ctx context.Context, db *sql.DB, name string) {
	t.Helper()
	content, err := migrations.FS.ReadFile(name)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(content))
	require.NoError(t, err, "真实迁移必须可执行")
}

func pluginAdaptationPostgresRepository(t *testing.T) (*pluginRepository, context.Context) {
	t.Helper()
	db, ctx := pluginAdaptationPostgresDatabase(t)
	pluginAdaptationApplyMigration(t, ctx, db, "244_plugin_host_adaptation.sql")
	return &pluginRepository{db: db}, ctx
}

func pluginAdaptationInstall(t *testing.T, ctx context.Context, repo *pluginRepository) (*service.PluginInstallation, []service.PluginBinding) {
	t.Helper()
	key := "local.test.adaptation"
	plugin := &service.PluginInstallation{
		PluginKey: key, Name: "适配测试插件", Version: "1.0.0", Manifest: service.PluginManifest{ID: key, Name: "适配测试插件", Version: "1.0.0"},
		ArtifactData: []byte("unchanged-plugin-package"), ArtifactPath: "/test/plugin.s2plugin", InstallPath: "/test/plugin", BinaryPath: "/test/plugin/bin",
		BinarySHA256: strings.Repeat("a", 64), SignatureStatus: service.PluginSignatureTrusted, State: service.PluginStateDisabled,
	}
	bindings := []service.PluginBinding{{Capability: service.PluginCapabilityOpenAIOAuthOutbound, Platform: service.PlatformOpenAI, AccountType: service.AccountTypeOAuth, RolloutPercent: 100}}
	installed, err := repo.Install(ctx, plugin, bindings)
	require.NoError(t, err)
	require.False(t, installed.HostAdaptationEnabled)
	return installed, bindings
}

func TestPluginHostAdaptationPostgresMigrationDefaultsExistingInstallationsOff(t *testing.T) {
	db, ctx := pluginAdaptationPostgresDatabase(t)
	_, err := db.ExecContext(ctx, `
		INSERT INTO sub2api_plugin_installations
		(plugin_key,name,version,artifact_path,install_path,binary_path,binary_sha256,config_encrypted,state)
		VALUES ('local.existing.plugin','已有插件','1.0.0','/test/package','/test/install','/test/bin',$1,'saved-config','enabled')
	`, strings.Repeat("a", 64))
	require.NoError(t, err)
	pluginAdaptationApplyMigration(t, ctx, db, "244_plugin_host_adaptation.sql")
	repo := &pluginRepository{db: db}
	installed, err := repo.GetByKey(ctx, "local.existing.plugin")
	require.NoError(t, err)
	assert.False(t, installed.HostAdaptationEnabled)
	assert.Equal(t, "saved-config", installed.ConfigEncrypted)
	assert.Equal(t, service.PluginStateEnabled, installed.State)
	require.NoError(t, repo.UpdateHostAdaptation(ctx, installed.ID, true, installed.BinarySHA256, false))
	pluginAdaptationApplyMigration(t, ctx, db, "244_plugin_host_adaptation.sql")
	installed, err = repo.GetByID(ctx, installed.ID)
	require.NoError(t, err)
	assert.True(t, installed.HostAdaptationEnabled, "迁移重入不得覆盖管理员已经保存的开关")
}

func TestPluginHostAdaptationPostgresCompareAndSwapPreservesPluginData(t *testing.T) {
	repo, ctx := pluginAdaptationPostgresRepository(t)
	installed, bindings := pluginAdaptationInstall(t, ctx, repo)
	require.NoError(t, repo.UpdateConfig(ctx, installed.ID, "encrypted-existing-config", installed.BinarySHA256))
	bindings[0].Enabled = true
	now := time.Now()
	require.NoError(t, repo.UpdateBindingsAndState(ctx, installed.ID, bindings, service.PluginStateEnabled, "", &now, service.PluginStateDisabled, installed.BinarySHA256))
	require.ErrorIs(t, repo.UpdateHostAdaptation(ctx, installed.ID, true, strings.Repeat("b", 64), false), service.ErrPluginStateChanged)
	require.NoError(t, repo.UpdateHostAdaptation(ctx, installed.ID, true, installed.BinarySHA256, false))
	require.ErrorIs(t, repo.UpdateHostAdaptation(ctx, installed.ID, false, installed.BinarySHA256, false), service.ErrPluginStateChanged)
	current, err := repo.GetByID(ctx, installed.ID)
	require.NoError(t, err)
	assert.True(t, current.HostAdaptationEnabled)
	assert.Equal(t, "encrypted-existing-config", current.ConfigEncrypted)
	assert.Equal(t, service.PluginStateEnabled, current.State)
	require.Len(t, current.Bindings, 1)
	assert.True(t, current.Bindings[0].Enabled)
	artifact, err := repo.GetArtifact(ctx, installed.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("unchanged-plugin-package"), artifact)
	listed, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.True(t, listed[0].HostAdaptationEnabled)
	require.NoError(t, repo.UpdateHostAdaptation(ctx, installed.ID, false, installed.BinarySHA256, true))
}

func TestPluginHostAdaptationPostgresReinstallPreservesSwitch(t *testing.T) {
	repo, ctx := pluginAdaptationPostgresRepository(t)
	installed, bindings := pluginAdaptationInstall(t, ctx, repo)
	require.NoError(t, repo.UpdateConfig(ctx, installed.ID, "encrypted-existing-config", installed.BinarySHA256))
	require.NoError(t, repo.UpdateHostAdaptation(ctx, installed.ID, true, installed.BinarySHA256, false))
	replacement := *installed
	replacement.Version = "1.1.0"
	replacement.Manifest.Version = replacement.Version
	replacement.BinarySHA256 = strings.Repeat("b", 64)
	replacement.ArtifactData = []byte("updated-package")
	replacement.HostAdaptationEnabled = false
	updated, err := repo.Install(ctx, &replacement, bindings)
	require.NoError(t, err)
	assert.Equal(t, installed.ID, updated.ID)
	assert.True(t, updated.HostAdaptationEnabled, "同插件升级保留管理员授权，不能被包里的默认值覆盖")
	assert.Equal(t, "encrypted-existing-config", updated.ConfigEncrypted)
	require.ErrorIs(t, repo.UpdateHostAdaptation(ctx, updated.ID, false, installed.BinarySHA256, true), service.ErrPluginStateChanged)
	require.NoError(t, repo.UpdateHostAdaptation(ctx, updated.ID, false, updated.BinarySHA256, true))
}

func TestPluginHostAdaptationPostgresConcurrentChangeHasSingleWinner(t *testing.T) {
	repo, ctx := pluginAdaptationPostgresRepository(t)
	installed, _ := pluginAdaptationInstall(t, ctx, repo)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			results <- repo.UpdateHostAdaptation(ctx, installed.ID, true, installed.BinarySHA256, false)
		}()
	}
	close(start)
	succeeded := 0
	for range 2 {
		if err := <-results; err == nil {
			succeeded++
		} else {
			require.ErrorIs(t, err, service.ErrPluginStateChanged)
		}
	}
	assert.Equal(t, 1, succeeded)
}
