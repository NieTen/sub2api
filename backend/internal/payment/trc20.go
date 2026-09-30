package payment

import "time"

const (
	TypeUSDTTRC20     = "usdt_trc20"
	TRC20USDTContract = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	TRC20USDTDecimals = 6
	// 链上仍使用微 USDT；新收款金额按 0.01 USDT 分配，最多增加 0.99 USDT。
	TRC20USDTUnitsPerToken int64 = 1000000
	TRC20AmountStepUnits   int64 = 10000
	TRC20AmountExtraSteps  int64 = 99
)

// TRC20Transfer 只承载已由固化节点校验成功的主网 USDT 转入。
type TRC20Transfer struct {
	TransactionHash string
	WalletAddress   string
	AmountUnits     int64
	BlockTimestamp  time.Time
}
