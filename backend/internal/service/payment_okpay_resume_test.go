//go:build unit

package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func okpayResumeTestClaims() ResumeTokenClaims {
	now := time.Now().Unix()
	return ResumeTokenClaims{
		OrderID: 42, UserID: 7, ProviderInstanceID: "19", ProviderKey: payment.TypeOKPay,
		PaymentType: payment.TypeOKPay, CanonicalReturnURL: "https://zzzai.pro/payment/result",
		IssuedAt: now, ExpiresAt: now + int64((24*time.Hour)/time.Second),
	}
}

// 单独构造带有效签名的畸形输入，验证解析器不会只验签而忽略主体和时间边界。
func okpayResumeTestRawToken(key []byte, values []uint64, namespace string) string {
	payload := make([]byte, len(values)*8)
	for index, value := range values {
		binary.BigEndian.PutUint64(payload[index*8:], value)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(namespace + encoded))
	return "op1." + encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestOKPayResumeTokenRoundTripAndFixedLength(t *testing.T) {
	t.Parallel()
	svc := NewPaymentResumeService([]byte("okpay-resume-test-signing-key"))
	for _, tc := range []struct {
		name string
		oid  int64
		uid  int64
		pi   string
	}{
		{"常用编号", 42, 7, "19"},
		{"最大编号仍保持固定长度", math.MaxInt64, math.MaxInt64, "9223372036854775807"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := okpayResumeTestClaims()
			claims.OrderID, claims.UserID, claims.ProviderInstanceID = tc.oid, tc.uid, tc.pi
			token, err := svc.CreateOKPayToken(claims)
			require.NoError(t, err)
			require.Len(t, token, 102)
			parts := strings.Split(token, ".")
			require.Len(t, parts, 3)
			require.Equal(t, "op1", parts[0])
			payload, err := base64.RawURLEncoding.DecodeString(parts[1])
			require.NoError(t, err)
			require.Len(t, payload, 40)
			signature, err := base64.RawURLEncoding.DecodeString(parts[2])
			require.NoError(t, err)
			require.Len(t, signature, sha256.Size, "不能通过截短签名缩减恢复链接")
			got, err := svc.ParseToken(token)
			require.NoError(t, err)
			claims.CanonicalReturnURL = ""
			require.Equal(t, claims, *got)
		})
	}
}

func TestOKPayResumeTokenDefaultsExpiry(t *testing.T) {
	t.Parallel()
	svc := NewPaymentResumeService([]byte("okpay-resume-test-signing-key"))
	claims := okpayResumeTestClaims()
	claims.IssuedAt, claims.ExpiresAt = 0, 0
	before := time.Now().Unix()
	token, err := svc.CreateOKPayToken(claims)
	require.NoError(t, err)
	got, err := svc.ParseToken(token)
	require.NoError(t, err)
	require.GreaterOrEqual(t, got.IssuedAt, before)
	require.LessOrEqual(t, got.IssuedAt, time.Now().Unix())
	require.Equal(t, int64(24*60*60), got.ExpiresAt-got.IssuedAt)
}

