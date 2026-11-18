package sys

import (
	"context"
	"errors"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/internal/library/cache"
)

type profileCycleRepairCheckpoint struct {
	ProfileVersion int64
	Cursor         int64
	Channel        int64 // Positive channel ID or negative profile ID.
	Version        int64
	NextAt         int64
}

func (c *profileCycleRepairCheckpoint) selectTarget(memberID, version int64) {
	if c.Channel != memberID || c.Version != version {
		c.Channel = memberID
		c.Version = version
		c.Cursor = 0
	}
}

func (c *profileCycleRepairCheckpoint) selectProfileVersion(version int64) {
	if c.ProfileVersion != version {
		c.ProfileVersion = version
		c.Cursor = 0
	}
}

type profileCycleOverridePolicy struct {
	TenantID  int64 `orm:"tenant_id"`
	AccountID int64 `orm:"account_id"`
	ProfileID int64 `orm:"profile_id"`
	Enabled   bool  `orm:"cycle_override_enabled"`
	Days      int   `orm:"cycle_override_days"`
	Version   int64 `orm:"cycle_override_version"`
}

func profileCycleDaysForPermission(storedDays int, vip bool, freeDays int) int {
	if vip {
		return defaultCycleDays(storedDays)
	}
	return freeDays
}

// effectiveProfileCycleOverrideDays also supports callers that already loaded state.
func (s *sSysPublish) effectiveProfileCycleOverrideDays(ctx context.Context, tenantID int64, storedDays int) int {
	return s.effectiveChannelCycleDays(ctx, tenantID, storedDays)
}

func applyProfileCycleOverride(config profileCycleChannelConfig, policy profileCycleOverridePolicy) profileCycleChannelConfig {
	config.OverrideVersion = policy.Version
	config.OverrideEnabled = policy.Enabled
	if policy.Enabled {
		config.Mode = "time"
		config.Days = defaultCycleDays(policy.Days)
		config.PublishTime = ""
	}
	return config
}

// profileCycleConfigForProfile never changes channel availability or permission.
func (s *sSysPublish) profileCycleConfigForProfile(ctx context.Context, config profileCycleChannelConfig, tenantID, accountID, profileID int64) (profileCycleChannelConfig, error) {
	if tenantID <= 0 || accountID < 0 || profileID <= 0 || (config.Id > 0 && config.TenantId != tenantID) {
		return config, gerror.New("资料循环配置归属不合法")
	}
	var policy profileCycleOverridePolicy
	err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("profile_id,cycle_override_enabled,cycle_override_days,cycle_override_version").
		Where("tenant_id", tenantID).Where("account_id", accountID).Where("profile_id", profileID).
		WhereNull("deleted_at").Scan(&policy)
	if err != nil {
		return config, gerror.Wrap(err, "读取资料独立循环配置失败")
	}
	if policy.ProfileID != profileID {
		return config, gerror.New("资料不存在或无权访问")
	}
	if policy.Enabled && policy.Days <= 0 {
		return config, gerror.New("资料独立循环天数不合法")
	}
	if policy.Enabled {
		policy.Days = s.effectiveProfileCycleOverrideDays(ctx, tenantID, policy.Days)
	}
	return applyProfileCycleOverride(config, policy), nil
}

// Negative members in the existing account repair index represent profile-scoped
// work. The existing repair checkpoint retains its version and mapping cursor.
func (s *sSysPublish) rebuildProfileCycleOverridePlans(ctx context.Context, tenantID, accountID, profileID int64) error {
	if tenantID <= 0 || accountID <= 0 || profileID <= 0 {
		return gerror.New("资料循环重算参数不合法")
	}
	if !cache.Initialized() {
		return gerror.New("资料循环调度缓存未初始化")
	}
	version, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantID).Where("account_id", accountID).Where("profile_id", profileID).
		WhereNull("deleted_at").Value("cycle_override_version")
	if err != nil {
		return gerror.Wrap(err, "读取资料循环重算版本失败")
	}
	if version.IsNil() {
		return nil
	}
	scope := publishAccountScope{TenantID: tenantID, AccountID: accountID}
	if err := markAccountCycleRepairChannel(ctx, scope, -profileID, version.Int64()); err != nil {
		return gerror.Wrap(err, "登记资料循环重算失败")
	}
	return s.markAccountSchedule(ctx, scope, true)
}

