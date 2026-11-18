package sys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
)

const (
	failedCycleRecoveryCursorKey = "youban_publish:profile:failed_cycle:cursor:v1"
	failedCycleRecoveryBatch     = 10
	failedCycleRecoveryTargets   = 100
)

// The two numbers occupying the third field have different historical
// meanings in other cycle formats. Accept only the exact due/source and
// account/source/plan formats emitted by dueProfileCycleOperationNo and
// claimAccountProfileCycle. Never infer a relation from a prefix or timestamp.
func failedCycleDueSource(operation string, profileID int64) (sourceID, channelID int64, ok bool) {
	parts := strings.Split(operation, ":")
	if len(parts) != 6 || parts[0] != "cycle_batch" || parts[1] != "due" {
		return 0, 0, false
	}
	sourceID, sourceOK := cycleRecoveryPositive(parts[2])
	_, attemptOK := cycleRecoveryPositive(parts[3])
	profile, profileOK := cycleRecoveryPositive(parts[4])
	channelID, channelOK := cycleRecoveryPositive(parts[5])
	return sourceID, channelID, sourceOK && attemptOK && profileOK && channelOK && profile == profileID
}

func cycleRecoveryPositive(value string) (int64, bool) {
	n, err := strconv.ParseInt(value, 10, 64)
	return n, err == nil && n > 0 && strconv.FormatInt(n, 10) == value
}

type failedCycleRecoveryJob struct {
	ID              int64  `orm:"id"`
	TenantID        int64  `orm:"tenant_id"`
	AccountID       int64  `orm:"account_id"`
	ProfileID       int64  `orm:"profile_id"`
	ChannelID       int64  `orm:"channel_id"`
	OperationNo     string `orm:"operation_no"`
	Status          JobStatus
	SendPhase       SendPhase   `orm:"send_phase"`
	SentAt          *gtime.Time `orm:"sent_at"`
	RecoveryOfJobID *int64      `orm:"recovery_of_job_id"`
	TimeoutParentID *int64      `orm:"timeout_parent_job_id"`
}

// Explicit non-NULL links are authoritative, including zero (no link). Legacy
// parsing is used only when both links are NULL; conflicting links fail closed.
func failedCycleRecoveryParent(job failedCycleRecoveryJob) (int64, bool) {
	parsed, parsedOK := failedCycleLegacyParent(job)
	if job.RecoveryOfJobID != nil || job.TimeoutParentID != nil {
		var parent int64
		for _, link := range []*int64{job.RecoveryOfJobID, job.TimeoutParentID} {
			if link == nil {
				continue
			}
			if *link <= 0 || (parent > 0 && parent != *link) {
				return 0, false
			}
			parent = *link
		}
		return parent, parent > 0 && parent < job.ID && (!parsedOK || parsed == parent)
	}
	return parsed, parsedOK && parsed < job.ID
}

func failedCycleLegacyParent(job failedCycleRecoveryJob) (int64, bool) {
	if source, channel, ok := failedCycleDueSource(job.OperationNo, job.ProfileID); ok {
		return source, channel == job.ChannelID
	}
	parts := strings.Split(job.OperationNo, ":")
	if len(parts) == 7 && parts[0] == "profile_cycle" && parts[1] == "account" && parts[5] == "plan" {
		account, a := cycleRecoveryPositive(parts[2])
		profile, p := cycleRecoveryPositive(parts[3])
		source, s := cycleRecoveryPositive(parts[4])
		_, generation := cycleRecoveryPositive(parts[6])
		return source, a && p && s && generation && account == job.AccountID && profile == job.ProfileID
	}
	if len(parts) == 3 && parts[0] == "auto-recover" {
		source, s := cycleRecoveryPositive(parts[1])
		profile, p := cycleRecoveryPositive(parts[2])
		return source, s && p && profile == job.ProfileID
	}
	return 0, false
}

func failedCycleSameIdentity(a, b failedCycleRecoveryJob) bool {
	return a.TenantID == b.TenantID && a.AccountID == b.AccountID &&
		a.ProfileID == b.ProfileID && a.ChannelID == b.ChannelID
}

func failedCycleJobScope(mod *gdb.Model, state profilePublishOperationState) *gdb.Model {
	return mod.Where("tenant_id", state.TenantId).Where("account_id", state.AccountId).
		Where("profile_id", state.ProfileId)
}

const failedCycleJobFields = "id,tenant_id,account_id,profile_id,channel_id,operation_no,status,send_phase,sent_at,recovery_of_job_id,timeout_parent_job_id"