func TestOKPayResumeTokenRejectsInvalidCreateClaims(t *testing.T) {
	t.Parallel()
	svc := NewPaymentResumeService([]byte("okpay-resume-test-signing-key"))
	for _, instanceID := range []string{"", "0", "-1", "+1", "01", " 1", "1 ", "1.0", "1e2", "instance-1", "9223372036854775808"} {
		t.Run("实例编号_"+instanceID, func(t *testing.T) {
			claims := okpayResumeTestClaims()
			claims.ProviderInstanceID = instanceID
			token, err := svc.CreateOKPayToken(claims)
			require.Error(t, err)
			require.Empty(t, token)
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*ResumeTokenClaims)
	}{
		{"零订单号", func(c *ResumeTokenClaims) { c.OrderID = 0 }},
		{"负订单号", func(c *ResumeTokenClaims) { c.OrderID = -1 }},
		{"零用户号", func(c *ResumeTokenClaims) { c.UserID = 0 }},
		{"负用户号", func(c *ResumeTokenClaims) { c.UserID = -1 }},
		{"负签发时间", func(c *ResumeTokenClaims) { c.IssuedAt = -1 }},
		{"负到期时间", func(c *ResumeTokenClaims) { c.ExpiresAt = -1 }},
		{"到期等于签发", func(c *ResumeTokenClaims) { c.ExpiresAt = c.IssuedAt }},
		{"到期早于签发", func(c *ResumeTokenClaims) { c.ExpiresAt = c.IssuedAt - 1 }},
		{"超出一天有效期", func(c *ResumeTokenClaims) { c.ExpiresAt++ }},
		{"错误服务商", func(c *ResumeTokenClaims) { c.ProviderKey = payment.TypeEasyPay }},
		{"错误支付类型", func(c *ResumeTokenClaims) { c.PaymentType = payment.TypeAlipay }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := okpayResumeTestClaims()
			tc.change(&claims)
			token, err := svc.CreateOKPayToken(claims)
			require.Error(t, err)
			require.Empty(t, token)
		})
	}
}

