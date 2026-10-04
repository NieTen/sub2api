# UI 缺陷审查与前后对照

## 审查范围与约束

本轮日期：2026 年 10 月 3 日。目的为检查既有定制页面的确定性 UI 缺陷，并以最小影响修正。使用 `ui-ux-pro-max` 技能检查窄屏布局、长文本、弹窗滚动、浅色与暗色可读性、键盘焦点、控件名称及错误反馈，保持原有青色主题和页面组织。

上游差异基准为 `3040209f205472038c1ba745a1bedd2edd9053b1`，本轮工作起点为 `a66e831d7`。审查清单来自以下命令输出中的 83 个 `.vue` 文件（含 29 个 `views` 文件），并补充直接相关组件：

```powershell
git diff --name-only 3040209f205472038c1ba745a1bedd2edd9053b1 HEAD -- frontend/src
```

下文的“修改前”指本轮工作起点，不是原始上游界面。“源码检查”表示核对模板、样式、交互及关联组件，不能代替真实浏览器测量；“浏览器检查”仅用于已完成的本地页面场景。没有确认缺陷的文件保留原样，不为统一形式而修改。本记录不代表所有窗口尺寸、语言、数据或外部嵌入内容均已测试。

本轮调整以局部模板、响应式样式、辅助语义为主。必要交互调整限于弹窗焦点管理、工单面板定位和接入指南的错误状态与键盘焦点显示。保留现有 API、支付和接入协议、业务流程，不加依赖、不重构；审查阶段不发布、不修改版本；后续经用户授权与桌面修正一并纳入 v0.2.13.3。

## 已完成的浏览器前后对照

[打开并排截图对照](UI缺陷前后对照.html)。截图归档于 [UI审查截图-20261003](UI审查截图-20261003/)，使用本地合成数据。

以下数值为指定合成场景的实测，不能推定所有生产数据都有相同尺寸。横向指标为 `scrollWidth`；发送区指标为其底部相对于浏览器视口的位置。

| 场景 | 修改前 | 修改后 | 证据与边界 |
| --- | --- | --- | --- |
| 支付服务商卡片，375px 窗口，长名称及操作按钮 | 横向内容宽度 551px，超出窗口 | 横向内容宽度 375px | 浏览器检查；名称可断行，类型和操作区换行。[修改前](UI审查截图-20261003/provider-before.jpg) · [修改后](UI审查截图-20261003/provider-after.jpg) |
| 账户批量操作栏，375px 窗口 | 横向内容宽度 505px | 横向内容宽度 375px | 浏览器检查；工具栏及按钮换行。[修改前](UI审查截图-20261003/bulk-before.jpg) · [修改后](UI审查截图-20261003/bulk-after.jpg) |
| 邮件模板编辑器，窄屏及长内容 | 横向内容宽度 1841px | 横向内容宽度 367px | 浏览器检查；编辑和预览列允许收缩，长变量断行。[修改前](UI审查截图-20261003/email-before.jpg) · [修改后](UI审查截图-20261003/email-after.jpg) |
| 管理员工单，375px 窗口、50 条上下文 | 发送区底部位于 1521px，需要继续滚动页面 | 发送区底部位于 779px；对应视口高度 812px | 浏览器检查；上下文在面板内滚动，回复区保留在视口内。[修改前](UI审查截图-20261003/ticket-mobile-before.jpg) · [修改后](UI审查截图-20261003/ticket-mobile-after.jpg) |
| 用户工单长会话 | 发送区底部位于 1120px | 发送区底部位于 779px | 浏览器检查；用户与管理员共用会话布局，分别检查，不仅根据共用组件推断 |
| 首页，30 位连续字符站点名称 | 标题裁切 | 标题折行，导航仍可见 | 浏览器检查；未缩小整页字体。[修改前](UI审查截图-20261003/home-long-before.jpg) · [修改后](UI审查截图-20261003/home-long-after.jpg) |
| Claude Desktop 接入指南，暗色、375px 与 1440px | 检查当前指南布局 | 两种宽度均已检查 | 浏览器检查；[手机截图](UI审查截图-20261003/setup-desktop-dark-mobile.jpg) · [桌面截图](UI审查截图-20261003/setup-desktop-dark-wide.jpg)。这不是 CC Switch 实际导入或客户端连接成功的验证 |

另外完成支付配置表单、群消息长姓名的同尺寸对照，登录显隐按钮名称与状态检查，工单 768×1024 和 1440×900 暗色检查。截图见归档目录；这些检查不等于所有同类页面、全部表单状态均经过实测。

## 逐页审查记录