// rebuildProfileCycleMapping serializes projection with profile configuration writes.
// It only updates scheduling fields; successful Job lifecycle remains unchanged.
func (s *sSysPublish) rebuildProfileCycleMapping(ctx context.Context, config profileCycleChannelConfig, row profileCycleRescheduleRow, now *gtime.Time) error {
	return s.rebuildProfileCycleMappings(ctx, config, []profileCycleRescheduleRow{row}, now)
}

// prepareProfileCycleDue is shared by account candidates and legacy dispatch.
// A stale version is projected first and can only dispatch on a later turn.
func (s *sSysPublish) prepareProfileCycleDue(ctx context.Context, row profileCycleDueRow) (bool, error) {
	config, err := s.profileCycleChannelConfigById(ctx, row.ChannelId)
	if err != nil {
		return false, err
	}
	config, err = s.profileCycleConfigForProfile(ctx, config, row.TenantId, row.AccountId, row.ProfileId)
	if err != nil {
		return false, err
	}
	if !profileCycleDueNeedsRebuild(row, config, gtime.Now(), isDevelopMode(ctx)) {
		return true, nil
	}
	var mapping profileCycleRescheduleRow
	err = g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Fields("id AS relation_id,tenant_id,account_id,profile_id,channel_id,last_job_id AS job_id").
		Where("tenant_id", row.TenantId).Where("account_id", row.AccountId).
		Where("profile_id", row.ProfileId).Where("channel_id", row.ChannelId).
		Where("last_job_id", row.JobId).Where("status", "active").Scan(&mapping)
	if err != nil || mapping.RelationId == 0 {
		return false, err
	}
	return false, s.rebuildProfileCycleMapping(ctx, config, mapping, gtime.Now())
}

func profileCycleDueNeedsRebuild(row profileCycleDueRow, config profileCycleChannelConfig, now *gtime.Time, develop bool) bool {
	if !profileCycleChannelUsable(config) || row.ProfileCycleVersion != config.OverrideVersion {
		return true
	}
	if config.OverrideEnabled {
		return row.SentAt == nil || calculateScheduledProfileCycleAt(config.Days, "", row.SentAt, row.ProfileId, row.ChannelId, develop).After(now)
	}
	return false
}

type profileCycleProjection struct {
	Row     profileCycleRescheduleRow
	Config  profileCycleChannelConfig
	NextAt  *gtime.Time
	Enabled int
}

// calculateProfileCycleProjection has no I/O. Missing success evidence clears the
// plan but still acknowledges the current profile version and boolean projection.
func calculateProfileCycleProjection(row profileCycleRescheduleRow, config profileCycleChannelConfig, policy profileCycleOverridePolicy, job telegramJobRecord, now *gtime.Time, develop bool) profileCycleProjection {
	config = applyProfileCycleOverride(config, policy)
	plan := profileCycleProjection{Row: row, Config: config}
	validSource := job.Id == row.JobId && job.SentAt != nil &&
		(job.Status == JobStatusSent || job.Status == JobStatusSuperseded) &&
		!isDownTelegramOperationNo(job.OperationNo) && !isMessagePushOperationNo(job.OperationNo)
	if !validSource || !profileCycleChannelUsable(config) {
		return plan
	}
	plan.Enabled = 1
	plan.NextAt = calculateScheduledProfileCycleAt(config.Days, config.PublishTime, job.SentAt, row.ProfileId, config.Id, develop)
	if !config.OverrideEnabled && (plan.NextAt == nil || !plan.NextAt.After(now)) {
		plan.NextAt = profileCycleOverdueAt(now, row.ProfileId, config.Id)
	}
	return plan
}