func TestOKPayResumeTokenRejectsSignedInvalidPayload(t *testing.T) {
	t.Parallel()
	key := []byte("okpay-resume-test-signing-key")
	svc := NewPaymentResumeService(key)
	now := uint64(time.Now().Unix())
	valid := []uint64{42, 7, 19, now, now + 86400}
	for _, tc := range []struct {
		name   string
		values []uint64
	}{
		{"空主体", nil},
		{"缺少字段", valid[:4]},
		{"额外字段", append(append([]uint64(nil), valid...), 1)},
		{"缺少订单号", []uint64{0, 7, 19, now, now + 86400}},
		{"缺少用户号", []uint64{42, 0, 19, now, now + 86400}},
		{"缺少实例号", []uint64{42, 7, 0, now, now + 86400}},
		{"无签发时间", []uint64{42, 7, 19, 0, now + 86400}},
		{"无到期时间", []uint64{42, 7, 19, now, 0}},
		{"已过期", []uint64{42, 7, 19, now - 3600, now - 60}},
		{"到期等于签发", []uint64{42, 7, 19, now, now}},
		{"时间顺序错误", []uint64{42, 7, 19, now, now - 1}},
		{"有效期超限", []uint64{42, 7, 19, now, now + 86401}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.ParseToken(okpayResumeTestRawToken(key, tc.values, "op1."))
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
	for index, field := range []string{"订单号", "用户号", "实例号", "签发时间", "到期时间"} {
		t.Run(field+"超出有符号整数范围", func(t *testing.T) {
			values := append([]uint64(nil), valid...)
			values[index] = uint64(math.MaxInt64) + 1
			got, err := svc.ParseToken(okpayResumeTestRawToken(key, values, "op1."))
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
}

func TestOKPayResumeTokenRejectsTamperingAndNamespaceConfusion(t *testing.T) {
	t.Parallel()
	key := []byte("okpay-resume-test-signing-key")
	svc := NewPaymentResumeService(key)
	token, err := svc.CreateOKPayToken(okpayResumeTestClaims())
	require.NoError(t, err)
	parts := strings.Split(token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	payload[7] ^= 1
	now := uint64(time.Now().Unix())
	legacy, err := svc.CreateToken(okpayResumeTestClaims())
	require.NoError(t, err)
	legacyParts := strings.Split(legacy, ".")
	for name, candidate := range map[string]string{
		"修改订单主体":  "op1." + base64.RawURLEncoding.EncodeToString(payload) + "." + parts[2],
		"错误版本":    strings.Replace(token, "op1.", "op2.", 1),
		"缺少版本":    strings.TrimPrefix(token, "op1."),
		"错误密钥":    okpayResumeTestRawToken([]byte("different-signing-key"), []uint64{42, 7, 19, now, now + 86400}, "op1."),
		"未签入版本":   okpayResumeTestRawToken(key, []uint64{42, 7, 19, now, now + 86400}, ""),
		"错误签名域":   okpayResumeTestRawToken(key, []uint64{42, 7, 19, now, now + 86400}, "wechat."),
		"截短签名":    token[:len(token)-1],
		"空签名":     "op1." + parts[1] + ".",
		"空载荷":     "op1.." + parts[2],
		"多余片段":    token + ".extra",
		"载荷编码损坏":  "op1.!" + parts[1][1:] + "." + parts[2],
		"载荷带填充":   "op1." + parts[1] + "==." + parts[2],
		"旧格式套新前缀": "op1." + legacy,
		"新签名套旧主体": "op1." + legacyParts[0] + "." + svc.sign("op1."+legacyParts[0]),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := svc.ParseToken(candidate)
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
}

func TestOKPayResumeTokenPreservesLegacyAndKeyRotation(t *testing.T) {
	t.Parallel()
	oldKey := []byte("old-okpay-resume-signing-key")
	newKey := []byte("new-okpay-resume-signing-key")
	oldService := NewPaymentResumeService(oldKey)
	rotated := NewPaymentResumeService(newKey, oldKey)
	claims := okpayResumeTestClaims()
	oldCompact, err := oldService.CreateOKPayToken(claims)
	require.NoError(t, err)
	legacy, err := oldService.CreateToken(claims)
	require.NoError(t, err)
	for _, token := range []string{oldCompact, legacy} {
		got, err := rotated.ParseToken(token)
		require.NoError(t, err)
		require.Equal(t, claims.OrderID, got.OrderID)
		require.Equal(t, claims.UserID, got.UserID)
		require.Equal(t, claims.ProviderInstanceID, got.ProviderInstanceID)
	}
	legacyClaims, err := rotated.ParseToken(legacy)
	require.NoError(t, err)
	require.Equal(t, claims, *legacyClaims, "旧格式必须保留其原有返回地址和其他主体信息")
	current, err := rotated.CreateOKPayToken(claims)
	require.NoError(t, err)
	_, err = NewPaymentResumeService(newKey).ParseToken(current)
	require.NoError(t, err)
	_, err = oldService.ParseToken(current)
	require.Error(t, err, "轮换后签发必须使用新密钥")
	_, err = NewPaymentResumeService(newKey).ParseToken(oldCompact)
	require.Error(t, err, "移除旧验证密钥后必须拒绝旧签名")
}

func TestOKPayResumeTokenRequiresSigningKeyAndRejectsWeChatUse(t *testing.T) {
	t.Parallel()
	key := []byte("okpay-resume-test-signing-key")
	svc := NewPaymentResumeService(key)
	token, err := svc.CreateOKPayToken(okpayResumeTestClaims())
	require.NoError(t, err)
	for _, unconfigured := range []*PaymentResumeService{NewPaymentResumeService(nil), NewPaymentResumeService(nil, key)} {
		created, err := unconfigured.CreateOKPayToken(okpayResumeTestClaims())
		require.Error(t, err)
		require.Empty(t, created)
		got, err := unconfigured.ParseToken(token)
		require.Error(t, err)
		require.Nil(t, got)
	}
	_, err = svc.ParseWeChatPaymentResumeToken(token)
	require.Error(t, err, "订单恢复令牌不能作为微信授权恢复凭据")
	wechat, err := svc.CreateWeChatPaymentResumeToken(WeChatPaymentResumeClaims{OpenID: "test-openid", PaymentType: payment.TypeWxpay})
	require.NoError(t, err)
	_, err = svc.ParseToken(wechat)
	require.Error(t, err, "微信授权令牌不能作为订单恢复凭据")
}
