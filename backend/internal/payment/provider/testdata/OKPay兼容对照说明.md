# OKPay 与附件 PHP 的离线兼容对照

本目录使用假商户 ID 和假 Token，验证本项目请求与用户提供的 `OkayPay (1).php` 签名协议是否一致。所有网络请求都由测试中的本地服务器接收，不会创建真实订单、查询真实余额或联系支付网关。

## 黄金数据来源

`okpay_php_requests.json` 由 PHP 7.4.3 CLI 直接加载原附件，调用其原始 `sign` 方法生成。文件记录了原附件的 SHA-256、生成日期、假凭据、输入参数、PHP 签名和 PHP 表单正文。生成时还调用了原附件 `checkSign`，验证 Go 实际发送的请求。

`okpay_php_reference.php` 保留附件中的 `sign` 和 `checkSign` 方法原文，仅增加本地输入输出和构造器。它不包含支付、转账或查询网络调用。

用例覆盖中文、组合字符、表情、Token 中的加号及百分号、Token 首尾空白、前导零商户 ID、金额字符串、空可选参数和字符串零过滤。

## 执行方式

在后端目录运行：

```powershell
go test -p 1 ./internal/payment/provider -run TestOKPayRequestsMatchIndependentPHPFixtures -count=1
```

固定黄金值校验不依赖 PHP。若当前环境的 PATH 可以找到 `php`，测试会额外执行原 PHP 算法：重新计算黄金值，并通过 PHP 的 `parse_str` 与 `checkSign` 验证 Go 实际表单。缺少 PHP 时只跳过这一可选子测试，不影响常规 CI。

## 已验证范围与限制

正常商户 ID 的十组用例中，PHP 可以验证 Go 的签名，表单解析后的字段也一致。常规支付请求按解析后的字段和签名比较；诊断专用的 PHP 兼容请求还要求表单正文与 PHP 黄金值逐字节一致。当前常规请求仍存在已确认的发送差异：PHP 最后追加 `sign`，Go 按字段名字排序；PHP 将 `~` 编码为 `%7E`，Go 保留 `~`。PHP 示例的 User-Agent 是 `HTTP CLIENT`、Accept 是 `*/*`，当前 Go 请求使用默认 User-Agent 和 `application/json`。

额外离线检查发现 Go 构造器会去除商户 ID 首尾空白，附件 PHP 保留原值；Go 签名仍与自己发送的 ID 一致，原 PHP 可以完成验签。该输入值规范化差异不属于本文件的正常 ID 黄金用例。

这些证据只能证明签名和参数编码兼容，不能证明真实商户认证通过，也不能把请求头差异认定为认证失败的根因。真实环境还需在获得授权后，以服务端保存的商户配置执行只读诊断。原始 Token、签名、余额和上游响应正文不应出现在诊断结果或日志中。