// profileCycleProjectionStatements keeps the write count constant for a page.
// Typed VALUES also handles pages consisting entirely of NULL next_at values.
func profileCycleProjectionStatements(plans []profileCycleProjection) (jobSQL, mappingSQL, acknowledgementSQL string, args []any) {
	values := make([]string, 0, len(plans))
	args = make([]any, 0, len(plans)*13)
	for _, plan := range plans {
		row, config := plan.Row, plan.Config
		values = append(values, "(?::bigint,?::bigint,?::bigint,?::bigint,?::bigint,?::bigint,?::integer,?::integer,?::text,?::timestamp,?::bigint,?::bigint,?::boolean)")
		args = append(args, row.RelationId, row.JobId, row.TenantId, row.AccountId, row.ProfileId, row.ChannelId,
			plan.Enabled, config.Days, config.PublishTime, plan.NextAt, config.Version, config.OverrideVersion, config.OverrideEnabled)
	}
	source := "(VALUES " + strings.Join(values, ",") + ") AS v(relation_id,job_id,tenant_id,account_id,profile_id,channel_id,enabled,days,clock,next_at,channel_version,profile_version,override_enabled)"
	jobSQL = "UPDATE " + publishTgJobTable + " j SET cycle_enabled=v.enabled,cycle_days=v.days,cycle_publish_time=v.clock,next_cycle_at=v.next_at,profile_cycle_version=v.profile_version FROM " + source +
		" WHERE j.id=v.job_id AND j.tenant_id=v.tenant_id AND j.account_id=v.account_id AND j.profile_id=v.profile_id AND j.channel_id=v.channel_id AND COALESCE(j.profile_cycle_version,0)<=v.profile_version" +
		" AND j.status IN ('" + JobStatusSent.String() + "','" + JobStatusSuperseded.String() + "')" +
		" AND EXISTS (SELECT 1 FROM " + publishChannelProfileTable + " cp WHERE cp.id=v.relation_id AND cp.tenant_id=v.tenant_id AND cp.account_id=v.account_id AND cp.profile_id=v.profile_id AND cp.channel_id=v.channel_id AND cp.last_job_id=j.id AND cp.status='active' AND COALESCE(cp.applied_profile_cycle_version,0)<=v.profile_version)"
	mappingSQL = "UPDATE " + publishChannelProfileTable + " cp SET next_cycle_at=v.next_at,applied_cycle_config_version=v.channel_version,cycle_override_enabled=v.override_enabled FROM " + source +
		" WHERE cp.id=v.relation_id AND cp.tenant_id=v.tenant_id AND cp.account_id=v.account_id AND cp.profile_id=v.profile_id AND cp.channel_id=v.channel_id AND cp.last_job_id=v.job_id AND cp.status='active' AND COALESCE(cp.applied_profile_cycle_version,0)<=v.profile_version RETURNING cp.id"
	acknowledgementSQL = "UPDATE " + publishChannelProfileTable + " cp SET applied_profile_cycle_version=v.profile_version FROM " + source +
		" WHERE cp.id=v.relation_id AND cp.tenant_id=v.tenant_id AND cp.account_id=v.account_id AND cp.profile_id=v.profile_id AND cp.channel_id=v.channel_id AND cp.last_job_id=v.job_id AND cp.status='active' AND COALESCE(cp.applied_profile_cycle_version,0)<=v.profile_version AND cp.next_cycle_at IS NOT DISTINCT FROM v.next_at AND COALESCE(cp.applied_cycle_config_version,0)=v.channel_version AND cp.cycle_override_enabled=v.override_enabled" +
		" AND EXISTS (SELECT 1 FROM " + publishProfileStateTable + " ps WHERE ps.tenant_id=v.tenant_id AND ps.account_id=v.account_id AND ps.profile_id=v.profile_id AND ps.deleted_at IS NULL AND ps.cycle_override_version=v.profile_version AND ps.cycle_override_enabled=v.override_enabled)"
	return
}

