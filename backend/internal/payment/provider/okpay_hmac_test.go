package provider

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type okpayHMACVector struct {
	name     string
	raw      string
	expected string
}

type okpayHMACInvalidCase struct {
	name   string
	fields okpayArray
}

func TestOKPayHMACMatchesPublishedGoldenVectors(t *testing.T) {
	// 固定值来自 Dujiao-Next 提交 d2e44618 的 OKPay 测试；不使用当前实现重算期望值。
	// 来源：https://github.com/dujiao-next/dujiao-next/blob/d2e44618de03068cbbe874a23a6d7c6a249a79ec/internal/modules/payment/infrastructure/gateway/okpay/okpay_test.go
	vectors := []okpayHMACVector{
		{
			name:     "请求向量A",
			raw:      `{"amount":"100.5","coin":"USDT","id":"10001","nonce":"a1b2c3d4e5","timestamp":"1782680000","unique_id":"ORDER-20260628-001"}`,
			expected: "7444ADFD8E4F4DA09D752DDF9345E0EE56DC25090FCFAF675DD042830E5E3F79",
		},
		{
			name:     "嵌套回调向量B",
			raw:      `{"status":"success","code":200,"data":{"order_id":"abc123def456","unique_id":"ORDER-20260628-001","pay_user_id":123456789,"amount":"100.5","coin":"USDT","status":1,"type":"deposit"},"id":10001,"sign":"64B09C8847849FA6921D8FFBDF8E406D4A8EA623E53970712350F61783403F7D"}`,
			expected: "64B09C8847849FA6921D8FFBDF8E406D4A8EA623E53970712350F61783403F7D",
		},
		{
			name:     "零布尔空值向量C",
			raw:      `{"a":0,"b":"0","c":null,"d":"","e":false,"f":"hello","id":7,"nest":{"x":1,"y":2}}`,
			expected: "8BC0AF979075038025DDD51B6F4A2E6CF3FF9B5B5371EB2268D303F89883E92A",
		},
	}
	for _, vector := range vectors {
		t.Run(vector.name, func(t *testing.T) {
			fields, err := okpayDecodeJSON(vector.raw)
			require.NoError(t, err)
			sign, err := okpayHMACSign(fields, "TESTtoken123456789abcdefghijABCD")
			require.NoError(t, err)
			require.Equal(t, vector.expected, sign)
			// 交换根对象顺序并替换顶层签名不影响结果。
			reordered := make(okpayArray, 0, len(fields)+1)
			for index := len(fields) - 1; index >= 0; index-- {
				if fields[index].key != "sign" {
					reordered = append(reordered, fields[index])
				}
			}
			reordered = append(reordered, okpayField{key: "sign", value: "被替换的签名"})
			sign, err = okpayHMACSign(reordered, "TESTtoken123456789abcdefghijABCD")
			require.NoError(t, err)
			require.Equal(t, vector.expected, sign)
			changed := append(okpayArray{}, fields...)
			changed = append(changed, okpayField{key: "extra", value: "0"})
			sign, err = okpayHMACSign(changed, "TESTtoken123456789abcdefghijABCD")
			require.NoError(t, err)
			require.NotEqual(t, vector.expected, sign, "新增零值字段必须改变签名")
		})
	}
}

func TestOKPayHMACPreservesRawValuesAndToken(t *testing.T) {
	fields := okpayArray{
		{key: "zero", value: json.Number("-0")},
		{key: "number", value: json.Number("1.2300e+04")},
		{key: "memo", value: " 中文 +%20&= "},
		{key: "id", value: "7"},
		{key: "flag", value: false},
	}
	token := " Token+%2B%20&=密钥 "
	sign, err := okpayHMACSign(fields, token)
	require.NoError(t, err)
	// 此固定值由 .NET HMACSHA256 对手写原文独立计算，覆盖不 trim、不解码及科学计数法。
	require.Equal(t, "B2AFDD0443AE961F4612F3310FF87DFA488830C3E64F57420625A5B3183CD4BD", sign)
	for _, alteredToken := range []string{strings.TrimSpace(token), okpayPHPURLDecode(token), token + "x"} {
		altered, err := okpayHMACSign(fields, alteredToken)
		require.NoError(t, err)
		require.NotEqual(t, sign, altered)
	}
	for _, alteredValue := range []any{json.Number("12300"), "1.2300E+04", " 1.2300e+04", json.Number("1.23e4")} {
		alteredFields := append(okpayArray{}, fields...)
		alteredFields[1].value = alteredValue
		altered, err := okpayHMACSign(alteredFields, token)
		require.NoError(t, err)
		require.NotEqual(t, sign, altered, "数字和值不能被重新格式化")
	}
}