文件均相对于 `frontend/src/`。下列 29 个 `views` 文件覆盖本次上游差异清单，包括其中的页面局部组件。“保留”仅表示本轮源码检查未确认需要修改的缺陷。

| 页面及文件 | 修改前或检查重点 | 本轮处理结果 | 验证方式 |
| --- | --- | --- | --- |
| 首页 `views/HomeView.vue` | 长站名挤占导航，连续字符标题裁切；暗色厂商文字 | 导航列可收缩、标题可折行，补充暗色文字颜色 | 源码检查；长站名浏览器前后对照 |
| 密钥用量 `views/KeyUsageView.vue` | 用量内容及窄屏容器 | 未确认需改缺陷，保留 | 源码检查 |
| 账户管理 `views/admin/AccountsView.vue` | 批量操作、账户编辑和检测弹窗 | 页面本身保留；相关批量操作换行、表单名称修正见组件表 | 源码检查；批量栏 375px 浏览器对照 |
| 邮件群发 `views/admin/BulkEmailsView.vue` | 窄屏横向溢出、暗色提示文字 | 操作区换行；结合共用容器和编辑器修正收缩；补暗色提示及错误文字 | 源码检查 |
| 成员绑定 `views/admin/CommunityMembersView.vue` | 搜索栏最小宽度、长 TG 名称、状态卡选中语义 | 手机标签分列、搜索框可收缩，长名称折行，补选中状态与焦点 | 源码检查；不据截图文件名推定全部成员状态已实测 |
| 社群设置 `views/admin/CommunitySettingsView.vue` | 表单组最小宽度、手机留白、暗色提示 | 表单组可收缩，手机内边距缩减，补暗色文字 | 源码检查 |
| 分组管理 `views/admin/GroupsView.vue` | Codex 透支开关与下一分组控件名称 | 复用原翻译补可访问名称和标签关联；保留业务 | 源码检查 |
| 模型检测 `views/admin/ModelDetectionView.vue` | 暗色辅助文字、表格与操作区可读性 | 补暗色说明、状态和证据文字；保持检测流程 | 源码检查 |
| 插件管理 `views/admin/PluginsView.vue` | 灰度比例输入与可见标签未关联 | 为标签和输入补成对 `for`、`id` | 源码检查 |
| 系统设置 `views/admin/SettingsView.vue` | 工单回复邮件开关没有可访问名称 | 使用既有标题翻译补 `aria-label` | 源码检查 |
| 通知与支持设置 `views/admin/SupportSettingsView.vue` | 暗色加载、帮助和错误反馈 | 补对应暗色文字颜色，保持配置逻辑 | 源码检查 |
| 运维日志 `views/admin/ops/components/OpsSystemLogTable.vue` | 长日志、OKPay 诊断内容和 JSON 滚动 | 原有内部滚动可承载内容，本轮未确认需改缺陷，保留 | 源码检查 |
| 管理员订单 `views/admin/orders/AdminOrdersView.vue` | 长订单号、退款原因、筛选框名称 | 订单值可收缩并断行，原因换行，筛选框补名称 | 源码检查 |
| 支付看板 `views/admin/orders/AdminPaymentDashboardView.vue` | 排行榜长邮箱挤占金额 | 排行项可换行、长邮箱断行，保持统计数据逻辑 | 源码检查 |
| 邮件模板 `views/admin/settings/EmailTemplateEditor.vue` | 编辑和预览列被长内容撑宽、长变量标签 | 列与标题可收缩，变量断行，移动内边距与焦点修正 | 源码检查；浏览器前后对照 |
| 首页模型编辑 `views/admin/settings/HomeModelsEditor.vue` | 模型列表与编辑表单 | 未确认需改缺陷，保留 | 源码检查 |
| 邮箱验证 `views/auth/EmailVerifyView.vue` | 长邮箱无法及时换行 | 邮箱支持连续字符断行 | 源码检查 |
| 登录 `views/auth/LoginView.vue` | 密码显隐图标缺少名称、状态及明显焦点 | 补可访问名称、按下状态和焦点；仅调整该图标触摸区域 | 源码检查；375px 浏览器显隐状态检查 |
| 注册 `views/auth/RegisterView.vue` | 密码显隐控件名称、焦点及触摸区域 | 补密码显隐语义，调整相关图标区域 | 源码检查 |
| 用户社群 `views/community/CommunityView.vue` | 长 TG 名称、暗色说明、状态文字 | 长名称可断行，暗色说明可读；保留入群、核验流程 | 源码检查 |
| 公共嵌入 `views/public/StaticEmbedView.vue` | 外层嵌入容器 | 未确认外层需改缺陷，保留；不保证任意被嵌入页面的布局 | 源码检查 |
| 工单 `views/support/TicketsView.vue` | 长上下文使发送区落到页面底部，手机切换工单后面板位置不稳 | 会话高度随视口约束，上下文与编辑区分别滚动；回复按钮保留，详情完成后定位且取消过时会话定位 | 源码检查；管理端及用户端浏览器前后对照 |
| 自定义页面 `views/user/CustomPageView.vue` | 自定义内容承载容器 | 未确认外层需改缺陷，保留；外部内容不作为本轮通过项 | 源码检查 |
| 用户仪表盘 `views/user/DashboardView.vue` | 页面及侧栏接入指南 | 页面保留；指南子模块修复不可用状态提示和重绘后的键盘焦点 | 源码检查；指南显示回归另列 |
| API 密钥 `views/user/KeysView.vue` | 一键接入弹窗、长密钥名称、客户端指南 | 页面保留；弹窗焦点及名称折行修正；Desktop 指南未改协议 | 源码检查；Desktop 指南暗色375px／1440px浏览器检查 |
| 支付二维码 `views/user/PaymentQRCodeView.vue` | 二维码尺寸、长信息与操作区 | 本轮未确认需改缺陷，保留 | 源码检查 |
| 支付结果 `views/user/PaymentResultView.vue` | 长订单号挤出标签、页面留白 | 订单值收缩和断行，局部留白调整；恢复及支付状态逻辑保留 | 源码检查 |
| 充值 `views/user/PaymentView.vue` | 长账户及套餐名、套餐文案、错误提示、续费关闭按钮 | 名称和说明换行、价格区可换行，文案内部滚动；错误补 `alert`，关闭按钮补名称及焦点 | 源码检查 |
| 我的订单 `views/user/UserOrdersView.vue` | 状态筛选和退款原因输入缺少名称 | 复用现有翻译补可访问名称 | 源码检查 |

