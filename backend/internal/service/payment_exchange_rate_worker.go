package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const paymentExchangeRateLeaderLockKey = "payment:exchange-rate:leader"

func (s *PaymentExchangeRateService) SetLeaderLock(cache LeaderLockCache, db *sql.DB) {
	if s != nil {
		s.lockCache = cache
		s.db = db
	}
}

func (s *PaymentExchangeRateService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for s.ctx.Err() == nil {
				delay, err := s.refreshIfDue(s.ctx)
				if err != nil && s.ctx.Err() == nil {
					slog.Warn("USDT/CNY 汇率采集失败", "error", err)
				}
				if delay <= 0 {
					delay = time.Second
				}
				timer := time.NewTimer(delay)
				select {
				case <-s.ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	})
}

func (s *PaymentExchangeRateService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
	s.wg.Wait()
}

// refreshIfDue 在领导锁内读取最近采集时间，重启及多实例不会重复抓取同一采集周期。
func (s *PaymentExchangeRateService) refreshIfDue(ctx context.Context) (time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return time.Minute, err
	}
	lockCtx, cancelLock := context.WithTimeout(ctx, 2*time.Second)
	release, acquired := tryAcquireSingletonLeaderLock(lockCtx, s.lockCache, s.db, paymentExchangeRateLeaderLockKey, s.instanceID, time.Minute)
	cancelLock()
	if !acquired {
		return time.Minute, nil
	}
	defer release()
	runCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	latest, err := s.repo.Latest(runCtx)
	if err != nil {
		return time.Minute, fmt.Errorf("读取最近汇率采集时间失败: %w", err)
	}
	fetchedAt := s.now().UTC()
	if latest != nil && latest.FetchedAt != nil && !latest.FetchedAt.After(fetchedAt) {
		next := latest.FetchedAt.Add(PaymentExchangeRateInterval)
		if next.After(fetchedAt) {
			return next.Sub(fetchedAt), nil
		}
	}
	point, fetchErr := s.fetchOKX(runCtx, fetchedAt)
	if fetchErr != nil {
		if ctx.Err() != nil {
			return time.Minute, ctx.Err()
		}
		// 即使兜底未配置，也保存 unavailable 事件，不能让旧成功记录重新变成最新状态。
		point, _ = s.fallbackQuote(runCtx, &fetchedAt, s.now().UTC(), fetchErr.Error())
	}
	if point == nil {
		return time.Minute, errors.New("汇率采集未生成有效状态")
	}
	if err := s.repo.Insert(runCtx, *point); err != nil {
		return time.Minute, fmt.Errorf("保存汇率采集记录失败: %w", err)
	}
	return fetchedAt.Add(PaymentExchangeRateInterval).Sub(s.now().UTC()), fetchErr
}
