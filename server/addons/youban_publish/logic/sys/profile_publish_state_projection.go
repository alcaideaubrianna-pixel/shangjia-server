package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
	"hotgo/internal/service"
)

const (
	profilePublishNoteIndexStatusCacheTTL = 10 * time.Second
	profilePublishStateRecoveryStaleAfter = 15 * time.Minute
	profilePublishStateRecoveryCursorKey  = "youban_publish:profile:publish_state:recovery:cursor"
	profilePublishStateRecoveryCursorTTL  = 30 * 24 * time.Hour
)

func profilePublishNoteIndexStatusCacheKey(tenantId, accountId, profileId int64, status string) string {
	return fmt.Sprintf("youban_publish:note_index:task_status:%d:%d:%d:%s", tenantId, accountId, profileId, status)
}

func noteIndexStatusRecentlyWritten(ctx context.Context, tenantId, accountId, profileId int64, status string) bool {
	if !cache.Initialized() {
		return false
	}
	value, err := cache.Instance().Get(ctx, profilePublishNoteIndexStatusCacheKey(tenantId, accountId, profileId, status))
	return err == nil && !value.IsNil()
}

func markNoteIndexStatusWritten(ctx context.Context, tenantId, accountId, profileId int64, status string) {
	if !cache.Initialized() {
		return
	}
	_ = cache.Instance().Set(
		ctx,
		profilePublishNoteIndexStatusCacheKey(tenantId, accountId, profileId, status),
		1,
		profilePublishNoteIndexStatusCacheTTL,
	)
}

type profilePublishOperationState struct {
	TenantId             int64                   `orm:"tenant_id"`
	AccountId            int64                   `orm:"account_id"`
	ProfileId            int64                   `orm:"profile_id"`
	PublishOperationNo   string                  `orm:"publish_operation_no"`
	PublishTaskStatus    sysin.PublishTaskStatus `orm:"publish_task_status"`
	PublishTaskUpdatedAt *gtime.Time             `orm:"publish_task_updated_at"`
}

type profilePublishRecoveryCursor struct {
	Status         string    `json:"status"`
	UpdatedAt      time.Time `json:"updated_at"`
	ProfileId      int64     `json:"profile_id"`
	OtherUpdatedAt time.Time `json:"other_updated_at"`
	OtherProfileId int64     `json:"other_profile_id"`
}

func (s *sSysPublish) beginProfilePublishOperation(ctx context.Context, tenantId, accountId, profileId int64, operationNo string) error {
	operationNo = strings.TrimSpace(operationNo)
	// BOT 采集没有协议号账户，account_id=0 是合法的发布归属。
	if tenantId <= 0 || accountId < 0 || profileId <= 0 || operationNo == "" {
		return gerror.New("上架操作状态参数不完整")
	}
	return s.writeProfilePublishOperationState(ctx, tenantId, accountId, profileId, operationNo, sysin.PublishTaskStatusPending, false)
}

func (s *sSysPublish) beginProfilePublishOperationTx(ctx context.Context, tx gdb.TX, tenantId, accountId, profileId int64, operationNo string) error {
	operationNo = strings.TrimSpace(operationNo)
	// BOT 采集没有协议号账户，account_id=0 是合法的发布归属。
	if tenantId <= 0 || accountId < 0 || profileId <= 0 || operationNo == "" {
		return gerror.New("上架操作状态参数不完整")
	}
	return s.writeProfilePublishOperationStateTx(ctx, tx, tenantId, accountId, profileId, operationNo, sysin.PublishTaskStatusPending, false)
}

func (s *sSysPublish) updateProfilePublishOperationState(ctx context.Context, job telegramJobRecord, rawStatus interface{}) error {
	var status sysin.PublishTaskStatus
	switch value := rawStatus.(type) {
	case sysin.PublishTaskStatus:
		status = value
	case string:
		status = sysin.PublishTaskStatus(value)
	default:
		return nil
	}
	status = sysin.PublishTaskStatus(strings.TrimSpace(status.String()))
	if job.TenantId <= 0 || job.AccountId <= 0 || job.ProfileId <= 0 || strings.TrimSpace(job.OperationNo) == "" || status == "" || !status.Valid() {
		return nil
	}
	// A superseded job is historical data. Its delayed retry/cleanup callback
	// must never resurrect a failed state after a newer job has replaced it.
	// This guard is intentionally placed before the operation projection so an
	// old cycle job cannot become the current operation again.
	if status == sysin.PublishTaskStatusFailed || status == sysin.PublishTaskStatusCanceled {
		current, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Fields("status").Where("id", job.Id).One()
		if err != nil {
			return gerror.Wrap(err, "读取资料上架任务状态失败")
		}
		if current["status"].String() == telegramJobStatusSuperseded {
			return nil
		}
	}
	return s.writeProfilePublishOperationState(ctx, job.TenantId, job.AccountId, job.ProfileId, job.OperationNo, status, true)
}

