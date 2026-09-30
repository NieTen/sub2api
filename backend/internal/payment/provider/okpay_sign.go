package provider

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// PHP 数组保持插入顺序；只有最外层参数参与 ksort，不能递归排序 data。
type okpayField struct {
	key   string
	value any
}
type okpayArray []okpayField

func (a okpayArray) get(key string) (any, bool) {
	for _, field := range a {
		if field.key == key {
			return field.value, true
		}
	}
	return nil, false
}

func okpaySign(fields okpayArray, token string) (string, error) {
	filtered := make(okpayArray, 0, len(fields))
	for _, field := range fields {
		if field.key != "sign" && okpayPHPTruthy(field.value) {
			filtered = append(filtered, field)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].key < filtered[j].key })
	var pairs []string
	for _, field := range filtered {
		if err := okpayPHPQuery(&pairs, field.key, field.value); err != nil {
			return "", err
		}
	}
	// http_build_query 后再 urldecode 等价于原始键值拼接；样例把 token 也放在 urldecode 内。
	canonical := strings.Join(pairs, "&") + "&token=" + okpayPHPURLDecode(token)
	sum := md5.Sum([]byte(canonical))
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}

func okpayPHPTruthy(value any) bool {
	switch value := value.(type) {
	case nil:
		return false
	case bool:
		return value
	case string:
		return value != "" && value != "0"
	case json.Number:
		n, err := strconv.ParseFloat(string(value), 64)
		return err == nil && n != 0
	case okpayArray:
		return len(value) > 0
	case okpayJSONArray:
		return len(value) > 0
	default:
		return true
	}
}

func okpayPHPQuery(pairs *[]string, key string, value any) error {
	if value == nil {
		return nil
	}
	if values, ok := value.(okpayJSONArray); ok {
		for index, item := range values {
			if err := okpayPHPQuery(pairs, key+"["+strconv.Itoa(index)+"]", item); err != nil {
				return err
			}
		}
		return nil
	}
	if fields, ok := value.(okpayArray); ok {
		for _, field := range fields {
			if err := okpayPHPQuery(pairs, key+"["+field.key+"]", field.value); err != nil {
				return err
			}
		}
		return nil
	}
	text, err := okpayPHPScalar(value)
	if err != nil {
		return err
	}
	*pairs = append(*pairs, key+"="+text)
	return nil
}

func okpayPHPScalar(value any) (string, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case bool:
		if value {
			return "1", nil
		}
		return "0", nil
	case json.Number:
		raw := string(value)
		if !strings.ContainsAny(raw, ".eE") {
			if integer, err := strconv.ParseInt(raw, 10, 64); err == nil {
				return strconv.FormatInt(integer, 10), nil
			}
		}
		number, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return "", fmt.Errorf("OKPay 数字参数无效")
		}
		// PHP 默认 precision=14，浮点字符串的指数没有多余前导零，并保留指数形式的小数点。
		formatted := strconv.FormatFloat(number, 'G', 14, 64)
		if index := strings.IndexByte(formatted, 'E'); index >= 0 {
			mantissa := formatted[:index]
			if !strings.Contains(mantissa, ".") {
				mantissa += ".0"
			}
			exponent, _ := strconv.Atoi(formatted[index+1:])
			formatted = fmt.Sprintf("%sE%+d", mantissa, exponent)
		}
		return formatted, nil
	default:
		return "", fmt.Errorf("OKPay 签名参数类型无效")
	}
}

func okpayPHPURLDecode(value string) string {
	var decoded strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '+' {
			decoded.WriteByte(' ')
			continue
		}
		if value[i] == '%' && i+2 < len(value) {
			if b, err := strconv.ParseUint(value[i+1:i+3], 16, 8); err == nil {
				decoded.WriteByte(byte(b))
				i += 2
				continue
			}
		}
		decoded.WriteByte(value[i])
	}
	return decoded.String()
}

