<?php
// 此文件仅供本机诊断入口调用；凭据只从标准输入读取，不记录任何原始请求或响应。
ini_set('display_errors', '0');
ini_set('log_errors', '0');
error_reporting(0);

const OKPAY_DIAGNOSTIC_URL = 'https://api.okaypay.me/shop/balance';
const OKPAY_DIAGNOSTIC_RESPONSE_LIMIT = 1048576;

function okpayDiagnosticSign(array $data, $id, $token)
{
    // 保持附件原算法：过滤 PHP 假值、按键排序、URL 解码后 MD5，并转大写。
    $data['id'] = $id;
    $data = array_filter($data);
    ksort($data);
    $data['sign'] = strtoupper(md5(urldecode(http_build_query($data) . '&token=' . $token)));
    return $data;
}

function okpayDiagnosticHmacSign(array $data, $id, $token, $timestamp, $nonce)
{
    // 新协议依据第三方迁移记录；本机余额认证已实测通过，仍保留三组只读对照。
    // 来源：https://github.com/dujiao-next/dujiao-next/commit/ad9b7e2d2902650d9c2b57b89dce31b5bf6aeb80
    $data['id'] = $id;
    $data['timestamp'] = $timestamp;
    $data['nonce'] = $nonce;
    unset($data['sign']);
    $data = array_filter($data, function ($value) { return $value !== null && $value !== ''; });
    ksort($data, SORT_STRING);
    $parts = [];
    foreach ($data as $key => $value) {
        if (!is_scalar($value)) {
            throw new RuntimeException('HMAC 协议字段无效');
        }
        $parts[] = $key . '=' . (is_bool($value) ? ($value ? 'true' : 'false') : (string) $value);
    }
    $data['sign'] = strtoupper(hash_hmac('sha256', implode('&', $parts), $token));
    return $data;
}

function okpayDiagnosticWarnings($id, $token)
{
    $boundary = '/\A[\s\p{Z}]|[\s\p{Z}]\z/u';
    $invisible = '/[\p{Cc}\p{Cf}\p{Z}\x{034F}\x{115F}\x{1160}\x{17B4}\x{17B5}\x{180B}-\x{180F}\x{2800}\x{3164}\x{FE00}-\x{FE0F}\x{FFA0}\x{E0100}-\x{E01EF}]/u';
    return [
        'id_boundary_whitespace' => preg_match($boundary, $id) === 1,
        'id_invisible' => preg_match($invisible, $id) === 1,
        'token_boundary_whitespace' => preg_match($boundary, $token) === 1,
        'token_invisible' => preg_match($invisible, $token) === 1,
    ];
}

function okpayDiagnosticMessageCategory($response)
{
    $categories = [
        '身份认证失败' => 'auth_failed', '认证失败' => 'auth_failed',
        'authentication failed' => 'auth_failed', 'unauthorized' => 'auth_failed',
        '签名错误' => 'signature_failed', '签名验证失败' => 'signature_failed',
        '签名校验失败' => 'signature_failed', 'invalid signature' => 'signature_failed',
        'signature error' => 'signature_failed',
        '参数错误' => 'invalid_parameters', '参数不完整' => 'invalid_parameters',
        '缺少参数' => 'invalid_parameters', '参数缺失' => 'invalid_parameters',
        'invalid parameters' => 'invalid_parameters', 'missing parameters' => 'invalid_parameters',
        '请求过于频繁' => 'rate_limited', '请求频繁' => 'rate_limited',
        'too many requests' => 'rate_limited', 'rate limit exceeded' => 'rate_limited',
        '商户不存在' => 'merchant_invalid', '无效商户' => 'merchant_invalid',
        'merchant not found' => 'merchant_invalid', 'invalid merchant' => 'merchant_invalid',
    ];
    foreach (['msg', 'message'] as $key) {
        if (isset($response->$key) && is_string($response->$key)) {
            $message = strtolower(trim($response->$key));
            if (isset($categories[$message])) {
                return $categories[$message];
            }
        }
    }
    return 'unknown_business_error';
}