func (s *sSysPublish) clearProfilePublishOperationState(ctx context.Context, job telegramJobRecord) error {
	if job.TenantId <= 0 || job.AccountId <= 0 || job.ProfileId <= 0 || strings.TrimSpace(job.OperationNo) == "" {
		return nil
	}
	return s.writeProfilePublishOperationState(ctx, job.TenantId, job.AccountId, job.ProfileId, job.OperationNo, "", true)
}

func (s *sSysPublish) cancelProfilePublishOperation(ctx context.Context, profileId int64) error {
	if profileId <= 0 {
		return nil
	}
	now := gtime.Now()
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model(publishProfileStateTable).Safe().Ctx(ctx).
			Where("profile_id", profileId).
			WhereNull("deleted_at").
			Where("publish_task_status <> ''").
			Data(g.Map{"publish_task_status": "", "publish_task_updated_at": now, "updated_at": now}).
			Update(); err != nil {
			return gerror.Wrap(err, "取消资料上架操作状态失败")
		}
		if _, err := tx.Model(publishNoteIndexTable).Unscoped().Safe().Ctx(ctx).
			Where("profile_id", profileId).
			WhereNull("deleted_at").
			Where("task_status <> ''").
			Data(g.Map{"task_status": ""}).
			Update(); err != nil {
			return gerror.Wrap(err, "清理资料列表上架状态失败")
		}
		return nil
	})
}

func (s *sSysPublish) profilePublishOperationIsCurrent(ctx context.Context, job telegramJobRecord) (bool, error) {
	if job.TenantId <= 0 || job.AccountId <= 0 || job.ProfileId <= 0 || strings.TrimSpace(job.OperationNo) == "" {
		return false, nil
	}
	count, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).
		Where("publish_operation_no", strings.TrimSpace(job.OperationNo)).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查当前资料上架操作失败")
	}
	return count > 0, nil
}

func (s *sSysPublish) writeProfilePublishOperationState(ctx context.Context, tenantId, accountId, profileId int64, operationNo string, status sysin.PublishTaskStatus, requireCurrentOperation bool) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return s.writeProfilePublishOperationStateTx(ctx, tx, tenantId, accountId, profileId, operationNo, status, requireCurrentOperation)
	})
}

func (s *sSysPublish) writeProfilePublishOperationStateTx(ctx context.Context, tx gdb.TX, tenantId, accountId, profileId int64, operationNo string, status sysin.PublishTaskStatus, requireCurrentOperation bool) error {
	now := gtime.Now()
	// A late failure callback must never overwrite a successful delivery.
	// Cycle/recovery callbacks can carry an older operation number while the
	// same profile already has successful jobs from another operation.
	if status == sysin.PublishTaskStatusFailed {
		var summary struct {
			Sent   int `orm:"sent"`
			Active int `orm:"active"`
		}
		err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("account_id", accountId).
			Where("profile_id", profileId).
			Fields("SUM(CASE WHEN status='sent' THEN 1 ELSE 0 END) AS sent, SUM(CASE WHEN status IN ('pending','sending','confirming','failed_retry','unknown') OR (status='failed' AND send_phase='timeout_wait_cleanup') THEN 1 ELSE 0 END) AS active").
			Scan(&summary)
		if err != nil {
			return gerror.Wrap(err, "检查资料已成功发送任务失败")
		}
		if summary.Sent > 0 && summary.Active == 0 {
			status = sysin.PublishTaskStatusPublished
		}
	}
	stateMod := tx.Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("profile_id", profileId).
		WhereNull("deleted_at")
	if requireCurrentOperation {
		stateMod = stateMod.Where("publish_operation_no", strings.TrimSpace(operationNo))
	}
	if requireCurrentOperation && (status == sysin.PublishTaskStatusPending || status == sysin.PublishTaskStatusPublishing) {
		stateMod = stateMod.Where("publish_task_status <> ?", sysin.PublishTaskStatusFailed)
	}
	data := g.Map{
		"publish_task_status":     status.String(),
		"publish_task_updated_at": now,
		"updated_at":              now,
	}
	if !requireCurrentOperation {
		data["publish_operation_no"] = strings.TrimSpace(operationNo)
	}
	result, err := stateMod.Where("publish_task_status <> ? OR publish_operation_no <> ?", status.String(), strings.TrimSpace(operationNo)).Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新资料上架操作状态失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	// A previous write of the same status cannot prove that the index still has
	// that status after an intervening transition (pending -> publishing -> pending).
	indexMod := tx.Model(publishNoteIndexTable).Unscoped().Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("profile_id", profileId).
		WhereNull("deleted_at").
		Where("task_status <> ?", status)
	if _, err = indexMod.Data(g.Map{"task_status": status.String()}).Update(); err != nil {
		return gerror.Wrap(err, "更新资料列表上架状态失败")
	}
	markNoteIndexStatusWritten(ctx, tenantId, accountId, profileId, status.String())
	return nil
}