func TestOKPayHMACNestedSortingAndArrayPaths(t *testing.T) {
	fields := okpayArray{
		{key: "id", value: "42"},
		{key: "data", value: okpayArray{
			{key: "z", value: json.Number("2")},
			{key: "a", value: okpayJSONArray{
				okpayArray{
					{key: "x", value: false},
					{key: "sign", value: "child-sign"},
					{key: "amount", value: json.Number("1.2300")},
				},
				json.Number("0"), nil, "",
			}},
		}},
		{key: "sign", value: "顶层签名不参与"},
	}
	sign, err := okpayHMACSign(fields, "nested-token")
	require.NoError(t, err)
	// 原文为 data.a[0].amount=1.2300&data.a[0].sign=child-sign&data.a[0].x=false&data.a[1]=0&data.z=2&id=42。
	require.Equal(t, "E17649FB14ED03DEEC5FB1577883F8E2A66C342FDE6312DF517152CCB73E1787", sign)
	data := fields[1].value.(okpayArray)
	data[0], data[1] = data[1], data[0]
	child := data[0].value.(okpayJSONArray)[0].(okpayArray)
	child[0], child[2] = child[2], child[0]
	reordered, err := okpayHMACSign(fields, "nested-token")
	require.NoError(t, err)
	require.Equal(t, sign, reordered, "所有嵌套对象的字段顺序均不影响签名")
	child[1].value = "被修改的内层签名字段"
	changed, err := okpayHMACSign(fields, "nested-token")
	require.NoError(t, err)
	require.NotEqual(t, sign, changed, "嵌套 sign 字段必须参与签名")
}

func TestOKPayHMACDistinguishesNumericObjectKeysFromArrayIndices(t *testing.T) {
	objectFields, err := okpayDecodeJSON(`{"data":{"0":"first"}}`)
	require.NoError(t, err)
	object, err := okpayHMACSign(objectFields, "shape-token")
	require.NoError(t, err)
	require.Equal(t, "417CBEAF1E167CE8CD6FFA000CD489983A36A88DF7987500422F0D07A76684AB", object)
	arrayFields, err := okpayDecodeJSON(`{"data":["first"]}`)
	require.NoError(t, err)
	array, err := okpayHMACSign(arrayFields, "shape-token")
	require.NoError(t, err)
	require.Equal(t, "1A9CE6DDFAD23CDE5955A7F964C8B6B56145935E5AD3E6C5F3096B5125B48501", array)
	require.NotEqual(t, object, array)
}

func TestOKPayHMACFiltersOnlyNullAndEmptyStrings(t *testing.T) {
	base := okpayArray{{key: "id", value: "7"}}
	original, err := okpayHMACSign(base, "filter-token")
	require.NoError(t, err)
	filtered := append(okpayArray{}, base...)
	filtered = append(filtered,
		okpayField{key: "nil", value: nil},
		okpayField{key: "empty", value: ""},
		okpayField{key: "empty_object", value: okpayArray{}},
		okpayField{key: "empty_array", value: okpayJSONArray{}},
	)
	sign, err := okpayHMACSign(filtered, "filter-token")
	require.NoError(t, err)
	require.Equal(t, original, sign)
	for _, value := range []any{json.Number("0"), "0", false, true, " ", "\t"} {
		retained := append(okpayArray{}, filtered...)
		retained = append(retained, okpayField{key: "retained", value: value})
		sign, err := okpayHMACSign(retained, "filter-token")
		require.NoError(t, err)
		require.NotEqual(t, original, sign, "空白和布尔、零值不能被过滤")
	}
}