function okpayDiagnosticSafeCode($response, array $sensitive)
{
    if (!is_object($response) || !isset($response->code) ||
        (!is_string($response->code) && !is_int($response->code))) {
        return null;
    }
    $code = (string) $response->code;
    if (preg_match('/\A[0-9]{1,8}\z/', $code) !== 1) {
        return null;
    }
    foreach ($sensitive as $value) {
        // 双向包含均隐藏，防止短业务代码意外成为凭据片段。
        if ($value !== '' && (strpos($code, $value) !== false || strpos($value, $code) !== false)) {
            return null;
        }
    }
    return $code;
}

function okpayDiagnosticClassify($httpStatus, $body, $curlError, $overflow, array $sensitive)
{
    $result = ['http_status' => $httpStatus >= 100 && $httpStatus <= 599 ? $httpStatus : null,
        'reason' => 'invalid_response', 'business_code' => null];
    if ($overflow) {
        return $result;
    }
    if ($curlError !== 0) {
        // 仅保留固定原因；绝不调用或显示 curl_error 原始文本。
        $result['reason'] = in_array($curlError, [35, 51, 53, 54, 58, 59, 60, 64, 66, 77, 80, 82, 83, 90, 91], true)
            ? 'tls_failed' : 'network_error';
        return $result;
    }
    if ($httpStatus < 200 || $httpStatus >= 300) {
        $result['reason'] = 'network_error';
    }
    $response = json_decode($body);
    if (json_last_error() !== JSON_ERROR_NONE || !is_object($response)) {
        $result['reason'] = 'invalid_response';
        return $result;
    }
    $result['business_code'] = okpayDiagnosticSafeCode($response, $sensitive);
    $hasState = false;
    $rejected = false;
    foreach (['status' => ['success'], 'code' => ['200', '10000']] as $key => $successValues) {
        if (!property_exists($response, $key)) {
            continue;
        }
        $hasState = true;
        $value = $response->$key;
        if ((!is_string($value) && !is_int($value)) || (string) $value === '') {
            $result['reason'] = 'invalid_response';
            return $result;
        }
        if (!in_array((string) $value, $successValues, true)) {
            $rejected = true;
        }
    }
    if ($rejected) {
        $result['reason'] = okpayDiagnosticMessageCategory($response);
        return $result;
    }
    if (!$hasState) {
        $category = okpayDiagnosticMessageCategory($response);
        $result['reason'] = $category === 'unknown_business_error' ? 'invalid_response' : $category;
        return $result;
    }
    if ($httpStatus < 200 || $httpStatus >= 300) {
        return $result;
    }
    if (!isset($response->status, $response->code)) {
        return $result;
    }
    if (!isset($response->data) || !is_object($response->data)) {
        return $result;
    }
    $hasBalance = false;
    foreach (['usdt', 'trx', 'cny'] as $currency) {
        if (!property_exists($response->data, $currency)) {
            continue;
        }
        $value = $response->data->$currency;
        if (is_int($value) || is_float($value)) {
            $validBalance = is_finite($value) && $value >= 0;
        } else {
            $validBalance = is_string($value) && strlen($value) <= 32
                && preg_match('/\A[0-9]+(?:\.[0-9]+)?\z/', $value) === 1;
        }
        if (!$validBalance) {
            return $result;
        }
        $hasBalance = true;
    }
    if ($hasBalance) {
        $result['reason'] = 'success';
    }
    return $result;
}

function okpayDiagnosticAppendResponse(&$body, $chunk, &$overflow)
{
    if (strlen($body) + strlen($chunk) > OKPAY_DIAGNOSTIC_RESPONSE_LIMIT) {
        $overflow = true;
        return 0;
    }
    $body .= $chunk;
    return strlen($chunk);
}

