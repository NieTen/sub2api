package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	PaymentExchangeRateInterval          = 30 * time.Minute
	PaymentExchangeRateHistoryWindow     = 72 * time.Hour
	PaymentExchangeRateSourceOKX         = "okx"
	PaymentExchangeRateSourceFallback    = "fallback"
	PaymentExchangeRateSourceUnavailable = "unavailable"
)

var ErrPaymentExchangeRateUnavailable = errors.New("USDT/CNY 实时报价不可用，且未配置有效兜底汇率")

// PaymentExchangeRateQuote 表示 1 USDT 对应的人民币金额；不可用时 Rate 为 0。
type PaymentExchangeRateQuote struct {
	Rate           float64    `json:"rate"`
	Source         string     `json:"source"`
	FetchedAt      *time.Time `json:"fetched_at,omitempty"`
	ObservedAt     time.Time  `json:"observed_at"`
	FallbackReason string     `json:"fallback_reason,omitempty"`
	SamplePrices   []float64  `json:"sample_prices"`
	SampleCount    int        `json:"sample_count"`
	Aggregation    string     `json:"aggregation"`
}

// PaymentExchangeRatePoint 与当前报价使用相同结构，但保存的是采集当时的实际状态。
type PaymentExchangeRatePoint = PaymentExchangeRateQuote

type PaymentExchangeRateRepository interface {
	Latest(context.Context) (*PaymentExchangeRatePoint, error)
	Insert(context.Context, PaymentExchangeRatePoint) error
	History(context.Context, time.Time, time.Time) ([]PaymentExchangeRatePoint, error)
}

type PaymentExchangeRateService struct {
	repo           PaymentExchangeRateRepository
	fallbackGetter func(context.Context) (float64, error)
	client         *http.Client
	endpoint       string
	now            func() time.Time
	lockCache      LeaderLockCache
	db             *sql.DB
	instanceID     string
	ctx            context.Context
	cancel         context.CancelFunc
	startOnce      sync.Once
	stopOnce       sync.Once
	wg             sync.WaitGroup
}

func NewPaymentExchangeRateService(repo PaymentExchangeRateRepository, configService *PaymentConfigService) *PaymentExchangeRateService {
	ctx, cancel := context.WithCancel(context.Background())
	svc := &PaymentExchangeRateService{
		repo: repo, client: &http.Client{Timeout: 15 * time.Second},
		endpoint: okxC2CExchangeRateEndpoint, now: time.Now,
		instanceID: uuid.NewString(), ctx: ctx, cancel: cancel,
	}
	if configService != nil {
		svc.fallbackGetter = configService.GetUSDTCNYFallbackRate
	}
	return svc
}

func validPaymentExchangeRate(rate float64) bool {
	return !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate >= 1 && rate <= 100
}

// GetQuote 只读取已持久化采集结果与当前兜底配置，前台请求不会触发外部网络访问。
func (s *PaymentExchangeRateService) GetQuote(ctx context.Context) (*PaymentExchangeRateQuote, error) {
	if s == nil {
		return nil, ErrPaymentExchangeRateUnavailable
	}
	now := s.now().UTC()
	var latest *PaymentExchangeRatePoint
	var err error
	if s.repo != nil {
		latest, err = s.repo.Latest(ctx)
	} else {
		err = errors.New("汇率历史仓储未配置")
	}
	reason := "尚未采集到 OKX 实时报价"
	if err != nil {
		reason = "读取实时报价失败"
	} else if latest != nil {
		if latest.Source == PaymentExchangeRateSourceOKX && validPaymentExchangeRate(latest.Rate) && latest.FetchedAt != nil && !latest.ObservedAt.IsZero() && !latest.FetchedAt.After(now) && !latest.ObservedAt.After(now) && now.Sub(*latest.FetchedAt) < PaymentExchangeRateInterval && now.Sub(latest.ObservedAt) < PaymentExchangeRateInterval {
			quote := *latest
			quote.SamplePrices = append([]float64{}, latest.SamplePrices...)
			return &quote, nil
		}
		if latest.Source == PaymentExchangeRateSourceOKX {
			reason = "OKX 实时报价已超过 30 分钟或时间无效"
		} else if latest.FallbackReason != "" {
			reason = latest.FallbackReason
		} else {
			reason = "最近一次 OKX 抓取失败"
		}
	}
	var fetchedAt *time.Time
	if latest != nil {
		fetchedAt = latest.FetchedAt
	}
	return s.fallbackQuote(ctx, fetchedAt, now, reason)
}

func (s *PaymentExchangeRateService) fallbackQuote(ctx context.Context, fetchedAt *time.Time, observedAt time.Time, reason string) (*PaymentExchangeRateQuote, error) {
	quote := &PaymentExchangeRateQuote{Source: PaymentExchangeRateSourceUnavailable, FetchedAt: fetchedAt, ObservedAt: observedAt, FallbackReason: reason, SamplePrices: []float64{}, Aggregation: "configured_fallback"}
	if s.fallbackGetter == nil {
		return quote, ErrPaymentExchangeRateUnavailable
	}
	rate, err := s.fallbackGetter(ctx)
	if err != nil {
		quote.FallbackReason += "；读取兜底配置失败"
		return quote, fmt.Errorf("%w: %v", ErrPaymentExchangeRateUnavailable, err)
	}
	if !validPaymentExchangeRate(rate) {
		return quote, ErrPaymentExchangeRateUnavailable
	}
	quote.Rate = rate
	quote.Source = PaymentExchangeRateSourceFallback
	return quote, nil
}

func (s *PaymentExchangeRateService) History(ctx context.Context) ([]PaymentExchangeRatePoint, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("汇率历史仓储未配置")
	}
	until := s.now().UTC()
	return s.repo.History(ctx, until.Add(-PaymentExchangeRateHistoryWindow), until)
}
