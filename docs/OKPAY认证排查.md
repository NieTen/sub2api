# OKPay 认证排查

## 当前状态与证据

`v0.2.11.5` 将 OKPay 默认签名改为 HMAC-SHA256，并保留显式旧协议选项。

2026 年 10 月 1 日，用户运行本机诊断工具并提供真实只读结果。同一组凭据访问同一 `https://api.okaypay.me/shop/balance` 接口：

| 请求 | HTTP 状态 | 业务结果 |
| --- | --- | --- |
| 旧 MD5 表单 | 200 | 身份认证失败 |
| 旧 MD5 JSON | 200 | 身份认证失败 |
| HMAC-SHA256 表单 | 200 | 成功，业务代码 `200` |

商户 ID 和 Token 的空白、不可见字符检查结果均为 `false`。这次对照已确认新协议对该商户的余额认证有效。用户随后确认后台当前配置和 HMAC 认证均通过，并先后报告下单业务代码 `400`、升级 `v0.2.11.6` 后的「OKPay 返回或回调地址须为 HTTPS 地址」。两种提示对应不同阶段，应按该次请求的日志分别定位；实际支付、查单和回调仍待验证。

这里的 `400` 是上游响应中的业务代码，不是上游 HTTP 状态码，也不能证明凭据错误。只读余额认证不包含下单专属参数，认证通过不代表下单已经通过。

`v0.2.11.6` 的「返回或回调地址须为 HTTPS 地址」来自本站 `CreatePayment` 的本地校验，发生在签名和 HTTP 请求之前，不能指明两个地址中的哪一个失败。它也可能由缺少域名、地址格式、内嵌用户名密码或控制字符引起，不能仅按文字断定是 HTTP。旧版调试日志只覆盖后续网关请求流程，因此该错误可能没有对应的 `OKPay debug`。`v0.2.11.7` 补齐此处日志，并分别提示失败字段和固定原因。

随后用户提供实例 `#1` 的日志：返回地址使用了请求传入的 HTTP/IP 地址，`request_sent=false`；实例回调地址 `https://zzzai.pro` 对应的 HTTPS 校验有效。结合当时客户端地址优先的选择顺序，已定位到 HTTP 返回地址覆盖后台 HTTPS 配置，导致本站在发出请求前拒绝下单。`v0.2.11.8` 调整 OKPay 返回地址选择顺序，优先使用本次实际选中实例的非空配置；这不等同于确认实际支付或入账成功。

用户随后完成真实 ABCD 下单对照：322 字节长返回地址的 A、C 组均返回业务代码 `400`，32 字节短返回地址的 B、D 组均创建成功；有无 POST `status=0` 未改变各自结果。证据指向长短返回 URL 与查询参数的组合差异，尚不能区分纯长度和查询内容的影响。本次 `v0.2.11.9` 保留 `status=0`，仅为 OKPay 签发固定 102 字符的紧凑恢复令牌；缩短后的正式返回地址仍需真实下单、付款、回调和入账验证。

`payment gateway error: OKPay 返回业务失败状态（身份认证失败……）` 表示程序收到了上游业务拒绝。本站将支付网关失败映射为 HTTP 503，不能据此认定网关不可访问，也不能仅凭提示认定商户 ID 或 Token 填错。

本次协议适配依据用户指定的第三方 Dujiao-Next 参考项目：