关联扩展页面：`views/admin/orders/AdminPaymentPlansView.vue` 仅补售卖开关的 `switch` 语义、名称与状态；`views/admin/orders/PlanEditDialog.vue` 仅补表单名称及售卖开关语义。二者为源码检查，不标记为浏览器验证。

## 关联组件清单

以下路径相对于 `frontend/src/`。合并列出的文件具有相同处理结论；每个上游差异组件均保留在清单中。

### 外壳、账户与模型检测

| 组件 | 修改前后与结论 | 验证方式 |
| --- | --- | --- |
| `App.vue` | 检查应用容器，未确认需改缺陷，保留 | 源码检查 |
| `components/account/AccountUsageCell.vue`、`CodexOverdraftStats.vue`、`CodexOverdraftStatus.vue`、`UsageProgressBar.vue` | 检查定制用量及透支显示，未确认需改缺陷，保留 | 源码检查 |
| `components/account/CreateAccountModal.vue`、`EditAccountModal.vue` | 为新增 Codex 透支开关补可访问名称 | 源码检查 |
| `components/admin/AdminComplianceDialog.vue` | 显式启用弹窗焦点约束 | 源码检查 |
| `components/admin/account/AccountBulkActionsBar.vue` | 原横排在小窗口拥挤；操作栏和按钮可换行 | 源码检查；375px 浏览器对照 |
| `components/admin/account/AccountTableFilters.vue`、`BatchAccountTestModal.vue` | 未确认需改缺陷，保留 | 源码检查 |
| `components/admin/account/BatchScheduledTestModal.vue` | 为模型、计划表达式及记录数补标签关联 | 源码检查 |
| `components/admin/account/ImportDataModal.vue` | 输入输出补名称，结果操作区允许换行 | 源码检查 |
| `components/admin/account/ManualModelDetectionDialog.vue`、`ModelDetectionModelPicker.vue`、`ModelDetectionRunTable.vue` | 补暗色说明、表头、状态和空态文字 | 源码检查 |
| `components/common/BaseDialog.vue` | 长标题断行、关闭按钮名称及焦点；仅顶层弹窗处理键盘操作；处理无可聚焦项、禁用项和关闭后的焦点恢复 | 源码检查；顶层键盘和焦点恢复回归；长标题浏览器检查 |
| `components/common/VersionBadge.vue` | 未确认需改缺陷，保留 | 源码检查 |
| `components/common/VipCommunityPrompt.vue` | 提示中的连续字符长群名允许断行 | 源码检查 |
| `components/layout/AppHeader.vue` | 未确认需改缺陷，保留 | 源码检查 |
| `components/layout/AppSidebar.vue` | 通知设置原来匹配平级工单、群发和社群路径，造成双重高亮；改为仅自身路径与支持设置别名匹配 | 源码检查；定向导航回归 |
| `components/layout/AuthLayout.vue` | 长站名和页脚允许断行，补暗色辅助文字 | 源码检查 |

