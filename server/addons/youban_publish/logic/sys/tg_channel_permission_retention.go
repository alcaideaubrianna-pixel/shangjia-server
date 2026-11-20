package sys

import (
	"context"
	"errors"
	"fmt"
	"time"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Called under the existing recovery distributed lock. Each transaction and
// scan is bounded; no Telegram calls, profile/media deletion or Redis purge.
func (s *sSysPublish) maintainPermissionPausedBacklog(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Low-priority retention must not exhaust the budget for current permission scopes.
	retentionErr := s.prunePermissionCancelledJobs(ctx)
	return errors.Join(retentionErr, s.scanPermissionBacklogScopes(ctx))
}

type permissionRetentionCursor struct {
	UpdatedAt string `json:"updatedAt"`
	ID        int64  `json:"id"`
}

// Called under the existing recovery lease. Advance only after the transaction commits.
func (s *sSysPublish) prunePermissionCancelledJobs(ctx context.Context) error {
	if !cache.Initialized() {
		return nil
	}
	admitted, err := cache.Instance().SetIfNotExist(ctx, publishconsts.PermissionRetentionProbeKey, true, time.Minute)
	if err != nil || !admitted {
		return err
	}
	value, err := cache.Instance().Get(ctx, publishconsts.PermissionRetentionCursorKey)
	if err != nil {
		return err
	}
	var cursor permissionRetentionCursor
	if !value.IsNil() {
		if err := value.Struct(&cursor); err != nil {
			return err
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := g.DB().Transaction(bounded, func(ctx context.Context, tx gdb.TX) error {
		return prunePermissionCancelledJobsTx(ctx, tx, gtime.New(time.Now().AddDate(0, -1, 0)), &cursor)
	}); err != nil {
		return err
	}
	if err := cache.Instance().Set(ctx, publishconsts.PermissionRetentionCursorKey, cursor, 24*time.Hour); err != nil {
		return err
	}
	if cursor.ID == 0 {
		// Empty/end-of-pass probes do not need to scan historical jobs every recovery tick.
		return cache.Instance().Set(ctx, publishconsts.PermissionRetentionProbeKey, true, 10*time.Minute)
	}
	return nil
}

func prunePermissionCancelledJobsTx(ctx context.Context, tx gdb.TX, cutoff *gtime.Time, cursors ...*permissionRetentionCursor) error {
	// First use the retention index to obtain at most 200 IDs. Correlated evidence
	// checks and row locks only run on that bounded page; ineligible rows cannot
	// monopolize every pass because the committed cursor moves past them.
	var candidates []struct {
		ID        int64       `orm:"id"`
		UpdatedAt *gtime.Time `orm:"updated_at"`
	}
	model := tx.Model(publishTgJobTable).Safe().Ctx(ctx).Fields("id,updated_at").
		Where("status", telegramJobStatusSuperseded).Where("last_dispatch_error", channelPermissionCancelledMessage).
		Where("updated_at <= ?", cutoff)
	if len(cursors) > 0 && cursors[0] != nil && cursors[0].UpdatedAt != "" && cursors[0].ID > 0 {
		model = model.Where("(updated_at,id) > (?,?)", cursors[0].UpdatedAt, cursors[0].ID)
	}
	if err := model.OrderAsc("updated_at").OrderAsc("id").Limit(200).Scan(&candidates); err != nil {
		return err
	}
	if len(candidates) == 0 {
		if len(cursors) > 0 && cursors[0] != nil {
			*cursors[0] = permissionRetentionCursor{}
		}
		return nil
	}
	pageIDs := make([]int64, 0, len(candidates))
	for _, row := range candidates {
		pageIDs = append(pageIDs, row.ID)
	}
	defer func() {
		if len(cursors) > 0 && cursors[0] != nil {
			last := candidates[len(candidates)-1]
			*cursors[0] = permissionRetentionCursor{ID: last.ID, UpdatedAt: last.UpdatedAt.Time.Format("2006-01-02 15:04:05.000000")}
		}
	}()
	var jobs []telegramJobRecord
	if err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).Fields("id").
		WhereIn("id", pageIDs).
		Where("status", telegramJobStatusSuperseded).Where("last_dispatch_error", channelPermissionCancelledMessage).
		Where("updated_at <= ?", cutoff).Where(permissionUntouchedJobSQL()).
		OrderAsc("updated_at").OrderAsc("id").Limit(200).LockUpdate().Scan(&jobs); err != nil {
		return err
	}
	ids := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.Id)
	}
	if len(ids) == 0 {
		return nil
	}
	// These jobs never contacted Telegram. Their transport and UI dispatch logs
	// can be removed atomically, while all source profiles remain untouched.
	for _, table := range []string{publishTgOutboxTable, publishSuccessRecordTable} {
		if _, err := tx.Model(table).Safe().Ctx(ctx).Unscoped().WhereIn("job_id", ids).Delete(); err != nil {
			return err
		}
	}
	_, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).Unscoped().WhereIn("id", ids).Delete()
	return err
}

// These shared cache cursors are scheduling hints only. Resetting them repeats
// safe checks; PostgreSQL and the existing locked parking transaction own state.
const (
	permissionBacklogCursorKey   = publishconsts.TelegramPermissionBacklogCursorKey
	permissionBacklogScopeLimit  = 10
	permissionBacklogJobPageSize = 10
	permissionBacklogCursorTTL   = 24 * time.Hour
)

