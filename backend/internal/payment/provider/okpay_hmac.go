package provider

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	okpayHMACMaxDepth = 16
	okpayNonceBytes   = 16
)

var okpayJSONNumberPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// JSON 对象与数组必须保留各自的类型，否则数字对象键会被误签为数组下标。
type okpayJSONArray []any

// okpayHMACSign 保留原始字符串、数字文本及密钥，使用扁平字段的 ASCII 顺序签名。
func okpayHMACSign(fields okpayArray, token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("OKPay HMAC 密钥不能为空")
	}
	flat := make(map[string]string)
	seen := make(map[string]struct{})
	if err := okpayHMACFlattenObject(fields, "", 0, flat, seen); err != nil {
		return "", err
	}
	keys := make([]string, 0, len(flat))
	for key := range flat {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+flat[key])
	}
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(strings.Join(pairs, "&")))
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil))), nil
}

func okpayHMACFlattenObject(fields okpayArray, prefix string, depth int, flat map[string]string, seen map[string]struct{}) error {
	if depth > okpayHMACMaxDepth {
		return fmt.Errorf("OKPay HMAC 参数嵌套过深")
	}
	objectKeys := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field.key == "" || strings.ContainsAny(field.key, "&=") || strings.IndexFunc(field.key, unicode.IsControl) >= 0 {
			return fmt.Errorf("OKPay HMAC 参数键名无效")
		}
		if _, exists := objectKeys[field.key]; exists {
			return fmt.Errorf("OKPay HMAC 参数键名重复")
		}
		objectKeys[field.key] = struct{}{}
		// 仅排除顶层签名，嵌套对象中名为 sign 的业务字段仍须参与签名。
		if depth == 0 && field.key == "sign" {
			continue
		}
		key := field.key
		if prefix != "" {
			key = prefix + "." + key
		}
		if err := okpayHMACFlattenValue(field.value, key, depth+1, flat, seen); err != nil {
			return err
		}
	}
	return nil
}

func okpayHMACFlattenValue(value any, key string, depth int, flat map[string]string, seen map[string]struct{}) error {
	if depth > okpayHMACMaxDepth {
		return fmt.Errorf("OKPay HMAC 参数嵌套过深")
	}
	// 连空值和容器也占用路径，防止被过滤字段掩盖 data.amount 与嵌套 data 的冲突。
	if _, exists := seen[key]; exists {
		return fmt.Errorf("OKPay HMAC 扁平字段路径冲突")
	}
	seen[key] = struct{}{}
	switch value := value.(type) {
	case nil:
		return nil
	case okpayArray:
		return okpayHMACFlattenObject(value, key, depth, flat, seen)
	case okpayJSONArray:
		for index, child := range value {
			if err := okpayHMACFlattenValue(child, key+"["+strconv.Itoa(index)+"]", depth+1, flat, seen); err != nil {
				return err
			}
		}
	default:
		text, err := okpayHMACScalar(value)
		if err != nil {
			return err
		}
		if text != "" {
			flat[key] = text
		}
	}
	return nil
}

func okpayHMACScalar(value any) (string, error) {
	switch value := value.(type) {
	case nil:
		return "", nil
	case string:
		return value, nil
	case json.Number:
		if !okpayJSONNumberPattern.MatchString(string(value)) {
			return "", fmt.Errorf("OKPay HMAC 数字参数无效")
		}
		return string(value), nil
	case bool:
		return strconv.FormatBool(value), nil
	default:
		return "", fmt.Errorf("OKPay HMAC 参数类型无效")
	}
}

func okpayNonce() (string, error) {
	return okpayNonceFrom(rand.Reader)
}

func okpayNonceFrom(source io.Reader) (string, error) {
	buffer := make([]byte, okpayNonceBytes)
	if _, err := io.ReadFull(source, buffer); err != nil {
		// 随机源失败时终止请求，不能用时间戳代替不可预测的随机数。
		return "", fmt.Errorf("OKPay 安全随机数生成失败")
	}
	return hex.EncodeToString(buffer), nil
}
