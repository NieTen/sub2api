package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

const trc20TestWallet = "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7"

type trc20TestTransport struct {
	handle func(*http.Request) (*http.Response, error)
}

func (t trc20TestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return t.handle(request)
}

func trc20TestJSON(value any) *http.Response {
	raw, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}
}

func newTRC20TestProvider(t *testing.T) *TRC20 {
	t.Helper()
	p, err := NewTRC20("1", map[string]string{"walletAddress": trc20TestWallet, "apiKey": "test-key"})
	require.NoError(t, err)
	return p
}

func TestTRC20ConfigRejectsInvalidAddressAndUntrustedNetwork(t *testing.T) {
	for _, config := range []map[string]string{
		{"walletAddress": trc20TestWallet},
		{"walletAddress": trc20TestWallet, "apiKey": "key", "apiBase": "https://nile.trongrid.io"},
		{"walletAddress": trc20TestWallet, "apiKey": "key", "apiBase": "https://api.trongrid.io.evil.example"},
		{"walletAddress": trc20TestWallet[:33] + "1", "apiKey": "key"},
		{"walletAddress": trc20TestWallet, "apiKey": "key", "currency": "TRX"},
	} {
		_, err := NewTRC20("1", config)
		require.Error(t, err)
	}
	require.NoError(t, ValidateTRC20WalletAddress(payment.TRC20USDTContract))
	contractHex, err := trc20AddressHex(payment.TRC20USDTContract)
	require.NoError(t, err)
	require.Equal(t, trc20USDTContractHex, contractHex)
	p := newTRC20TestProvider(t)
	_, err = p.CreatePayment(context.Background(), payment.CreatePaymentRequest{Amount: "10.00"})
	require.Error(t, err)
	_, err = p.VerifyNotification(context.Background(), `{"paid":true}`, nil)
	require.Error(t, err)
	_, err = p.Refund(context.Background(), payment.RefundRequest{})
	require.Error(t, err)
}