type permissionBacklogScope struct {
	TenantId  int64       `json:"tenant_id"`
	ChannelId int64       `json:"channel_id"`
	Paused    int         `json:"paused"`
	ResumedAt *gtime.Time `json:"resumed_at"`
}

func (s *sSysPublish) scanPermissionBacklogScopes(ctx context.Context) error {
	var after permissionBacklogScope
	cached, err := cache.Instance().Get(ctx, permissionBacklogCursorKey)
	if err != nil {
		return err
	}
	if !cached.IsNil() {
		if err := cached.Scan(&after); err != nil {
			return err
		}
	}
	var scopes []permissionBacklogScope
	// Page the small circuit table before applying eligibility. The primary key
	// supplies both the seek and order without inspecting historical jobs.
	err = g.DB().Model(channelPermissionCircuitTable).Safe().Ctx(ctx).
		Fields("tenant_id,channel_id,paused,resumed_at").
		Where("(tenant_id,channel_id) > (?,?)", after.TenantId, after.ChannelId).
		OrderAsc("tenant_id").OrderAsc("channel_id").Limit(permissionBacklogScopeLimit).Scan(&scopes)
	if err != nil {
		return err
	}
	if len(scopes) == 0 {
		return cache.Instance().Set(ctx, permissionBacklogCursorKey, permissionBacklogScope{}, permissionBacklogCursorTTL)
	}
	for _, scope := range scopes {
		// Advance scope fairness even if one channel repeatedly exhausts its
		// budget. Its job cursor commits only after successful processing.
		if err := cache.Instance().Set(ctx, permissionBacklogCursorKey, scope, permissionBacklogCursorTTL); err != nil {
			return err
		}
		if scope.Paused != 1 && scope.ResumedAt == nil {
			continue
		}
		for _, status := range []JobStatus{telegramJobStatusPending, telegramJobStatusRetryWait} {
			if err := s.scanPermissionBacklogJobs(ctx, scope, status); err != nil {
				return err
			}
		}
	}
	return nil
}

func permissionBacklogCandidatesSQL(scope permissionBacklogScope, status JobStatus, after int64) (string, []interface{}) {
	// Equality on all three leading index columns keeps ORDER BY id bounded
	// by idx_ybp_tg_job_tenant_channel_status, including evidence-heavy scopes.
	return "SELECT id FROM " + publishTgJobTable +
			" WHERE tenant_id=? AND channel_id=? AND status=? AND id>? ORDER BY id LIMIT ?",
		[]interface{}{scope.TenantId, scope.ChannelId, status, after, permissionBacklogJobPageSize}
}

func (s *sSysPublish) scanPermissionBacklogJobs(ctx context.Context, scope permissionBacklogScope, status JobStatus) error {
	key := fmt.Sprintf("%s:jobs:%d:%d:%s", permissionBacklogCursorKey, scope.TenantId, scope.ChannelId, status)
	cached, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return err
	}
	var after int64
	if !cached.IsNil() {
		after = cached.Int64()
	}
	query, args := permissionBacklogCandidatesSQL(scope, status, after)
	candidates, err := g.DB().GetAll(ctx, query, args...)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return cache.Instance().Set(ctx, key, int64(0), permissionBacklogCursorTTL)
	}
	ids := make([]int64, 0, len(candidates))
	for _, row := range candidates {
		ids = append(ids, row["id"].Int64())
	}
	var jobs []telegramJobRecord
	// Expensive evidence and timestamp checks run only on this bounded ID page.
	// Recheck circuit state here and again in the existing parking transaction.
	err = permissionBacklogEligibleJobsModel(g.DB().Model(publishTgJobTable).Safe().Ctx(ctx), scope, status, ids).
		OrderAsc("id").Limit(permissionBacklogJobPageSize).Scan(&jobs)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if _, err := s.postponePermissionPausedJob(ctx, job); err != nil {
			return err
		}
	}
	// Advance over excluded candidates too; otherwise remote evidence or a
	// post-resume job at the head could permanently starve eligible older jobs.
	return cache.Instance().Set(ctx, key, ids[len(ids)-1], permissionBacklogCursorTTL)
}

func permissionBacklogEligibleJobsModel(mod *gdb.Model, scope permissionBacklogScope, status JobStatus, ids []int64) *gdb.Model {
	return mod.Fields("id,tenant_id,channel_id,status").WhereIn("id", ids).
		Where("tenant_id", scope.TenantId).Where("channel_id", scope.ChannelId).Where("status", status).
		Where(permissionUntouchedJobSQL()).
		Where("LOWER(TRIM(operation_no)) NOT LIKE ?", "down:%").
		Where("operation_no NOT LIKE ?", "message_push:%").Where("operation_no NOT LIKE ?", "message_push_plan:%").
		Where("EXISTS (SELECT 1 FROM " + channelPermissionCircuitTable + " c WHERE c.tenant_id=" + publishTgJobTable + ".tenant_id AND c.channel_id=" + publishTgJobTable + ".channel_id AND (c.paused=1 OR " + publishTgJobTable + ".created_at<=c.resumed_at))")
}
