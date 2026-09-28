package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type modelDetectionRepository struct{ db *sql.DB }

func NewModelDetectionRepository(db *sql.DB) service.ModelDetectionRepository {
	return &modelDetectionRepository{db: db}
}

const detectionPlanColumns = `p.id,p.account_id,a.name,p.model_id,p.enabled,p.schedule_type,p.interval_minutes,p.daily_time,p.timezone,p.reference_model,p.drop_threshold,p.max_results,p.baseline_score,p.baseline_version,p.baseline_run_id,p.baseline_generation,p.last_run_at,p.next_run_at,p.created_at,p.updated_at`
const detectionRunColumns = `id,plan_id,account_id,account_name,model_id,status,verdict,score,baseline_score,drop_points,fingerprint,details,error_message,suite_version,progress,requests_total,trigger,plan_snapshot,COALESCE(lease_token,''),started_at,finished_at,created_at`

type detectionScanner interface{ Scan(...any) error }
type detectionQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func scanDetectionPlan(row detectionScanner) (*service.ModelDetectionPlan, error) {
	p := &service.ModelDetectionPlan{}
	err := row.Scan(&p.ID, &p.AccountID, &p.AccountName, &p.ModelID, &p.Enabled, &p.ScheduleType, &p.IntervalMinutes, &p.DailyTime, &p.Timezone, &p.ReferenceModel, &p.DropThreshold, &p.MaxResults, &p.BaselineScore, &p.BaselineVersion, &p.BaselineRunID, &p.BaselineGeneration, &p.LastRunAt, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrModelDetectionNotFound
	}
	p.ReferenceModel = p.ModelID
	return p, err
}
func scanDetectionRun(row detectionScanner) (*service.ModelDetectionRun, error) {
	r := &service.ModelDetectionRun{}
	var fingerprint, details, snapshot []byte
	err := row.Scan(&r.ID, &r.PlanID, &r.AccountID, &r.AccountName, &r.ModelID, &r.Status, &r.Verdict, &r.Score, &r.BaselineScore, &r.DropPoints, &fingerprint, &details, &r.ErrorMessage, &r.SuiteVersion, &r.Progress, &r.RequestsTotal, &r.Trigger, &snapshot, &r.LeaseToken, &r.StartedAt, &r.FinishedAt, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrModelDetectionNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(fingerprint, &r.Fingerprint); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(details, &r.Details); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(snapshot, &r.PlanSnapshot); err != nil {
		return nil, err
	}
	return r, nil
}
func detectionPlan(ctx context.Context, q detectionQuerier, id int64, lock bool) (*service.ModelDetectionPlan, error) {
	query := `SELECT ` + detectionPlanColumns + ` FROM model_detection_plans p JOIN accounts a ON a.id=p.account_id WHERE p.id=$1 AND a.deleted_at IS NULL`
	if lock {
		query += ` FOR UPDATE OF p`
	}
	return scanDetectionPlan(q.QueryRowContext(ctx, query, id))
}
func (r *modelDetectionRepository) GetPlan(ctx context.Context, id int64) (*service.ModelDetectionPlan, error) {
	return detectionPlan(ctx, r.db, id, false)
}

func insertDetectionPlan(ctx context.Context, tx *sql.Tx, p *service.ModelDetectionPlan) (int64, error) {
	p.ReferenceModel = p.ModelID
	next, err := service.ModelDetectionNextRun(p, time.Now())
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO model_detection_plans(account_id,model_id,enabled,schedule_type,interval_minutes,daily_time,timezone,reference_model,drop_threshold,max_results,next_run_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`, p.AccountID, p.ModelID, p.Enabled, p.ScheduleType, p.IntervalMinutes, p.DailyTime, p.Timezone, p.ReferenceModel, p.DropThreshold, p.MaxResults, next).Scan(&id)
	return id, err
}
func (r *modelDetectionRepository) SavePlan(ctx context.Context, p *service.ModelDetectionPlan) (*service.ModelDetectionPlan, error) {
	p.ReferenceModel = p.ModelID
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id := p.ID
	if id == 0 {
		var accountID int64
		if err = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, p.AccountID).Scan(&accountID); errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrAccountNotFound
		}
		if err != nil {
			return nil, err
		}
		id, err = insertDetectionPlan(ctx, tx, p)
	} else {
		old, getErr := detectionPlan(ctx, tx, id, true)
		if getErr != nil {
			return nil, getErr
		}
		if old.AccountID != p.AccountID {
			return nil, service.ErrModelDetectionInvalid
		}
		next, nextErr := service.ModelDetectionNextRun(p, time.Now())
		if nextErr != nil {
			return nil, nextErr
		}
		changed := old.ModelID != p.ModelID
		_, err = tx.ExecContext(ctx, `UPDATE model_detection_plans SET model_id=$2,enabled=$3,schedule_type=$4,interval_minutes=$5,daily_time=$6,timezone=$7,reference_model=$8,drop_threshold=$9,max_results=$10,next_run_at=$11,baseline_score=CASE WHEN $12 THEN NULL ELSE baseline_score END,baseline_version=CASE WHEN $12 THEN '' ELSE baseline_version END,baseline_run_id=CASE WHEN $12 THEN NULL ELSE baseline_run_id END,baseline_generation=baseline_generation+CASE WHEN $12 THEN 1 ELSE 0 END,updated_at=NOW() WHERE id=$1`, id, p.ModelID, p.Enabled, p.ScheduleType, p.IntervalMinutes, p.DailyTime, p.Timezone, p.ReferenceModel, p.DropThreshold, p.MaxResults, next, changed)
	}
	if err != nil {
		return nil, detectionRepositoryError(err)
	}
	if !p.Enabled {
		// 暂停取消尚未开始的定时任务；管理员明确发起的手动任务继续排队。
		if _, err = tx.ExecContext(ctx, `UPDATE model_detection_runs SET status='error',error_message='检测计划已暂停，尚未开始的定时任务已取消',finished_at=NOW() WHERE plan_id=$1 AND status='queued' AND trigger='scheduled'`, id); err != nil {
			return nil, err
		}
	}
	result, err := detectionPlan(ctx, tx, id, false)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
func detectionRepositoryError(err error) error {
	var pg *pq.Error
	if errors.As(err, &pg) && pg.Code == "23505" {
		if pg.Constraint == "idx_model_detection_plans_account_model" {
			return service.ErrModelDetectionDuplicate.WithCause(err)
		}
		if pg.Constraint == "idx_model_detection_runs_active_plan" {
			return service.ErrModelDetectionBusy.WithCause(err)
		}
	}
	return err
}
func (r *modelDetectionRepository) ListPlans(ctx context.Context, accountID int64) ([]*service.ModelDetectionPlan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+detectionPlanColumns+` FROM model_detection_plans p JOIN accounts a ON a.id=p.account_id WHERE a.deleted_at IS NULL AND ($1::bigint=0 OR p.account_id=$1) ORDER BY p.id DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []*service.ModelDetectionPlan{}
	for rows.Next() {
		p, e := scanDetectionPlan(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
func (r *modelDetectionRepository) ResetBaseline(ctx context.Context, id int64) (*service.ModelDetectionPlan, error) {
	_, err := r.db.ExecContext(ctx, `UPDATE model_detection_plans SET baseline_score=NULL,baseline_version='',baseline_run_id=NULL,baseline_generation=baseline_generation+1,updated_at=NOW() WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	return r.GetPlan(ctx, id)
}

func enqueueDetection(ctx context.Context, tx *sql.Tx, p *service.ModelDetectionPlan, trigger string) (*service.ModelDetectionRun, error) {
	// 所选模型就是本轮参考，旧计划的手工参考不再参与新任务。
	p.ReferenceModel = p.ModelID
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM model_detection_runs WHERE plan_id=$1 AND status IN ('queued','running'))`, p.ID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, service.ErrModelDetectionBusy
	}
	snapshot, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	fingerprint, err := json.Marshal(service.ModelDetectionFingerprint{Status: "inconclusive", ReferenceModel: p.ReferenceModel})
	if err != nil {
		return nil, err
	}
	result, err := scanDetectionRun(tx.QueryRowContext(ctx, `INSERT INTO model_detection_runs(plan_id,account_id,account_name,model_id,status,suite_version,trigger,plan_snapshot,fingerprint) VALUES($1,$2,$3,$4,'queued',$5,$6,$7,$8) RETURNING `+detectionRunColumns, p.ID, p.AccountID, p.AccountName, p.ModelID, service.ModelDetectionSuiteVersion, trigger, snapshot, fingerprint))
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE model_detection_plans SET next_run_at=NULL,updated_at=NOW() WHERE id=$1`, p.ID)
	return result, err
}
func (r *modelDetectionRepository) Enqueue(ctx context.Context, id int64, trigger string) (*service.ModelDetectionRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	p, err := detectionPlan(ctx, tx, id, true)
	if err != nil {
		return nil, err
	}
	run, err := enqueueDetection(ctx, tx, p, trigger)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return run, nil
}

// RunAccount 锁住账号后复用已有计划，同账号并发点击不会产生重复的暂停计划。
func (r *modelDetectionRepository) RunAccount(ctx context.Context, p *service.ModelDetectionPlan) (*service.ModelDetectionRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var accountID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, p.AccountID).Scan(&accountID); errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM model_detection_plans WHERE account_id=$1 AND model_id=$2 ORDER BY enabled DESC,id LIMIT 1 FOR UPDATE`, p.AccountID, p.ModelID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id, err = insertDetectionPlan(ctx, tx, p)
	}
	if err != nil {
		return nil, err
	}
	plan, err := detectionPlan(ctx, tx, id, false)
	if err != nil {
		return nil, err
	}
	// 复用原计划的周期与能力基线，参考始终跟随所选模型。
	plan.ReferenceModel = plan.ModelID
	run, err := enqueueDetection(ctx, tx, plan, "manual")
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return run, nil
}
func (r *modelDetectionRepository) EnqueueDue(ctx context.Context, limit int) error {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT `+detectionPlanColumns+` FROM model_detection_plans p JOIN accounts a ON a.id=p.account_id WHERE p.enabled=TRUE AND p.next_run_at<=NOW() AND a.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM model_detection_runs r WHERE r.plan_id=p.id AND r.status IN ('queued','running')) ORDER BY p.next_run_at,p.id LIMIT $1 FOR UPDATE OF p SKIP LOCKED`, limit)
	if err != nil {
		return err
	}
	plans := []*service.ModelDetectionPlan{}
	for rows.Next() {
		p, e := scanDetectionPlan(rows)
		if e != nil {
			rows.Close()
			return e
		}
		plans = append(plans, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range plans {
		if _, err = enqueueDetection(ctx, tx, p, "scheduled"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *modelDetectionRepository) Claim(ctx context.Context, lease time.Duration) (*service.ModelDetectionRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// 跨实例串行化认领的短事务，整个集群最多同时运行两轮，网络请求不持有事务锁。
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(241,1)`); err != nil {
		return nil, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(id) FROM model_detection_runs WHERE status='running'`).Scan(&count); err != nil {
		return nil, err
	}
	if count >= 2 {
		return nil, nil
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT r.id FROM model_detection_runs r JOIN accounts a ON a.id=r.account_id WHERE r.status='queued' AND a.deleted_at IS NULL ORDER BY r.id LIMIT 1 FOR UPDATE OF r SKIP LOCKED`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// 升级前已排队的快照同样改为所选模型，不改动已结束历史证据。
	run, err := scanDetectionRun(tx.QueryRowContext(ctx, `UPDATE model_detection_runs SET status='running',started_at=NOW(),lease_token=$2,lease_until=NOW()+($3::double precision*INTERVAL '1 second'),plan_snapshot=jsonb_set(plan_snapshot,'{reference_model}',to_jsonb(model_id),true),fingerprint=jsonb_set(fingerprint,'{reference_model}',to_jsonb(model_id),true) WHERE id=$1 AND status='queued' RETURNING `+detectionRunColumns, id, uuid.NewString(), lease.Seconds()))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return run, nil
}
func detectionChanged(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return service.ErrModelDetectionLease
	}
	return nil
}
func (r *modelDetectionRepository) Progress(ctx context.Context, run *service.ModelDetectionRun) error {
	details, err := json.Marshal(run.Details)
	if err != nil {
		return err
	}
	return detectionChanged(r.db.ExecContext(ctx, `UPDATE model_detection_runs SET progress=$3,details=$4 WHERE id=$1 AND lease_token=$2 AND status='running' AND lease_until>NOW()`, run.ID, run.LeaseToken, run.Progress, details))
}

// Complete 在持有计划锁的事务内比较基线并完成任务；旧租约或被重置的基线不能覆盖新状态。
func (r *modelDetectionRepository) Complete(ctx context.Context, run *service.ModelDetectionRun) error {
	// 终态与派生字段在仓储边界统一校验，拒绝无效分数和旧调用方残留的判断结果。
	if run == nil || run.ID <= 0 || run.PlanID <= 0 || run.SuiteVersion == "" {
		return service.ErrModelDetectionInvalid
	}
	run.Verdict = ""
	run.BaselineScore = nil
	run.DropPoints = nil
	switch run.Status {
	case "completed":
		if run.Score == nil || math.IsNaN(*run.Score) || math.IsInf(*run.Score, 0) || *run.Score < 0 || *run.Score > 100 || run.SuiteVersion != service.ModelDetectionSuiteVersion {
			return service.ErrModelDetectionInvalid
		}
	case "error", "inconclusive":
		run.Score = nil
	default:
		return service.ErrModelDetectionInvalid
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, err := detectionPlan(ctx, tx, run.PlanID, true)
	if err != nil {
		return err
	}
	if run.Status == "completed" && run.Score != nil {
		if p.BaselineGeneration != run.PlanSnapshot.BaselineGeneration || p.ModelID != run.ModelID {
			run.Status = "inconclusive"
			run.Score = nil
			run.ErrorMessage = "检测期间计划模型或基线已修改，本次结果不参与基线比较"
		} else if p.BaselineScore == nil || p.BaselineVersion != run.SuiteVersion {
			run.Verdict = "baseline"
			run.BaselineScore = run.Score
			zero := float64(0)
			run.DropPoints = &zero
		} else {
			run.BaselineScore = p.BaselineScore
			drop := *p.BaselineScore - *run.Score
			run.DropPoints = &drop
			run.Verdict = "normal"
			if drop >= run.PlanSnapshot.DropThreshold {
				run.Verdict = "suspected_drop"
			}
		}
	}
	fingerprint, err := json.Marshal(run.Fingerprint)
	if err != nil {
		return err
	}
	details, err := json.Marshal(run.Details)
	if err != nil {
		return err
	}
	if err = detectionChanged(tx.ExecContext(ctx, `UPDATE model_detection_runs SET status=$3,verdict=$4,score=$5,baseline_score=$6,drop_points=$7,fingerprint=$8,details=$9,error_message=$10,progress=$11,finished_at=NOW(),lease_token=NULL,lease_until=NULL WHERE id=$1 AND lease_token=$2 AND status='running' AND lease_until>NOW()`, run.ID, run.LeaseToken, run.Status, run.Verdict, run.Score, run.BaselineScore, run.DropPoints, fingerprint, details, run.ErrorMessage, run.Progress)); err != nil {
		return err
	}
	if run.Verdict == "baseline" {
		_, err = tx.ExecContext(ctx, `UPDATE model_detection_plans SET baseline_score=$2,baseline_version=$3,baseline_run_id=$4 WHERE id=$1`, p.ID, run.Score, run.SuiteVersion, run.ID)
		if err != nil {
			return err
		}
	}
	next, err := service.ModelDetectionNextRun(p, time.Now())
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE model_detection_plans SET last_run_at=NOW(),next_run_at=$2,updated_at=NOW() WHERE id=$1`, p.ID, next); err != nil {
		return err
	}
	// 基线对应的证据额外保留；删除始终限定当前计划与已完成的旧记录。
	if _, err = tx.ExecContext(ctx, `DELETE FROM model_detection_runs WHERE plan_id=$1 AND finished_at IS NOT NULL AND id<>COALESCE((SELECT baseline_run_id FROM model_detection_plans WHERE id=$1),0) AND id IN (SELECT id FROM model_detection_runs WHERE plan_id=$1 AND finished_at IS NOT NULL ORDER BY id DESC OFFSET $2)`, p.ID, p.MaxResults); err != nil {
		return err
	}
	return tx.Commit()
}

// ExpireLeases 中断任务只记录失败，不重放可能已计费的模型请求。
func (r *modelDetectionRepository) ExpireLeases(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT plan_id FROM model_detection_runs WHERE (status='running' AND lease_until<=NOW()) OR (status='queued' AND NOT EXISTS(SELECT 1 FROM accounts a WHERE a.id=model_detection_runs.account_id AND a.deleted_at IS NULL)) LIMIT 50`)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = r.expirePlan(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (r *modelDetectionRepository) expirePlan(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 删除账号仍保留可查看的失败记录，避免排队任务占住唯一索引。
	p, err := scanDetectionPlan(tx.QueryRowContext(ctx, `SELECT `+detectionPlanColumns+` FROM model_detection_plans p JOIN accounts a ON a.id=p.account_id WHERE p.id=$1 FOR UPDATE OF p`, id))
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE model_detection_runs SET status='error',error_message='检测进程中断、执行超时或账号已删除；未自动重发请求',finished_at=NOW(),lease_token=NULL,lease_until=NULL WHERE plan_id=$1 AND ((status='running' AND lease_until<=NOW()) OR (status='queued' AND NOT EXISTS(SELECT 1 FROM accounts a WHERE a.id=model_detection_runs.account_id AND a.deleted_at IS NULL)))`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count > 0 {
		next, e := service.ModelDetectionNextRun(p, time.Now())
		if e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, `UPDATE model_detection_plans SET next_run_at=$2,last_run_at=NOW(),updated_at=NOW() WHERE id=$1`, id, next); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *modelDetectionRepository) GetRun(ctx context.Context, id int64) (*service.ModelDetectionRun, error) {
	return scanDetectionRun(r.db.QueryRowContext(ctx, `SELECT `+detectionRunColumns+` FROM model_detection_runs WHERE id=$1`, id))
}
func (r *modelDetectionRepository) listCompactRuns(ctx context.Context, where string, args ...any) ([]*service.ModelDetectionRun, error) {
	columns := strings.Replace(detectionRunColumns, ",details,", ",'[]'::jsonb AS details,", 1)
	return r.listProjectedRuns(ctx, columns, where, args...)
}
func (r *modelDetectionRepository) listProjectedRuns(ctx context.Context, columns, where string, args ...any) ([]*service.ModelDetectionRun, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+columns+` FROM model_detection_runs `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []*service.ModelDetectionRun{}
	for rows.Next() {
		run, e := scanDetectionRun(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, run)
	}
	return items, rows.Err()
}
func (r *modelDetectionRepository) History(ctx context.Context, accountID int64, limit int, before int64) ([]*service.ModelDetectionRun, error) {
	return r.listCompactRuns(ctx, `WHERE account_id=$1 AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT $3`, accountID, before, limit)
}
func (r *modelDetectionRepository) Summaries(ctx context.Context, ids []int64) ([]service.ModelDetectionSummary, error) {
	items := []service.ModelDetectionSummary{}
	if len(ids) == 0 {
		return items, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT a.id,COUNT(DISTINCT p.id),COUNT(DISTINCT r.id) FILTER(WHERE r.status IN ('queued','running')) FROM accounts a LEFT JOIN model_detection_plans p ON p.account_id=a.id LEFT JOIN model_detection_runs r ON r.plan_id=p.id AND r.status IN ('queued','running') WHERE a.id=ANY($1) AND a.deleted_at IS NULL GROUP BY a.id ORDER BY a.id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item service.ModelDetectionSummary
		if err = rows.Scan(&item.AccountID, &item.PlanCount, &item.RunningCount); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	runs, err := r.listCompactRuns(ctx, `WHERE id IN (SELECT MAX(id) FROM model_detection_runs WHERE account_id=ANY($1) GROUP BY account_id)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	latest := map[int64]*service.ModelDetectionRun{}
	for _, run := range runs {
		run.Details = nil
		latest[run.AccountID] = run
	}
	for i := range items {
		items[i].LatestRun = latest[items[i].AccountID]
	}
	return items, nil
}
func (r *modelDetectionRepository) Overview(ctx context.Context) (*service.ModelDetectionOverview, error) {
	o := &service.ModelDetectionOverview{}
	var err error
	if o.Active, err = r.listCompactRuns(ctx, `WHERE status IN ('queued','running') ORDER BY id`); err != nil {
		return nil, err
	}
	if o.Recent, err = r.listCompactRuns(ctx, `WHERE finished_at IS NOT NULL ORDER BY finished_at DESC,id DESC LIMIT 10`); err != nil {
		return nil, err
	}
	if o.Plans, err = r.ListPlans(ctx, 0); err != nil {
		return nil, err
	}
	var sum float64
	scored := 0
	for _, run := range o.Active {
		run.Details = nil
	}
	for _, run := range o.Recent {
		run.Details = nil
		o.Stats.Total++
		switch run.Verdict {
		case "baseline", "normal":
			o.Stats.Normal++
		case "suspected_drop":
			o.Stats.SuspectedDrop++
		}
		if run.Status == "error" || run.Status == "inconclusive" {
			o.Stats.Error++
		}
		if run.Score != nil && run.Status == "completed" {
			sum += *run.Score
			scored++
		}
	}
	if scored > 0 {
		avg := sum / float64(scored)
		o.Stats.AverageScore = &avg
	}
	return o, nil
}

var _ service.ModelDetectionRepository = (*modelDetectionRepository)(nil)
