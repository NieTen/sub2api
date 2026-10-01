<?php
// 本工具会创建独立测试订单；只有标准输入 execute 严格为 true 时才允许请求固定网关。
ini_set('display_errors', '0');
ini_set('log_errors', '0');
error_reporting(0);
define('OKPAY_DIAGNOSTIC_LIBRARY_ONLY', true);
require __DIR__ . '/diagnose-okpay.php';

const OKPAY_CHECKOUT_URL = 'https://api.okaypay.me/shop/payLink';
const OKPAY_CHECKOUT_LONG_URL_BYTES = 322;
const OKPAY_CHECKOUT_NAME = 'Sub2API Probe';

function okpayCheckoutBaseURL($value)
{
    if (!is_string($value) || $value === '' || strlen($value) > 200
        || preg_match('/[\x00-\x20\x7F\\\\]/', $value) === 1
        || strpos($value, '?') !== false || strpos($value, '#') !== false
        || preg_match('/\Ahttps:\/\//i', $value) !== 1) {
        throw new RuntimeException('基础地址无效');
    }
    $url = parse_url($value);
    if ($url === false || !isset($url['scheme'], $url['host']) || strtolower($url['scheme']) !== 'https'
        || $url['host'] === '' || array_key_exists('user', $url) || array_key_exists('pass', $url)
        || array_key_exists('query', $url) || array_key_exists('fragment', $url)
        || (isset($url['path']) && $url['path'] !== '' && $url['path'] !== '/')
        || (isset($url['port']) && ($url['port'] < 1 || $url['port'] > 65535))
        || filter_var($value, FILTER_VALIDATE_URL) === false) {
        throw new RuntimeException('基础地址无效');
    }
    return rtrim($value, '/');
}

function okpayCheckoutDecodeInput($raw)
{
    if (!is_string($raw) || strlen($raw) > 65536) {
        throw new RuntimeException('输入无效');
    }
    $input = json_decode($raw, true);
    if (json_last_error() !== JSON_ERROR_NONE || !is_array($input) || count($input) !== 4
        || !isset($input['id'], $input['token'], $input['base_url'], $input['execute'])
        || !is_string($input['id']) || $input['id'] === ''
        || !is_string($input['token']) || $input['token'] === ''
        || $input['execute'] !== true) {
        throw new RuntimeException('输入无效');
    }
    $input['base_url'] = okpayCheckoutBaseURL($input['base_url']);
    return $input;
}

function okpayCheckoutOrderID()
{
    return 'probe' . bin2hex(random_bytes(8));
}

function okpayCheckoutLongURL($base)
{
    // 两个标识和恢复值均为合成数据，不对应本站订单，也不是有效的恢复令牌。
    $syntheticID = okpayCheckoutOrderID();
    $query = ['order_id' => $syntheticID, 'out_trade_no' => $syntheticID, 'resume_token' => '', 'status' => 'success'];
    $prefix = $base . '/payment/result?';
    $tokenBytes = OKPAY_CHECKOUT_LONG_URL_BYTES - strlen($prefix . http_build_query($query, '', '&', PHP_QUERY_RFC3986));
    $invalidPrefix = 'invalid-probe-';
    if ($tokenBytes < 32) {
        throw new RuntimeException('基础地址过长');
    }
    $query['resume_token'] = $invalidPrefix . str_repeat('x', $tokenBytes - strlen($invalidPrefix));
    $url = $prefix . http_build_query($query, '', '&', PHP_QUERY_RFC3986);
    if (strlen($url) !== OKPAY_CHECKOUT_LONG_URL_BYTES) {
        throw new RuntimeException('测试地址构造失败');
    }
    return $url;
}

function okpayCheckoutModes()
{
    return ['current_long_status', 'short_status', 'long_no_status', 'short_no_status'];
}

function okpayCheckoutFields($mode, $base, $longURL)
{
    if (!in_array($mode, okpayCheckoutModes(), true)) {
        throw new RuntimeException('测试模式无效');
    }
    $fields = [
        'unique_id' => okpayCheckoutOrderID(), 'name' => OKPAY_CHECKOUT_NAME,
        'amount' => '1.51', 'coin' => 'USDT',
        'return_url' => in_array($mode, ['current_long_status', 'long_no_status'], true) ? $longURL : $base . '/payment/result',
        'callback_url' => $base . '/api/v1/payment/webhook/okpay',
    ];
    if (in_array($mode, ['current_long_status', 'short_status'], true)) {
        $fields['status'] = '0';
    }
    return $fields;
}