func TestOKPayHMACRejectsAmbiguousOrUnsupportedFields(t *testing.T) {
	cases := []okpayHMACInvalidCase{
		{name: "点号字段与对象碰撞", fields: okpayArray{{key: "data.amount", value: "1"}, {key: "data", value: okpayArray{{key: "amount", value: "1"}}}}},
		{name: "对象与点号字段碰撞", fields: okpayArray{{key: "data", value: okpayArray{{key: "amount", value: "1"}}}, {key: "data.amount", value: "1"}}},
		{name: "空字段也不能掩盖碰撞", fields: okpayArray{{key: "data.amount", value: ""}, {key: "data", value: okpayArray{{key: "amount", value: nil}}}}},
		{name: "数组下标碰撞", fields: okpayArray{{key: "data[0]", value: "1"}, {key: "data", value: okpayJSONArray{"1"}}}},
		{name: "容器路径碰撞", fields: okpayArray{{key: "data.inner", value: okpayArray{}}, {key: "data", value: okpayArray{{key: "inner", value: okpayArray{}}}}}},
		{name: "重复顶层字段", fields: okpayArray{{key: "id", value: "7"}, {key: "id", value: "7"}}},
		{name: "重复内层字段", fields: okpayArray{{key: "data", value: okpayArray{{key: "id", value: "7"}, {key: "id", value: "7"}}}}},
		{name: "重复签名字段", fields: okpayArray{{key: "sign", value: "a"}, {key: "sign", value: "b"}}},
		{name: "空键名", fields: okpayArray{{key: "", value: "value"}}},
		{name: "键名含参数分隔符", fields: okpayArray{{key: "data&id", value: "7"}}},
		{name: "键名含等号", fields: okpayArray{{key: "data=id", value: "7"}}},
		{name: "键名含控制字符", fields: okpayArray{{key: "data\x00", value: "7"}}},
		{name: "不接受丢失精度的浮点类型", fields: okpayArray{{key: "amount", value: float64(1.23)}}},
		{name: "不接受未声明的对象类型", fields: okpayArray{{key: "data", value: map[string]any{"id": "7"}}}},
	}
	for _, invalidNumber := range []string{"", "NaN", "Inf", "true", " 1", "01", "+1", ".1", "1.", "1e", "1&admin=1"} {
		cases = append(cases, okpayHMACInvalidCase{name: "无效数字_" + invalidNumber, fields: okpayArray{{key: "amount", value: json.Number(invalidNumber)}}})
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			sign, err := okpayHMACSign(testCase.fields, "private-test-token")
			require.Error(t, err)
			require.Empty(t, sign)
			require.NotContains(t, err.Error(), "private-test-token")
		})
	}
	sign, err := okpayHMACSign(okpayArray{{key: "id", value: "7"}}, "")
	require.Error(t, err)
	require.Empty(t, sign)
}

func TestOKPayHMACEnforcesDepthLimit(t *testing.T) {
	var value any = "leaf"
	for level := 1; level < okpayHMACMaxDepth; level++ {
		value = okpayArray{{key: "nested", value: value}}
	}
	fields := okpayArray{{key: "data", value: value}}
	_, err := okpayHMACSign(fields, "depth-token")
	require.NoError(t, err)
	_, err = okpayHMACSign(okpayArray{{key: "outer", value: fields}}, "depth-token")
	require.Error(t, err)
	var array any = "leaf"
	for level := 0; level < okpayHMACMaxDepth; level++ {
		array = okpayJSONArray{array}
	}
	_, err = okpayHMACSign(okpayArray{{key: "data", value: array}}, "depth-token")
	require.Error(t, err)
}

func TestOKPayNonceUsesFullRandomBytesAndFailsClosed(t *testing.T) {
	randomBytes := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0xff}
	nonce, err := okpayNonceFrom(bytes.NewReader(randomBytes))
	require.NoError(t, err)
	require.Equal(t, "000102030405060708090a0b0c0d0eff", nonce)
	for length := 0; length < okpayNonceBytes; length++ {
		nonce, err := okpayNonceFrom(bytes.NewReader(randomBytes[:length]))
		require.Error(t, err)
		require.Empty(t, nonce, "随机源不足时不能返回部分随机数或时间戳")
	}
	nonce, err = okpayNonce()
	require.NoError(t, err)
	decoded, err := hex.DecodeString(nonce)
	require.NoError(t, err)
	require.Len(t, decoded, okpayNonceBytes)
	require.Equal(t, strings.ToLower(nonce), nonce)
}