- [协议升级提交](https://github.com/dujiao-next/dujiao-next/commit/ad9b7e2d2902650d9c2b57b89dce31b5bf6aeb80)、[问题报告 #294](https://github.com/dujiao-next/dujiao-next/issues/294) 与 [合并请求 #293](https://github.com/dujiao-next/dujiao-next/pull/293) 记录了旧 MD5 请求出现“身份认证失败”的同类现象。
- [固定版本签名实现](https://github.com/dujiao-next/dujiao-next/blob/d2e44618de03068cbbe874a23a6d7c6a249a79ec/internal/modules/payment/infrastructure/gateway/okpay/okpay.go#L558) 和 [固定测试向量](https://github.com/dujiao-next/dujiao-next/blob/d2e44618de03068cbbe874a23a6d7c6a249a79ec/internal/modules/payment/infrastructure/gateway/okpay/okpay_test.go#L11) 用于核对签名结果。

这些参考资料属于第三方实现与离线证据，不能冒充官方新版文档。参考项目未实现 `/balance`、`/checkDeposit`；本项目已将共同签名规则用于余额诊断、下单及查单。用户实测已补充余额认证和短返回地址创建测试订单的证据，正式支付、查单和回调仍待验证。

原附件 `OkayPay (1).php` 已在 PHP 7.4.3 中完成离线对照：十组正常商户 ID 的签名、签名原文及解析后的字段与旧 MD5 实现一致，原 PHP 的 `checkSign` 接受 Go 发出的测试请求。这只证明旧协议兼容，不证明旧协议仍被当前网关接受。非空 Token 在配置保存和新协议签名时保持原值，不改写 `+`、`%` 或首尾空白。

## 协议选择与历史订单

| 项目 | 默认 HMAC-SHA256 | 显式旧 MD5 |
| --- | --- | --- |
| 实例配置 | `signatureAlgorithm=hmac_sha256`；缺字段时使用此值 | `signatureAlgorithm=legacy_md5` |
| 公共认证字段 | `id`、Unix 秒 `timestamp`、随机 `nonce`、`sign` | `id`、`sign` |
| 签名结果 | 大写 64 位 HMAC-SHA256 | 大写 32 位 MD5 |
| Token 用法 | 原样用作 HMAC 密钥 | 拼接 `&token=` 后参与 PHP URL 解码 |
| 零值处理 | 保留 `0`、`false`，仅过滤空字符串和空值 | 按原 PHP 规则过滤假值 |
| 嵌套字段 | 对象使用点号、数组使用索引，全部扁平键排序 | 方括号展开，外层排序 |
| 请求及回调 | 请求为表单，回调要求 JSON 对象 | 请求为表单，兼容 JSON 和旧 PHP 嵌套表单回调 |

新订单在服务商快照的 `signature_algorithm` 字段记录所选协议。查单和回调使用订单快照；历史订单缺少该字段时按 `legacy_md5` 处理，不能因修改实例配置而改用 HMAC 验签。

实现已兼容成功响应中的 `status=success` 以及业务代码 `200`、`10000`，仍校验响应结构、商户、币种、订单和金额。签名失败不会自动降级到另一算法；创建支付失败不会自动重试，避免重复下单。

## 后台三组只读诊断

进入「系统设置 → 支付设置 → 管理服务商」，在已保存的 OKPay 实例上点击「诊断认证」。停用实例也可诊断；编辑后需先保存，诊断不读取尚未保存的表单。

服务端读取并固定同一份实例配置，以相同商户 ID 和 Token 向 `/balance` 各发送一次请求，总时限为 35 秒：

| 标识 | 请求含义 |
| --- | --- |
| `current` | 使用已保存的签名算法及当前 Go 传输方式 |
| `php_reference` | 使用原 MD5 算法及 Go 模拟的附件 PHP 表单、请求头 |
| `hmac_sha256` | 使用 HMAC-SHA256 签名及表单请求 |

诊断不创建订单、不转账、不自动重试。返回固定 `reason` 分类和可安全展示的可选 HTTP 状态、业务代码；不返回 Token、签名、余额、请求原文或上游完整响应。页面按固定分类显示中文说明，不直接展示上游文本。

| 结论 | 可以确认的范围 |
| --- | --- |
| `both_authenticated` | 三组均通过，只能证明当前配置在此服务器通过余额接口认证 |
| `hmac_only_authenticated` | HMAC 通过、旧 MD5 被拒绝，支持余额接口使用新协议的判断 |
| `legacy_only_authenticated` | 旧 MD5 通过、HMAC 被拒绝，需要继续核对该商户的协议要求 |
| `both_rejected` | 三组均被业务拒绝，应结合固定原因分类继续排查，不能直接归因于凭据 |
| `inconclusive` | 存在网络、HTTP、响应格式异常，或同一协议的两组结果矛盾，暂不能比较算法 |

余额诊断不能覆盖下单接口的金额、商品名及回调地址等专属字段。`php_reference` 由 Go 发出，不代表真实 PHP/cURL 验证。实例编号（如 `#4`）是本站编号，不参与签名；多个实例并存时，应核对失败订单的 `provider_instance_id`。

## 下单失败的临时调试日志

`v0.2.11.6` 增加按 OKPay 实例控制的「调试日志」，`v0.2.11.7` 补充下单前的本地校验记录。开关默认关闭；旧配置缺少 `debugLogging` 时也关闭。用于查看脱敏后的参数摘要、本地校验原因和上游错误原因，不改变正常支付流程或自动重试策略。

1. 在「系统设置 → 支付设置 → 管理服务商」编辑实际用于下单的 OKPay 实例，将「调试日志」改为「临时开启」（`debugLogging=true`），保存实例。若新版表单已提示某项基础地址无效，先按提示修正该项再保存。
2. 在「运维监控 → 系统日志」检查「运行时日志配置（立即生效）」中的「级别」。通常使用 `info` 即可，无需全局 `debug`；若当前为 `error`，先改为 `info` 并点击「保存并应用」。
3. 重新发起一次正常支付以复现下单失败。仅点击「诊断认证」仍只查询余额，不能取得下单失败的原因。
4. 回到「运维监控 → 系统日志」，选择对应时间范围，在「关键词」输入 `OKPay debug` 并搜索，展开失败日志中的「OKPay 调试详情」。详情按普通文本 JSON 显示，可点击「复制」用于排查。筛选级别选「全部」或 `warn`，避免只筛选 `error` 而漏掉日志；核对 `instance_id` 与实际下单实例一致。
5. 先看 `stage`。若为 `validation`，按下表查看 `validation_field`、`validation_reason`，并同时检查 `request.return_url` 与 `request.callback_url` 的摘要，按「地址来源与修正」处理。若为 `upstream`，再看 `request_sent`、`http_status`、安全业务代码及脱敏后的 `upstream_messages`，不要把业务代码当成 HTTP 状态。
6. 修正后再发起一次正常支付确认结果。复现完成后，将该实例「调试日志」恢复为「关闭（默认）」并保存；临时调整过运行时日志级别时，也恢复原设置。

调试请求失败记为 `Warn`，会写入后台系统日志；成功记为 `Info`，不写入后台系统日志，应查看容器日志或已启用的文件日志。正常的全局 `info` 级别即可生成两者。后台详情只选取约定的安全字段，包含实例、接口、HTTP 状态、业务代码、请求摘要及脱敏错误原因，不记录 Token、`sign` 值、完整 URL 或响应 `data`，也不会将上游原文直接返回给付款用户。

日志中的 `result=success` 仅表示该请求的 HTTP 和业务状态检查通过，不代表用户已经付款或账户已经入账。

### 日志字段判读

| 字段或组合 | 含义与处理 |
| --- | --- |
| `stage=validation`、`result=validation_failed`、`request_sent=false`、`http_status=0` | 下单在本地校验失败，尚未签名或发送 HTTP 请求。此处 `0` 不是上游返回的状态码，应先修正本地参数。 |
| `validation_field=return_url` | 最终同步返回地址失败；结合实际实例的 `returnUrl` 配置与地址选择顺序排查，不能仅凭 `source=request` 认定来自浏览器。 |
| `validation_field=callback_url` | 实际异步回调地址失败；常规网页下单使用实例保存的 `notifyUrl`。 |
| `validation_field=amount` / `unique_id` | 金额或商户订单号校验失败，查看原因与说明；不是地址或余额认证问题。 |
| `validation_reason` / `validation_message` | 固定原因代码及中文说明，不包含完整地址或请求原文。顶层字段记录首先遇到的失败；两个地址各自的摘要仍可揭示其他地址问题。 |
| `stage=upstream` | 进入网关请求处理流程；本地校验来源字段不在此阶段记录。是否已尝试发送请求仍以 `request_sent` 为准，不能只看 `stage` 推断上游收到请求。 |
| `http_status` / `business_code` | 分别是 HTTP 状态与响应中的安全业务代码。`http_status=0` 表示没有已记录的 HTTP 响应状态；具体原因结合阶段和结果判断。 |
| `request.return_url` / `request.callback_url` | 两个实际地址的安全摘要；包含 `present`、`bytes`、`https`、`host`、`path_bytes`、`query_bytes`、`query_params`，以及以下校验信息，不包含完整 URL、路径或查询值。 |
| 地址摘要中的 `scheme` / `valid` | `scheme` 分类为 `http`、`https`、`relative` 或 `other`；`valid` 表示非空地址通过当前 HTTPS 校验。空地址的 `valid=false` 本身不代表此次失败，须结合 `present` 和失败字段。 |
| 地址摘要中的 `validation_reason` | 该地址的固定校验原因；通过校验或地址为空时为空字符串。两个地址都不合格时，可分别查看。 |
| 地址摘要中的 `source` | **仅 Provider 层本地校验失败日志提供**：`request` 指服务层传给 Provider 的最终请求，可能已经选用了实例配置，不等于必然来自浏览器；`provider_config` 指 Provider 内部使用实例配置兜底，`empty` 为两者均为空。上游阶段不提供此字段。 |
| 地址摘要中的 `has_userinfo` / `has_control_chars` | 标记解析出的用户名密码部分，以及被检测到的换行、回车或空字符。解析失败时其他摘要可能不完整，应优先看 `validation_reason`。 |

| `validation_reason` | 处理方法 |
| --- | --- |
| `scheme_not_https` | 检查实际来源是否为 HTTP 或缺少协议，改为已经配置 TLS 的完整 `https://` 地址。 |
| `missing_host` / `parse_error` | 补齐域名，检查地址格式、端口和转义字符；重新填写相应基础地址。 |
| `userinfo_not_allowed` / `control_characters` | 移除地址中的账号密码或换行、回车、空字符，重新填写。 |
| `invalid_amount` / `amount_precision` | 金额必须有效且大于零，最多两位小数；检查订单金额计算。 |
| `missing_order_id` | 商户订单号为空，检查本站订单创建流程。 |

### 地址来源与修正

`v0.2.11.8` 仅调整 OKPay：服务层在生成恢复凭据之前，先选用本次实际选中实例中非空的 `returnUrl`。它是管理员保存的可信返回地址，支持合法 HTTPS 子路径及普通查询参数；配置非空但无效时明确报错，不静默回退。只有配置为空时，才对客户端传入地址执行同源、规范结果页路径校验。

确定返回目标后，系统保留普通查询参数，移除原有 `order_id`、`out_trade_no`、`resume_token`、`status` 和片段，再生成新的恢复 Token 并追加当前订单参数。客户端的 HTTP/IP 地址不会再覆盖已保存的 HTTPS 配置，其他服务商的地址选择方式不变。后台表单录入的仍是**基础地址**，由系统拼接固定路径，因此表单基础地址仍不接受查询参数；这与服务端兼容已保存完整 `returnUrl` 的普通查询参数是不同层次的规则。

非空配置无效时，服务层返回 `PAYMENT_PROVIDER_MISCONFIGURED`，提示「OKPay 实例配置的返回地址（returnUrl）无效：…」，元数据包含 `provider`、`instance_id`、`field=return_url`、`source=provider_config`。此错误在生成恢复 Token 前发生，不会进入 Provider，也不会生成对应的 `OKPay debug`；应在服务端常规日志搜索 `[PaymentService] Resolve payment return URL failed`，核对提示的实例配置。这里的错误元数据与「日志字段判读」中的 Provider 地址摘要属于不同层次。

常规下单的异步回调继续使用实例保存的 `notifyUrl`。实例 `#1` 本次日志已证明回调地址有效，应重点核对同步返回地址及实际下单实例。

1. 进入「系统设置 → 支付设置 → 管理服务商」，编辑实际下单的 OKPay 实例，确认「同步跳转地址」为本站可访问的 HTTPS 基础地址，例如 `https://zzzai.pro`。右侧固定路径由系统拼接，不要重复填写 `/payment/result`；「异步通知地址」同理不要重复填写 `/api/v1/payment/webhook/okpay`。
2. 若出现 `PAYMENT_PROVIDER_MISCONFIGURED`，按元数据中的实例编号修正非空的完整 `returnUrl`，并查询上述服务端常规日志；不能期待自动回退到浏览器地址。只有该配置确实为空时，再检查客户端地址是否满足同源及规范结果页校验，并使用本站 HTTPS 页面下单。
3. 编辑表单会分别校验两个基础地址：要求完整 HTTPS 地址，允许已配置的反代子路径，禁止账号密码、查询参数、片段、控制字符和反斜杠。基础地址输入框留空会使用当前管理页面的 origin 生成并保存配置，不等于最终 `returnUrl` 为空；管理页面为 HTTP 时应手动填写正确的 HTTPS 基础地址。
4. 如需基础子路径，例如 `https://example.com/sub2api`，确认反向代理确实将拼接后的回调与返回路径转发到本站。保存后重新发起一次正常支付验证；解读后续 `source=request` 时，应按“服务层传给 Provider 的最终请求”理解，不再直接归因于浏览器。

Docker 部署也可读取容器日志。以下使用默认容器名；自定义部署需替换为实际容器名：

```text
docker logs --since 10m --tail 500 sub2api
```

查找其中的 `OKPay debug` 条目。调试开关在服务商实例中保存，日志级别可通过上述运行时入口调整，无需增加新的环境变量、脚本或接口。

## 请求已发出但上游返回业务 400

用户在 `v0.2.11.8` 提供的新日志显示：实例 `#1` 的返回与回调地址都通过 HTTPS 校验，`request_sent=true`、`http_status=200`、`stage=upstream`，上游业务代码为 `400`，消息仅为「订单创建失败」。这确认先前的 HTTP 返回地址问题已解决，但上游没有返回具体字段或失败原因。继续增加本站日志也不能凭空获得上游数据库或商户侧的错误详情。

对照附件和 [Dujiao-Next 固定版本的下单构造](https://github.com/dujiao-next/dujiao-next/blob/d2e44618de03068cbbe874a23a6d7c6a249a79ec/internal/modules/payment/infrastructure/gateway/okpay/okpay.go#L188-L207)，本机工具对以下两项差异进行了真实对照：

- 本站显式发送 `status=0`；参考项目没有设置该可选参数时省略它。旧 PHP 的 `array_filter` 也会过滤字符串 `"0"`，旧文档则说明默认状态为 `0`。此次 A/C 与 B/D 对照均表明，省略这个 POST 参数没有改变结果，因此正式请求继续保留 `status=0`。它与返回 URL 中的 `status=success` 查询参数是两处不同字段。
- 长返回地址为 `322` 字节，包含恢复令牌等四个查询参数；短返回地址为 `32` 字节，不含查询参数。此次只有短地址创建成功，但长度和查询内容同时变化，仍未分离这两个因素。现有文档和公开源码没有说明 `255` 或 `256` 字节限制，不能把常见数据库字段长度当成已确认的 OKPay 限制。

金额 `1.51 USDT`、21 字符商户订单号、13 字符名称也没有现成证据证明违反限制。HMAC 余额认证通过不能证明所有下单参数都被接受。

### 本机下单参数对照工具

脚本 `scripts/diagnose-okpay-checkout.ps1` 默认仅显示说明，不联网；其 PHP 部分已在 PHP 7.3.4、7.4.3 通过离线自检。离线自检命令为：

```powershell
pwsh -NoProfile -File ./scripts/diagnose-okpay-checkout.ps1 -SelfTest
```

明确添加以下开关后，脚本会隐藏输入商户编号和 Token，最多向固定 OKPay `payLink` 接口创建四笔 **1.51 USDT 的未支付测试订单**；不会发起付款、打开支付链接或写入本站订单。请勿在商户后台支付这些 `probe` 开头的测试订单。

```powershell
pwsh -NoProfile -File ./scripts/diagnose-okpay-checkout.ps1 -ExecuteTestOrders
```

返回及回调站点默认采用本次日志中的 `https://zzzai.pro`；必要时用 `-BaseUrl` 指定其他 HTTPS 站点根地址。四组使用相同金额、币种、名称及回调地址，只对照以下两个变量；订单号与防重放参数每次重新生成。

| 组别 | 返回地址 | POST `status` 参数 | 用户实测结果 |
| --- | --- | --- | --- |
| A | 322 字节，带合成的四个查询参数 | `0` | 业务代码 `400`，订单创建失败 |
| B | 同站点 `/payment/result`，无查询参数；此次为 32 字节 | `0` | 创建成功 |
| C | 与 A 相同的长地址结构 | 省略 | 业务代码 `400`，订单创建失败 |
| D | 与 B 相同的短地址 | 省略 | 创建成功 |

长地址使用无效的合成恢复参数，不需要真实订单链接或恢复凭据。脚本不输出商户凭据、签名、平台订单号、支付链接、响应原文或响应 `data`。它使用 TLS 验证，不跟随重定向，不自动重试；网络、认证、限流或无法确定结果的响应会使后续请求停止。超时不代表上游没有创建订单，不应直接重复整套测试。

以上是用户已完成的真实对照结果。A/B、C/D 支持继续排查返回 URL 与查询参数组合，A/C、B/D 已排除此轮 POST `status` 的决定性影响。尚未得到只改变长度、保持查询语义一致的结果，因此不能认定纯长度是唯一原因，更不能声称长度上限为 255。B、D 的测试链接创建成功仍不代表实际付款、回调和入账正常。

### v0.2.11.9 紧凑恢复令牌

正式返回地址继续携带恢复令牌，以恢复跨域或未登录状态下的订单信息。OKPay 新令牌固定为 **102 字符**，使用专属版本前缀 `op1.` 和完整 HMAC-SHA256 签名，保留用户 ID、订单 ID、服务商实例、签发时间及过期时间的签名与校验。服务商和支付类型由 OKPay 专属版本固定；令牌不再重复保存 `CanonicalReturnURL` 及这两个常量字段。旧格式恢复 Token 继续兼容，其他支付服务商仍使用原格式；签名密钥缺失时保留原有处理行为。

返回目标仍在签发前选择和校验，继续优先使用所选实例的非空 HTTPS 配置，并保留合法子路径及普通查询参数。支付结果页路径、订单号、查询参数传递和恢复失败兜底保持原流程，不改用 URL 片段，也不新增数据库存储。

合成示例中，基础返回地址为 `https://zzzai.pro/payment/result`，采用 6 位订单 ID、21 字符商户订单号并附加既有四个查询参数时，完整地址为 **214 字节**。这是本地示例结果，不是用户真实新订单地址的长度；实际长度还受域名、子路径、普通查询参数及订单号位数影响。缩短后的正式 URL 尚需在商户环境验证真实下单、付款、回调、查单和账户入账，不能仅凭离线长度或 B、D 组结果宣布正式充值流程已恢复。

## 本机真实 PHP 认证对照

在仓库目录使用 PowerShell 7 执行：

```powershell
pwsh -NoProfile -File ./scripts/diagnose-okpay.ps1
```

脚本优先使用本机 phpstudy 的 PHP 7.4.3，隐藏输入商户 ID 和 Token，只经标准输入传给 PHP，不将凭据写入命令行、环境变量或文件。工具固定访问 `https://api.okaypay.me/shop/balance`，分别执行一次旧 MD5 表单、旧 MD5 JSON、新 HMAC-SHA256 表单请求。

脚本启用 TLS 校验、禁止重定向和自动重试；输出仅含 HTTP 状态、安全业务代码、固定分类及输入字符检查结果，不输出余额、签名或上游原文。单次请求最多 10 秒，父进程最多等待 40 秒。

旧协议失败而 HMAC 通过时，可支持该商户余额接口采用新协议的判断，但仍需验证已实现的下单、查单和回调。本机通过而服务器失败时，应继续比较配置与运行环境，不能直接认定 IP 限制；三组都失败时按原因分类排查，不能直接认定 Token 错误。

仅离线自检时使用 `-SelfTest`；`-PHPPath` 可指定 PHP，`-CaFile` 可指定现有可信 CA 文件。三组脚本已在 PHP 7.3.4、7.4.3 通过假凭据离线自检；用户提供的真实只读结果见本文开头。

签名黄金值和回归范围见 [OKPay 离线兼容对照](../backend/internal/payment/provider/testdata/OKPay兼容对照说明.md)。离线测试不访问真实网关，不能代替商户环境验证。
