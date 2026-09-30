# OKPay 认证排查

## 当前状态与证据

`v0.2.11.5` 将 OKPay 默认签名改为 HMAC-SHA256，并保留显式旧协议选项。

2026 年 10 月 1 日，用户运行本机诊断工具并提供真实只读结果。同一组凭据访问同一 `https://api.okaypay.me/shop/balance` 接口：

| 请求 | HTTP 状态 | 业务结果 |
| --- | --- | --- |
| 旧 MD5 表单 | 200 | 身份认证失败 |
| 旧 MD5 JSON | 200 | 身份认证失败 |
| HMAC-SHA256 表单 | 200 | 成功，业务代码 `200` |

商户 ID 和 Token 的空白、不可见字符检查结果均为 `false`。这次对照已确认新协议对该商户的余额认证有效；真实下单、支付、查单和回调仍未测试，不能据此宣称整个充值流程已恢复。

`payment gateway error: OKPay 返回业务失败状态（身份认证失败……）` 表示程序收到了上游业务拒绝。本站将支付网关失败映射为 HTTP 503，不能据此认定网关不可访问，也不能仅凭提示认定商户 ID 或 Token 填错。

本次协议适配依据用户指定的第三方 Dujiao-Next 参考项目：

- [协议升级提交](https://github.com/dujiao-next/dujiao-next/commit/ad9b7e2d2902650d9c2b57b89dce31b5bf6aeb80)、[问题报告 #294](https://github.com/dujiao-next/dujiao-next/issues/294) 与 [合并请求 #293](https://github.com/dujiao-next/dujiao-next/pull/293) 记录了旧 MD5 请求出现“身份认证失败”的同类现象。
- [固定版本签名实现](https://github.com/dujiao-next/dujiao-next/blob/d2e44618de03068cbbe874a23a6d7c6a249a79ec/internal/modules/payment/infrastructure/gateway/okpay/okpay.go#L558) 和 [固定测试向量](https://github.com/dujiao-next/dujiao-next/blob/d2e44618de03068cbbe874a23a6d7c6a249a79ec/internal/modules/payment/infrastructure/gateway/okpay/okpay_test.go#L11) 用于核对签名结果。

这些参考资料属于第三方实现与离线证据，不能冒充官方新版文档。参考项目未实现 `/balance`、`/checkDeposit`；本项目已将共同签名规则用于余额诊断、下单及查单。用户实测已补充余额认证证据，其余真实支付接口仍待验证。

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

## 本机真实 PHP 对照

在仓库目录使用 PowerShell 7 执行：

```powershell
pwsh -NoProfile -File ./scripts/diagnose-okpay.ps1
```

脚本优先使用本机 phpstudy 的 PHP 7.4.3，隐藏输入商户 ID 和 Token，只经标准输入传给 PHP，不将凭据写入命令行、环境变量或文件。工具固定访问 `https://api.okaypay.me/shop/balance`，分别执行一次旧 MD5 表单、旧 MD5 JSON、新 HMAC-SHA256 表单请求。

脚本启用 TLS 校验、禁止重定向和自动重试；输出仅含 HTTP 状态、安全业务代码、固定分类及输入字符检查结果，不输出余额、签名或上游原文。单次请求最多 10 秒，父进程最多等待 40 秒。

旧协议失败而 HMAC 通过时，可支持该商户余额接口采用新协议的判断，但仍需验证已实现的下单、查单和回调。本机通过而服务器失败时，应继续比较配置与运行环境，不能直接认定 IP 限制；三组都失败时按原因分类排查，不能直接认定 Token 错误。

仅离线自检时使用 `-SelfTest`；`-PHPPath` 可指定 PHP，`-CaFile` 可指定现有可信 CA 文件。三组脚本已在 PHP 7.3.4、7.4.3 通过假凭据离线自检；用户提供的真实只读结果见本文开头。

签名黄金值和回归范围见 [OKPay 离线兼容对照](../backend/internal/payment/provider/testdata/OKPay兼容对照说明.md)。离线测试不访问真实网关，不能代替商户环境验证。
