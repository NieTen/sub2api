package provider

import (
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTRC20RateLimitSurvivesProviderRecreation(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			first, err := NewTRC20("first", map[string]string{"walletAddress": trc20TestWallet})
			require.NoError(t, err)
			second, err := NewTRC20("second", map[string]string{"walletAddress": trc20TestWallet})
			require.NoError(t, err)
			require.Same(t, first.requestGate, second.requestGate)
			now := time.Now()
			gate := &trc20RequestGate{now: func() time.Time { return now }}
			first.requestGate, second.requestGate = gate, gate
			requests := 0
			transport := trc20TestTransport{handle: func(*http.Request) (*http.Response, error) {
				requests++
				if requests == 1 {
					return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {"90"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				}
				return trc20TestJSON(trc20AccountPage{Success: true}), nil
			}}
			first.client.Transport, second.client.Transport = transport, transport
			var result trc20AccountPage
			require.ErrorContains(t, first.requestJSON(context.Background(), http.MethodGet, "/test", nil, &result), "HTTP")
			require.ErrorContains(t, second.requestJSON(context.Background(), http.MethodGet, "/test", nil, &result), "自动重试")
			require.Equal(t, 1, requests, "冷却中的重建实例不得再次请求节点")
			now = now.Add(90 * time.Second)
			require.NoError(t, second.requestJSON(context.Background(), http.MethodGet, "/test", nil, &result))
			require.Equal(t, 2, requests)
		})
	}
}

func TestTRC20BackoffGrowsAndResetsAfterSuccess(t *testing.T) {
	now := time.Now()
	gate := &trc20RequestGate{now: func() time.Time { return now }}
	key := sha256.Sum256([]byte("test"))
	for _, expected := range []time.Duration{30, 60, 120, 240, 300, 300} {
		gate.deferRequests(key, "")
		require.Equal(t, expected*time.Second, gate.states[key].blockedUntil.Sub(now))
		require.Error(t, gate.wait(context.Background(), key, false))
		gate.succeeded(key)
		require.Error(t, gate.wait(context.Background(), key, false), "并发旧响应不能提前解除冷却")
		now = now.Add(expected * time.Second)
		require.NoError(t, gate.wait(context.Background(), key, false))
	}
	gate.succeeded(key)
	gate.deferRequests(key, "")
	require.Equal(t, trc20RetryBase, gate.states[key].blockedUntil.Sub(now))
}

func TestTRC20RetryAfterDateAndCredentialIsolation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	gate := &trc20RequestGate{now: func() time.Time { return now }}
	publicKey := sha256.Sum256([]byte(trc20APIBase + "\x00"))
	credentialKey := sha256.Sum256([]byte(trc20APIBase + "\x00optional-key"))
	gate.deferRequests(publicKey, now.Add(10*time.Minute).Format(http.TimeFormat))
	require.Equal(t, 10*time.Minute, gate.states[publicKey].blockedUntil.Sub(now))
	require.Error(t, gate.wait(context.Background(), publicKey, true))
	require.NoError(t, gate.wait(context.Background(), credentialKey, false), "公共配额受限时不阻止独立凭据查询")
}

func TestTRC20PublicWaitCanBeCancelledWithoutReservingFutureSlots(t *testing.T) {
	gate := &trc20RequestGate{}
	key := sha256.Sum256([]byte("public"))
	require.NoError(t, gate.wait(context.Background(), key, true))
	firstSlot := gate.states[key].nextAllowed
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, gate.wait(ctx, key, true), context.DeadlineExceeded)
	require.Equal(t, firstSlot, gate.states[key].nextAllowed)
	gate.now = func() time.Time { return firstSlot }
	require.NoError(t, gate.wait(context.Background(), key, true))
}

func TestTRC20RequestGateDiscardsIdleCredentialsButKeepsActiveCooldown(t *testing.T) {
	now := time.Now()
	gate := &trc20RequestGate{now: func() time.Time { return now }}
	stale := sha256.Sum256([]byte("stale"))
	cooling := sha256.Sum256([]byte("cooling"))
	current := sha256.Sum256([]byte("current"))
	require.NoError(t, gate.wait(context.Background(), stale, false))
	gate.deferRequests(cooling, "7200")
	now = now.Add(trc20RequestGateIdleTTL)
	require.NoError(t, gate.wait(context.Background(), current, false))
	require.NotContains(t, gate.states, stale)
	require.Contains(t, gate.states, cooling)
	require.Contains(t, gate.states, current)
}