function okpayDiagnosticOptions($mode, array $signed, $writeCallback)
{
    $isForm = $mode !== 'document_json';
    $options = [
        CURLOPT_URL => OKPAY_DIAGNOSTIC_URL,
        CURLOPT_POST => true,
        CURLOPT_POSTFIELDS => $isForm ? http_build_query($signed) : json_encode($signed, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES),
        CURLOPT_HTTPHEADER => ['Content-Type: ' . ($isForm ? 'application/x-www-form-urlencoded' : 'application/json'), 'Accept: */*', 'Expect:'],
        CURLOPT_USERAGENT => 'HTTP CLIENT',
        CURLOPT_HEADER => false,
        CURLOPT_WRITEFUNCTION => $writeCallback,
        CURLOPT_FOLLOWLOCATION => false,
        CURLOPT_MAXREDIRS => 0,
        CURLOPT_PROTOCOLS => CURLPROTO_HTTPS,
        CURLOPT_REDIR_PROTOCOLS => CURLPROTO_HTTPS,
        CURLOPT_SSL_VERIFYPEER => true,
        CURLOPT_SSL_VERIFYHOST => 2,
        CURLOPT_CONNECTTIMEOUT => 10,
        CURLOPT_TIMEOUT => 10,
        CURLOPT_NOSIGNAL => true,
        CURLOPT_PROXY => '',
        CURLOPT_VERBOSE => false,
    ];
    $caFile = ini_get('curl.cainfo');
    if (is_string($caFile) && $caFile !== '') {
        $options[CURLOPT_CAINFO] = $caFile;
    }
    return $options;
}

function okpayDiagnosticRequest($mode, array $signed, array $sensitive)
{
    $body = '';
    $overflow = false;
    $curl = curl_init();
    if ($curl === false) {
        return ['mode' => $mode, 'http_status' => null, 'reason' => 'network_error', 'business_code' => null];
    }
    try {
        $callback = function ($handle, $chunk) use (&$body, &$overflow) {
            return okpayDiagnosticAppendResponse($body, $chunk, $overflow);
        };
        if (!curl_setopt_array($curl, okpayDiagnosticOptions($mode, $signed, $callback))) {
            return ['mode' => $mode, 'http_status' => null, 'reason' => 'network_error', 'business_code' => null];
        }
        // 每种模式只有这一次请求，不进行重试、重定向或其他接口探测。
        curl_exec($curl);
        $result = okpayDiagnosticClassify((int) curl_getinfo($curl, CURLINFO_HTTP_CODE),
            $body, curl_errno($curl), $overflow, $sensitive);
        return ['mode' => $mode] + $result;
    } finally {
        curl_close($curl);
        $body = '';
    }
}

