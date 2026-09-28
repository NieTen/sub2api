package service

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestModelDetectionTimeoutKeepsQuestionAndPartialEvidence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := &detectionTestRepo{}
		run := &ModelDetectionRun{PlanID: 1, ModelID: "gpt-5.6-sol", Status: "running"}
		probe := &detectionTestProbe{call: func(ctx context.Context, _ int, prompt string) (string, error) {
			require.Len(t, run.Details, 1)
			require.Equal(t, prompt, run.Details[0].Prompt)
			require.Equal(t, "running", run.Details[0].Status)
			<-ctx.Done()
			return "1, 23, 45, 67", ctx.Err()
		}}
		s := NewModelDetectionService(repo, nil, probe)
		s.executeRun(context.Background(), run)
		require.Equal(t, "error", repo.complete.Status)
		require.Nil(t, repo.complete.Score)
		require.Empty(t, repo.complete.Verdict)
		require.Len(t, repo.complete.Details, 1)
		detail := repo.complete.Details[0]
		require.NotEmpty(t, detail.Prompt)
		require.Equal(t, "1, 23, 45, 67", detail.Response)
		require.Equal(t, "error", detail.Status)
		require.Contains(t, detail.ErrorMessage, "超时")
		require.EqualValues(t, modelDetectionRequestTimeout.Milliseconds(), detail.DurationMS)
		require.Equal(t, ModelDetectionRequestTimeoutSeconds, detail.TimeoutSeconds)
		require.Equal(t, 1, probe.calls)
	})
}

func TestModelDetectionFourSlowRequestsFitRunAndLease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := &detectionTestRepo{}
		probe := &detectionTestProbe{call: func(ctx context.Context, _ int, _ string) (string, error) {
			select {
			case <-time.After(9 * time.Minute):
				return strings.Repeat("42 ", 300), nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}}
		s := NewModelDetectionService(repo, nil, probe)
		started := time.Now()
		s.executeRun(context.Background(), &ModelDetectionRun{PlanID: 1, Status: "running"})
		require.Equal(t, 4, probe.calls)
		require.Equal(t, 36*time.Minute, time.Since(started))
		require.Equal(t, "inconclusive", repo.complete.Status, "四题均完成，第四题因模拟答卷格式无效而非超时结束")
		require.Contains(t, repo.complete.ErrorMessage, "格式")
		require.Len(t, repo.complete.Details, 4)
		require.Greater(t, modelDetectionLeaseDuration, modelDetectionRunTimeout)
	})
}