### 社群、工单与接入指南

| 组件 | 修改前后与结论 | 验证方式 |
| --- | --- | --- |
| `components/community/CommunityAvatar.vue` | 详情入口头像按钮由 40px 调整为 44px，补点击光标，保留不收缩规则 | 源码检查 |
| `components/community/CommunityMemberDialog.vue` | 长姓名可断行，补暗色文字和弹窗焦点约束 | 源码检查 |
| `components/community/CommunityMessageMedia.vue` | 音频控件限制在可用宽度，下载入口补焦点及触摸区域 | 源码检查 |
| `components/community/CommunityMessagesPanel.vue` | 消息容器可收缩，发送者长名可断行；滚动区域可获键盘焦点，补暗色提示和明确焦点 | 源码检查；375px 长姓名浏览器对照 |
| `components/support/CommunicationsLayout.vue` | 允许共用内容区域收缩，避免子页面按内容最小宽度撑开 | 源码检查；随管理员工单浏览器检查 |
| `components/support/SupportComposer.vue` | 增加仅由工单使用的紧凑编辑显示，保留默认群发编辑高度；图片移除按钮留在容器内并补焦点和合适触摸区域 | 源码检查；随工单浏览器对照 |
| `components/support/SupportImage.vue` | 图片不超过可用宽度，补暗色文案及预览弹窗焦点 | 源码检查 |
| `components/keys/CcSwitchImportModal.vue` | 显式启用弹窗焦点约束 | 源码检查 |
| `components/keys/UseKeyModal.vue` | 密钥及分组长名称折行，客户端选择补状态和焦点 | 源码检查 |
| `components/keys/ClaudeDesktopSetup.vue`、`QuickSetupModal.vue`、`SetupCodeBlock.vue` | 未确认本轮需改缺陷，保留当前指南和命令实现；不虚构 Desktop JSON 粘贴入口或深链支持 | 源码检查；Desktop 指南暗色375px／1440px检查 |
| `components/user/dashboard/SidebarDashboard.vue` | 页面组件保留，指南修正位于下列关联脚本 | 源码检查 |
| `components/user/dashboard/sidebar-overlay.js`（关联脚本） | TypeSafe 与无效服务地址显示明确不可用提示；异步重绘保留指南内键盘焦点，不抢外部焦点 | 源码检查；定向回归包含已知基线失败，见测试记录 |

### 支付组件

| 组件 | 修改前后与结论 | 验证方式 |
| --- | --- | --- |
| `components/payment/ProviderCard.vue` | 长名称、类型、操作区原来撑宽；允许收缩和换行，类型按钮补选中状态及焦点，改善未选中暗色文字 | 源码检查；375px 浏览器前后对照 |
| `components/payment/PaymentProviderDialog.vue` | 手机表单改为单列、较宽窗口保留原分列；回调地址与固定路径手机堆叠；字段、显隐按钮、折叠操作补名称、状态和焦点 | 源码检查；375px 浏览器对照；语义定位及实际提交 URL 回归 |
| `components/payment/PaymentProviderList.vue`（关联扩展） | 标题操作区允许换行，刷新按钮补名称 | 源码检查 |
| `components/payment/PaymentMethodSelector.vue`、`AmountInput.vue` | 选择项补选中状态和焦点，自定义金额补可访问名称 | 源码检查 |
| `components/payment/OkpayDiagnosticPanel.vue` | 长服务商名称、错误信息可断行 | 源码检查 |
| `components/payment/PaymentStatusPanel.vue` | 成功状态长订单号允许收缩、断行，标签保留 | 源码检查 |
| `components/payment/UsdtExchangeDetails.vue` | 长兜底原因换行 | 源码检查 |
| `components/payment/OrderTable.vue`、`PaymentQRDialog.vue`、`Trc20PaymentDetails.vue`、`UsdtExchangePreview.vue`、`UsdtRateSettings.vue` | 检查列表、二维码、链上地址、金额和费率说明；未确认本轮需改缺陷，保留 | 源码检查 |
| `components/payment/OrderStatusBadge.vue`、`ToggleSwitch.vue`、`SubscriptionPlanCard.vue`、`StripePaymentInline.vue`（关联扩展） | 未确认需改缺陷，保留 | 源码检查 |
| `components/admin/payment/AdminOrderDetail.vue` | 长退款原因可收缩并换行 | 源码检查 |
| `components/admin/payment/AdminOrderTable.vue` | 搜索与筛选控件补名称 | 源码检查 |
| `components/admin/payment/AdminRefundDialog.vue` | 日期、扣款选项区域可换行，金额及原因输入补名称 | 源码检查 |
| `components/admin/payment/OrderStatsCards.vue` | 小窗口使用单列，较宽窗口保留两列／四列 | 源码检查 |
| `components/admin/payment/TopUsersLeaderboard.vue` | 排名标识保持宽度，长邮箱和金额可断行 | 源码检查 |
| `components/admin/payment/UsdtRateChart.vue` | 错误及兜底原因长内容可换行 | 源码检查 |
| `components/admin/payment/PaymentMethodChart.vue`、`DailyRevenueChart.vue`（后者为关联扩展） | 未确认需改缺陷，保留 | 源码检查 |

