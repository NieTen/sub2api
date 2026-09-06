package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func bulkEmailTestThreshold(value string) *string { return &value }

func TestBulkEmailRecipientFilterKeepsExactDecimal(t *testing.T) {
	for input, expected := range map[string]string{
		"000100.50000000": "100.5", "0": "0", "0.00000001": "0.00000001",
		"999999999999.99999999": "999999999999.99999999",
	} {
		t.Run(input, func(t *testing.T) {
			filter, err := (BulkEmailRecipientFilter{BalanceCondition: BulkEmailBalanceGreaterThan, BalanceThreshold: &input, RechargeCondition: BulkEmailRechargePaid}).Normalize()
			require.NoError(t, err)
			require.Equal(t, expected, *filter.BalanceThreshold)
			raw, err := json.Marshal(filter)
			require.NoError(t, err)
			require.Contains(t, string(raw), `"balance_threshold":"`+expected+`"`)
		})
	}
}

func TestBulkEmailRecipientFilterRejectsInvalidAmounts(t *testing.T) {
	for _, input := range []string{"", " ", " 1", "1 ", "-1", "NaN", "Infinity", "1e3", "1e1000000", "+1", "1.000000001", "1000000000000", "1,000", "1;DROP TABLE users"} {
		t.Run(input, func(t *testing.T) {
			_, err := (BulkEmailRecipientFilter{BalanceCondition: BulkEmailBalanceGreaterThan, BalanceThreshold: &input}).Normalize()
			require.ErrorIs(t, err, ErrBulkEmailRecipientFilter)
		})
	}
	_, err := (BulkEmailRecipientFilter{BalanceCondition: BulkEmailBalanceGreaterThan}).Normalize()
	require.ErrorIs(t, err, ErrBulkEmailRecipientFilter)
}

func TestBulkEmailRecipientFilterRejectsUnknownOrStaleConditions(t *testing.T) {
	for _, filter := range []BulkEmailRecipientFilter{
		{BalanceCondition: "bogus"}, {RechargeCondition: "greater_than"},
		{BalanceCondition: BulkEmailBalancePositive, BalanceThreshold: bulkEmailTestThreshold("1")},
		{BalanceCondition: BulkEmailBalanceNonPositive, BalanceThreshold: bulkEmailTestThreshold("0")},
		{BalanceThreshold: bulkEmailTestThreshold("50")},
	} {
		_, err := filter.Normalize()
		require.ErrorIs(t, err, ErrBulkEmailRecipientFilter)
	}
}

func TestBulkEmailRecipientFilterDefaultsRemainCompatible(t *testing.T) {
	filter, err := (BulkEmailRecipientFilter{}).Normalize()
	require.NoError(t, err)
	require.Equal(t, BulkEmailFilterAll, filter.BalanceCondition)
	require.Equal(t, BulkEmailFilterAll, filter.RechargeCondition)
	require.Nil(t, filter.BalanceThreshold)
}

func TestBulkEmailCreatePassesFilterToSnapshot(t *testing.T) {
	settings := newNotificationEmailMemorySettingRepo()
	require.NoError(t, settings.Set(context.Background(), SettingKeySMTPHost, "smtp.example.com"))
	repo := &bulkEmailTestRepository{}
	svc := NewBulkEmailService(repo, NewEmailService(settings, nil), nil)
	input := BulkEmailCreateInput{Subject: "通知", Body: "正文", AllActive: true, RecipientFilter: BulkEmailRecipientFilter{
		BalanceCondition: BulkEmailBalanceGreaterThan, BalanceThreshold: bulkEmailTestThreshold("000100.5000"), RechargeCondition: BulkEmailRechargePaid,
	}}
	batch, err := svc.Create(context.Background(), 8, input)
	require.NoError(t, err)
	require.Equal(t, "100.5", *repo.batch.RecipientFilter.BalanceThreshold)
	require.Equal(t, BulkEmailRechargePaid, batch.RecipientFilter.RechargeCondition)
	// 条件无效时不能退化为向全部用户创建草稿。
	input.RecipientFilter.BalanceCondition = "unsupported"
	_, err = svc.Create(context.Background(), 8, input)
	require.ErrorIs(t, err, ErrBulkEmailRecipientFilter)
	require.Equal(t, 1, repo.drafts)
}