// rebuildProfileCycleMappings uses three bounded reads and two set-based writes.
// Lock order is state -> channel -> successful Job -> mapping, with no per-row SQL.
func (s *sSysPublish) rebuildProfileCycleMappings(ctx context.Context, fallback profileCycleChannelConfig, rows []profileCycleRescheduleRow, now *gtime.Time) error {
	if len(rows) == 0 {
		return nil
	}
	if len(rows) > profileCycleRescheduleBatchSize {
		return gerror.New("资料循环重算超过单页上限")
	}
	var profileIDs, channelIDs, jobIDs, tenantIDs, accountIDs []int64
	for i := range rows {
		if rows[i].ChannelId == 0 {
			rows[i].ChannelId = fallback.Id
		}
		profileIDs = append(profileIDs, rows[i].ProfileId)
		channelIDs = append(channelIDs, rows[i].ChannelId)
		jobIDs = append(jobIDs, rows[i].JobId)
		tenantIDs = append(tenantIDs, rows[i].TenantId)
		accountIDs = append(accountIDs, rows[i].AccountId)
	}
	plansByID := make(map[int64]profileCycleProjection, len(rows))
	var applied gdb.Result
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var policies []profileCycleOverridePolicy
		if err := tx.Model(publishProfileStateTable).Safe().Ctx(ctx).
			Fields("tenant_id,account_id,profile_id,cycle_override_enabled,cycle_override_days,cycle_override_version").
			WhereIn("profile_id", profileIDs).WhereIn("tenant_id", tenantIDs).WhereIn("account_id", accountIDs).
			WhereNull("deleted_at").OrderAsc("profile_id").LockShared().Scan(&policies); err != nil {
			return err
		}
		policyByScope := make(map[[3]int64]profileCycleOverridePolicy, len(policies))
		for _, policy := range policies {
			policyByScope[[3]int64{policy.TenantID, policy.AccountID, policy.ProfileID}] = policy
		}
		var configs []profileCycleChannelConfig
		if err := tx.Model(publishChannelTable).Safe().Ctx(ctx).
			Fields("id,tenant_id,cycle_publish_enabled,cycle_publish_mode,cycle_publish_days,cycle_publish_time,status,publish_direction,cycle_config_version").
			WhereIn("id", channelIDs).WhereIn("tenant_id", tenantIDs).WhereNull("deleted_at").OrderAsc("id").LockShared().Scan(&configs); err != nil {
			return err
		}
		configByID := make(map[int64]profileCycleChannelConfig, len(configs))
		vipByTenant := make(map[int64]bool)
		freeDays := maxConfigInt(ctx, "youbanPublish.cycle.freeIntervalDays", 15)
		for _, config := range configs {
			vip, checked := vipByTenant[config.TenantId]
			if !checked {
				status, err := s.tenantVipStatus(ctx, config.TenantId)
				vip = err == nil && tenantVipStatusActive(status)
				vipByTenant[config.TenantId] = vip
			}
			config.Days = profileCycleDaysForPermission(config.Days, vip, freeDays)
			configByID[config.Id] = config
		}
		var jobs []telegramJobRecord
		if err := tx.Model(publishTgJobTable+" j").Safe().Ctx(ctx).
			InnerJoin("hg_content_profile p", "p.id=j.profile_id AND p.deleted_at IS NULL AND p.status=1 AND p.visibility='public'").
			Fields("j.id,j.tenant_id,j.account_id,j.profile_id,j.channel_id,j.sent_at,j.operation_no,j.status").
			WhereIn("j.id", jobIDs).WhereIn("j.tenant_id", tenantIDs).WhereIn("j.account_id", accountIDs).
			WhereIn("j.status", []JobStatus{JobStatusSent, JobStatusSuperseded}).OrderAsc("j.id").Lock("FOR UPDATE OF j").Scan(&jobs); err != nil {
			return err
		}
		jobByID := make(map[int64]telegramJobRecord, len(jobs))
		for _, job := range jobs {
			jobByID[job.Id] = job
		}
		plans := make([]profileCycleProjection, 0, len(rows))
		develop := isDevelopMode(ctx)
		for _, row := range rows {
			policy, exists := policyByScope[[3]int64{row.TenantId, row.AccountId, row.ProfileId}]
			if !exists {
				continue
			}
			if policy.Enabled && policy.Days <= 0 {
				return gerror.New("资料独立循环天数不合法")
			}
			config, exists := configByID[row.ChannelId]
			if !exists || config.TenantId != row.TenantId {
				config = profileCycleChannelConfig{Id: row.ChannelId, TenantId: row.TenantId}
			}
			if policy.Enabled {
				policy.Days = profileCycleDaysForPermission(policy.Days, vipByTenant[row.TenantId], freeDays)
			}
			effective := applyProfileCycleOverride(config, policy)
			job := jobByID[row.JobId]
			if job.TenantId != row.TenantId || job.AccountId != row.AccountId || job.ProfileId != row.ProfileId || job.ChannelId != row.ChannelId {
				job = telegramJobRecord{}
			}
			plan := calculateProfileCycleProjection(row, config, policy, job, now, develop)
			if row.PreserveCurrent && row.NextAt != nil && row.AppliedChannelVersion >= effective.Version && row.AppliedProfileVersion >= policy.Version && plan.Enabled == 1 {
				plan.NextAt = row.NextAt
			}
			plans = append(plans, plan)
			plansByID[row.RelationId] = plan
		}
		if len(plans) == 0 {
			return nil
		}
		jobSQL, mappingSQL, _, args := profileCycleProjectionStatements(plans)
		if _, err := tx.Exec(jobSQL, args...); err != nil {
			return err
		}
		var err error
		applied, err = tx.GetAll(mappingSQL, args...)
		return err
	})
	if err != nil {
		return gerror.Wrap(err, "批量重算资料循环计划失败")
	}
	completed := make([]profileCycleProjection, 0, len(applied))
	for _, result := range applied {
		plan := plansByID[result["id"].Int64()]
		if err := indexRebuiltProfileCycleDue(ctx, plan); err != nil {
			return err
		}
		completed = append(completed, plan)
	}
	if len(completed) == 0 {
		return nil
	}
	_, _, acknowledgementSQL, args := profileCycleProjectionStatements(completed)
	_, err = g.DB().Exec(ctx, acknowledgementSQL, args...)
	return err
}

