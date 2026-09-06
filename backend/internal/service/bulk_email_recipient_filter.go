package service

import (
	"regexp"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const (
	BulkEmailFilterAll          = "all"
	BulkEmailBalancePositive    = "positive"
	BulkEmailBalanceNonPositive = "non_positive"
	BulkEmailBalanceGreaterThan = "greater_than"
	BulkEmailRechargePaid       = "recharged"
)

var ErrBulkEmailRecipientFilter = infraerrors.BadRequest("BULK_EMAIL_RECIPIENT_FILTER_INVALID", "收件人筛选无效：金额须为非负十进制数，最多12位整数和8位小数")
var bulkEmailThresholdPattern = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,8})?$`)

// BulkEmailRecipientFilter 两类条件取交集，金额使用字符串保留数据库的八位小数精度。
type BulkEmailRecipientFilter struct {
	BalanceCondition  string  `json:"balance_condition"`
	BalanceThreshold  *string `json:"balance_threshold,omitempty"`
	RechargeCondition string  `json:"recharge_condition"`
}

func (f BulkEmailRecipientFilter) Normalize() (BulkEmailRecipientFilter, error) {
	if f.BalanceCondition == "" {
		f.BalanceCondition = BulkEmailFilterAll
	}
	if f.RechargeCondition == "" {
		f.RechargeCondition = BulkEmailFilterAll
	}
	switch f.BalanceCondition {
	case BulkEmailFilterAll, BulkEmailBalancePositive, BulkEmailBalanceNonPositive:
		// 拒绝遗留阈值，避免界面展示条件与实际生效条件不一致。
		if f.BalanceThreshold != nil {
			return f, ErrBulkEmailRecipientFilter
		}
	case BulkEmailBalanceGreaterThan:
		if f.BalanceThreshold == nil || !bulkEmailThresholdPattern.MatchString(*f.BalanceThreshold) {
			return f, ErrBulkEmailRecipientFilter
		}
		amount, err := decimal.NewFromString(*f.BalanceThreshold)
		if err != nil {
			return f, ErrBulkEmailRecipientFilter
		}
		normalized := amount.String()
		f.BalanceThreshold = &normalized
	default:
		return f, ErrBulkEmailRecipientFilter
	}
	if f.RechargeCondition != BulkEmailFilterAll && f.RechargeCondition != BulkEmailRechargePaid {
		return f, ErrBulkEmailRecipientFilter
	}
	return f, nil
}
