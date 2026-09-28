package service

import (
	"context"
	"encoding/json"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/modelquality"
	"github.com/Wei-Shaw/sub2api/internal/pkg/modeltrace"
)

const ModelDetectionSuiteVersion = "modeltrace-" + modeltrace.BankVersion + "+" + modelquality.Version

var (
	ErrModelDetectionInvalid     = infraerrors.BadRequest("MODEL_DETECTION_INVALID", "模型检测配置无效")
	ErrModelDetectionNotFound    = infraerrors.NotFound("MODEL_DETECTION_NOT_FOUND", "模型检测记录不存在")
	ErrModelDetectionBusy        = infraerrors.Conflict("MODEL_DETECTION_BUSY", "该计划已有排队或运行中的检测")
	ErrModelDetectionDuplicate   = infraerrors.Conflict("MODEL_DETECTION_DUPLICATE", "该账号的此模型已有检测计划，请编辑现有计划")
	ErrModelDetectionLease       = infraerrors.Conflict("MODEL_DETECTION_LEASE_LOST", "模型检测任务租约已失效")
	ErrModelDetectionUnsupported = infraerrors.BadRequest("MODEL_DETECTION_UNSUPPORTED", "该账号或模型暂不支持文本能力检测")
)

// ModelDetectionPlan 每个计划对应一个账号及一个自选模型，基线只由成功检测或管理员重置改变。
type ModelDetectionPlan struct {
	ID                 int64      `json:"id"`
	AccountID          int64      `json:"account_id"`
	AccountName        string     `json:"account_name"`
	ModelID            string     `json:"model_id"`
	Enabled            bool       `json:"enabled"`
	ScheduleType       string     `json:"schedule_type"`
	IntervalMinutes    int        `json:"interval_minutes"`
	DailyTime          string     `json:"daily_time"`
	Timezone           string     `json:"timezone"`
	ReferenceModel     string     `json:"reference_model"`
	DropThreshold      float64    `json:"drop_threshold"`
	MaxResults         int        `json:"max_results"`
	BaselineScore      *float64   `json:"baseline_score"`
	BaselineVersion    string     `json:"baseline_version"`
	BaselineRunID      *int64     `json:"baseline_run_id"`
	BaselineGeneration int64      `json:"baseline_generation"`
	LastRunAt          *time.Time `json:"last_run_at"`
	NextRunAt          *time.Time `json:"next_run_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ModelDetectionFingerprint 指纹判断与能力分数独立，缺少参考库不推断模型冒充。
type ModelDetectionFingerprint struct {
	Status         string          `json:"status"`
	ReferenceModel string          `json:"reference_model"`
	Analysis       json.RawMessage `json:"analysis,omitempty"`
	Message        string          `json:"message,omitempty"`
}

type ModelDetectionDetail struct {
	Kind          string          `json:"kind"`
	Prompt        string          `json:"prompt"`
	Response      string          `json:"response"`
	ExpectedCount int             `json:"expected_count,omitempty"`
	Evaluation    json.RawMessage `json:"evaluation,omitempty"`
}

// ModelDetectionRun 保存执行时配置快照，编辑计划不会修改历史证据。
type ModelDetectionRun struct {
	ID            int64                     `json:"id"`
	PlanID        int64                     `json:"plan_id"`
	AccountID     int64                     `json:"account_id"`
	AccountName   string                    `json:"account_name"`
	ModelID       string                    `json:"model_id"`
	Status        string                    `json:"status"`
	Verdict       string                    `json:"verdict"`
	Score         *float64                  `json:"score"`
	BaselineScore *float64                  `json:"baseline_score"`
	DropPoints    *float64                  `json:"drop_points"`
	Fingerprint   ModelDetectionFingerprint `json:"fingerprint"`
	Details       []ModelDetectionDetail    `json:"details,omitempty"`
	ErrorMessage  string                    `json:"error_message"`
	SuiteVersion  string                    `json:"suite_version"`
	Progress      int                       `json:"progress"`
	RequestsTotal int                       `json:"requests_total"`
	Trigger       string                    `json:"trigger"`
	StartedAt     *time.Time                `json:"started_at"`
	FinishedAt    *time.Time                `json:"finished_at"`
	CreatedAt     time.Time                 `json:"created_at"`
	PlanSnapshot  ModelDetectionPlan        `json:"plan_snapshot"`
	LeaseToken    string                    `json:"-"`
}

type ModelDetectionHistory struct {
	Items        []*ModelDetectionRun `json:"items"`
	NextBeforeID *int64               `json:"next_before_id"`
}

type ModelDetectionSummary struct {
	AccountID    int64              `json:"account_id"`
	PlanCount    int                `json:"plan_count"`
	RunningCount int                `json:"running_count"`
	LatestRun    *ModelDetectionRun `json:"latest_run"`
}

type ModelDetectionStats struct {
	Total         int      `json:"total"`
	Normal        int      `json:"normal"`
	SuspectedDrop int      `json:"suspected_drop"`
	Error         int      `json:"error"`
	AverageScore  *float64 `json:"average_score"`
}

type ModelDetectionOverview struct {
	Active []*ModelDetectionRun  `json:"active"`
	Recent []*ModelDetectionRun  `json:"recent"`
	Stats  ModelDetectionStats   `json:"stats"`
	Plans  []*ModelDetectionPlan `json:"plans"`
}

type ModelDetectionRepository interface {
	SavePlan(context.Context, *ModelDetectionPlan) (*ModelDetectionPlan, error)
	GetPlan(context.Context, int64) (*ModelDetectionPlan, error)
	ListPlans(context.Context, int64) ([]*ModelDetectionPlan, error)
	ResetBaseline(context.Context, int64) (*ModelDetectionPlan, error)
	Enqueue(context.Context, int64, string) (*ModelDetectionRun, error)
	RunAccount(context.Context, *ModelDetectionPlan) (*ModelDetectionRun, error)
	EnqueueDue(context.Context, int) error
	Claim(context.Context, time.Duration) (*ModelDetectionRun, error)
	Progress(context.Context, *ModelDetectionRun) error
	Complete(context.Context, *ModelDetectionRun) error
	ExpireLeases(context.Context) error
	GetRun(context.Context, int64) (*ModelDetectionRun, error)
	History(context.Context, int64, int, int64) ([]*ModelDetectionRun, error)
	Summaries(context.Context, []int64) ([]ModelDetectionSummary, error)
	Overview(context.Context) (*ModelDetectionOverview, error)
}

type ModelDetectionProbe interface {
	RunModelDetectionPrompt(context.Context, int64, string, string) (string, error)
}
