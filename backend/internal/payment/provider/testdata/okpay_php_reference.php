<?php
// 附件 OkayPay (1).php 的原始 sign/checkSign 方法保持不变，仅使用假凭据做离线对照。
class OKPayPHPReference
{
    protected $id;
    protected $token;

    public function __construct($id, $token)
    {
        $this->id = $id;
        $this->token = $token;
    }

    public function sign(array $data)
    {
        $data['id'] = $this->id;
        $data = array_filter($data);
        ksort($data);
        $data['sign'] = strtoupper(md5(urldecode(http_build_query($data) . '&token=' . $this->token)));
        return $data;
    }

    // 校验数据签名
    public function checkSign($data)
    {
        $in_sign = $data['sign'];
        unset($data['sign']);
        $data = array_filter($data);
        ksort($data);
        $sign = strtoupper(md5(urldecode(http_build_query($data) . '&token=' . $this->token)));
        return $in_sign == $sign ? true : false;
    }

}

$fixtures = json_decode(file_get_contents($argv[1]), true);
$candidates = json_decode(stream_get_contents(STDIN), true);
$results = [];
foreach ($fixtures['cases'] as $index => $fixture) {
    $reference = new OKPayPHPReference($fixture['id'], $fixture['token']);
    $signed = $reference->sign($fixture['fields']);
    parse_str($candidates[$index]['body'], $received);
    $results[] = [
        'name' => $fixture['name'],
        'signature' => $signed['sign'],
        'body' => http_build_query($signed),
        'accepts_go_request' => $reference->checkSign($received),
    ];
}
echo json_encode($results, JSON_UNESCAPED_UNICODE);