func TestTRC20RequiresExactConfirmedSuccessfulTransfer(t *testing.T) {
	start := time.Unix(1790730000, 0)
	expiry := start.Add(15 * time.Minute)
	paidAt := start.Add(time.Minute)
	for _, tc := range []struct {
		name     string
		change   func(*trc20AccountTransfer, *trc20TransactionInfo)
		wantPaid bool
	}{
		{name: "精确转入并成功固化", wantPaid: true},
		{name: "金额差一微USDT拒绝", change: func(row *trc20AccountTransfer, _ *trc20TransactionInfo) { row.Value = "10000002" }},
		{name: "伪造同名币拒绝", change: func(row *trc20AccountTransfer, _ *trc20TransactionInfo) { row.TokenInfo.Address = trc20TestWallet }},
		{name: "错误精度拒绝", change: func(row *trc20AccountTransfer, _ *trc20TransactionInfo) { row.TokenInfo.Decimals = 18 }},
		{name: "其他收款地址拒绝", change: func(row *trc20AccountTransfer, _ *trc20TransactionInfo) { row.To = payment.TRC20USDTContract }},
		{name: "下单前转账拒绝", change: func(row *trc20AccountTransfer, _ *trc20TransactionInfo) {
			row.BlockTimestamp = start.Add(-time.Second).UnixMilli()
		}},
		{name: "过期转账拒绝", change: func(row *trc20AccountTransfer, _ *trc20TransactionInfo) {
			row.BlockTimestamp = expiry.Add(time.Second).UnixMilli()
		}},
		{name: "非法指数金额拒绝", change: func(row *trc20AccountTransfer, _ *trc20TransactionInfo) { row.Value = "1e7" }},
		{name: "链上执行失败拒绝", change: func(_ *trc20AccountTransfer, info *trc20TransactionInfo) { info.Receipt.Result = "OUT_OF_ENERGY" }},
		{name: "回执哈希不同拒绝", change: func(_ *trc20AccountTransfer, info *trc20TransactionInfo) { info.ID = strings.Repeat("b", 64) }},
		{name: "回执收款地址不同拒绝", change: func(_ *trc20AccountTransfer, info *trc20TransactionInfo) {
			info.Logs[0].Topics[2] = strings.Repeat("0", 64)
		}},
		{name: "回执金额不一致拒绝", change: func(_ *trc20AccountTransfer, info *trc20TransactionInfo) {
			info.Logs[0].Data = fmt.Sprintf("%064x", int64(10000002))
		}},
		{name: "没有成功Transfer日志拒绝", change: func(_ *trc20AccountTransfer, info *trc20TransactionInfo) { info.Logs = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newTRC20TestProvider(t)
			walletHex, err := trc20AddressHex(trc20TestWallet)
			require.NoError(t, err)
			row := trc20AccountTransfer{TransactionID: strings.Repeat("a", 64), TokenInfo: trc20TokenInfo{Address: payment.TRC20USDTContract, Decimals: 6}, BlockTimestamp: paidAt.UnixMilli(), To: trc20TestWallet, Type: "Transfer", Value: "10000001"}
			info := trc20TransactionInfo{ID: row.TransactionID, BlockNumber: 90000000, BlockTimestamp: row.BlockTimestamp, Receipt: trc20TransactionReceipt{Result: "SUCCESS"}, Logs: []trc20TransactionLog{{Address: trc20USDTContractHex, Topics: []string{trc20TransferTopic, strings.Repeat("0", 64), strings.Repeat("0", 24) + walletHex}, Data: fmt.Sprintf("%064x", int64(10000001))}}}
			if tc.change != nil {
				tc.change(&row, &info)
			}
			p.client.Transport = trc20TestTransport{handle: func(request *http.Request) (*http.Response, error) {
				require.Equal(t, "api.trongrid.io", request.URL.Host)
				require.Equal(t, "test-key", request.Header.Get("TRON-PRO-API-KEY"))
				if request.Method == http.MethodGet {
					require.Equal(t, "true", request.URL.Query().Get("only_confirmed"))
					require.Equal(t, "true", request.URL.Query().Get("only_to"))
					require.Equal(t, payment.TRC20USDTContract, request.URL.Query().Get("contract_address"))
					return trc20TestJSON(trc20AccountPage{Success: true, Data: []trc20AccountTransfer{row}}), nil
				}
				require.Equal(t, "/walletsolidity/gettransactioninfobyid", request.URL.Path)
				return trc20TestJSON(info), nil
			}}
			transfer, err := p.FindConfirmedTransfer(context.Background(), trc20TestWallet, 10000001, start, expiry)
			require.NoError(t, err)
			if tc.wantPaid {
				require.NotNil(t, transfer)
				require.Equal(t, int64(10000001), transfer.AmountUnits)
			} else {
				require.Nil(t, transfer)
			}
		})
	}
}

func TestTRC20PaginationUsesFingerprintAndRetainsFilters(t *testing.T) {
	p := newTRC20TestProvider(t)
	requests := 0
	p.client.Transport = trc20TestTransport{handle: func(request *http.Request) (*http.Response, error) {
		requests++
		require.Equal(t, "true", request.URL.Query().Get("only_confirmed"))
		if requests == 1 {
			return trc20TestJSON(trc20AccountPage{Success: true, Meta: trc20PageMeta{Fingerprint: "cursor-2"}}), nil
		}
		require.Equal(t, "cursor-2", request.URL.Query().Get("fingerprint"))
		return trc20TestJSON(trc20AccountPage{Success: true}), nil
	}}
	transfer, err := p.FindConfirmedTransfer(context.Background(), trc20TestWallet, 10000001, time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	require.Nil(t, transfer)
	require.Equal(t, 2, requests)
}

func TestTRC20RateLimitRemainsRetryable(t *testing.T) {
	p := newTRC20TestProvider(t)
	p.client.Transport = trc20TestTransport{handle: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"error":"rate limit"}`))}, nil
	}}
	_, err := p.FindConfirmedTransfer(context.Background(), trc20TestWallet, 10000001, time.Now().Add(-time.Hour), time.Now())
	require.ErrorContains(t, err, "HTTP 429")
}
