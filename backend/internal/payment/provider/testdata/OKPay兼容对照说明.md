# OKPay 新旧协议的离线兼容对照

本次 `v0.2.11.5` 改动默认使用 `hmac_sha256`，原附件 MD5 通过显式 `legacy_md5` 保留，目前处于发布流程中。以下两类证据分别验证新协议与旧协议；旧 PHP 黄金值不能证明新 HMAC 协议，也不能证明真实商户认证通过。所有用例使用假凭据，网络请求仅由测试中的本地服务器接收。

## HMAC-SHA256 固定向量

新协议参考 Dujiao-Next 的 [协议升级提交](https://github.com/dujiao-next/dujiao-next/commit/ad9b7e2d2902650d9c2b57b89dce31b5bf6aeb80)，固定值取自 [提交 d2e44618 的测试](https://github.com/dujiao-next/dujiao-next/blob/d2e44618de03068cbbe874a23a6d7c6a249a79ec/internal/modules/payment/infrastructure/gateway/okpay/okpay_test.go#L11)。这是第三方证据，不是官方新版文档。

`../okpay_hmac_test.go` 中的 `TestOKPayHMACMatchesPublishedGoldenVectors` 保存三个固定期望值，覆盖请求、嵌套 JSON 回调和零值、布尔值、空值处理；不使用被测实现计算期望值。例如假 Token 为 `TESTtoken123456789abcdefghijABCD`，字段为 `id=10001`、`timestamp=1782680000`、`nonce=a1b2c3d4e5`、`amount=100.5`、`coin=USDT`、`unique_id=ORDER-20260628-001` 时，固定签名为：

```text
7444ADFD8E4F4DA09D752DDF9345E0EE56DC25090FCFAF675DD042830E5E3F79
```

同文件还覆盖原始 Token 和字段值保持、嵌套排序、数组路径、歧义输入拒绝等边界。`../okpay_protocol_test.go` 覆盖新协议下单、查单、诊断及 JSON 回调，并校验失败时不自动降级。

## 旧 MD5 黄金数据

`okpay_php_requests.json` 由 PHP 7.4.3 CLI 直接加载用户提供的 `OkayPay (1).php`，调用原始 `sign` 方法生成，记录生成日期、假凭据、输入参数、签名和表单正文。原附件 SHA-256 为：

```text
E46FCCF0132FF9AC948B18F004914F4C6F86C4786ED2FEB074164B065E2E4937
```

`okpay_php_reference.php` 保留附件 `sign`、`checkSign` 方法，仅增加本地输入输出和构造器，不包含任何网关请求。十组正常商户 ID 用例覆盖中文、组合字符、表情、特殊 Token、前导零 ID、金额、空参数和字符串零过滤。

这些测试使用显式 `legacy_md5`。原 PHP 可以验证 Go 的签名，表单解析字段一致；诊断专用的 PHP 兼容请求还要求正文与黄金值逐字节一致。常规旧 MD5 请求与 PHP 仍有签名字段顺序、`~` 编码及请求头差异，但已验证用例解析后不影响签名。

额外检查发现 Go 构造器会去除商户 ID 首尾空白，原 PHP 保留；Go 签名与其实际发送值一致。该输入规范化差异不属于正常 ID 黄金用例，也不能据此推断商户配置错误。

## 执行方式与边界

在后端目录运行：

```powershell
go test -p 1 ./internal/payment/provider -run 'TestOKPay(HMAC|NewProtocol|RequestsMatchIndependentPHPFixtures)' -count=1
```

固定黄金值不依赖 PHP。PATH 中存在 `php` 时，旧协议测试还会通过原 PHP 算法、`parse_str` 和 `checkSign` 验证 Go 实际表单；缺少 PHP 时仅跳过该可选差分步骤，不影响固定值校验。

2026 年 10 月 1 日，用户另行提供本机真实只读结果：同一组凭据、同一余额接口下，旧 MD5 表单和 JSON 请求均返回 HTTP 200、身份认证失败；HMAC 表单返回 HTTP 200、成功及业务代码 `200`。ID 和 Token 的空白、不可见字符检查均为 `false`。该结果确认新协议对该商户余额认证有效，独立于这里的假凭据测试；真实下单、支付、查单和回调仍未测试。

后台与本机脚本的三组只读诊断及证据边界见 [OKPay 认证排查](../../../../../docs/OKPAY认证排查.md)。诊断不输出 Token、签名、余额或上游原文。