// Walk at most eight explicit recovery/timeout handoffs. An arbitrary later
// success, or a recovery of another channel/owner, is never evidence.
func failedCycleDescendsFrom(ctx context.Context, tx gdb.TX, leaf, source failedCycleRecoveryJob) (bool, error) {
	seen := make(map[int64]bool)
	for depth := 0; depth < 8; depth++ {
		if !failedCycleSameIdentity(leaf, source) || seen[leaf.ID] {
			return false, nil
		}
		if leaf.ID == source.ID {
			return leaf.Status == JobStatusSent, nil
		}
		seen[leaf.ID] = true
		parentID, ok := failedCycleRecoveryParent(leaf)
		if !ok {
			return false, nil
		}
		if parentID == source.ID {
			return source.Status == JobStatusSent || source.Status == JobStatusSuperseded || source.Status == JobStatusFailed, nil
		}
		var parent failedCycleRecoveryJob
		if err := tx.Model(publishTgJobTable).Ctx(ctx).Fields(failedCycleJobFields).
			Where("id", parentID).Scan(&parent); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			return false, err
		}
		if parent.ID == 0 || !failedCycleSameIdentity(leaf, parent) ||
			(parent.Status != JobStatusSuperseded && parent.Status != JobStatusFailed) {
			return false, nil
		}
		leaf = parent
	}
	return false, nil
}

