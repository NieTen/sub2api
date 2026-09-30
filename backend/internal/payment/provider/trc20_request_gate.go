package provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	trc20PublicRequestInterval = time.Second
	trc20RetryBase             = 30 * time.Second
	trc20RetryMaximum          = 5 * time.Minute
	trc20RequestGateIdleTTL    = time.Hour
)

// 同一进程中的后台轮询与用户查单共享限流状态，避免重建服务商后绕过冷却。
var sharedTRC20RequestGate = &trc20RequestGate{}

type trc20RequestState struct {
	nextAllowed  time.Time
	blockedUntil time.Time
	lastUsed     time.Time
	failures     int
}

type trc20RequestGate struct {
	mu     sync.Mutex
	states map[[32]byte]trc20RequestState
	now    func() time.Time
}

func (g *trc20RequestGate) currentTime() time.Time {
	if g.now != nil {
		return g.now()
	}
	return time.Now()
}

func (g *trc20RequestGate) wait(ctx context.Context, key [32]byte, public bool) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		g.mu.Lock()
		now := g.currentTime()
		if g.states == nil {
			g.states = make(map[[32]byte]trc20RequestState)
		}
		// 仅保留近期使用或仍在冷却的凭据哈希，不保存明文 Key。
		for staleKey, state := range g.states {
			if now.Sub(state.lastUsed) >= trc20RequestGateIdleTTL && !state.blockedUntil.After(now) {
				delete(g.states, staleKey)
			}
		}
		state := g.states[key]
		state.lastUsed = now
		g.states[key] = state
		if state.blockedUntil.After(now) {
			remaining := state.blockedUntil.Sub(now).Round(time.Second)
			g.mu.Unlock()
			return fmt.Errorf("TRON 主网查询暂时受限，约 %s 后自动重试", remaining)
		}
		delay := state.nextAllowed.Sub(now)
		if !public || delay <= 0 {
			if public {
				state.nextAllowed = now.Add(trc20PublicRequestInterval)
			}
			g.states[key] = state
			g.mu.Unlock()
			return nil
		}
		g.mu.Unlock()
		// 等待结束后重新竞争时间槽；取消的请求不会占用未来的排队名额。
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (g *trc20RequestGate) deferRequests(key [32]byte, retryAfter string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.currentTime()
	if g.states == nil {
		g.states = make(map[[32]byte]trc20RequestState)
	}
	state := g.states[key]
	if state.failures < 5 {
		state.failures++
	}
	delay := trc20RetryBase * time.Duration(1<<(state.failures-1))
	if delay > trc20RetryMaximum {
		delay = trc20RetryMaximum
	}
	until := now.Add(delay)
	// 服务端可以指定更长的冷却时间；错误或已经过期的值不缩短本地退避。
	if seconds, err := strconv.ParseInt(strings.TrimSpace(retryAfter), 10, 32); err == nil && seconds > 0 {
		if candidate := now.Add(time.Duration(seconds) * time.Second); candidate.After(until) {
			until = candidate
		}
	} else if candidate, err := http.ParseTime(retryAfter); err == nil && candidate.After(until) {
		until = candidate
	}
	if until.After(state.blockedUntil) {
		state.blockedUntil = until
	}
	state.lastUsed = now
	g.states[key] = state
}

func (g *trc20RequestGate) succeeded(key [32]byte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	state, exists := g.states[key]
	// 早发出的成功请求不能清除其他并发请求刚触发的冷却。
	if exists && !state.blockedUntil.After(g.currentTime()) {
		state.failures = 0
		state.blockedUntil = time.Time{}
		g.states[key] = state
	}
}