func okpayDecodeJSON(raw string) (okpayArray, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	value, err := okpayDecodeJSONValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("OKPay JSON 包含多余内容")
	}
	fields, ok := value.(okpayArray)
	if !ok || !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return nil, fmt.Errorf("OKPay 参数须为 JSON 对象")
	}
	return fields, nil
}

func okpayDecodeJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 16 {
		return nil, fmt.Errorf("OKPay 参数嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, nested := token.(json.Delim)
	if !nested {
		return token, nil
	}
	if delimiter != '{' && delimiter != '[' {
		return nil, fmt.Errorf("OKPay JSON 无效")
	}
	fields := okpayArray{}
	for decoder.More() {
		key := strconv.Itoa(len(fields))
		if delimiter == '{' {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			var ok bool
			key, ok = keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("OKPay JSON 键名无效")
			}
		}
		if _, exists := fields.get(key); exists {
			return nil, fmt.Errorf("OKPay JSON 参数重复")
		}
		value, err := okpayDecodeJSONValue(decoder, depth+1)
		if err != nil {
			return nil, err
		}
		fields = append(fields, okpayField{key: key, value: value})
	}
	_, err = decoder.Token()
	if delimiter == '[' {
		values := make(okpayJSONArray, len(fields))
		for index, field := range fields {
			values[index] = field.value
		}
		return values, err
	}
	return fields, err
}

func okpayDecodeForm(raw string) (okpayArray, error) {
	fields := okpayArray{}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		rawKey, rawValue, _ := strings.Cut(pair, "=")
		key, err := url.QueryUnescape(rawKey)
		if err != nil {
			return nil, fmt.Errorf("OKPay 表单键名无效")
		}
		value, err := url.QueryUnescape(rawValue)
		if err != nil {
			return nil, fmt.Errorf("OKPay 表单值无效")
		}
		keys, err := okpayFormKeys(key)
		if err != nil {
			return nil, err
		}
		if err = okpayInsertForm(&fields, keys, value); err != nil {
			return nil, err
		}
	}
	return fields, nil
}

func okpayFormKeys(key string) ([]string, error) {
	base, rest, nested := strings.Cut(key, "[")
	if base == "" || strings.ContainsAny(base, " ].\x00") {
		return nil, fmt.Errorf("OKPay 表单键名无效")
	}
	keys := []string{base}
	for nested {
		part, tail, found := strings.Cut(rest, "]")
		if !found || strings.ContainsAny(part, "[\x00") || len(keys) >= 16 {
			return nil, fmt.Errorf("OKPay 表单层级无效")
		}
		keys = append(keys, part)
		if tail == "" {
			break
		}
		if tail[0] != '[' {
			return nil, fmt.Errorf("OKPay 表单层级无效")
		}
		rest = tail[1:]
	}
	return keys, nil
}

func okpayInsertForm(fields *okpayArray, keys []string, value string) error {
	key := keys[0]
	if key == "" {
		next := int64(0)
		for _, field := range *fields {
			if index, err := strconv.ParseInt(field.key, 10, 64); err == nil && index >= next {
				next = index + 1
			}
		}
		key = strconv.FormatInt(next, 10)
	}
	index := -1
	for i, field := range *fields {
		if field.key == key {
			index = i
			break
		}
	}
	if len(keys) == 1 {
		if index >= 0 {
			return fmt.Errorf("OKPay 表单参数重复")
		}
		*fields = append(*fields, okpayField{key: key, value: value})
		return nil
	}
	child := okpayArray{}
	if index >= 0 {
		var ok bool
		child, ok = (*fields)[index].value.(okpayArray)
		if !ok {
			return fmt.Errorf("OKPay 表单参数类型冲突")
		}
	}
	if err := okpayInsertForm(&child, keys[1:], value); err != nil {
		return err
	}
	if index >= 0 {
		(*fields)[index].value = child
	} else {
		*fields = append(*fields, okpayField{key: key, value: child})
	}
	return nil
}
