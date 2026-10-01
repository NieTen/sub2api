#requires -Version 7.0
<#
.SYNOPSIS
使用本机 PHP 对 OKPay 下单参数进行受控对照。
.DESCRIPTION
商户编号和 Token 均隐藏输入，只经 UTF-8 无 BOM 标准输入传给 PHP。
默认仅显示说明，不联网。显式传入 ExecuteTestOrders 后，最多创建四笔未支付的 1.51 USDT 测试订单。
不发起付款、不打开支付链接、不写入本站订单。不输出凭据、签名、订单号或原始响应。
.PARAMETER PHPPath
可选的本机 php.exe 路径；默认优先使用 phpstudy 的 PHP 7.4.3，其次 7.3.4。
.PARAMETER CaFile
可选的现有可信 CA 证书文件；默认使用 PHP 附带的 CA，其次本机 Git 的 CA。
.PARAMETER SelfTest
仅使用内置假凭据执行离线自检，不询问真实凭据，不访问支付网关。
.PARAMETER BaseUrl
测试返回和回调使用的 HTTPS 站点根地址，默认使用本次日志中的站点。
.PARAMETER ExecuteTestOrders
明确执行最多四次真实下单请求；不自动重试，遇到无法确定是否已创建订单的响应会停止。
.EXAMPLE
pwsh -NoProfile -File ./scripts/diagnose-okpay-checkout.ps1 -SelfTest
.EXAMPLE
pwsh -NoProfile -File ./scripts/diagnose-okpay-checkout.ps1 -ExecuteTestOrders
#>
[CmdletBinding()]
param(
    [string]$PHPPath,
    [string]$CaFile,
    [string]$BaseUrl = 'https://zzzai.pro',
    [switch]$SelfTest,
    [switch]$ExecuteTestOrders
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if ($SelfTest -and $ExecuteTestOrders) {
    Write-Host 'SelfTest 与 ExecuteTestOrders 不能同时使用。'
    exit 1
}
if (-not $SelfTest -and -not $ExecuteTestOrders) {
    Write-Host '默认不联网。此工具用于定位 OKPay「订单创建失败」，不是余额认证诊断。'
    Write-Host '显式添加 -ExecuteTestOrders 后，最多向 OKPay 创建四笔未支付的 1.51 USDT 测试订单，不发起付款。'
    Write-Host '四组仅对比：322 字节返回地址/短返回地址，以及发送 status=0/省略 status；防重放参数和订单号各自生成。'
    Write-Host '测试订单不进入本站数据库，支付链接不会显示或打开。请勿在商户后台支付这些 probe 开头的订单。'
    Write-Host '先离线检查：pwsh -NoProfile -File ./scripts/diagnose-okpay-checkout.ps1 -SelfTest'
    Write-Host '执行对照：pwsh -NoProfile -File ./scripts/diagnose-okpay-checkout.ps1 -ExecuteTestOrders'
    exit 0
}
$diagnosticProcess = $null
$merchantSecure = $null
$tokenSecure = $null
$merchantPlain = $null
$tokenPlain = $null
$inputJson = $null
$exitStatus = 1

function ConvertFrom-DiagnosticSecureString {
    param([Security.SecureString]$Value)
    $buffer = [IntPtr]::Zero
    try {
        $buffer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Value)
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($buffer)
    } finally {
        if ($buffer -ne [IntPtr]::Zero) {
            [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($buffer)
        }
    }
}

try {
    if (-not $PHPPath) {
        foreach ($candidate in @(
            'C:/code/php/phpstudy_pro/Extensions/php/php7.4.3nts/php.exe',
            'C:/code/php/phpstudy_pro/Extensions/php/php7.3.4nts/php.exe'
        )) {
            if (Test-Path -LiteralPath $candidate -PathType Leaf) {
                $PHPPath = $candidate
                break
            }
        }
    }
    if (-not $PHPPath -or -not (Test-Path -LiteralPath $PHPPath -PathType Leaf)) {
        throw '本机 PHP 不可用'
    }
    $resolvedPHP = (Resolve-Path -LiteralPath $PHPPath).ProviderPath
    $phpDirectory = Split-Path -LiteralPath $resolvedPHP
    $curlExtension = Join-Path $phpDirectory 'ext/php_curl.dll'
    $phpScript = Join-Path $PSScriptRoot 'diagnose-okpay-checkout.php'
    if (-not (Test-Path -LiteralPath $phpScript -PathType Leaf) -or
        -not (Test-Path -LiteralPath $curlExtension -PathType Leaf)) {
        throw '诊断脚本或 PHP 扩展不可用'
    }
    if ($CaFile) {
        if (-not (Test-Path -LiteralPath $CaFile -PathType Leaf)) {
            throw '指定的 CA 文件不存在'
        }
        $CaFile = (Resolve-Path -LiteralPath $CaFile).ProviderPath
    } else {
        $caDirectory = Join-Path $phpDirectory 'ssl'
        if (Test-Path -LiteralPath $caDirectory -PathType Container) {
            $caCandidate = Get-ChildItem -LiteralPath $caDirectory -Filter 'cacert*.pem' -File |
                Sort-Object Name -Descending | Select-Object -First 1
            if ($caCandidate) { $CaFile = $caCandidate.FullName }
        }
        if (-not $CaFile -and (Test-Path -LiteralPath 'C:/app/Git/mingw64/etc/ssl/certs/ca-bundle.crt' -PathType Leaf)) {
            $CaFile = 'C:/app/Git/mingw64/etc/ssl/certs/ca-bundle.crt'
        }
    }

    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $resolvedPHP
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardInput = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $utf8 = [Text.UTF8Encoding]::new($false)
    $startInfo.StandardInputEncoding = $utf8
    $startInfo.StandardOutputEncoding = $utf8
    $startInfo.StandardErrorEncoding = $utf8
    # 隔离已有 PHP 日志、自动载入脚本和调试扩展；命令参数仅包含本机路径及固定设置。
    foreach ($argument in @('-n', '-d', 'display_errors=0', '-d', 'log_errors=0',
        '-d', 'error_reporting=0', '-d', ('extension_dir=' + (Join-Path $phpDirectory 'ext')),
        '-d', 'extension=php_curl.dll')) {
        $startInfo.ArgumentList.Add($argument)
    }
    if ($CaFile) {
        $startInfo.ArgumentList.Add('-d')
        $startInfo.ArgumentList.Add('curl.cainfo=' + $CaFile)
    }
    $startInfo.ArgumentList.Add($phpScript)
    if ($SelfTest) {
        $startInfo.ArgumentList.Add('--self-test')
    } else {
        Write-Host '固定接口：https://api.okaypay.me/shop/payLink；最多创建四笔未支付的 1.51 USDT 测试订单，不发起付款。'
        Write-Host '对比返回地址长度及 status 参数；只使用合成恢复参数，不会使用真实订单或恢复凭据。'
        Write-Host '测试支付链接不显示、不打开；请勿在商户后台支付 probe 开头的测试订单。'
        $merchantSecure = Read-Host '商户 ID（隐藏输入，不自动去除空白）' -AsSecureString
        $tokenSecure = Read-Host 'Token（隐藏输入，不自动去除空白）' -AsSecureString
        $merchantPlain = ConvertFrom-DiagnosticSecureString $merchantSecure
        $tokenPlain = ConvertFrom-DiagnosticSecureString $tokenSecure
        $inputJson = @{ id = $merchantPlain; token = $tokenPlain; base_url = $BaseUrl; execute = $true } | ConvertTo-Json -Compress
    }

    $diagnosticProcess = [Diagnostics.Process]::new()
    $diagnosticProcess.StartInfo = $startInfo
    if (-not $diagnosticProcess.Start()) { throw '无法启动本机 PHP' }
    $stdoutTask = $diagnosticProcess.StandardOutput.ReadToEndAsync()
    $stderrTask = $diagnosticProcess.StandardError.ReadToEndAsync()
    if (-not $SelfTest) { $diagnosticProcess.StandardInput.Write($inputJson) }
    $diagnosticProcess.StandardInput.Close()
    # 明文仅在父子进程内存中短暂存在，不写入文件、环境变量、命令行或日志。
    $merchantPlain = $null
    $tokenPlain = $null
    $inputJson = $null
    if (-not $diagnosticProcess.WaitForExit(55000)) {
        $diagnosticProcess.Kill($true)
        throw '本机诊断超时'
    }
    $safeOutput = $stdoutTask.GetAwaiter().GetResult()
    $null = $stderrTask.GetAwaiter().GetResult()
    if ($diagnosticProcess.ExitCode -ne 0) { throw '本机诊断失败' }
    $report = $safeOutput | ConvertFrom-Json -AsHashtable
    if ($SelfTest) {
        if ($report.kind -ne 'self_test' -or $report.passed -isnot [bool] -or -not $report.passed) {
            throw '离线自检未通过'
        }
        Write-Host '离线自检：通过。已验证签名、四组请求差异、返回地址长度、响应分类和传输约束；未访问网关。'
    } else {
        $reasonLabels = @{
            success = '测试支付链接创建成功（未付款）'; order_creation_failed = '订单创建失败'
            auth_failed = '身份认证失败'; signature_failed = '签名错误'
            invalid_parameters = '其他业务拒绝（参数错误）'; rate_limited = '其他业务拒绝（限流）'
            merchant_invalid = '其他业务拒绝（商户无效）'; unknown_business_error = '其他业务拒绝（未知原因）'
            network_error = '网络错误'; tls_failed = '网络错误（TLS 连接或验证失败）'; invalid_response = '响应格式异常'
            http_error = 'HTTP 请求失败'
        }
        $warningLabels = [ordered]@{
            id_boundary_whitespace = '商户 ID 首尾空白'; id_invisible = '商户 ID 不可见字符'
            token_boundary_whitespace = 'Token 首尾空白'; token_invisible = 'Token 不可见字符'
        }
        if ($report.kind -ne 'checkout_probe' -or $report.stopped -isnot [bool] -or $report.checks.Count -lt 1 -or $report.checks.Count -gt 4) { throw '诊断输出无效' }
        foreach ($name in $warningLabels.Keys) {
            if ($report.credential_warnings[$name] -isnot [bool]) { throw '诊断输出无效' }
        }
        $expectedModes = @('current_long_status', 'short_status', 'long_no_status', 'short_no_status')
        for ($index = 0; $index -lt $report.checks.Count; $index++) {
            $check = $report.checks[$index]
            if ($check.mode -ne $expectedModes[$index] -or -not $reasonLabels.ContainsKey($check.reason)) {
                throw '诊断输出无效'
            }
            if ($null -ne $check.http_status -and
                (($check.http_status -isnot [long] -and $check.http_status -isnot [int]) -or
                $check.http_status -lt 100 -or $check.http_status -gt 599)) { throw '诊断输出无效' }
            if ($null -ne $check.business_code -and
                ($check.business_code -isnot [string] -or $check.business_code -cnotmatch '\A[0-9]{1,8}\z')) {
                throw '诊断输出无效'
            }
            if ($check.checkout_created -isnot [bool] -or $check.request.status_present -isnot [bool] -or
                ($check.request.return_bytes -isnot [long] -and $check.request.return_bytes -isnot [int]) -or
                $check.request.return_bytes -lt 1 -or $check.request.return_bytes -gt 512) { throw '诊断输出无效' }
            if (($check.reason -eq 'success') -ne $check.checkout_created) { throw '诊断输出无效' }
            if ($check.request.status_present -ne ($index -lt 2)) { throw '诊断组合不一致' }
            if (($index -eq 0 -or $index -eq 2) -and $check.request.return_bytes -ne 322) { throw '长地址诊断组合不一致' }
            if (($index -eq 1 -or $index -eq 3) -and $check.request.return_bytes -ge 322) { throw '短地址诊断组合不一致' }
        }
        foreach ($name in $warningLabels.Keys) {
            Write-Host ($warningLabels[$name] + '：' + $report.credential_warnings[$name].ToString().ToLowerInvariant())
        }
        for ($index = 0; $index -lt $report.checks.Count; $index++) {
            $check = $report.checks[$index]
            $label = @('A：长返回地址 + status=0', 'B：短返回地址 + status=0', 'C：长返回地址 + 省略 status', 'D：短返回地址 + 省略 status')[$index]
            $httpStatus = if ($null -eq $check.http_status) { '无' } else { [string]$check.http_status }
            $businessCode = if ($null -eq $check.business_code) { '' } else { '；业务代码=' + $check.business_code }
            Write-Host ($label + '：HTTP=' + $httpStatus + '；分类=' + $reasonLabels[$check.reason] + $businessCode + '；返回地址字节=' + $check.request.return_bytes)
        }
        if ($report.stopped) {
            Write-Host '已提前停止，未发送剩余组合。网络中断或异常响应不代表上游没有创建订单，请勿立即重复执行。'
        }
        Write-Host '请只提供上面的四组分类结果；不要提供 Token、签名或支付链接。'
        Write-Host '短地址成功而对应长地址失败时，优先定位返回参数或长度兼容；省略 status 才成功时，优先定位 status 兼容。'
        Write-Host '全部失败不能单独认定密钥或长度限制；全部成功则需比较生产环境与合成请求的剩余差异。'
        Write-Host '成功仅说明测试链接已创建，不表示实际支付或入账成功。'
    }
    $exitStatus = 0
} catch {
    # 不展示异常对象以及子进程的原始标准输出、标准错误，避免错误路径反射敏感数据。
    Write-Host '诊断未完成：请检查本机 PHP、curl 扩展、CA 文件及 HTTPS 根地址；底层错误详情已隐藏。'
    if ($ExecuteTestOrders) { Write-Host '若请求已发出，上游可能已创建测试订单；不要仅凭此提示重复执行。' }
} finally {
    $merchantPlain = $null
    $tokenPlain = $null
    $inputJson = $null
    if ($merchantSecure) { $merchantSecure.Dispose() }
    if ($tokenSecure) { $tokenSecure.Dispose() }
    if ($diagnosticProcess) {
        try { if (-not $diagnosticProcess.HasExited) { $diagnosticProcess.Kill($true) } } catch { }
        $diagnosticProcess.Dispose()
    }
}
exit $exitStatus