function okpayCheckoutValidPayURL($value)
{
    if (!is_string($value) || $value === '' || strlen($value) > 8192
        || preg_match('/[\x00-\x20\x7F\\\\]/', $value) === 1
        || preg_match('/%(?![0-9a-f]{2})/i', $value) === 1) {
        return false;
    }
    $url = parse_url($value);
    return $url !== false && isset($url['scheme'], $url['host'])
        && strtolower($url['scheme']) === 'https' && $url['host'] !== ''
        && !array_key_exists('user', $url) && !array_key_exists('pass', $url)
        && filter_var($value, FILTER_VALIDATE_URL) !== false;
}

function okpayCheckoutMessageCategory($response)
{
    // 汇总两个消息字段，认证或限流必须优先中止，不能被另一个“订单创建失败”覆盖。
    $categories = [];
    foreach (['msg', 'message'] as $key) {
        if (!isset($response->$key) || !is_string($response->$key)) {
            continue;
        }
        $category = okpayDiagnosticMessageCategory((object) [$key => $response->$key]);
        $categories[$category] = true;
        if (trim($response->$key) === '订单创建失败') {
            $categories['order_creation_failed'] = true;
        }
    }
    foreach (['auth_failed', 'signature_failed', 'merchant_invalid', 'rate_limited', 'order_creation_failed', 'invalid_parameters'] as $category) {
        if (isset($categories[$category])) {
            return $category;
        }
    }
    return 'unknown_business_error';
}

function okpayCheckoutClassify($httpStatus, $body, $curlError, $overflow, array $sensitive)
{
    $result = ['http_status' => $httpStatus >= 100 && $httpStatus <= 599 ? $httpStatus : null,
        'reason' => 'invalid_response', 'business_code' => null, 'checkout_created' => false];
    if ($overflow) {
        return $result;
    }
    if ($curlError !== 0) {
        $transport = okpayDiagnosticClassify($httpStatus, '', $curlError, false, $sensitive);
        $result['reason'] = $transport['reason'];
        return $result;
    }
    $response = json_decode($body);
    $validResponse = json_last_error() === JSON_ERROR_NONE && is_object($response);
    if ($validResponse) {
        $result['business_code'] = okpayDiagnosticSafeCode($response, $sensitive);
    }
    if ($httpStatus === 429 || $httpStatus === 401 || $httpStatus === 403) {
        $result['reason'] = $httpStatus === 429 ? 'rate_limited' : 'auth_failed';
        return $result;
    }
    if ($httpStatus < 200 || $httpStatus >= 300) {
        $result['reason'] = 'http_error';
        return $result;
    }
    if (!$validResponse) {
        return $result;
    }
    foreach (['status', 'code'] as $key) {
        if (property_exists($response, $key) &&
            ((!is_string($response->$key) && !is_int($response->$key)) || (string) $response->$key === '')) {
            return $result;
        }
    }
    $hasStatus = property_exists($response, 'status');
    $hasCode = property_exists($response, 'code');
    $statusOK = $hasStatus && $response->status === 'success';
    $codeOK = $hasCode && in_array((string) $response->code, ['200', '10000'], true);
    if (!$statusOK || !$codeOK) {
        $category = okpayCheckoutMessageCategory($response);
        if (($hasStatus && !$statusOK) || ($hasCode && !$codeOK) || $category !== 'unknown_business_error') {
            $result['reason'] = $category;
        }
        return $result;
    }
    if (!isset($response->data) || !is_object($response->data)
        || !isset($response->data->order_id, $response->data->pay_url)
        || (!is_string($response->data->order_id) && !is_int($response->data->order_id))
        || trim((string) $response->data->order_id) === ''
        || !okpayCheckoutValidPayURL($response->data->pay_url)) {
        return $result;
    }
    $result['reason'] = 'success';
    $result['checkout_created'] = true;
    return $result;
}

function okpayCheckoutOptions(array $signed, $writeCallback)
{
    // 复用 TLS、超时、禁止代理/重定向等限制，端点始终由代码固定。
    $options = okpayDiagnosticOptions('hmac_sha256_form', $signed, $writeCallback);
    $options[CURLOPT_URL] = OKPAY_CHECKOUT_URL;
    return $options;
}