func (s *sSysPublish) recoverProfilePublishOperationStates(ctx context.Context, limit int) error {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	// The caller holds the distributed telegram job recovery lock. The cursor is
	// shared across instances and is only a scheduling hint; job state remains
	// authoritative. Do not restart from the head when the cache is unavailable.
	if !cache.Initialized() {
		return gerror.New("资料上架恢复游标缓存未初始化")
	}
	value, err := cache.Instance().Get(ctx, profilePublishStateRecoveryCursorKey)
	if err != nil {
		return gerror.Wrap(err, "读取资料上架恢复游标失败")
	}
	cursor := profilePublishRecoveryCursor{Status: sysin.PublishTaskStatusPending.String()}
	if !value.IsNil() {
		if err = json.Unmarshal([]byte(value.String()), &cursor); err != nil {
			return gerror.Wrap(err, "解析资料上架恢复游标失败")
		}
		if (cursor.Status != sysin.PublishTaskStatusPending.String() && cursor.Status != sysin.PublishTaskStatusPublishing.String()) || (cursor.UpdatedAt.IsZero() && cursor.ProfileId != 0) || (cursor.OtherUpdatedAt.IsZero() && cursor.OtherProfileId != 0) {
			return gerror.New("资料上架恢复游标无效")
		}
	}
	saveCursor := func() error {
		data, marshalErr := json.Marshal(cursor)
		if marshalErr != nil {
			return gerror.Wrap(marshalErr, "编码资料上架恢复游标失败")
		}
		if cacheErr := cache.Instance().Set(ctx, profilePublishStateRecoveryCursorKey, string(data), profilePublishStateRecoveryCursorTTL); cacheErr != nil {
			return gerror.Wrap(cacheErr, "保存资料上架恢复游标失败")
		}
		return nil
	}
	rotateCursor := func() {
		if cursor.Status == sysin.PublishTaskStatusPending.String() {
			cursor.Status = sysin.PublishTaskStatusPublishing.String()
		} else {
			cursor.Status = sysin.PublishTaskStatusPending.String()
		}
		cursor.UpdatedAt, cursor.OtherUpdatedAt = cursor.OtherUpdatedAt, cursor.UpdatedAt
		cursor.ProfileId, cursor.OtherProfileId = cursor.OtherProfileId, cursor.ProfileId
	}
	recoveryBefore := time.Now().Add(-profilePublishStateRecoveryStaleAfter)
	var states []profilePublishOperationState
	query := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("tenant_id,account_id,profile_id,publish_operation_no,publish_task_status,publish_task_updated_at").
		WhereNull("deleted_at").
		// BOT 资料允许使用 account_id=0，不能在状态恢复时被过滤。
		Where("account_id >= 0").
		Where("publish_task_status IN ('pending','publishing')").
		Where("publish_task_status", cursor.Status).
		Where("publish_operation_no <> ''").
		Where("publish_task_updated_at < ?", recoveryBefore)
	if !cursor.UpdatedAt.IsZero() {
		query = query.Where("publish_task_updated_at >= ?", cursor.UpdatedAt).
			Where("(publish_task_updated_at > ? OR (publish_task_updated_at = ? AND profile_id > ?))", cursor.UpdatedAt, cursor.UpdatedAt, cursor.ProfileId)
	}
	if err := query.
		OrderAsc("publish_task_updated_at").
		OrderAsc("profile_id").
		Limit(limit).
		Scan(&states); err != nil {
		return gerror.Wrap(err, "读取待恢复资料上架状态失败")
	}
	if len(states) == 0 {
		cursor.UpdatedAt = time.Time{}
		cursor.ProfileId = 0
		rotateCursor()
		return saveCursor()
	}
	for _, state := range states {
		if err := ctx.Err(); err != nil {
			return err
		}
		if state.PublishTaskUpdatedAt == nil {
			return gerror.New("待恢复资料上架游标字段无效")
		}
		// Checkpoint before processing: a stuck or failed row cannot monopolize
		// the next run, and will be retried after the cursor wraps.
		cursor.UpdatedAt = state.PublishTaskUpdatedAt.Time
		cursor.ProfileId = state.ProfileId
		if err := saveCursor(); err != nil {
			return err
		}
		if err := s.recoverProfilePublishOperationState(ctx, state); err != nil {
			g.Log().Warningf(ctx, "恢复资料上架状态失败 profileId:%d operationNo:%s err:%+v", state.ProfileId, state.PublishOperationNo, err)
		}
	}
	rotateCursor()
	return saveCursor()
}

