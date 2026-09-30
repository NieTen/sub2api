package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	trc20APIBase         = "https://api.trongrid.io"
	trc20USDTContractHex = "a614f803b6fd780986a42c78ec9c7f77e6ded13c"
	trc20TransferTopic   = "ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	trc20MaxPages        = 100
)

// TRC20 只读主网已确认交易，不持有私钥，也不会构造或广播转账。
type TRC20 struct {
	instanceID    string
	walletAddress string
	apiKey        string
	apiBase       string
	client        *http.Client
	requestGate   *trc20RequestGate
}

func NewTRC20(instanceID string, config map[string]string) (*TRC20, error) {
	address := strings.TrimSpace(config["walletAddress"])
	if _, err := trc20AddressHex(address); err != nil {
		return nil, fmt.Errorf("TRC20 收款地址无效: %w", err)
	}
	base := strings.TrimRight(strings.TrimSpace(config["apiBase"]), "/")
	if base == "" {
		base = trc20APIBase
	}
	// 确认数据只能来自主网官方节点，禁止误接测试网或自定义伪造数据源。
	if base != trc20APIBase {
		return nil, errors.New("TRC20 仅支持官方主网 https://api.trongrid.io")
	}
	if currency := strings.TrimSpace(config["currency"]); currency != "" && !strings.EqualFold(currency, "USDT") {
		return nil, errors.New("TRC20 直收仅支持 USDT")
	}
	if strings.ContainsAny(config["apiKey"], "\r\n\x00") {
		return nil, errors.New("TRC20 的 TronGrid API Key 不能包含换行或空字符")
	}
	// 留空时使用官方公共读取；API Key 仅用于提高查询配额，不参与收款验证。
	return &TRC20{instanceID: instanceID, walletAddress: address, apiKey: strings.TrimSpace(config["apiKey"]), apiBase: base, requestGate: sharedTRC20RequestGate, client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (p *TRC20) Name() string        { return "USDT (TRC20)" }
func (p *TRC20) ProviderKey() string { return payment.TypeUSDTTRC20 }
func (p *TRC20) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeUSDTTRC20}
}
func (p *TRC20) WalletAddress() string { return p.walletAddress }
func (p *TRC20) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"currency": "USDT", "merchant_id": p.walletAddress, "wallet_address": p.walletAddress, "network": "TRC20"}
}
func (p *TRC20) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, errors.New("TRC20 订单必须先持久化分配唯一收款金额")
}
func (p *TRC20) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, errors.New("TRC20 必须依据持久化订单的地址、金额和有效时间窗查单")
}
func (p *TRC20) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, errors.New("TRC20 不接收外部支付回调，仅信任已确认链上交易")
}
func (p *TRC20) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, errors.New("TRC20 直收不支持自动退款")
}

// ValidateTRC20WalletAddress 校验主网地址长度、网络前缀与 Base58Check 校验和。
func ValidateTRC20WalletAddress(address string) error {
	_, err := trc20AddressHex(address)
	return err
}

func trc20AddressHex(address string) (string, error) {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	if len(address) != 34 || address[0] != 'T' {
		return "", errors.New("需要以 T 开头的 34 位 TRON 主网地址")
	}
	n := new(big.Int)
	for _, char := range address {
		index := strings.IndexRune(alphabet, char)
		if index < 0 {
			return "", errors.New("地址包含无效 Base58 字符")
		}
		n.Mul(n, big.NewInt(58)).Add(n, big.NewInt(int64(index)))
	}
	decoded := n.Bytes()
	if len(decoded) != 25 || decoded[0] != 0x41 {
		return "", errors.New("地址网络或长度不正确")
	}
	first := sha256.Sum256(decoded[:21])
	second := sha256.Sum256(first[:])
	if !bytes.Equal(decoded[21:], second[:4]) {
		return "", errors.New("地址校验和不正确")
	}
	return hex.EncodeToString(decoded[1:21]), nil
}

type trc20TokenInfo struct {
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
}
type trc20AccountTransfer struct {
	TransactionID  string         `json:"transaction_id"`
	TokenInfo      trc20TokenInfo `json:"token_info"`
	BlockTimestamp int64          `json:"block_timestamp"`
	To             string         `json:"to"`
	Type           string         `json:"type"`
	Value          string         `json:"value"`
}
type trc20PageMeta struct {
	Fingerprint string `json:"fingerprint"`
}
type trc20AccountPage struct {
	Success bool                   `json:"success"`
	Data    []trc20AccountTransfer `json:"data"`
	Meta    trc20PageMeta          `json:"meta"`
}
type trc20TransactionReceipt struct {
	Result string `json:"result"`
}
type trc20TransactionLog struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
}
type trc20TransactionInfo struct {
	ID             string                  `json:"id"`
	BlockNumber    int64                   `json:"blockNumber"`
	BlockTimestamp int64                   `json:"blockTimeStamp"`
	Receipt        trc20TransactionReceipt `json:"receipt"`
	Logs           []trc20TransactionLog   `json:"log"`
}