function okpayCheckoutRequest(array $signed, array $sensitive)
{
    $body = '';
    $overflow = false;
    $curl = curl_init();
    if ($curl === false) {
        return okpayCheckoutClassify(0, '', 1, false, $sensitive);
    }
    try {
        $callback = function ($handle, $chunk) use (&$body, &$overflow) {
            return okpayDiagnosticAppendResponse($body, $chunk, $overflow);
        };
        if (!curl_setopt_array($curl, okpayCheckoutOptions($signed, $callback))) {
            return okpayCheckoutClassify(0, '', 1, false, $sensitive);
        }
        // 每个组合只请求一次，不重试、不跳转，也不访问返回的收银台地址。
        curl_exec($curl);
        return okpayCheckoutClassify((int) curl_getinfo($curl, CURLINFO_HTTP_CODE), $body,
            curl_errno($curl), $overflow, $sensitive);
    } finally {
        curl_close($curl);
        $body = '';
    }
}

function okpayCheckoutShouldStop($reason)
{
    return !in_array($reason, ['success', 'order_creation_failed', 'invalid_parameters', 'unknown_business_error'], true);
}

function okpayCheckoutProbe(array $input, $requester = null)
{
    if (!isset($input['execute']) || $input['execute'] !== true) {
        throw new RuntimeException('未明确执行');
    }
    $base = okpayCheckoutBaseURL($input['base_url']);
    $longURL = okpayCheckoutLongURL($base);
    $report = ['kind' => 'checkout_probe',
        'credential_warnings' => okpayDiagnosticWarnings($input['id'], $input['token']),
        'checks' => [], 'stopped' => false, 'gateway_may_have_created_order' => false];
    foreach (okpayCheckoutModes() as $mode) {
        if ($requester === null && count($report['checks']) > 0) {
            // 真实四组之间留出间隔，离线注入测试不等待，也不会发起网络调用。
            sleep(1);
        }
        $summary = ['return_bytes' => in_array($mode, ['current_long_status', 'long_no_status'], true)
            ? OKPAY_CHECKOUT_LONG_URL_BYTES : strlen($base . '/payment/result'),
            'status_present' => in_array($mode, ['current_long_status', 'short_status'], true)];
        try {
            $fields = okpayCheckoutFields($mode, $base, $longURL);
            $signed = okpayDiagnosticHmacSign($fields, $input['id'], $input['token'], time(), bin2hex(random_bytes(16)));
            $sensitive = [$input['id'], $input['token'], $signed['sign'], $signed['nonce'], $signed['unique_id'], $longURL];
            // 即使没有收到响应，也不能保证网关未建单；后续仅提示此固定布尔标记。
            $report['gateway_may_have_created_order'] = true;
            $result = $requester === null ? okpayCheckoutRequest($signed, $sensitive) : $requester($signed, $sensitive);
        } catch (Throwable $error) {
            $result = ['http_status' => null, 'reason' => 'network_error', 'business_code' => null, 'checkout_created' => false];
        }
        $report['checks'][] = ['mode' => $mode] + $result + ['request' => $summary];
        unset($fields, $signed, $sensitive);
        if (okpayCheckoutShouldStop($result['reason'])) {
            $report['stopped'] = true;
            break;
        }
    }
    return $report;
}