// All reads and the typed projection write use the supplied transaction. Tests
// can supply an isolated DB without reading or changing default g.DB config.
func (s *sSysPublish) recoverFailedCyclePublishState(ctx context.Context, db gdb.DB, snapshot profilePublishOperationState) (bool, error) {
	sourceID, oldChannelID, due := failedCycleDueSource(snapshot.PublishOperationNo, snapshot.ProfileId)
	if snapshot.PublishOperationNo == "" || snapshot.PublishTaskStatus != sysin.PublishTaskStatusFailed || snapshot.PublishTaskUpdatedAt == nil {
		return false, nil
	}
	repaired := false
	reason := "unproven_source_or_targets"
	err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Match normal completion's state -> content lock order.
		current, err := failedCycleJobScope(tx.Model(publishProfileStateTable).Ctx(ctx), snapshot).
			WhereNull("deleted_at").Where("publish_operation_no", snapshot.PublishOperationNo).
			Where("publish_task_status", snapshot.PublishTaskStatus).
			Where("publish_task_updated_at", snapshot.PublishTaskUpdatedAt).LockUpdate().One()
		if err != nil || current.IsEmpty() {
			reason = "projection_changed"
			return err
		}
		canceled, err := profileOperationCanceledTx(ctx, tx, telegramJobRecord{
			TenantId: snapshot.TenantId, AccountId: snapshot.AccountId, ProfileId: snapshot.ProfileId,
			OperationNo: snapshot.PublishOperationNo,
		})
		if err != nil || canceled {
			reason = "operation_canceled"
			return err
		}
		profile, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where("id", snapshot.ProfileId).
			WhereNull("deleted_at").Fields("status,visibility").LockUpdate().One()
		if err != nil {
			return err
		}
		if profile["status"].Int() != 1 || profile["visibility"].String() != consts.ContentVisibilityPublic {
			reason = "profile_not_public"
			return nil
		}
		// Use explicit active states for indexed probes. Unknown latest target
		// states are rejected separately, without scanning all history NOT IN.
		active, err := failedCycleJobScope(tx.Model(publishTgJobTable).Ctx(ctx), snapshot).
			WhereIn("status", telegramJobActiveStatuses()).
			Fields("id").Limit(1).One()
		if err != nil || !active.IsEmpty() {
			reason = "active_job"
			return err
		}
		timeout, err := failedCycleJobScope(tx.Model(publishTgJobTable).Ctx(ctx), snapshot).
			Where("status", JobStatusFailed).Where("send_phase", "timeout_wait_cleanup").Fields("id").Limit(1).One()
		if err != nil || !timeout.IsEmpty() {
			reason = "timeout_cleanup_active"
			return err
		}
		var source failedCycleRecoveryJob
		if due {
			if err = failedCycleJobScope(tx.Model(publishTgJobTable).Ctx(ctx), snapshot).
				Where("id", sourceID).Fields(failedCycleJobFields).Scan(&source); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					reason = "source_missing_or_identity_changed"
					return nil
				}
				return err
			}
			if source.ID == 0 || source.ChannelID != oldChannelID ||
				(source.Status != JobStatusSent && source.Status != JobStatusSuperseded && source.Status != JobStatusFailed) {
				return nil
			}
		}
		// Match profile_read_access.go: manual selections override defaults.
		// Existing active plans remain an additional conservative guard.
		rows, err := tx.GetAll("SELECT channel_id FROM "+publishChannelProfileTable+
			" WHERE tenant_id=? AND account_id=? AND profile_id=? AND status='active'"+
			" UNION SELECT channel_id FROM "+publishProfileChannelTable+
			" WHERE tenant_id=? AND account_id=? AND profile_id=? AND is_manual=1 AND deleted_at IS NULL"+
			" UNION SELECT id AS channel_id FROM "+publishChannelTable+
			" WHERE tenant_id=? AND status=1 AND publish_direction='up' AND is_default_selected=1 AND deleted_at IS NULL"+
			" AND NOT EXISTS (SELECT 1 FROM "+publishProfileChannelTable+" WHERE tenant_id=? AND account_id=? AND profile_id=? AND is_manual=1 AND deleted_at IS NULL) LIMIT ?",
			snapshot.TenantId, snapshot.AccountId, snapshot.ProfileId,
			snapshot.TenantId, snapshot.AccountId, snapshot.ProfileId, snapshot.TenantId,
			snapshot.TenantId, snapshot.AccountId, snapshot.ProfileId, failedCycleRecoveryTargets+1)
		if err != nil || len(rows) == 0 || len(rows) > failedCycleRecoveryTargets {
			reason = "target_set_empty_or_over_limit"
			return err
		}
		latest := make(map[int64]failedCycleRecoveryJob, len(rows))
		media, err := tx.Model(publishMediaTable).Ctx(ctx).
			Where("profile_id", snapshot.ProfileId).WhereNull("deleted_at").
			Fields("COUNT(CASE WHEN purpose='display' THEN 1 END) AS display, COUNT(CASE WHEN purpose='verify' THEN 1 END) AS verify").One()
		if err != nil {
			return err
		}
		displayExpected := media["display"].Int()
		if displayExpected == 0 {
			displayExpected = 1 // text-only publication still requires a message
		}
		verifyExpected := media["verify"].Int()
		for _, row := range rows {
			channelID := row["channel_id"].Int64()
			var job failedCycleRecoveryJob
			if err = failedCycleJobScope(tx.Model(publishTgJobTable).Ctx(ctx), snapshot).
				Where("channel_id", channelID).Fields(failedCycleJobFields).OrderDesc("id").Limit(1).Scan(&job); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					reason = "target_latest_missing"
					return nil
				}
				return err
			}
			if job.ID == 0 || job.Status != JobStatusSent {
				reason = "target_latest_not_sent"
				return nil
			}
			messages, readErr := tx.Model(publishTgMessageTable).Ctx(ctx).Where("job_id", job.ID).
				Where("status", "sent").WhereNull("deleted_at").
				Fields("COUNT(CASE WHEN purpose='display' THEN 1 END) AS display, COUNT(CASE WHEN purpose='verify' THEN 1 END) AS verify").One()
			if readErr != nil {
				return readErr
			}
			if messages["display"].Int() < displayExpected || messages["verify"].Int() < verifyExpected ||
				telegramJobCompletionError(job.SendPhase.String(), verifyExpected, messages["verify"].Int()) != nil {
				reason = "target_verification_incomplete"
				return nil
			}
			latest[channelID] = job
		}
		if due {
			leaf, exists := latest[oldChannelID]
			if !exists || leaf.ID <= source.ID {
				reason = "recovery_not_after_failure"
				return nil
			}
			// Both columns are timestamp without time zone. Compare their wall
			// times inside PostgreSQL: driver-decoded UTC and a local snapshot
			// must not turn an earlier/equal sent_at into a later instant.
			newer, readErr := tx.Model(publishTgJobTable+" j").Ctx(ctx).
				InnerJoin(publishProfileStateTable+" ps",
					"ps.tenant_id=j.tenant_id AND ps.account_id=j.account_id AND ps.profile_id=j.profile_id").
				Where("j.id", leaf.ID).Where("ps.publish_operation_no", snapshot.PublishOperationNo).
				Where("ps.publish_task_status", sysin.PublishTaskStatusFailed).WhereNull("ps.deleted_at").
				Where("j.sent_at > ps.publish_task_updated_at").Fields("j.id").One()
			if readErr != nil {
				return readErr
			}
			if newer.IsEmpty() {
				reason = "recovery_not_after_failure"
				return nil
			}
			linked, err := failedCycleDescendsFrom(ctx, tx, leaf, source)
			if err != nil || !linked {
				return err
			}
		}
		var oldJobs []failedCycleRecoveryJob
		if err = failedCycleJobScope(tx.Model(publishTgJobTable).Ctx(ctx), snapshot).
			Where("operation_no", snapshot.PublishOperationNo).Fields(failedCycleJobFields).
			Limit(failedCycleRecoveryTargets + 1).Scan(&oldJobs); err != nil {
			return err
		}
		if len(oldJobs) > failedCycleRecoveryTargets || (!due && len(oldJobs) == 0) {
			return nil
		}
		if !due {
			// Same-operation repair is intentionally separate from migration
			// proof: every current target must belong to this exact operation.
			for _, leaf := range latest {
				if leaf.OperationNo != snapshot.PublishOperationNo {
					reason = "same_operation_targets_incomplete"
					return nil
				}
			}
		}
		for _, old := range oldJobs {
			leaf, exists := latest[old.ChannelID]
			if !exists {
				return nil
			}
			linked, err := failedCycleDescendsFrom(ctx, tx, leaf, old)
			if err != nil || !linked {
				return err
			}
		}
		if err = s.writeProfilePublishOperationStateTx(ctx, tx, snapshot.TenantId, snapshot.AccountId,
			snapshot.ProfileId, snapshot.PublishOperationNo, "", true, snapshot); err != nil {
			return err
		}
		repaired = true
		reason = "due_source_recovered_other_targets_healthy"
		if !due {
			reason = "same_operation_all_targets_completed"
		}
		return nil
	})
	if err != nil {
		return false, gerror.Wrap(err, "收口旧循环失败投影失败")
	}
	fields := g.Map{"event": "failed_cycle_projection_reconcile", "tenantId": snapshot.TenantId,
		"accountId": snapshot.AccountId, "profileId": snapshot.ProfileId, "operationNo": snapshot.PublishOperationNo,
		"sourceJobId": sourceID, "channelId": oldChannelID, "repaired": repaired, "reason": reason}
	if repaired {
		g.Log().Info(ctx, "旧循环失败投影已收口", fields)
	} else {
		g.Log().Info(ctx, "发布失败投影保留", fields)
	}
	return repaired, nil
}

