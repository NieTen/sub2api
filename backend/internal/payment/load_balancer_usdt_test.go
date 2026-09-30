//go:build unit

package payment

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func newUSDTSelectionTestBalancer(t *testing.T) (*DefaultLoadBalancer, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	return NewDefaultLoadBalancer(client, nil), mock
}

func usdtSelectionTestInstanceRows(providerKey, method string, limits ChannelLimits) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "provider_key", "name", "config", "supported_types", "enabled", "payment_mode", "limits", "sort_order"}).AddRow(1, providerKey, "测试收款渠道", "{}", method, true, "qrcode", makeLimitsJSON(method, limits), 0)
}

func expectUSDTSelectionDailyUsage(mock sqlmock.Sqlmock, used float64, readErr error) {
	query := mock.ExpectQuery(`SELECT .*payment_orders.*GROUP BY`)
	if readErr != nil {
		query.WillReturnError(readErr)
		return
	}
	query.WillReturnRows(sqlmock.NewRows([]string{"provider_instance_id", "sum"}).AddRow("1", used))
}

func TestUSDTSelectInstanceRejectsNativeLimitViolations(t *testing.T) {
	for _, providerKey := range []string{TypeOKPay, TypeUSDTTRC20} {
		for _, test := range []struct {
			name    string
			limits  ChannelLimits
			used    float64
			amount  float64
			readErr error
		}{
			{name: "低于单笔下限", limits: ChannelLimits{SingleMin: 10}, amount: 9.99},
			{name: "高于单笔上限", limits: ChannelLimits{SingleMax: 10}, amount: 10.01},
			{name: "日剩余额度不足", limits: ChannelLimits{DailyLimit: 100}, used: 95, amount: 5.01},
			{name: "无法读取日用量", limits: ChannelLimits{DailyLimit: 100}, amount: 1, readErr: errors.New("模拟数据库查询失败")},
		} {
			t.Run(providerKey+"/"+test.name, func(t *testing.T) {
				lb, mock := newUSDTSelectionTestBalancer(t)
				mock.ExpectQuery(`SELECT .*payment_provider_instances.*ORDER BY`).WillReturnRows(usdtSelectionTestInstanceRows(providerKey, providerKey, test.limits))
				expectUSDTSelectionDailyUsage(mock, test.used, test.readErr)
				selection, err := lb.SelectInstance(context.Background(), "", providerKey, StrategyRoundRobin, test.amount)
				require.NoError(t, err)
				require.Nil(t, selection, "所有原生渠道超限时不能退回已超限渠道")
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestUSDTSelectInstanceKeepsEasyPayCustomMethodFallback(t *testing.T) {
	for _, method := range []string{TypeOKPay, TypeUSDTTRC20} {
		for _, readErr := range []error{nil, errors.New("模拟数据库查询失败")} {
			t.Run(method, func(t *testing.T) {
				lb, mock := newUSDTSelectionTestBalancer(t)
				mock.ExpectQuery(`SELECT .*payment_provider_instances.*ORDER BY`).WillReturnRows(usdtSelectionTestInstanceRows(TypeEasyPay, method, ChannelLimits{SingleMax: 1, DailyLimit: 1}))
				expectUSDTSelectionDailyUsage(mock, 5, readErr)
				selection, err := lb.SelectInstance(context.Background(), "", method, StrategyRoundRobin, 10)
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, TypeEasyPay, selection.ProviderKey)
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestUSDTSelectInstanceAllowsExactLimitsAndUnlimitedDaily(t *testing.T) {
	for _, providerKey := range []string{TypeOKPay, TypeUSDTTRC20} {
		for _, test := range []struct {
			name    string
			limits  ChannelLimits
			used    float64
			readErr error
		}{
			{name: "恰好达到各限额", limits: ChannelLimits{SingleMin: 10, SingleMax: 10, DailyLimit: 100}, used: 90},
			{name: "没有设置日额度", limits: ChannelLimits{SingleMax: 10}, readErr: errors.New("模拟数据库查询失败")},
		} {
			t.Run(providerKey+"/"+test.name, func(t *testing.T) {
				lb, mock := newUSDTSelectionTestBalancer(t)
				mock.ExpectQuery(`SELECT .*payment_provider_instances.*ORDER BY`).WillReturnRows(usdtSelectionTestInstanceRows(providerKey, providerKey, test.limits))
				expectUSDTSelectionDailyUsage(mock, test.used, test.readErr)
				selection, err := lb.SelectInstance(context.Background(), "", providerKey, StrategyLeastAmount, 10)
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, providerKey, selection.ProviderKey)
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}