func (s *sSysPublish) recoverMissingProfilePublishOperationStates(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var jobs []telegramJobRecord
	if err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=j.profile_id AND ps.tenant_id=j.tenant_id AND ps.account_id=j.account_id AND ps.deleted_at IS NULL").
		Fields("j.tenant_id,j.account_id,j.profile_id,j.operation_no,j.status").
		// BOT 资料允许使用 account_id=0，恢复投影时必须保留零账户任务。
		Where("j.account_id >= 0").
		WhereNull("j.task_id").
		WhereIn("j.status", []string{"pending", "sending", "failed_retry", "unknown"}).
		Where("j.operation_no <> ''").
		Where("ps.publish_task_status = ''").
		OrderDesc("j.id").
		Limit(limit).
		Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取缺失投影的资料上架任务失败")
	}
	seen := make(map[int64]struct{}, limit)
	recovered := 0
	for _, job := range jobs {
		if job.ProfileId <= 0 || recovered >= limit {
			continue
		}
		if _, ok := seen[job.ProfileId]; ok {
			continue
		}
		seen[job.ProfileId] = struct{}{}
		if err := s.beginProfilePublishOperation(ctx, job.TenantId, job.AccountId, job.ProfileId, job.OperationNo); err != nil {
			g.Log().Warningf(ctx, "补建资料上架状态失败 profileId:%d operationNo:%s err:%+v", job.ProfileId, job.OperationNo, err)
			continue
		}
		if job.Status == "sending" {
			if err := s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPublishing); err != nil {
				g.Log().Warningf(ctx, "恢复资料发送中状态失败 profileId:%d operationNo:%s err:%+v", job.ProfileId, job.OperationNo, err)
				continue
			}
		}
		recovered++
	}
	if recovered > 0 {
		g.Log().Infof(ctx, "已补建资料上架状态：%d条", recovered)
	}
	return nil
}