func (p *TRC20) requestJSON(ctx context.Context, method, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, p.apiBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.apiKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", p.apiKey)
	}
	gate := p.requestGate
	if gate == nil {
		gate = sharedTRC20RequestGate
	}
	gateKey := sha256.Sum256([]byte(trc20APIBase + "\x00" + p.apiKey))
	if err = gate.wait(ctx, gateKey, p.apiKey == ""); err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			gate.deferRequests(gateKey, "")
		}
		return fmt.Errorf("查询 TRON 主网失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			gate.deferRequests(gateKey, resp.Header.Get("Retry-After"))
		}
		return fmt.Errorf("TRON 主网查询返回 HTTP %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024))
	if err = decoder.Decode(out); err != nil {
		return fmt.Errorf("TRON 主网响应格式无效: %w", err)
	}
	gate.succeeded(gateKey)
	return nil
}

func validTRC20Hash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// FindConfirmedTransfer 同时校验账户转账列表和固化节点的成功回执及 Transfer 日志。
func (p *TRC20) FindConfirmedTransfer(ctx context.Context, address string, amountUnits int64, createdAt, expiresAt time.Time) (*payment.TRC20Transfer, error) {
	addressHex, err := trc20AddressHex(address)
	if err != nil || amountUnits <= 0 || !expiresAt.After(createdAt) {
		return nil, errors.New("TRC20 查单条件无效")
	}
	params := url.Values{"only_confirmed": {"true"}, "only_to": {"true"}, "contract_address": {payment.TRC20USDTContract}, "min_timestamp": {strconv.FormatInt(createdAt.UnixMilli(), 10)}, "max_timestamp": {strconv.FormatInt(expiresAt.UnixMilli(), 10)}, "limit": {"200"}, "order_by": {"block_timestamp,asc"}}
	seen := map[string]bool{}
	for page := 0; page < trc20MaxPages; page++ {
		var response trc20AccountPage
		if err = p.requestJSON(ctx, http.MethodGet, "/v1/accounts/"+address+"/transactions/trc20?"+params.Encode(), nil, &response); err != nil {
			return nil, err
		}
		if !response.Success {
			return nil, errors.New("TRON 主网未确认查单成功")
		}
		for _, transfer := range response.Data {
			if transfer.To != address || transfer.Type != "Transfer" || transfer.TokenInfo.Address != payment.TRC20USDTContract || transfer.TokenInfo.Decimals != payment.TRC20USDTDecimals || !validTRC20Hash(transfer.TransactionID) {
				continue
			}
			units, parseErr := strconv.ParseInt(transfer.Value, 10, 64)
			if parseErr != nil || units != amountUnits || transfer.Value != strconv.FormatInt(units, 10) {
				continue
			}
			at := time.UnixMilli(transfer.BlockTimestamp)
			if at.Before(createdAt) || at.After(expiresAt) {
				continue
			}
			var info trc20TransactionInfo
			payload, _ := json.Marshal(map[string]string{"value": transfer.TransactionID})
			if err = p.requestJSON(ctx, http.MethodPost, "/walletsolidity/gettransactioninfobyid", payload, &info); err != nil {
				return nil, err
			}
			if !strings.EqualFold(info.ID, transfer.TransactionID) || info.BlockNumber <= 0 || info.Receipt.Result != "SUCCESS" || info.BlockTimestamp != transfer.BlockTimestamp {
				continue
			}
			for _, event := range info.Logs {
				if strings.ToLower(event.Address) != trc20USDTContractHex || len(event.Topics) != 3 || strings.ToLower(event.Topics[0]) != trc20TransferTopic || len(event.Topics[2]) != 64 || strings.ToLower(event.Topics[2]) != strings.Repeat("0", 24)+addressHex || len(event.Data) != 64 {
					continue
				}
				value, ok := new(big.Int).SetString(event.Data, 16)
				if !ok || !value.IsInt64() || value.Int64() != amountUnits {
					continue
				}
				return &payment.TRC20Transfer{TransactionHash: strings.ToLower(transfer.TransactionID), WalletAddress: address, AmountUnits: amountUnits, BlockTimestamp: at}, nil
			}
		}
		fingerprint := response.Meta.Fingerprint
		if fingerprint == "" {
			return nil, nil
		}
		if seen[fingerprint] {
			return nil, errors.New("TRON 主网分页游标重复")
		}
		seen[fingerprint] = true
		params.Set("fingerprint", fingerprint)
	}
	return nil, errors.New("TRON 主网交易分页超过安全上限，请稍后重试")
}
