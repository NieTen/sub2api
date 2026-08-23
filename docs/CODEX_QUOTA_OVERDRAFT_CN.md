# Codex 额度透支功能说明

本文说明本项目移植的 Codex 额度透支能力。该能力默认关闭，支持在账号级控制是否允许透支、限定哪些账号类型参与透支，并支持在分组级配置“本组用尽后交给下一个分组继续透支”的接力链路。

## 功能范围

- 仅处理 OpenAI OAuth Codex 普通文本请求。
- 不处理 `/responses/compact`、远端压缩 v2、图片生成、embedding、count tokens、live 等非普通文本入口。
- 不新增数据库表，状态保存在 `accounts.extra` 的 `codex_quota_overdraft_probe` 字段中。
- 不删除或改写现有 CN Provider、Grok、Antigravity、Ollama Cloud、计费、并发和安全审计逻辑。

## 开启方式

代码默认关闭：

```yaml
gateway:
  codex_quota_overdraft_enabled: false
```

环境变量示例：

```env
GATEWAY_CODEX_QUOTA_OVERDRAFT_ENABLED=false
```

需要启用时改为 `true`。建议先在少量 OpenAI OAuth Codex 账号上观察日志和前端状态，再扩大范围。

## 账号级控制

账号透支控制保存在账号 `extra` 中：

- `codex_quota_overdraft_enabled`：是否允许该账号参与 Codex 透支
- `codex_quota_overdraft_account_types`：允许参与透支的账号类型白名单

当前支持的账号类型只有：

- `oauth`
- `setup-token`

未配置账号类型时，默认按 `oauth` 处理。也就是说，默认只允许 OAuth 账号参与透支，`setup-token` 需要手动勾选。

前端位置：

- 账号创建弹窗
- 账号编辑弹窗
- OpenAI Codex 透支区域会同步展示这两个控制项

## 分组级控制

分组透支控制保存在分组字段中：

- `codex_overdraft_enabled`：该 OpenAI 分组是否参与 Codex 透支
- `codex_overdraft_next_group_id`：当前分组用尽后接力的下一个 OpenAI 分组

规则如下：

- 只有 OpenAI 分组可以启用该功能
- 关闭透支时会自动清空接力分组
- 当前分组用尽后，如果接力分组存在且仍是可用的 OpenAI 分组，就继续往后调度
- A 分组用尽后可以接力到 B 分组；如果 B 也配置了接力分组，还可以继续往下传
- 代码会检测循环，避免 A -> B -> A 这种死循环

前端位置：

- 管理后台的分组创建/编辑弹窗
- 分组列表里新增了 `Codex 透支` 列

## 工作流程

1. 当账号的 `codex_5h_used_percent` 或 `codex_7d_used_percent` 达到 95%，且对应窗口尚未恢复时，普通文本请求进入透支候选。
2. 网关在出站请求的 `input` 末尾追加一组空操作 `custom_tool_call` 与 `custom_tool_call_output`，用于触发 Codex 兼容的额度复核路径。
3. 调度层会临时绕过由 5h/7d 阈值产生的本地预暂停，但不会绕过手工停用、账号错误、上游真实 429、并发、利润控制或其他运行时阻断。
4. 如果透支请求成功，协调器把当前额度周期记为 `passed`，并清理由额度阈值产生的临时暂停。
5. 如果上游返回明确的订阅额度 429，协调器把当前周期记为 `failed`，并暂停到对应恢复时间。
6. 如果探测结果无法确认，状态记为 `inconclusive`，不会写入失败暂停，后续仍按现有调度策略处理。

## 状态字段

前端账号用量接口会返回：

- `codex_quota_overdraft.status`：`pending`、`passed`、`failed`、`inconclusive`、`recovered`
- `codex_quota_overdraft.quota_window`：`5h`、`7d` 或 `multiple`
- `codex_quota_overdraft.attempts` / `limit`：已探测次数与上限
- `codex_quota_overdraft.recover_at`：预计恢复时间
- `five_hour.overdraft_active`、`seven_day.overdraft_active`：窗口是否处于透支期
- `five_hour.overdraft_stats`、`seven_day.overdraft_stats`：透支期内本地请求、Token 和账号成本统计

## 前端展示

账号列表的 OpenAI OAuth 用量区域会显示：

- 透支探测状态：探测中、透支中、已确认限额、探测无法确认、额度已恢复。
- 5h/7d 进度条上方的透支期统计：请求数、Token 数和账号计费金额。
- 鼠标悬停可查看探测模型、原因码、探测时间和预计恢复时间。
- 账号管理列表的状态筛选新增 `透支中`：选择后只显示当前处于 Codex 额度透支期的账号。
- 账号管理列表选择 `正常` 状态时，会排除正在透支的账号；如需查看这类账号，请单独选择 `透支中`。
- 分组管理页会显示每个 OpenAI 分组是否启用透支，以及接力到哪个分组。

## 运维建议

- 默认保持关闭，确认上游行为和账号池规模后再启用。
- 开启后重点观察日志中的 `codex_quota_overdraft_*` 事件。
- 如果遇到异常，可将 `gateway.codex_quota_overdraft_enabled` 或 `GATEWAY_CODEX_QUOTA_OVERDRAFT_ENABLED` 改回 `false` 并重启服务。
- 该功能不会绕过上游真实额度限制；一旦复核确认额度耗尽，账号仍会按恢复时间暂停。

## 验证命令

前端相关测试：

```bash
cd frontend
npm test -- --run \
  src/components/account/__tests__/CreateAccountModal.spec.ts \
  src/components/account/__tests__/EditAccountModal.spec.ts \
  src/components/account/__tests__/openaiCodexOverdraftControls.spec.ts \
  src/components/layout/__tests__/AppLayout.spec.ts \
  src/views/__tests__/HomeView.compact.spec.ts \
  src/views/public/__tests__/StaticEmbedView.spec.ts \
  src/views/admin/__tests__/GroupsView.columnSettings.spec.ts \
  src/views/admin/__tests__/GroupsView.duplicate.spec.ts
```

后端相关测试：

```bash
cd backend
go test ./internal/service -run 'TestCodexQuotaOverdraft|TestAccountTestService_OpenAI' -count=1
go test ./internal/repository -run 'Test.*CodexQuotaOverdraft' -count=1
go test ./internal/server/middleware -run TestEnhanceCSPPolicy -count=1
```

如果环境无法下载 `go.mod` 要求的 Go 工具链，需要先补齐对应 Go 版本。若本机仍在使用 32 位 Go 工具链，`internal/service` 包可能会因为地址空间受限而编译失败，请切换到 64 位 Go 再执行后端测试。