function okpayCheckoutSelfTest()
{
    $passed = true;
    $golden = okpayDiagnosticHmacSign(['amount' => '100.5', 'coin' => 'USDT', 'unique_id' => 'ORDER-20260628-001'],
        '10001', 'TESTtoken123456789abcdefghijABCD', 1782680000, 'a1b2c3d4e5');
    $passed = $passed && hash_equals('7444ADFD8E4F4DA09D752DDF9345E0EE56DC25090FCFAF675DD042830E5E3F79', $golden['sign']);
    $input = ['id' => 'fixture-merchant', 'token' => 'fixture-private-token', 'base_url' => 'https://site.example', 'execute' => true];
    $base = okpayCheckoutBaseURL($input['base_url'] . '/');
    $longURL = okpayCheckoutLongURL($base);
    $url = parse_url($longURL);
    parse_str($url['query'], $query);
    $passed = $passed && strlen($longURL) === 322 && $url['path'] === '/payment/result'
        && array_keys($query) === ['order_id', 'out_trade_no', 'resume_token', 'status']
        && strpos($query['resume_token'], 'invalid-probe-') === 0;
    $seen = [];
    $nonces = [];
    $calls = 0;
    $report = okpayCheckoutProbe($input, function ($signed, $sensitive) use (&$passed, &$seen, &$nonces, &$calls, $base) {
        $mode = okpayCheckoutModes()[$calls++];
        $isLong = in_array($mode, ['current_long_status', 'long_no_status'], true);
        $hasStatus = in_array($mode, ['current_long_status', 'short_status'], true);
        $passed = $passed && $signed['amount'] === '1.51' && $signed['coin'] === 'USDT'
            && $signed['name'] === OKPAY_CHECKOUT_NAME && strlen($signed['name']) === 13
            && $signed['callback_url'] === $base . '/api/v1/payment/webhook/okpay'
            && array_key_exists('status', $signed) === $hasStatus && (!$hasStatus || $signed['status'] === '0')
            && strlen($signed['return_url']) === ($isLong ? 322 : strlen($base . '/payment/result'))
            && ($isLong || $signed['return_url'] === $base . '/payment/result')
            && preg_match('/\Aprobe[0-9a-f]{16}\z/', $signed['unique_id']) === 1
            && !isset($seen[$signed['unique_id']]) && !isset($nonces[$signed['nonce']]);
        $seen[$signed['unique_id']] = true;
        $nonces[$signed['nonce']] = true;
        $options = okpayCheckoutOptions($signed, function () { return 0; });
        $passed = $passed && $options[CURLOPT_URL] === OKPAY_CHECKOUT_URL && $options[CURLOPT_POST] === true
            && $options[CURLOPT_FOLLOWLOCATION] === false && $options[CURLOPT_MAXREDIRS] === 0
            && $options[CURLOPT_SSL_VERIFYPEER] === true && $options[CURLOPT_SSL_VERIFYHOST] === 2
            && $options[CURLOPT_TIMEOUT] === 10 && $options[CURLOPT_CONNECTTIMEOUT] === 10
            && $options[CURLOPT_PROTOCOLS] === CURLPROTO_HTTPS && $options[CURLOPT_PROXY] === '';
        return okpayCheckoutClassify(200, '{"status":"warning","code":400,"msg":"订单创建失败"}', 0, false, $sensitive);
    });
    $passed = $passed && $calls === 4 && count($report['checks']) === 4 && !$report['stopped'];
    $success = '{"status":"success","code":200,"data":{"order_id":"private-order","pay_url":"https://cashier.example/private-pay-url"}}';
    $cases = [
        [200, $success, 0, false, 'success', true],
        [200, str_replace('"code":200', '"code":10000', $success), 0, false, 'success', true],
        [200, '{"status":"warning","code":400,"msg":"订单创建失败"}', 0, false, 'order_creation_failed', false],
        [200, '{"status":"warning","code":400,"msg":"订单创建失败 fixture-private-token"}', 0, false, 'unknown_business_error', false],
        [200, '{"status":"warning","msg":"身份认证失败"}', 0, false, 'auth_failed', false],
        [200, '{"status":"warning","msg":"签名错误"}', 0, false, 'signature_failed', false],
        [200, '{"status":"warning","msg":"请求过于频繁"}', 0, false, 'rate_limited', false],
        [429, '', 0, false, 'rate_limited', false],
        [401, '', 0, false, 'auth_failed', false],
        [503, $success, 0, false, 'http_error', false],
        [200, '{"status":"success","code":200,"data":{}}', 0, false, 'invalid_response', false],
        [200, str_replace('"code":200,', '', $success), 0, false, 'invalid_response', false],
        [200, str_replace('"status":"success",', '', $success), 0, false, 'invalid_response', false],
        [200, str_replace('https://cashier.example', 'http://cashier.example', $success), 0, false, 'invalid_response', false],
        [200, str_replace('private-order', ' ', $success), 0, false, 'invalid_response', false],
        [200, '<html>fixture-private-token</html>', 0, false, 'invalid_response', false],
        [0, '', 28, false, 'network_error', false],
        [0, '', 60, false, 'tls_failed', false],
        [200, '', 23, true, 'invalid_response', false],
    ];
    foreach ($cases as $case) {
        $classified = okpayCheckoutClassify($case[0], $case[1], $case[2], $case[3], ['fixture-private-token', 'fixture-merchant']);
        $passed = $passed && $classified['reason'] === $case[4] && $classified['checkout_created'] === $case[5];
        $encoded = json_encode($classified);
        foreach (['fixture-private-token', 'private-order', 'private-pay-url', 'data', 'pay_url'] as $private) {
            $passed = $passed && strpos($encoded, $private) === false;
        }
    }
    foreach (['network_error', 'tls_failed', 'http_error', 'invalid_response', 'auth_failed', 'signature_failed', 'merchant_invalid', 'rate_limited'] as $reason) {
        $stopCalls = 0;
        $stopped = okpayCheckoutProbe($input, function () use (&$stopCalls, $reason) {
            $stopCalls++;
            return ['http_status' => null, 'reason' => $reason, 'business_code' => null, 'checkout_created' => false];
        });
        $passed = $passed && $stopCalls === 1 && count($stopped['checks']) === 1 && $stopped['stopped']
            && $stopped['gateway_may_have_created_order'];
    }
    foreach ([
        ['订单创建失败', '身份认证失败', 'auth_failed'],
        ['订单创建失败', '请求过于频繁', 'rate_limited'],
        ['参数错误', '签名错误', 'signature_failed'],
        ['商户不存在', '订单创建失败', 'merchant_invalid'],
    ] as $mixed) {
        $stopCalls = 0;
        $stopped = okpayCheckoutProbe($input, function ($signed, $sensitive) use (&$stopCalls, $mixed) {
            $stopCalls++;
            return okpayCheckoutClassify(200, json_encode(['status' => 'warning', 'code' => 400,
                'msg' => $mixed[0], 'message' => $mixed[1]]), 0, false, $sensitive);
        });
        $passed = $passed && $stopCalls === 1 && $stopped['stopped']
            && count($stopped['checks']) === 1 && $stopped['checks'][0]['reason'] === $mixed[2];
    }
    $privateCode = okpayCheckoutClassify(200,
        '{"status":"warning","code":98765432,"msg":"订单创建失败","data":{"token":"fixture-private-token"}}',
        0, false, ['fixture-private-token', 'merchant98765432']);
    $passed = $passed && $privateCode['business_code'] === null
        && strpos(json_encode($privateCode), 'fixture-private-token') === false;
    foreach (['http://site.example', '/relative', 'https://site.example/path', 'https://user:pass@site.example',
        'https://@site.example', 'https://site.example?', 'https://site.example#', 'https://site.example\\path',
        "https://si\nte.example", 'https://site.example/%ZZ'] as $invalidBase) {
        $rejected = false;
        try {
            okpayCheckoutBaseURL($invalidBase);
        } catch (Throwable $error) {
            $rejected = true;
        }
        $passed = $passed && $rejected;
    }
    foreach ([false, 'true', 1, null] as $execute) {
        $invalid = $input;
        $invalid['execute'] = $execute;
        $rejected = false;
        try {
            okpayCheckoutDecodeInput(json_encode($invalid));
        } catch (Throwable $error) {
            $rejected = true;
        }
        $passed = $passed && $rejected;
    }
    $passed = $passed && okpayCheckoutDecodeInput(json_encode($input)) === $input;
    $body = '';
    $overflow = false;
    $passed = $passed && okpayDiagnosticAppendResponse($body, str_repeat('x', OKPAY_DIAGNOSTIC_RESPONSE_LIMIT), $overflow) === OKPAY_DIAGNOSTIC_RESPONSE_LIMIT;
    $passed = $passed && okpayDiagnosticAppendResponse($body, 'x', $overflow) === 0 && $overflow;
    return ['kind' => 'self_test', 'passed' => $passed];
}

try {
    if (PHP_SAPI !== 'cli' || !extension_loaded('curl')) {
        throw new RuntimeException('本机 PHP 不可用');
    }
    if ($argc === 2 && $argv[1] === '--self-test') {
        $report = okpayCheckoutSelfTest();
        echo json_encode($report, JSON_UNESCAPED_UNICODE);
        exit($report['passed'] ? 0 : 1);
    }
    if ($argc !== 1) {
        throw new RuntimeException('不支持的参数');
    }
    $raw = stream_get_contents(STDIN, 65537);
    $input = okpayCheckoutDecodeInput($raw);
    $raw = '';
    $report = okpayCheckoutProbe($input);
    unset($input);
    echo json_encode($report, JSON_UNESCAPED_UNICODE);
} catch (Throwable $error) {
    // 未通过输入检查时没有请求；任何异常都不回显原文、凭据或调用栈。
    echo '{"kind":"error"}';
    exit(1);
}