func (s *sSysPublish) recoverProfilePublishOperationState(ctx context.Context, state profilePublishOperationState) error {
	job := telegramJobRecord{
		TenantId: state.TenantId, AccountId: state.AccountId, ProfileId: state.ProfileId,
		OperationNo: strings.TrimSpace(state.PublishOperationNo),
	}
	if job.OperationNo == "" {
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	}
	row, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).
		Where("operation_no", job.OperationNo).
		Fields("COUNT(1) AS total," +
			"SUM(CASE WHEN status='failed' AND COALESCE(send_phase, '') <> 'timeout_wait_cleanup' THEN 1 ELSE 0 END) AS failed," +
			"SUM(CASE WHEN status='failed' AND send_phase='timeout_wait_cleanup' THEN 1 ELSE 0 END) AS timeout_wait_cleanup," +
			"SUM(CASE WHEN status='failed_retry' THEN 1 ELSE 0 END) AS failed_retry," +
			"SUM(CASE WHEN status='sending' THEN 1 ELSE 0 END) AS sending," +
			"SUM(CASE WHEN status IN ('pending','failed_retry','unknown','confirming') THEN 1 ELSE 0 END) AS pending," +
			"SUM(CASE WHEN status='sent' THEN 1 ELSE 0 END) AS sent," +
			"SUM(CASE WHEN status IN ('sent','superseded') THEN 1 ELSE 0 END) AS completed," +
			"MIN(created_at) AS first_job_at").
		One()
	if err != nil {
		return gerror.Wrap(err, "统计资料上架任务状态失败")
	}
	total := row["total"].Int()
	// A terminal failed projection can be stale when the final job callback
	// succeeded but the state/index projection failed afterwards. Repair only
	// when the current operation is fully terminal and contains at least one
	// successful Telegram job; real failed operations remain failed below.
	if state.PublishTaskStatus == sysin.PublishTaskStatusFailed &&
		row["failed"].Int() == 0 && row["failed_retry"].Int() == 0 &&
		row["sending"].Int() == 0 && row["pending"].Int() == 0 &&
		row["sent"].Int() > 0 && row["completed"].Int() == total {
		if !isCycleBatchOperation(job.OperationNo) {
			if _, err = s.syncProfilePublishState(ctx, job.ProfileId, 1, consts.ContentVisibilityPublic, gtime.Now()); err != nil {
				return gerror.Wrap(err, "修复资料上架状态失败")
			}
		}
		if err = s.clearProfilePublishOperationState(ctx, job); err != nil {
			return err
		}
		if err = s.syncProfileNoteIndex(ctx, job.ProfileId); err != nil {
			return err
		}
		service.SysContent().ClearHomeProfileCardsCache(ctx)
		g.Log().Infof(ctx, "修复资料上架失败状态 profileId:%d operationNo:%s", job.ProfileId, job.OperationNo)
		return nil
	}
	firstRecoveryAt := (*gtime.Time)(nil)
	var recoveryRow gdb.Record
	recoveryRow, err = g.DB().Model(publishTgJobLogTable+" l").Safe().Ctx(ctx).
		InnerJoin(publishTgJobTable+" j", "j.id=l.job_id").
		Fields("MIN(l.created_at) AS first_recovery_at").
		Where("j.tenant_id", job.TenantId).
		Where("j.account_id", job.AccountId).
		Where("j.profile_id", job.ProfileId).
		Where("j.operation_no", job.OperationNo).
		Where("l.action", "recovery").
		Where("l.status", "queued").One()
	if err != nil {
		return gerror.Wrap(err, "统计资料上架恢复时间失败")
	}
	firstRecoveryAt = recoveryRow["first_recovery_at"].GTime()
	if firstRecoveryAt == nil {
		firstRecoveryAt = row["first_job_at"].GTime()
	}
	staleRetry := firstRecoveryAt != nil && firstRecoveryAt.Before(gtime.Now().Add(-15*time.Minute))
	switch {
	case total == 0 || row["failed"].Int() > 0:
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	case row["sending"].Int() > 0:
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPublishing)
	case row["timeout_wait_cleanup"].Int() > 0:
		// The cleaner owns the handoff after reconciliation. Pending represents
		// that wait; it does not schedule or initiate another send.
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPending)
	case row["failed_retry"].Int() > 0 && staleRetry:
		g.Log().Warningf(ctx, "资料上架重试已超时，释放卡住的publishing状态 profileId:%d operationNo:%s", job.ProfileId, job.OperationNo)
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	case row["pending"].Int() > 0:
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPending)
	case row["completed"].Int() == total && row["sent"].Int() == 0:
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	case row["completed"].Int() == total:
		if !isCycleBatchOperation(job.OperationNo) {
			if _, err = s.syncProfilePublishState(ctx, job.ProfileId, 1, consts.ContentVisibilityPublic, gtime.Now()); err != nil {
				return err
			}
		}
		if err = s.clearProfilePublishOperationState(ctx, job); err != nil {
			return err
		}
		if err = s.syncProfileNoteIndex(ctx, job.ProfileId); err != nil {
			return err
		}
		service.SysContent().ClearHomeProfileCardsCache(ctx)
	}
	return nil
}