func (s *sSysPublish) recoverFailedCyclePublishStates(ctx context.Context) error {
	if !cache.Initialized() {
		return gerror.New("旧循环失败恢复游标缓存未初始化")
	}
	value, err := cache.Instance().Get(ctx, failedCycleRecoveryCursorKey)
	if err != nil {
		return err
	}
	cursor := profilePublishRecoveryCursor{}
	if !value.IsNil() {
		if err = json.Unmarshal([]byte(value.String()), &cursor); err != nil {
			return err
		}
		if cursor.ProfileId < 0 || cursor.UpdatedAt.IsZero() != (cursor.ProfileId == 0) {
			return gerror.New("旧循环失败恢复游标无效")
		}
	}
	// idx_ybp_profile_state_publish_active(status, updated_at, profile_id)
	// covers failed (the narrower pending/publishing recovery index does not).
	// Filter operation syntax after LIMIT so sparse due rows cannot cause an
	// unbounded walk of all failed projections in a single scheduler tick.
	query := g.DB().Model(publishProfileStateTable).Ctx(ctx).WhereNull("deleted_at").
		Where("publish_task_status <> ''").Where("publish_task_status", sysin.PublishTaskStatusFailed).
		Where("publish_task_updated_at < ?", time.Now().Add(-profilePublishStateRecoveryStaleAfter)).
		Fields("tenant_id,account_id,profile_id,publish_operation_no,publish_task_status,publish_task_updated_at")
	if !cursor.UpdatedAt.IsZero() {
		query = query.Where("(publish_task_updated_at,profile_id) > (?,?)", cursor.UpdatedAt, cursor.ProfileId)
	}
	var states []profilePublishOperationState
	if err = query.OrderAsc("publish_task_updated_at").OrderAsc("profile_id").Limit(failedCycleRecoveryBatch).Scan(&states); err != nil {
		return err
	}
	if len(states) == 0 {
		cursor = profilePublishRecoveryCursor{}
	}
	for _, state := range states {
		if err = ctx.Err(); err != nil {
			return err
		}
		if state.PublishTaskUpdatedAt == nil {
			return gerror.New("旧循环失败恢复游标时间缺失")
		}
		cursor.UpdatedAt, cursor.ProfileId = state.PublishTaskUpdatedAt.Time, state.ProfileId
		// Persist before work; a problematic profile cannot starve later rows.
		data, marshalErr := json.Marshal(cursor)
		if marshalErr != nil {
			return marshalErr
		}
		if err = cache.Instance().Set(ctx, failedCycleRecoveryCursorKey, string(data), profilePublishStateRecoveryCursorTTL); err != nil {
			return err
		}
		if _, recoverErr := s.recoverFailedCyclePublishState(ctx, g.DB(), state); recoverErr != nil {
			g.Log().Warning(ctx, "旧循环失败投影恢复失败", g.Map{"event": "failed_cycle_projection_error",
				"profileId": state.ProfileId, "operationNo": state.PublishOperationNo, "error": recoverErr.Error()})
		}
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return err
	}
	return cache.Instance().Set(ctx, failedCycleRecoveryCursorKey, string(data), profilePublishStateRecoveryCursorTTL)
}
