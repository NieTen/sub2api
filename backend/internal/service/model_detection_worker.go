package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/modelquality"
	"github.com/Wei-Shaw/sub2api/internal/pkg/modeltrace"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

// 长整数序列和推理题不能沿用短连通性探针的截止时间；租约须覆盖四次请求及保存时间。
const ModelDetectionRequestTimeoutSeconds = 600
const modelDetectionRequestTimeout = time.Duration(ModelDetectionRequestTimeoutSeconds) * time.Second
const modelDetectionRunTimeout = 4*modelDetectionRequestTimeout + time.Minute
const modelDetectionLeaseDuration = modelDetectionRunTimeout + time.Minute

// Start 两个受控工作线程只认领持久任务；停止时取消上游请求并等待线程退出。
func (s *ModelDetectionService) Start() {
	if s == nil || s.repo == nil || s.probe == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(3)
	go func() { defer s.wg.Done(); s.scheduleLoop(ctx) }()
	for i := 0; i < 2; i++ {
		go func() { defer s.wg.Done(); s.workerLoop(ctx) }()
	}
}
func (s *ModelDetectionService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		s.wg.Wait()
	}
}
func (s *ModelDetectionService) scheduleLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		work, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := s.repo.ExpireLeases(work)
		if err == nil {
			err = s.repo.EnqueueDue(work, 20)
		}
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.LegacyPrintf("service.model_detection", "模型检测任务扫描失败")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *ModelDetectionService) workerLoop(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		claimCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		run, err := s.repo.Claim(claimCtx, modelDetectionLeaseDuration)
		cancel()
		if err == nil && run != nil {
			s.executeRun(ctx, run)
			continue
		}
		if err != nil && ctx.Err() == nil {
			logger.LegacyPrintf("service.model_detection", "模型检测任务认领失败")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *ModelDetectionService) executeRun(parent context.Context, run *ModelDetectionRun) {
	ctx, cancel := context.WithTimeout(parent, modelDetectionRunTimeout)
	defer cancel()
	run.Fingerprint = ModelDetectionFingerprint{Status: "inconclusive", ReferenceModel: run.PlanSnapshot.ReferenceModel}
	run.Details = []ModelDetectionDetail{}
	// 保存路径使用独立短超时，使应用停止或上游超时也能记录本轮错误。
	defer func() {
		if recovered := recover(); recovered != nil {
			run.Status = "error"
			run.Verdict = ""
			run.Score = nil
			run.ErrorMessage = "检测执行异常，本次不参与能力下降判断"
		}
		saveCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := s.repo.Complete(saveCtx, run); err != nil {
			logger.LegacyPrintf("service.model_detection", "模型检测任务完成状态保存失败，过期租约将由后台回收")
		}
	}()
	if run.SuiteVersion != "" && run.SuiteVersion != ModelDetectionSuiteVersion {
		run.Status = "inconclusive"
		run.ErrorMessage = "排队期间检测题库已升级，请重新发起检测"
		return
	}
	outputs := make([]modeltrace.Output, 0, 3)
	for _, challenge := range modeltrace.GenerateChallenges() {
		text, err := s.executeDetectionPrompt(ctx, run, ModelDetectionDetail{Kind: "fingerprint", Prompt: challenge.Prompt, ExpectedCount: challenge.ExpectedCount})
		if err != nil {
			return
		}
		outputs = append(outputs, modeltrace.Output{Text: text, ExpectedCount: challenge.ExpectedCount})
	}
	run.Fingerprint = classifyDetectionFingerprint(outputs, run.PlanSnapshot.ReferenceModel)
	challenge := s.qualityChallenge()
	text, err := s.executeDetectionPrompt(ctx, run, ModelDetectionDetail{Kind: "quality", Prompt: challenge.Prompt})
	if err != nil {
		return
	}
	detail := &run.Details[len(run.Details)-1]
	evaluation, err := modelquality.Evaluate(challenge, text)
	if err != nil {
		run.Status = "inconclusive"
		run.ErrorMessage = "能力题回答格式无效、缺失或不完整，本次不参与能力下降判断"
		detail.Status = "error"
		detail.ErrorMessage = run.ErrorMessage
		return
	}
	detail.Evaluation, _ = json.Marshal(evaluation)
	score := evaluation.Score
	run.Score = &score
	run.Status = "completed"
}

// 发送请求前保存题目，超时也保留已经收到的回答和耗时，方便区分等待响应与中途断流。
func (s *ModelDetectionService) executeDetectionPrompt(ctx context.Context, run *ModelDetectionRun, detail ModelDetectionDetail) (string, error) {
	detail.Status = "running"
	detail.TimeoutSeconds = ModelDetectionRequestTimeoutSeconds
	run.Details = append(run.Details, detail)
	index := len(run.Details) - 1
	if err := s.repo.Progress(ctx, run); err != nil {
		run.Status = "error"
		run.ErrorMessage = "检测状态保存失败，尚未发送本题请求"
		run.Details[index].Status = "error"
		run.Details[index].ErrorMessage = run.ErrorMessage
		return "", err
	}
	started := time.Now()
	text, err := s.probeOne(ctx, run, detail.Prompt)
	run.Details[index].DurationMS = time.Since(started).Milliseconds()
	run.Details[index].Response = logredact.RedactText(text)
	if err != nil {
		markDetectionRequestError(run, err)
		run.Details[index].Status = "error"
		run.Details[index].ErrorMessage = run.ErrorMessage
		return "", err
	}
	run.Details[index].Status = "completed"
	run.Progress++
	if err := s.repo.Progress(ctx, run); err != nil {
		run.Status = "error"
		run.ErrorMessage = "检测状态保存失败，本次不参与能力下降判断"
		return "", err
	}
	return text, nil
}

func (s *ModelDetectionService) probeOne(ctx context.Context, run *ModelDetectionRun, prompt string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, modelDetectionRequestTimeout)
	defer cancel()
	text, err := s.probe.RunModelDetectionPrompt(requestCtx, run.AccountID, run.ModelID, prompt)
	if err == nil && requestCtx.Err() != nil {
		err = requestCtx.Err()
	}
	if len(text) > 256*1024 {
		return text[:256*1024], ErrModelDetectionInvalid
	}
	return text, err
}
func markDetectionRequestError(run *ModelDetectionRun, err error) {
	run.Status = "error"
	run.Verdict = ""
	run.Score = nil
	safeError := safeModelDetectionRequestError(err)
	run.ErrorMessage = safeError.Error()
	if errors.Is(err, ErrModelDetectionUnsupported) {
		run.Status = "inconclusive"
		run.Fingerprint.Status = "unsupported"
		run.ErrorMessage = "该账号或模型暂不支持文本能力检测"
	} else if errors.Is(safeError, ErrModelDetectionInvalid) {
		run.Status = "inconclusive"
	}
}
func classifyDetectionFingerprint(outputs []modeltrace.Output, reference string) ModelDetectionFingerprint {
	result := ModelDetectionFingerprint{Status: "inconclusive", ReferenceModel: reference}
	analysis, err := modeltrace.Analyze(outputs)
	if err != nil {
		result.Status = "invalid"
		result.Message = "数字指纹回答无效或严重截断，无法判断模型相似度"
		return result
	}
	result.Analysis, _ = json.Marshal(analysis)
	if reference == "" {
		result.Status = "unsupported"
		result.Message = "未指定候选库中的参考模型，仅展示相似度排名"
		return result
	}
	known := false
	for _, candidate := range analysis.Results {
		if candidate.Model == reference {
			known = true
			break
		}
	}
	if !known {
		result.Status = "unsupported"
		result.Message = "当前指纹库未收录此参考模型，仅展示相似度排名"
		return result
	}
	// 闭集概率不是身份凭证；三份有效回答且候选优势明显才显示疑似匹配/不一致。
	if analysis.ValidResponses < 3 || analysis.Probability < 0.75 || analysis.Margin < 0.20 {
		result.Message = "有效指纹或候选区分度不足，暂不判断模型是否一致"
		return result
	}
	result.Status = "match"
	if analysis.Prediction != reference {
		result.Status = "mismatch"
	}
	result.Message = modeltrace.Limitation
	return result
}