## 回归记录与已知边界

本节记录实际运行结果；未运行整个仓库的全部测试，不将定向通过扩写为全项目通过。

| 检查 | 已知结果 |
| --- | --- |
| 本轮支付模块 21 个修改模板的解析与编译 | 通过；该批生产修改的脚本内容与本轮起点一致，仅模板层变化。此结论不包含另外修改交互的通用弹窗、工单和指南脚本 |
| `AppSidebar.overlay.spec.ts` 与 `AppSidebar.spec.ts` | 单次定向运行 2 文件、35 项通过，覆盖当前页面、别名、子路径及相似路径的高亮 |
| `PaymentProviderDialog.spec.ts` | 34 项定向通过；回调与返回地址改为 `aria-label` 语义定位，保留原有四组实际 URL 提交断言，未改业务以满足测试 |
| `components/user/dashboard/__tests__/sidebar-overlay.spec.ts` | 存在 Codex `/v1` 断言失败，已在本轮起点 HEAD 复现，为已知基线问题。本轮不通过修改接入协议掩盖该失败；不能宣称该套件或全量回归全绿 |
| 首页、认证、接入与支付统一回归 | 34 文件、566 项：初次 562 项通过，4 项因旧测试按布局类名寻找回调输入而失败；改为新补充的语义名称定位后，支付配置 34 项重跑通过。所有原 URL 提交断言保留 |
| 账户、分组、模型检测与插件 | 9 文件、171 项通过 |
| 社群、群发、邮件模板与编辑器 | 10 文件、121 项通过 |
| BaseDialog 与工单时序 | 5 文件、18 项通过，包含新增 8 项弹窗键盘回归和 3 项工单竞态回归 |
| 导航、CCS 选择等其他定向回归 | 见上述导航结果；CCS 与弹窗相关 6 文件共 73 项通过，与其他行存在重复覆盖，不将数字直接相加 |
| 指南显示与焦点新增回归 | 新增 9 项通过；整个指南套件 36 通过、1 个基线失败，保留真实结果 |
| 定向 ESLint | 66 个已有变更文件及 2 个新增测试文件通过 |
| `pnpm run build` | 通过：包含 3 项语言键完整性测试、`vue-tsc -b` 类型检查、Vite 生产构建。仍存在原有浏览器数据版本及分包体积提示 |
| `git diff --check` | 通过 |

上述两个名称相似的侧栏测试属于不同模块：导航高亮回归通过，不代表用户仪表盘接入指南套件全部通过。自动化与本地合成 UI 检查也不等于真实支付、邮件投递、TG 操作或第三方客户端导入成功。

## 后续 AI 页面变更要求

后续每次由 AI 新增或修改页面，均应使用 `ui-ux-pro-max` 技能完成与变更相称的 UI 审查，并在本记录或对应的中文审查文档中写明前后对照：

1. 明确本次变更文件、原有设计和用户目标；优先沿用现有组件、主题和业务边界。
2. 先定位可复现缺陷，再做局部修正；没有确认问题的页面保留，避免无差别改造。
3. 检查 375px 窄屏、长名称／地址、弹窗滚动、暗色、键盘焦点、控件名称和错误反馈；按实际改动补查桌面宽度。
4. 区分源码检查、自动化检查和浏览器检查。浏览器实际检查过的场景保留前后截图及必要的尺寸数据，没有执行的检查明确标记。
5. 运行适当的定向回归，记录实际结果和基线失败；不得删除断言或改变业务协议来掩盖与 UI 无关的失败。
6. 更新页面与组件清单，注明修改前、修改后、保持原样的原因及未验证边界。发布沿用当次对话的有效授权与项目版本约定；本轮仅完成本地审查和修正。
