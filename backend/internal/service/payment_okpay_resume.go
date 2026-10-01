package service

import (
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	okpayResumeTokenPrefix = "op1."
	okpayResumePayloadSize = 5 * 8
)

// OKPay 返回链接使用固定长度的凭据；签名仍覆盖订单、用户、实例及有效期。
// 服务商与支付类型由独立版本标记固定，可信返回地址仍在签发前由服务层选择。
func (s *PaymentResumeService) CreateOKPayToken(claims ResumeTokenClaims) (string, error) {
	if err := s.ensureSigningKey(); err != nil {
		return "", err
	}
	instanceID, err := strconv.ParseInt(claims.ProviderInstanceID, 10, 64)
	if err != nil || instanceID <= 0 || strconv.FormatInt(instanceID, 10) != claims.ProviderInstanceID ||
		claims.ProviderKey != payment.TypeOKPay || claims.PaymentType != payment.TypeOKPay {
		return "", invalidOKPayResumeToken()
	}
	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
	}
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = claims.IssuedAt + int64(paymentResumeTokenTTL/time.Second)
	}
	if !validOKPayResumeClaims(claims) {
		return "", invalidOKPayResumeToken()
	}
	payload := make([]byte, okpayResumePayloadSize)
	for index, value := range []int64{claims.OrderID, claims.UserID, instanceID, claims.IssuedAt, claims.ExpiresAt} {
		binary.BigEndian.PutUint64(payload[index*8:], uint64(value))
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signedPayload := okpayResumeTokenPrefix + encoded
	return signedPayload + "." + s.sign(signedPayload), nil
}

func (s *PaymentResumeService) parseOKPayToken(token string) (*ResumeTokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "op1" || len(parts[1]) != 54 || len(parts[2]) != 43 ||
		!s.verifySignature(okpayResumeTokenPrefix+parts[1], parts[2]) {
		return nil, invalidOKPayResumeToken()
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(payload) != okpayResumePayloadSize {
		return nil, invalidOKPayResumeToken()
	}
	instanceID := int64(binary.BigEndian.Uint64(payload[16:24]))
	if instanceID <= 0 {
		return nil, invalidOKPayResumeToken()
	}
	claims := ResumeTokenClaims{
		OrderID: int64(binary.BigEndian.Uint64(payload[0:8])), UserID: int64(binary.BigEndian.Uint64(payload[8:16])),
		ProviderInstanceID: strconv.FormatInt(instanceID, 10), ProviderKey: payment.TypeOKPay, PaymentType: payment.TypeOKPay,
		IssuedAt: int64(binary.BigEndian.Uint64(payload[24:32])), ExpiresAt: int64(binary.BigEndian.Uint64(payload[32:40])),
	}
	if !validOKPayResumeClaims(claims) {
		return nil, invalidOKPayResumeToken()
	}
	if err := validatePaymentResumeExpiry(claims.ExpiresAt, "INVALID_RESUME_TOKEN", "resume token has expired"); err != nil {
		return nil, err
	}
	return &claims, nil
}

func validOKPayResumeClaims(claims ResumeTokenClaims) bool {
	return claims.OrderID > 0 && claims.UserID > 0 && claims.IssuedAt > 0 &&
		claims.ExpiresAt > claims.IssuedAt && claims.ExpiresAt-claims.IssuedAt <= int64(paymentResumeTokenTTL/time.Second)
}

func invalidOKPayResumeToken() error {
	return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "OKPay payment resume token is invalid")
}