function okpayDiagnosticSelfTest()
{
    // 黄金签名取自现有附件 PHP 对照样例，全部是明确的假凭据。
    $fixtures = [
        [['unique_id' => 'site-100'], '123', 'fixture-only-token', '47D9CA965CE009AE73BA70F8D8E7474B'],
        [['unique_id' => 'site-100', 'name' => '', 'amount' => '12.30', 'coin' => 'USDT', 'return_url' => '', 'callback_url' => '', 'status' => '0'], '123', 'fixture-only-token', '8462591F9459203318615885A8EC8AAF'],
        [['unique_id' => 'site-100', 'name' => '账户充值 A+B & = %2B ? ~ !', 'amount' => '12.30', 'coin' => 'USDT', 'return_url' => 'https://site.example/payment/result?name=A+B&encoded=%2B&tilde=~', 'callback_url' => 'https://site.example/api/v1/payment/webhook/okpay', 'status' => '0'], '123', 'token+plus%2Band%zz', '43BE9AAF5B8713347DAB70D779D360FF'],
    ];
    $passed = true;
    foreach ($fixtures as $fixture) {
        $signed = okpayDiagnosticSign($fixture[0], $fixture[1], $fixture[2]);
        $passed = $passed && hash_equals($fixture[3], $signed['sign']) && !array_key_exists('token', $signed);
    }
    $hmacSigned = okpayDiagnosticHmacSign(['amount' => '100.5', 'coin' => 'USDT', 'unique_id' => 'ORDER-20260628-001'],
        '10001', 'TESTtoken123456789abcdefghijABCD', 1782680000, 'a1b2c3d4e5');
    $passed = $passed && hash_equals('7444ADFD8E4F4DA09D752DDF9345E0EE56DC25090FCFAF675DD042830E5E3F79', $hmacSigned['sign']);
    $hmacFiltered = okpayDiagnosticHmacSign(['zero' => 0, 'false' => false, 'empty' => '', 'nil' => null, 'sign' => '旧假签名'],
        '10001', 'TESTtoken123456789abcdefghijABCD', 1782680000, 'a1b2c3d4e5');
    $passed = $passed && array_key_exists('zero', $hmacFiltered) && array_key_exists('false', $hmacFiltered)
        && !array_key_exists('empty', $hmacFiltered) && !array_key_exists('nil', $hmacFiltered);
    $cases = [
        [200, '{"status":"success","code":10000,"data":{"usdt":"2.34"}}', 0, false, 'success'],
        [200, '{"status":"success","code":200,"data":{"usdt":"2.34"}}', 0, false, 'success'],
        [200, '{"status":"success","code":200,"data":{"usdt":0.000001}}', 0, false, 'success'],
        [200, '{"status":"success","data":{"usdt":"2.34"}}', 0, false, 'invalid_response'],
        [200, '{"code":200,"data":{"usdt":"2.34"}}', 0, false, 'invalid_response'],
        [200, '{"status":"warning","msg":"身份认证失败"}', 0, false, 'auth_failed'],
        [401, '{"status":"warning","msg":"Authentication Failed"}', 0, false, 'auth_failed'],
        [200, '{"status":"warning","msg":"签名错误"}', 0, false, 'signature_failed'],
        [200, '{"status":"warning","message":"参数错误"}', 0, false, 'invalid_parameters'],
        [429, '{"code":429,"msg":"请求过于频繁"}', 0, false, 'rate_limited'],
        [200, '{"status":"warning","msg":"商户不存在"}', 0, false, 'merchant_invalid'],
        [200, '{"status":"warning","msg":"不能输出的假凭据"}', 0, false, 'unknown_business_error'],
        [200, '{"status":"success","code":400,"data":{"usdt":"2.34"}}', 0, false, 'unknown_business_error'],
        [200, '{"status":"warning","code":200,"data":{"usdt":"2.34"}}', 0, false, 'unknown_business_error'],
        [200, '{"status":true,"data":{"usdt":"2.34"}}', 0, false, 'invalid_response'],
        [200, '{"status":"success","code":{},"data":{"usdt":"2.34"}}', 0, false, 'invalid_response'],
        [200, '{"status":"success","data":{"usdt":true}}', 0, false, 'invalid_response'],
        [200, '{"status":"success","data":{}}', 0, false, 'invalid_response'],
        [200, '{"data":{"usdt":"2.34"}}', 0, false, 'invalid_response'],
        [200, '<html>不能输出的假凭据</html>', 0, false, 'invalid_response'],
        [302, '{"status":"success","data":{"usdt":"2.34"}}', 0, false, 'network_error'],
        [0, '', 28, false, 'network_error'],
        [0, '', 60, false, 'tls_failed'],
        [200, '', 23, true, 'invalid_response'],
    ];
    foreach ($cases as $case) {
        $classified = okpayDiagnosticClassify($case[0], $case[1], $case[2], $case[3], []);
        $passed = $passed && $classified['reason'] === $case[4];
    }
    foreach ([['100', 'fake'], ['123', 'fake10000token'], ['123', 'fake', 'signature10000']] as $sensitive) {
        $passed = $passed && okpayDiagnosticSafeCode((object) ['code' => 10000], $sensitive) === null;
    }
    $passed = $passed && okpayDiagnosticSafeCode((object) ['code' => 10000], ['123', 'fake']) === '10000';
    $passed = $passed && okpayDiagnosticSafeCode((object) ['code' => '123456789'], []) === null;
    $passed = $passed && okpayDiagnosticSafeCode((object) ['code' => 'raw-secret'], []) === null;
    $warnings = okpayDiagnosticWarnings(" 123\t", "fake\u{200B}token\u{FEFF}");
    $passed = $passed && $warnings === ['id_boundary_whitespace' => true, 'id_invisible' => true,
        'token_boundary_whitespace' => false, 'token_invisible' => true];
    $passed = $passed && !in_array(true, okpayDiagnosticWarnings('00123', 'fake+%2B-token'), true);
    $warnings = okpayDiagnosticWarnings("\u{00A0}123", "fake\r\n");
    $passed = $passed && $warnings['id_boundary_whitespace'] && $warnings['token_boundary_whitespace'];
    $body = '';
    $overflow = false;
    $passed = $passed && okpayDiagnosticAppendResponse($body, str_repeat('x', OKPAY_DIAGNOSTIC_RESPONSE_LIMIT), $overflow) === OKPAY_DIAGNOSTIC_RESPONSE_LIMIT;
    $passed = $passed && okpayDiagnosticAppendResponse($body, 'x', $overflow) === 0 && $overflow;
    $signed = okpayDiagnosticSign([], '00123', 'fake-only-token');
    foreach (['php_form', 'document_json', 'hmac_sha256_form'] as $mode) {
        $payload = $mode === 'hmac_sha256_form' ? $hmacSigned : $signed;
        $options = okpayDiagnosticOptions($mode, $payload, function () { return 0; });
        $passed = $passed && $options[CURLOPT_URL] === OKPAY_DIAGNOSTIC_URL
            && $options[CURLOPT_FOLLOWLOCATION] === false && $options[CURLOPT_MAXREDIRS] === 0
            && $options[CURLOPT_SSL_VERIFYPEER] === true && $options[CURLOPT_SSL_VERIFYHOST] === 2
            && $options[CURLOPT_TIMEOUT] === 10 && $options[CURLOPT_CONNECTTIMEOUT] === 10
            && $options[CURLOPT_PROTOCOLS] === CURLPROTO_HTTPS && $options[CURLOPT_PROXY] === '';
        if ($mode !== 'document_json') {
            parse_str($options[CURLOPT_POSTFIELDS], $decoded);
        } else {
            $decoded = json_decode($options[CURLOPT_POSTFIELDS], true);
        }
        if ($mode === 'hmac_sha256_form') {
            $payload['timestamp'] = (string) $payload['timestamp'];
            $passed = $passed && $decoded === $payload;
        } else {
            $passed = $passed && $decoded === $signed && array_keys($decoded) === ['id', 'sign'];
        }
    }
    return ['kind' => 'self_test', 'passed' => $passed];
}