type profileCycleIndexErrorKey struct{}

// The strict rebuild path records errors from the existing index primitives.
// Other callers retain the original void signatures and best-effort behavior.
func recordProfileCycleIndexError(ctx context.Context, err error) {
	if sink, ok := ctx.Value(profileCycleIndexErrorKey{}).(*error); ok && err != nil {
		*sink = errors.Join(*sink, err)
	}
}

func indexRebuiltProfileCycleDue(ctx context.Context, plan profileCycleProjection) error {
	if plan.Row.JobId <= 0 {
		return nil
	}
	if !cache.Initialized() {
		return gerror.New("资料循环索引缓存未初始化")
	}
	var failure error
	ctx = context.WithValue(ctx, profileCycleIndexErrorKey{}, &failure)
	if plan.NextAt == nil {
		removeProfileCycleDue(ctx, plan.Row.JobId)
	} else {
		indexProfileCycleDue(ctx, plan.Row.JobId, plan.NextAt)
		if failure == nil {
			indexAccountProfileCycleDue(ctx, publishAccountScope{TenantID: plan.Row.TenantId, AccountID: plan.Row.AccountId}, plan.Row.JobId, plan.NextAt)
		}
	}
	if failure != nil {
		return gerror.Wrap(failure, "写入重算资料循环索引失败")
	}
	return nil
}
