#requires -Version 7.0
<#
.SYNOPSIS
使用本机 PHP 对固定 OKPay 余额接口执行三次只读认证诊断。
.DESCRIPTION
商户编号和 Token 均隐藏输入，只经 UTF-8 无 BOM 标准输入传给 PHP。
不创建订单，不输出余额、凭据、签名或原始响应，不保存诊断日志。
.PARAMETER PHPPath
可选的本机 php.exe 路径；默认优先使用 phpstudy 的 PHP 7.4.3，其次 7.3.4。
.PARAMETER CaFile
可选的现有可信 CA 证书文件；默认使用 PHP 附带的 CA，其次本机 Git 的 CA。
.PARAMETER SelfTest
仅使用内置假凭据执行离线自检，不询问真实凭据，不访问支付网关。
.EXAMPLE
pwsh -NoProfile -File ./scripts/diagnose-okpay.ps1 -SelfTest
.EXAMPLE
pwsh -NoProfile -File ./scripts/diagnose-okpay.ps1
#>
[CmdletBinding()]
param(
    [string]$PHPPath,
    [string]$CaFile,
    [switch]$SelfTest
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
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
    $phpScript = Join-Path $PSScriptRoot 'diagnose-okpay.php'
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
        Write-Host '固定接口：https://api.okaypay.me/shop/balance；旧版 MD5 表单、旧版 MD5 JSON、新版 HMAC 表单各请求一次，不创建订单。'
        Write-Host '三组结果用于比较本商户的签名协议认证情况，不能替代下单及支付回调验证。'
        $merchantSecure = Read-Host '商户 ID（隐藏输入，不自动去除空白）' -AsSecureString
        $tokenSecure = Read-Host 'Token（隐藏输入，不自动去除空白）' -AsSecureString
        $merchantPlain = ConvertFrom-DiagnosticSecureString $merchantSecure
        $tokenPlain = ConvertFrom-DiagnosticSecureString $tokenSecure
        $inputJson = @{ id = $merchantPlain; token = $tokenPlain } | ConvertTo-Json -Compress
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
    if (-not $diagnosticProcess.WaitForExit(40000)) {
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
        Write-Host '离线自检：通过。已验证签名、分类、凭据警示、响应上限和传输约束；未访问网关。'
    } else {
        $reasonLabels = @{
            success = '成功'; auth_failed = '身份认证失败'; signature_failed = '签名错误'
            invalid_parameters = '其他业务拒绝（参数错误）'; rate_limited = '其他业务拒绝（限流）'
            merchant_invalid = '其他业务拒绝（商户无效）'; unknown_business_error = '其他业务拒绝（未知原因）'
            network_error = '网络错误'; tls_failed = '网络错误（TLS 连接或验证失败）'; invalid_response = '响应格式异常'
        }
        $warningLabels = [ordered]@{
            id_boundary_whitespace = '商户 ID 首尾空白'; id_invisible = '商户 ID 不可见字符'
            token_boundary_whitespace = 'Token 首尾空白'; token_invisible = 'Token 不可见字符'
        }
        if ($report.kind -ne 'diagnostic' -or $report.checks.Count -ne 3) { throw '诊断输出无效' }
        foreach ($name in $warningLabels.Keys) {
            if ($report.credential_warnings[$name] -isnot [bool]) { throw '诊断输出无效' }
        }
        $expectedModes = @('php_form', 'document_json', 'hmac_sha256_form')
        for ($index = 0; $index -lt 3; $index++) {
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
        }
        foreach ($name in $warningLabels.Keys) {
            Write-Host ($warningLabels[$name] + '：' + $report.credential_warnings[$name].ToString().ToLowerInvariant())
        }
        for ($index = 0; $index -lt 3; $index++) {
            $check = $report.checks[$index]
            $label = @('旧版 MD5：PHP 表单', '旧版 MD5：文档 JSON', '新版 HMAC-SHA256：PHP 表单')[$index]
            $httpStatus = if ($null -eq $check.http_status) { '无' } else { [string]$check.http_status }
            $businessCode = if ($null -eq $check.business_code) { '' } else { '；业务代码=' + $check.business_code }
            Write-Host ($label + '：HTTP=' + $httpStatus + '；分类=' + $reasonLabels[$check.reason] + $businessCode)
        }
        Write-Host '结果仅说明只读认证情况，不能证明下单正常。所有请求均失败也不能直接判定 Token 错误。'
        Write-Host '若本机通过而服务器拒绝，只能缩小到已保存配置或服务器环境，不能单独认定是 IP 或配置问题。'
    }
    $exitStatus = 0
} catch {
    # 不展示异常对象以及子进程的原始标准输出、标准错误，避免错误路径反射敏感数据。
    Write-Host '诊断未完成：请检查本机 PHP、curl 扩展、CA 文件及输入格式；底层错误详情已隐藏。'
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