try {
    if (PHP_SAPI !== 'cli' || !extension_loaded('curl')) {
        throw new RuntimeException('本机 PHP 不可用');
    }
    if ($argc === 2 && $argv[1] === '--self-test') {
        $report = okpayDiagnosticSelfTest();
        echo json_encode($report, JSON_UNESCAPED_UNICODE);
        exit($report['passed'] ? 0 : 1);
    }
    if ($argc !== 1) {
        throw new RuntimeException('不支持的参数');
    }
    $input = stream_get_contents(STDIN, 65537);
    if ($input === false || strlen($input) > 65536) {
        throw new RuntimeException('输入无效');
    }
    $credentials = json_decode($input, true);
    $input = '';
    if (json_last_error() !== JSON_ERROR_NONE || !is_array($credentials) || count($credentials) !== 2
        || !isset($credentials['id'], $credentials['token']) || !is_string($credentials['id'])
        || !is_string($credentials['token']) || $credentials['id'] === '' || $credentials['token'] === '') {
        throw new RuntimeException('输入无效');
    }
    $warnings = okpayDiagnosticWarnings($credentials['id'], $credentials['token']);
    $signed = okpayDiagnosticSign([], $credentials['id'], $credentials['token']);
    $sensitive = [$credentials['id'], $credentials['token'], $signed['sign']];
    $checks = [];
    foreach (['php_form', 'document_json'] as $mode) {
        $checks[] = okpayDiagnosticRequest($mode, $signed, $sensitive);
    }
    $hmacSigned = okpayDiagnosticHmacSign([], $credentials['id'], $credentials['token'], time(), bin2hex(random_bytes(16)));
    $hmacSensitive = [$credentials['id'], $credentials['token'], $hmacSigned['sign']];
    $checks[] = okpayDiagnosticRequest('hmac_sha256_form', $hmacSigned, $hmacSensitive);
    unset($credentials, $signed, $sensitive, $hmacSigned, $hmacSensitive);
    echo json_encode(['kind' => 'diagnostic', 'credential_warnings' => $warnings, 'checks' => $checks], JSON_UNESCAPED_UNICODE);
} catch (Throwable $error) {
    // 固定输出，不输出异常、调用栈、原始响应或输入片段。
    echo '{"kind":"error"}';
    exit(1);
}
