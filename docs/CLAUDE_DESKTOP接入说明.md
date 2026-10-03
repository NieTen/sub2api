# Claude Desktop 接入说明

适用版本：本站 v0.2.13.2，CC Switch v3.20.4。

## 本次修正

此前网页选择 Claude Desktop 后，实际生成了 Claude Code 的导入链接，随后要求用户使用应用内迁移按钮。CC Switch 仅在 Desktop 供应商列表为空时显示该按钮，已有供应商时无法按照原指引完成配置。

现在两个接入入口均提供 Desktop 专用字段复制向导，不再把 Desktop 请求转成 CLI 导入，也不把“已复制”表示为“已导入”。CLI 等已支持的导入链接保持原行为。

CC Switch v3.20.4 的深链解析器不接受 `app=claude-desktop`。网页无法替它增加协议能力，当前 Desktop 接入需要在应用内保存；并非已实现 Desktop 一键导入。

## 已有供应商时的操作

1. 在本站 API 密钥页面选择 Claude Desktop。
2. 打开 CC Switch 的 Claude Desktop 面板，即带显示器标记的 Claude 图标。
3. 点击右上角「＋」，选择自定义供应商。如果目标已存在，编辑该供应商即可。
4. 逐项复制供应商名称、请求地址、API Key。密钥在网页默认遮罩，复制得到完整值。
5. 根据网页推荐的接入方式配置模型。推荐只依据模型名称，仍需确保当前分组和密钥支持该模型。
6. 保存、启用目标供应商，完全退出并重启 Claude Desktop，新建会话验证。

无需删除任何已有供应商，也无需恢复整库备份来导入单个供应商。

## 模型与路由

- 标准 Claude 角色模型：选择「直连」。可让 Desktop 自动读取 `/v1/models`，或者在「模型列表」点击「添加模型」，填写所选模型 ID。
- 旧式 Claude ID 或其他模型：选择「模型映射」，上游格式选择「Anthropic Messages (原生)」，在 Sonnet 行的「实际请求模型」填入所选模型。留空角色沿用首个已填模型。
- 模型映射需开启 Desktop 本地路由，并在使用期间保持 CC Switch 运行。开关隐藏时，在「设置 → 路由 → 本地路由」开启主页面开关显示。
- 旧式末尾 `[1M]` 标记会从模型 ID 中剥离。只有确认模型支持 1M 上下文时，才在 CC Switch 单独勾选「声明支持 1M」。

## 验证范围

自动化测试覆盖 Desktop 不生成 CLI 深链、字段复制及密钥遮罩、地址子路径、直连与映射、旧式上下文标记、非法输入和状态重置；本地浏览器使用合成密钥验证界面与复制。未使用真实密钥完成 Desktop 对话，不把字段复制成功视为模型调用成功。

## 官方依据

- [v3.20.4 深链解析器](https://github.com/farion1231/cc-switch/blob/v3.20.4/src-tauri/src/deeplink/parser.rs)
- [供应商列表空态判断](https://github.com/farion1231/cc-switch/blob/v3.20.4/src/components/providers/ProviderList.tsx)
- [Desktop 专用表单](https://github.com/farion1231/cc-switch/blob/v3.20.4/src/components/providers/forms/ClaudeDesktopProviderForm.tsx)
- [Desktop 官方指南](https://github.com/farion1231/cc-switch/blob/v3.20.4/docs/user-manual/zh/2-providers/2.6-claude-desktop.md)
