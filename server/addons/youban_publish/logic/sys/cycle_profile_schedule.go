package sys

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
	hglock "hotgo/internal/library/hgrds/lock"
)

const (
	profileCycleScanBatchSize         = 200
	profileCycleMaxProducePerRun      = 5000
	profileCycleChannelBacklogDefault = 100
	profileCycleClaimLease            = 2 * time.Hour
	profileCycleRetryDelay            = time.Hour
	profileCycleFailureThreshold      = 3
	profileCycleRescheduleBatchSize   = 500
	profileCycleOverdueSpreadWindow   = 4 * time.Hour
	profileCycleDevelopSpreadWindow   = 5 * time.Minute
	profileCycleSummaryRefreshDelay   = 30 * time.Second
	profileCycleRescheduleClaimLease  = 30 * time.Minute
	profileCycleShardCount            = 64
)

const profileCycleChannelConfigCacheTTL = 30 * time.Second

const profileCycleDueZSetKey = "youban_publish:cycle:profile:due"

func profileCycleChannelConfigCacheKey(channelId int64) string {
	return fmt.Sprintf("youban_publish:cycle:channel_config:%d", channelId)
}

func profileCycleDueMember(jobId int64) string {
	return fmt.Sprintf("%d", jobId)
}

func indexProfileCycleDue(ctx context.Context, jobId int64, nextAt *gtime.Time) {
	if jobId <= 0 || nextAt == nil || !cache.Initialized() {
		return
	}
	if err := cache.ZSetAdd(ctx, profileCycleDueZSetKey, float64(nextAt.Unix()), profileCycleDueMember(jobId)); err != nil {
		g.Log().Debugf(ctx, "写入资料循环Redis调度索引失败 job:%d err:%+v", jobId, err)
	}
}

// accountProfileCycleDueKey 隔离账户候选；全局索引仅供旧调度使用。
func accountProfileCycleDueKey(scope publishAccountScope) string {
	return fmt.Sprintf("%s:account:%d:%d", profileCycleDueZSetKey, scope.TenantID, scope.AccountID)
}

// markAccountCycleRepairChannel 仅由已知业务归属或显式历史恢复写入dirty频道。
func markAccountCycleRepairChannel(ctx context.Context, scope publishAccountScope, channelID int64) error {
	if !cache.Initialized() || scope.TenantID <= 0 || scope.AccountID <= 0 || channelID <= 0 {
		return nil
	}
	client, err := cache.RedisClient()
	if err != nil {
		return err
	}
	key := accountProfileCycleDueKey(scope) + ":repair_channels"
	_, err = client.GroupScript().Eval(ctx, `redis.call("HINCRBY",KEYS[2],ARGV[1],1); redis.call("ZADD",KEYS[1],"NX",ARGV[2],ARGV[1]); return 1`, 2, []string{key, key + ":versions"}, []interface{}{strconv.FormatInt(channelID, 10), time.Now().UnixMilli()})
	return err
}

func indexAccountProfileCycleDue(ctx context.Context, scope publishAccountScope, jobID int64, nextAt *gtime.Time) {
	if cache.Initialized() && scope.TenantID > 0 && scope.AccountID > 0 && jobID > 0 && nextAt != nil {
		if err := cache.ZSetAdd(ctx, accountProfileCycleDueKey(scope), float64(nextAt.Unix()), profileCycleDueMember(jobID)); err != nil {
			g.Log().Warningf(ctx, "写入账户循环到期索引失败 tenantId:%d accountId:%d jobId:%d err:%+v", scope.TenantID, scope.AccountID, jobID, err)
			return
		}
		if err := scheduleAccountCycleWake(ctx, scope, nextAt.Time); err != nil {
			g.Log().Warningf(ctx, "登记账户循环到期唤醒失败 tenantId:%d accountId:%d jobId:%d err:%+v", scope.TenantID, scope.AccountID, jobID, err)
		}
	}
}

type accountCycleDueCursor struct {
	At *gtime.Time
	ID int64
}

// accountProfileCycleDueRows 优先账户Redis；PG每页只读取20条映射，再按这些主键验证。
// 补偿游标跨轮保存，无效队头不能永久挡住后面的资料；不使用全局候选过滤。
func (s *sSysPublish) accountProfileCycleDueRows(ctx context.Context, limit int, scope publishAccountScope) ([]profileCycleDueRow, error) {
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	if scope.TenantID <= 0 || scope.AccountID <= 0 {
		return nil, nil
	}
	key := accountProfileCycleDueKey(scope)
	var result []profileCycleDueRow
	seen := make(map[int64]bool)
	validate := func(ids []int64) error {
		if len(ids) == 0 {
			return nil
		}
		var rows []profileCycleDueRow
		err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
			InnerJoin(publishChannelProfileTable+" cp", "cp.last_job_id=j.id AND cp.tenant_id=j.tenant_id AND cp.account_id=j.account_id AND cp.channel_id=j.channel_id AND cp.profile_id=j.profile_id").
			InnerJoin("hg_content_profile p", "p.id=j.profile_id AND p.deleted_at IS NULL").
			InnerJoin(publishChannelTable+" c", "c.id=j.channel_id AND c.tenant_id=j.tenant_id AND c.deleted_at IS NULL").
			Fields("j.id AS job_id,j.tenant_id,j.account_id,j.profile_id,j.channel_id,j.sent_at,j.cycle_days,j.cycle_publish_time,cp.next_cycle_at,cp.cycle_generation AS dispatch_count,j.cycle_failure_count").
			WhereIn("j.id", ids).Where("j.tenant_id", scope.TenantID).Where("j.account_id", scope.AccountID).
			Where("cp.status", "active").WhereNotNull("cp.next_cycle_at").WhereLTE("cp.next_cycle_at", gtime.Now()).
			WhereIn("j.status", []string{"sent", "superseded"}).
			Where("p.status", 1).Where("p.visibility", consts.ContentVisibilityPublic).
			Where("c.status", 1).Where("c.cycle_publish_enabled", 1).Where("c.publish_direction", "up").
			Where("COALESCE(NULLIF(LOWER(TRIM(c.cycle_publish_mode)),''),'time')='time'").
			Where("NOT EXISTS (SELECT 1 FROM " + channelPermissionCircuitTable + " pc WHERE pc.tenant_id=j.tenant_id AND pc.channel_id=j.channel_id AND pc.paused=1)").
			Where("NOT EXISTS (SELECT 1 FROM " + publishTgJobTable + " a WHERE a.tenant_id=j.tenant_id AND a.account_id=j.account_id AND a.profile_id=j.profile_id AND a.channel_id=j.channel_id AND a.status IN ('pending','sending','confirming','failed_retry','unknown'))").
			Where("NOT EXISTS (SELECT 1 FROM " + publishProfileCycleRunChannelTable + " rc JOIN " + publishProfileCycleRunTable + " r ON r.id=rc.run_id WHERE r.tenant_id=j.tenant_id AND r.account_id=j.account_id AND r.profile_id=j.profile_id AND rc.channel_id=j.channel_id AND r.status IN ('pending','running','dispatching') AND rc.status IN ('pending','running','dispatching'))").
			OrderAsc("cp.next_cycle_at").OrderAsc("cp.id").Limit(20).Scan(&rows)
		if err != nil {
			return gerror.Wrap(err, "校验账户循环候选失败")
		}
		for _, id := range ids {
			if cache.Initialized() {
				_ = cache.ZSetRemove(ctx, key, profileCycleDueMember(id))
			}
		}
		for _, row := range rows {
			indexAccountProfileCycleDue(ctx, scope, row.JobId, row.NextCycleAt)
			if !seen[row.JobId] && len(result) < limit {
				result = append(result, row)
				seen[row.JobId] = true
			}
		}
		return nil
	}
	if cache.Initialized() {
		members, err := cache.ZSetRangeByScore(ctx, key, 0, float64(time.Now().Unix()), 20)
		if err == nil {
			var ids []int64
			for _, member := range members {
				id, parseErr := strconv.ParseInt(member, 10, 64)
				if parseErr == nil && id > 0 {
					ids = append(ids, id)
				} else {
					_ = cache.ZSetRemove(ctx, key, member)
				}
			}
			if err = validate(ids); err != nil {
				return nil, err
			}
		}
	}
	// 即使命中Redis也补一小页，防止有效但长期受阻的旧候选垄断索引。
	cursorKey := key + ":pg_cursor"
	var cursor accountCycleDueCursor
	if cache.Initialized() {
		if v, err := cache.Instance().Get(ctx, cursorKey); err == nil && !v.IsNil() {
			_ = v.Scan(&cursor)
		}
	}
	more := len(result) >= limit
	for page := 0; page < 3 && len(result) < limit; page++ {
		var candidates []struct {
			ID        int64
			LastJobID int64       `orm:"last_job_id"`
			NextAt    *gtime.Time `orm:"next_cycle_at"`
		}
		model := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
			Fields("id,last_job_id,next_cycle_at").Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).
			Where("status", "active").WhereNotNull("next_cycle_at").WhereLTE("next_cycle_at", gtime.Now())
		if cursor.At != nil {
			model = model.Where("(next_cycle_at,id)>(?,?)", cursor.At, cursor.ID)
		}
		if err := model.OrderAsc("next_cycle_at").OrderAsc("id").Limit(20).Scan(&candidates); err != nil {
			return nil, gerror.Wrap(err, "读取账户循环补偿候选失败")
		}
		more = len(candidates) == 20
		if len(candidates) == 0 {
			cursor = accountCycleDueCursor{}
			break
		}
		var ids []int64
		for _, c := range candidates {
			if c.LastJobID > 0 {
				ids = append(ids, c.LastJobID)
			}
			cursor = accountCycleDueCursor{At: c.NextAt, ID: c.ID}
		}
		if err := validate(ids); err != nil {
			return nil, err
		}
		if len(candidates) < 20 {
			cursor = accountCycleDueCursor{}
			break
		}
	}
	if cache.Initialized() {
		_ = cache.Instance().Set(ctx, cursorKey, cursor, time.Hour)
		_ = cache.Instance().Set(ctx, key+":more", more || len(result) >= limit, time.Minute)
	}
	return result, nil
}

// accountProfileCycleCandidatesMore 供账户消费者将有界前翻剩余工作安排到下一轮。
func accountProfileCycleCandidatesMore(ctx context.Context, scope publishAccountScope) bool {
	if !cache.Initialized() {
		return false
	}
	v, err := cache.Instance().Get(ctx, accountProfileCycleDueKey(scope)+":more")
	return err == nil && !v.IsNil() && v.Bool()
}

func removeProfileCycleDue(ctx context.Context, jobId int64) {
	if jobId <= 0 || !cache.Initialized() {
		return
	}
	_ = cache.ZSetRemove(ctx, profileCycleDueZSetKey, profileCycleDueMember(jobId))
}

func invalidateProfileCycleChannelConfigCache(ctx context.Context, channelId int64) {
	if channelId > 0 {
		_, _ = cache.Instance().Remove(ctx, profileCycleChannelConfigCacheKey(channelId))
	}
}

type profileCycleChannelConfig struct {
	Id          int64  `orm:"id"`
	TenantId    int64  `orm:"tenant_id"`
	Enabled     int    `orm:"cycle_publish_enabled"`
	Mode        string `orm:"cycle_publish_mode"`
	Days        int    `orm:"cycle_publish_days"`
	PublishTime string `orm:"cycle_publish_time"`
	Status      int    `orm:"status"`
	Direction   string `orm:"publish_direction"`
	Version     int64  `orm:"cycle_config_version"`
}

type profileCycleRescheduleRow struct {
	RelationId int64       `orm:"relation_id"`
	JobId      int64       `orm:"job_id"`
	ProfileId  int64       `orm:"profile_id"`
	SentAt     *gtime.Time `orm:"sent_at"`
}

type profileCycleDueRow struct {
	JobId              int64       `orm:"job_id"`
	TenantId           int64       `orm:"tenant_id"`
	AccountId          int64       `orm:"account_id"`
	ProfileId          int64       `orm:"profile_id"`
	ChannelId          int64       `orm:"channel_id"`
	SentAt             *gtime.Time `orm:"sent_at"`
	CycleDays          int         `orm:"cycle_days"`
	CyclePublishTime   string      `orm:"cycle_publish_time"`
	NextCycleAt        *gtime.Time `orm:"next_cycle_at"`
	DispatchCount      int         `orm:"dispatch_count"`
	CycleFailureCount  int         `orm:"cycle_failure_count"`
	ChannelEnabled     int         `orm:"channel_enabled"`
	ChannelDays        int         `orm:"channel_days"`
	ChannelPublishTime string      `orm:"channel_publish_time"`
	ChannelMode        string      `orm:"channel_mode"`
	ChannelStatus      int         `orm:"channel_status"`
	PublishDirection   string      `orm:"publish_direction"`
}

// syncTelegramJobCycleSchedule 复用会员和时钟规则，同步成功任务的兼容计划及当前频道资料计划。
func (s *sSysPublish) syncTelegramJobCycleSchedule(ctx context.Context, job telegramJobRecord) error {
	if job.Id <= 0 || job.ChannelId <= 0 || job.ProfileId <= 0 {
		return nil
	}
	config, err := s.profileCycleChannelConfigById(ctx, job.ChannelId)
	if err != nil {
		return err
	}
	now := gtime.Now()
	if job.SentAt == nil {
		job.SentAt = now
	}
	if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("channel_id", job.ChannelId).
		Where("profile_id", job.ProfileId).
		WhereNot("id", job.Id).
		Where("cycle_enabled", 1).
		Data(g.Map{"cycle_enabled": 0, "next_cycle_at": nil, "updated_at": now}).Update(); err != nil {
		return gerror.Wrap(err, "关闭资料旧循环计划失败")
	}
	data := g.Map{
		"cycle_enabled":         0,
		"cycle_days":            defaultCycleDays(config.Days),
		"cycle_publish_time":    strings.TrimSpace(config.PublishTime),
		"next_cycle_at":         nil,
		"updated_at":            now,
		"cycle_scheduler_shard": gdb.Raw("MOD(ABS(HASHTEXT(tenant_id::text || ':' || account_id::text)),64)"),
	}
	if profileCycleChannelUsable(config) {
		data["cycle_enabled"] = 1
		data["next_cycle_at"] = nextProfileCycleAt(ctx, config.Days, config.PublishTime, job.SentAt, job.ProfileId, job.ChannelId)
	}
	if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).WhereIn("status", []string{"sent", "superseded"}).Data(data).Update(); err != nil {
		return gerror.Wrap(err, "更新资料循环计划失败")
	}
	if dataNextAt, ok := data["next_cycle_at"]; ok && profileCycleChannelUsable(config) {
		if nextAt, ok := dataNextAt.(*gtime.Time); ok {
			indexProfileCycleDue(ctx, job.Id, nextAt)
			indexAccountProfileCycleDue(ctx, publishAccountScope{TenantID: job.TenantId, AccountID: job.AccountId}, job.Id, nextAt)
		}
	} else {
		removeProfileCycleDue(ctx, job.Id)
	}
	if err = projectChannelCyclePlans(ctx, []int64{job.Id}); err != nil {
		return err
	}
	if profileCycleChannelUsable(config) {
		if wakeErr := s.markAccountSchedule(ctx, publishAccountScope{TenantID: job.TenantId, AccountID: job.AccountId}, true); wakeErr != nil {
			g.Log().Warningf(ctx, "唤醒循环账户失败 account:%d err:%+v", job.AccountId, wakeErr)
		}
	}
	if err = s.enqueueCycleSummaryRefresh(ctx, job.ChannelId, profileCycleSummaryRefreshDelay); err != nil && ctx.Err() == nil {
		g.Log().Warningf(ctx, "提交频道循环汇总刷新失败 channel:%d job:%d err:%+v", job.ChannelId, job.Id, err)
	}
	return nil
}

func (s *sSysPublish) profileCycleChannelConfigById(ctx context.Context, channelId int64) (profileCycleChannelConfig, error) {
	var config profileCycleChannelConfig
	if channelId <= 0 {
		return config, nil
	}
	cacheKey := profileCycleChannelConfigCacheKey(channelId)
	if cached, err := cache.Instance().Get(ctx, cacheKey); err == nil && !cached.IsNil() {
		if err = cached.Scan(&config); err == nil && config.Id > 0 {
			return config, nil
		}
	}
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,cycle_publish_enabled,cycle_publish_mode,cycle_publish_days,cycle_publish_time,status,publish_direction,cycle_config_version").
		Where("id", channelId).WhereNull("deleted_at").Scan(&config)
	if err != nil {
		return config, gerror.Wrap(err, "读取频道循环配置失败")
	}
	config.Days = s.effectiveChannelCycleDays(ctx, config.TenantId, config.Days)
	_ = cache.Instance().Set(ctx, cacheKey, config, profileCycleChannelConfigCacheTTL)
	return config, nil
}

func (s *sSysPublish) effectiveChannelCycleDays(ctx context.Context, tenantId int64, storedDays int) int {
	vip, err := s.tenantVipStatus(ctx, tenantId)
	if err == nil && tenantVipStatusActive(vip) {
		return defaultCycleDays(storedDays)
	}
	return maxConfigInt(ctx, "youbanPublish.cycle.freeIntervalDays", 15)
}

func profileCycleChannelUsable(config profileCycleChannelConfig) bool {
	mode := strings.TrimSpace(config.Mode)
	return config.Id > 0 && config.Enabled == 1 && (mode == "" || mode == "time") && config.Status == 1 && strings.EqualFold(strings.TrimSpace(config.Direction), "up")
}

func sameProfileCycleConfig(first, second profileCycleChannelConfig) bool {
	firstUsable := profileCycleChannelUsable(first)
	secondUsable := profileCycleChannelUsable(second)
	if firstUsable != secondUsable {
		return false
	}
	if !firstUsable {
		return true
	}
	return defaultCycleDays(first.Days) == defaultCycleDays(second.Days) &&
		strings.TrimSpace(first.PublishTime) == strings.TrimSpace(second.PublishTime)
}

func nextProfileCycleAt(ctx context.Context, days int, publishTime string, sentAt *gtime.Time, profileId, channelId int64) *gtime.Time {
	return calculateScheduledProfileCycleAt(days, publishTime, sentAt, profileId, channelId, isDevelopMode(ctx))
}

func calculateScheduledProfileCycleAt(days int, publishTime string, sentAt *gtime.Time, profileId, channelId int64, develop bool) *gtime.Time {
	next := calculateProfileCycleAt(days, publishTime, sentAt, develop)
	if next == nil {
		return nil
	}
	if _, _, ok := parseCycleClock(publishTime); !ok {
		return next
	}
	window := profileCycleOverdueSpreadWindow
	if develop {
		window = profileCycleDevelopSpreadWindow
	}
	return next.Add(profileCycleSpreadOffset(profileId, channelId, window))
}

func calculateProfileCycleAt(days int, publishTime string, sentAt *gtime.Time, develop bool) *gtime.Time {
	if sentAt == nil {
		sentAt = gtime.Now()
	}
	if develop {
		return sentAt.Add(time.Duration(defaultCycleDays(days)) * time.Second)
	}
	next := sentAt.AddDate(0, 0, defaultCycleDays(days))
	hour, minute, ok := parseCycleClock(publishTime)
	if !ok {
		return next
	}
	value := next.Time
	return gtime.New(time.Date(value.Year(), value.Month(), value.Day(), hour, minute, 0, 0, value.Location()))
}

func profileCycleOverdueAt(now *gtime.Time, profileId, channelId int64) *gtime.Time {
	if now == nil {
		now = gtime.Now()
	}
	return now.Add(profileCycleSpreadOffset(profileId, channelId, profileCycleOverdueSpreadWindow))
}

func profileCycleSpreadOffset(profileId, _ int64, window time.Duration) time.Duration {
	spreadSeconds := uint64(window / time.Second)
	if spreadSeconds <= 0 {
		return 0
	}
	// Keep the same profile in the same position across a tenant's channels.
	// Channel delivery remains independent, so one failed channel cannot block others.
	offset := uint64(profileId) * 11400714819323198485 % spreadSeconds
	return time.Duration(offset) * time.Second
}

func (s *sSysPublish) rescheduleChannelProfileCycles(ctx context.Context, channelId int64) error {
	if channelId <= 0 {
		return nil
	}
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock := hglock.NewConfig(2*time.Hour, 100*time.Millisecond).Mutex(fmt.Sprintf("youban_publish:cycle_reschedule:%d", channelId))
	if err := lock.TryLock(lockCtx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return gerror.New("频道循环重算正在执行，请稍后重试")
		}
		return gerror.Wrap(err, "获取频道循环重算锁失败")
	}
	defer s.releaseTelegramChannelLease(context.Background(), lock)
	config, err := s.profileCycleChannelConfigById(ctx, channelId)
	if err != nil {
		return err
	}
	now := gtime.Now()
	if !profileCycleChannelUsable(config) {
		if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("channel_id", channelId).Where("cycle_enabled", 1).
			Data(g.Map{"cycle_enabled": 0, "next_cycle_at": nil, "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "关闭频道资料循环计划失败")
		}
		if _, err := g.DB().Model(publishChannelProfileTable).Ctx(ctx).Where("channel_id", channelId).Data(g.Map{"next_cycle_at": nil}).Update(); err != nil {
			return err
		}
		return s.finishChannelProfileCycleReschedule(ctx, config)
	}
	cleanupSQL := fmt.Sprintf("UPDATE %s SET cycle_enabled=0,next_cycle_at=NULL,updated_at=? WHERE channel_id=? AND cycle_enabled=1 AND NOT EXISTS (SELECT 1 FROM %s cp WHERE cp.last_job_id=%s.id AND cp.status='active')",
		publishTgJobTable, publishChannelProfileTable, publishTgJobTable)
	if _, err = g.DB().Exec(ctx, cleanupSQL, now, channelId); err != nil {
		return gerror.Wrap(err, "清理频道历史循环计划失败")
	}
	var cursor int64
	for {
		var rows []profileCycleRescheduleRow
		err = g.DB().Model(publishChannelProfileTable+" cp").Safe().Ctx(ctx).
			InnerJoin(publishTgJobTable+" j", "j.id=cp.last_job_id AND j.status IN ('sent','superseded')").
			Fields("cp.id AS relation_id,j.id AS job_id,j.profile_id,j.sent_at").
			Where("cp.channel_id", channelId).
			Where("cp.status", "active").
			WhereGT("cp.id", cursor).
			OrderAsc("cp.id").Limit(profileCycleRescheduleBatchSize).Scan(&rows)
		if err != nil {
			return gerror.Wrap(err, "读取频道资料循环基准失败")
		}
		if len(rows) == 0 {
			break
		}
		if err = s.updateProfileCycleRescheduleBatch(ctx, config, rows, now); err != nil {
			return err
		}
		cursor = rows[len(rows)-1].RelationId
	}
	// 兜底修复所有来源的空循环计划（包括用户自行上传资料）。
	// 这些资料可能由批次循环生成成功任务，但关联映射未投影 next_cycle_at。
	if _, err = g.DB().Exec(ctx, fmt.Sprintf("UPDATE %s cp SET next_cycle_at = j.sent_at + (? * INTERVAL '1 day'), updated_at=? FROM %s j WHERE cp.last_job_id=j.id AND cp.channel_id=? AND cp.status='active' AND cp.next_cycle_at IS NULL AND j.status IN ('sent','superseded') AND j.sent_at IS NOT NULL", publishChannelProfileTable, publishTgJobTable), config.Days, now, channelId); err != nil {
		return gerror.Wrap(err, "补齐频道资料循环计划失败")
	}
	return s.finishChannelProfileCycleReschedule(ctx, config)
}

func (s *sSysPublish) finishChannelProfileCycleReschedule(ctx context.Context, config profileCycleChannelConfig) error {
	if err := s.refreshChannelProfileCycleNextAt(ctx, config.Id); err != nil {
		return err
	}
	latest, err := s.profileCycleChannelConfigById(ctx, config.Id)
	if err != nil {
		return err
	}
	if sameProfileCycleConfig(config, latest) {
		return nil
	}
	_, _ = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("id", config.Id).Data(g.Map{
		"cycle_next_run_at": nil,
		"updated_at":        gtime.Now(),
	}).Update()
	return gerror.New("频道循环配置已变化，等待按最新设置重新计算")
}

func (s *sSysPublish) updateProfileCycleRescheduleBatch(ctx context.Context, config profileCycleChannelConfig, rows []profileCycleRescheduleRow, now *gtime.Time) error {
	if len(rows) == 0 {
		return nil
	}
	caseParts := make([]string, 0, len(rows))
	idParts := make([]string, 0, len(rows))
	args := make([]any, 0, 5+len(rows)*3)
	args = append(args, defaultCycleDays(config.Days), strings.TrimSpace(config.PublishTime))
	for _, row := range rows {
		nextAt := nextProfileCycleAt(ctx, config.Days, config.PublishTime, row.SentAt, row.ProfileId, config.Id)
		if nextAt == nil || !nextAt.After(now) {
			nextAt = profileCycleOverdueAt(now, row.ProfileId, config.Id)
		}
		caseParts = append(caseParts, "WHEN ? THEN ?")
		args = append(args, row.JobId, nextAt)
		idParts = append(idParts, "?")
	}
	args = append(args, now)
	for _, row := range rows {
		args = append(args, row.JobId)
	}
	sql := fmt.Sprintf("UPDATE %s SET cycle_enabled=1,cycle_days=?,cycle_publish_time=?,next_cycle_at=CASE id %s ELSE next_cycle_at END,updated_at=? WHERE id IN (%s)",
		publishTgJobTable, strings.Join(caseParts, " "), strings.Join(idParts, ","))
	if _, err := g.DB().Exec(ctx, sql, args...); err != nil {
		return gerror.Wrap(err, "批量重算频道资料循环时间失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.JobId)
	}
	if _, err := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).Where("channel_id", config.Id).WhereIn("last_job_id", ids).Data(g.Map{"applied_cycle_config_version": config.Version, "updated_at": now}).Update(); err != nil {
		return gerror.Wrap(err, "更新频道资料循环配置版本失败")
	}
	return projectChannelCyclePlans(ctx, ids)
}

func (s *sSysPublish) refreshChannelProfileCycleNextAt(ctx context.Context, channelId int64) error {
	if channelId <= 0 {
		return nil
	}
	// 频道本身已经是最小调度分片；使用有序 LIMIT 1 读取队首，避免对该频道全部资料执行 MIN 聚合。
	value, err := g.DB().Model(publishChannelProfileTable+" cp").Safe().Ctx(ctx).
		InnerJoin(publishTgJobTable+" j", "j.id=cp.last_job_id AND j.cycle_enabled=1 AND j.status IN ('sent','superseded')").
		Where("cp.channel_id", channelId).Where("cp.status", "active").
		Where("j.next_cycle_at IS NOT NULL").
		Fields("j.next_cycle_at").OrderAsc("j.next_cycle_at").Limit(1).Value()
	if err != nil {
		return gerror.Wrap(err, "刷新频道下次循环时间失败")
	}
	var nextAt any
	if !value.IsNil() {
		nextAt = value.Val()
	} else {
		// Empty channels still need a future checkpoint; keeping NULL would
		// enqueue the same reschedule task on every scheduler pass.
		nextAt = gtime.Now().Add(24 * time.Hour)
	}
	_, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channelId).Data(g.Map{
		"cycle_next_run_at": nextAt, "updated_at": gtime.Now(),
	}).Where("cycle_publish_mode", "time").Update()
	return err
}

func (s *sSysPublish) runProfileCycleDueScan(ctx context.Context) error {
	// 没有任何账户频道开启循环上架时直接返回，避免无意义的 Redis/PG 到期扫描。
	channelID, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("cycle_publish_enabled", 1).
		Where("status", 1).
		Where("publish_direction", "up").
		WhereNull("deleted_at").
		Limit(1).Value("id")
	if err != nil {
		return gerror.Wrap(err, "检查循环上架频道失败")
	}
	if channelID.Int64() <= 0 {
		g.Log().Debug(ctx, "没有开启循环上架的频道，跳过资料循环扫描")
		return nil
	}
	if globalStreamPushEngineEnabled() {
		return s.enqueueAccountSchedules(ctx)
	}
	produced := 0
	processed := 0
	affectedChannels := make(map[int64]struct{})
	channelBacklog := make(map[int64]int)
	processRows := func(rows []profileCycleDueRow) {
		for _, row := range rows {
			if produced >= profileCycleMaxProducePerRun {
				return
			}
			processed++
			created, handleErr := s.dispatchDueProfileCycle(ctx, row, channelBacklog)
			if handleErr != nil {
				g.Log().Warningf(ctx, "提交到期资料循环失败 job:%d channel:%d err:%+v", row.JobId, row.ChannelId, handleErr)
				continue
			}
			affectedChannels[row.ChannelId] = struct{}{}
			if created {
				produced++
				channelBacklog[row.ChannelId]++
			}
		}
	}

	// Redis is the normal candidate index. PG still validates every candidate
	// and remains the bounded fallback when Redis is empty or unavailable.
	if cache.Initialized() {
		members, cacheErr := cache.ZSetRangeByScore(ctx, profileCycleDueZSetKey, 0, float64(time.Now().Unix()), profileCycleMaxProducePerRun)
		if cacheErr == nil && len(members) > 0 {
			for produced < profileCycleMaxProducePerRun && processed < profileCycleMaxProducePerRun {
				rows, err := s.profileCycleDueRowsShard(ctx, profileCycleScanBatchSize, -1)
				if err != nil {
					return err
				}
				if len(rows) == 0 {
					break
				}
				processRows(rows)
				if len(rows) < profileCycleScanBatchSize {
					break
				}
			}
			for channelId := range affectedChannels {
				if err := s.refreshChannelProfileCycleNextAt(ctx, channelId); err != nil {
					g.Log().Warningf(ctx, "刷新频道循环时间失败 channel:%d err:%+v", channelId, err)
				}
			}
			g.Log().Infof(ctx, "资料循环Redis到期扫描完成 produced:%d channels:%d", produced, len(affectedChannels))
			return nil
		}
	}

	for shard := 0; shard < profileCycleShardCount; shard++ {
		lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		lock := hglock.NewConfig(10*time.Minute, 100*time.Millisecond).Mutex(fmt.Sprintf("youban_publish:profile_cycle_due_scan:%02d", shard))
		if err := lock.TryLock(lockCtx); err != nil {
			cancel()
			if gerror.Is(err, hglock.ErrLockFailed) {
				continue
			}
			return gerror.Wrapf(err, "获取资料循环分片扫描锁失败 shard:%d", shard)
		}
		cancel()
		produced, processed = 0, 0
		for produced < profileCycleMaxProducePerRun && processed < profileCycleMaxProducePerRun {
			rows, err := s.profileCycleDueRowsShard(ctx, profileCycleScanBatchSize, shard)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				break
			}
			processRows(rows)
		}
		s.releaseTelegramChannelLease(context.Background(), lock)
	}
	for channelId := range affectedChannels {
		if err := s.refreshChannelProfileCycleNextAt(ctx, channelId); err != nil {
			g.Log().Warningf(ctx, "刷新频道循环时间失败 channel:%d err:%+v", channelId, err)
		}
	}
	g.Log().Infof(ctx, "资料循环到期扫描完成 produced:%d channels:%d", produced, len(affectedChannels))
	return nil
}

func (s *sSysPublish) enqueuePendingProfileCycleReschedules(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 200
	}
	var channelIds []int64
	err := g.DB().Model(publishChannelTable+" c").Safe().Ctx(ctx).
		Fields("c.id").
		Where("c.cycle_publish_enabled", 1).
		Where("c.status", 1).
		Where("c.publish_direction", "up").
		WhereNull("c.cycle_next_run_at").
		WhereNull("c.deleted_at").
		OrderAsc("c.id").
		Limit(limit).
		Scan(&channelIds)
	if err != nil {
		return gerror.Wrap(err, "读取待恢复频道循环重算任务失败")
	}
	for _, channelId := range channelIds {
		if err = s.claimChannelProfileCycleReschedule(ctx, channelId); err != nil {
			return gerror.Wrapf(err, "领取待恢复频道循环重算任务失败 channel:%d", channelId)
		}
		if err = s.enqueueCycleReschedule(ctx, channelId, 0); err != nil {
			s.releaseChannelProfileCycleReschedule(ctx, channelId)
			return gerror.Wrapf(err, "提交待恢复频道循环重算任务失败 channel:%d", channelId)
		}
	}
	return nil
}

// enqueueDueProfileCycleReschedules recovers time-mode channels whose summary
// checkpoint is already due but whose profile-level plans are missing or stale.
// It deliberately only enqueues the existing reschedule task; the worker keeps
// ownership of the channel lock and cursor-based rebuild.
func (s *sSysPublish) enqueueDueProfileCycleReschedules(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	var channelIds []int64
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).As("c").
		Fields("c.id").
		Where("c.cycle_publish_enabled", 1).
		Where("c.cycle_publish_mode", "time").
		Where("c.status", 1).
		Where("c.publish_direction", "up").
		Where("c.cycle_active_run_id", 0).
		Where("c.cycle_next_run_at IS NOT NULL AND c.cycle_next_run_at<=?", gtime.Now()).
		WhereNull("c.deleted_at").
		Where(profileCycleRescheduleCandidateCondition("c")).
		OrderAsc("c.cycle_next_run_at").OrderAsc("c.id").Limit(limit).Scan(&channelIds)
	if err != nil {
		return gerror.Wrap(err, "读取到期时间循环恢复频道失败")
	}
	for _, channelId := range channelIds {
		if err = s.claimChannelProfileCycleReschedule(ctx, channelId); err != nil {
			return gerror.Wrapf(err, "领取到期时间循环恢复任务失败 channel:%d", channelId)
		}
		if err = s.enqueueCycleReschedule(ctx, channelId, 0); err != nil {
			s.releaseChannelProfileCycleReschedule(ctx, channelId)
			return gerror.Wrapf(err, "提交到期时间循环恢复任务失败 channel:%d", channelId)
		}
	}
	return nil
}

// profileCycleRescheduleCandidateCondition keeps healthy channels with normal
// due work out of the expensive full rebuild queue. The regular due scan owns
// that work; only missing or invalid profile plans require a full rebuild.
func profileCycleRescheduleCandidateCondition(channelAlias string) string {
	channelAlias = strings.TrimSpace(channelAlias)
	if channelAlias == "" {
		channelAlias = "c"
	}
	return `EXISTS (
		SELECT 1 FROM ` + publishChannelProfileTable + ` cp
		LEFT JOIN ` + publishTgJobTable + ` j ON j.id=cp.last_job_id
		WHERE cp.channel_id=` + channelAlias + `.id AND cp.status='active'
			AND (j.id IS NULL OR j.cycle_enabled<>1 OR j.next_cycle_at IS NULL)
	)`
}

// claimChannelProfileCycleReschedule moves the channel checkpoint forward after
// enqueueing. Without this lease every scheduler tick selects the same oldest
// channels, starving later channels when a rebuild is slow or retrying.
func (s *sSysPublish) claimChannelProfileCycleReschedule(ctx context.Context, channelId int64) error {
	if channelId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("id", channelId).
		Where("cycle_publish_mode", "time").
		Data(g.Map{"cycle_next_run_at": gtime.Now().Add(profileCycleRescheduleClaimLease)}).
		Update()
	return err
}

func (s *sSysPublish) releaseChannelProfileCycleReschedule(ctx context.Context, channelId int64) {
	if channelId <= 0 {
		return
	}
	_, _ = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("id", channelId).
		Where("cycle_publish_mode", "time").
		Data(g.Map{"cycle_next_run_at": gtime.Now()}).
		Update()
}

func (s *sSysPublish) profileCycleDueRows(ctx context.Context, limit int) ([]profileCycleDueRow, error) {
	return s.profileCycleDueRowsShard(ctx, limit, -1)
}

// profileCycleDueRowsShard 新引擎限定账户查询到期计划，旧引擎保留迁移期分片兼容。
func (s *sSysPublish) profileCycleDueRowsShard(ctx context.Context, limit, shard int, scopes ...publishAccountScope) ([]profileCycleDueRow, error) {
	if limit <= 0 {
		limit = profileCycleScanBatchSize
	}
	if len(scopes) > 0 {
		return s.accountProfileCycleDueRows(ctx, limit, scopes[0])
	}
	var rows []profileCycleDueRow
	model := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		InnerJoin(publishChannelProfileTable+" cp", "cp.last_job_id=j.id AND cp.status='active'").
		InnerJoin("hg_content_profile p", "p.id=j.profile_id AND p.deleted_at IS NULL").
		LeftJoin(publishChannelTable+" c", "c.id=j.channel_id AND c.deleted_at IS NULL").
		Fields("j.id AS job_id,j.tenant_id,j.account_id,j.profile_id,j.channel_id,j.sent_at,j.cycle_days,j.cycle_publish_time,j.next_cycle_at,j.dispatch_count,j.cycle_failure_count,COALESCE(cp.applied_cycle_config_version,0) AS applied_cycle_config_version,COALESCE(c.cycle_config_version,0) AS channel_cycle_config_version,"+
			"COALESCE(c.cycle_publish_enabled,0) AS channel_enabled,COALESCE(c.cycle_publish_days,0) AS channel_days,"+
			"COALESCE(c.cycle_publish_time,'') AS channel_publish_time,COALESCE(c.cycle_publish_mode,'time') AS channel_mode,COALESCE(c.status,0) AS channel_status,COALESCE(c.publish_direction,'') AS publish_direction").
		Where("j.cycle_enabled", 1).WhereIn("j.status", []string{"sent", "superseded"}).Where("(j.next_cycle_at IS NOT NULL AND j.next_cycle_at<=? OR COALESCE(cp.applied_cycle_config_version,0)<COALESCE(c.cycle_config_version,0))", gtime.Now()).
		Where("NOT EXISTS (SELECT 1 FROM "+channelPermissionCircuitTable+" pc WHERE pc.tenant_id=j.tenant_id AND pc.channel_id=j.channel_id AND pc.paused=1)").
		// 配置版本变化即使 next_cycle_at 为空也必须进入调度；普通到期任务仍要求到期。
		Where("(j.next_cycle_at IS NULL OR j.next_cycle_at<=?)", gtime.Now())
	model = accountScheduleModel(model, "j.tenant_id", "j.account_id", scopes)
	// 所有调度引擎统一检查子任务，防止到期扫描在发送未完成时重复创建循环任务。
	// 旧实现只在账户新引擎分支检查，导致普通扫描持续生成同一资料的 pending 任务并堵塞频道队列。
	model = model.Where("NOT EXISTS (SELECT 1 FROM " + publishTgJobTable + " active WHERE active.tenant_id=j.tenant_id AND active.account_id=j.account_id AND active.profile_id=j.profile_id AND active.channel_id=j.channel_id AND active.status IN ('pending','sending','failed_retry','unknown'))")

	if shard < 0 && cache.Initialized() {
		members, cacheErr := cache.ZSetRangeByScore(ctx, profileCycleDueZSetKey, 0, float64(time.Now().Unix()), limit)
		if cacheErr == nil && len(members) > 0 {
			jobIDs := make([]int64, 0, len(members))
			for _, member := range members {
				if jobID, parseErr := strconv.ParseInt(member, 10, 64); parseErr == nil && jobID > 0 {
					jobIDs = append(jobIDs, jobID)
				}
			}
			if len(jobIDs) > 0 {
				model = model.WhereIn("j.id", jobIDs)
			}
		}
	}
	// 循环翻新只处理当前仍可发布的资料；下架或非公开资料不再生成新的循环 Job。
	model = model.Where("p.status", 1).Where("p.visibility", consts.ContentVisibilityPublic)
	if shard >= 0 {
		model = model.Where("MOD(ABS(HASHTEXT(j.tenant_id::text || ':' || j.account_id::text)), ?) = ?", profileCycleShardCount, shard)
	}
	orderField := "j.next_cycle_at"
	if len(scopes) > 0 {
		orderField = "cp.next_cycle_at"
	}
	err := model.OrderAsc(orderField).OrderAsc("j.id").Limit(limit).Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取到期资料循环计划失败")
	}
	for _, row := range rows {
		if row.NextCycleAt != nil {
			indexProfileCycleDue(ctx, row.JobId, row.NextCycleAt)
			indexAccountProfileCycleDue(ctx, publishAccountScope{TenantID: row.TenantId, AccountID: row.AccountId}, row.JobId, row.NextCycleAt)
		}
	}
	return rows, nil
}

func (s *sSysPublish) dispatchDueProfileCycle(ctx context.Context, row profileCycleDueRow, channelBacklog map[int64]int) (bool, error) {
	config := profileCycleChannelConfig{
		Id: row.ChannelId, TenantId: row.TenantId, Enabled: row.ChannelEnabled, Days: row.ChannelDays,
		Mode: row.ChannelMode, PublishTime: row.ChannelPublishTime, Status: row.ChannelStatus, Direction: row.PublishDirection,
	}
	if !profileCycleChannelUsable(config) {
		return false, s.disableProfileCycleJob(ctx, row.JobId, "频道循环配置已关闭")
	}
	// A due job keeps the schedule captured when it was created. Changes to
	// tenant VIP status or channel interval apply to the next job after this
	// one succeeds; they must not postpone an already overdue profile.
	backlog, ok := channelBacklog[row.ChannelId]
	if !ok {
		var err error
		backlog, err = s.channelCycleBacklog(ctx, row.ChannelId)
		if err != nil {
			return false, err
		}
		channelBacklog[row.ChannelId] = backlog
	}
	if !profileCycleChannelHasCapacity(ctx, backlog) {
		nextAt := gtime.Now().Add(profileCycleRetryDelay)
		_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobId).Data(g.Map{
			"next_cycle_at":       nextAt,
			"last_dispatch_error": "频道循环队列积压，已延后领取", "updated_at": gtime.Now(),
		}).Update()
		indexProfileCycleDue(ctx, row.JobId, nextAt)
		indexAccountProfileCycleDue(ctx, publishAccountScope{TenantID: row.TenantId, AccountID: row.AccountId}, row.JobId, nextAt)
		return false, err
	}
	active, err := s.profileCycleChildActive(ctx, row.JobId)
	if err != nil {
		return false, err
	}
	if active {
		nextAt := gtime.Now().Add(profileCycleClaimLease)
		_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobId).Data(g.Map{
			"next_cycle_at": nextAt, "updated_at": gtime.Now(),
		}).Update()
		indexProfileCycleDue(ctx, row.JobId, nextAt)
		indexAccountProfileCycleDue(ctx, publishAccountScope{TenantID: row.TenantId, AccountID: row.AccountId}, row.JobId, nextAt)
		return false, err
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", row.JobId).Where("cycle_enabled", 1).WhereLTE("next_cycle_at", gtime.Now()).
		Data(g.Map{
			"next_cycle_at":       gtime.Now().Add(profileCycleClaimLease),
			"dispatched_at":       gtime.Now(),
			"dispatch_count":      gdb.Raw("dispatch_count + 1"),
			"last_dispatch_error": "",
			"updated_at":          gtime.Now(),
		}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "领取资料循环计划失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	removeProfileCycleDue(ctx, row.JobId)
	operationNo := dueProfileCycleOperationNo(row.JobId, row.DispatchCount+1, row.ProfileId, row.ChannelId)
	err = s.submitProfilePublish(ctx, row.ProfileId, row.TenantId, row.AccountId, 0, operationNo, []int64{row.ChannelId}, true)
	if errors.Is(err, errProfileCycleMediaPending) {
		// No send job was created. Media waiting is not a Telegram failure and
		// must not exhaust the source plan's consecutive send failure budget.
		nextAt := gtime.Now().Add(5 * time.Minute)
		_, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobId).
			Data(g.Map{"next_cycle_at": nextAt, "last_dispatch_error": err.Error()}).Update()
		if updateErr != nil {
			return false, gerror.Wrap(updateErr, "延后资料循环媒体等待失败")
		}
		indexProfileCycleDue(ctx, row.JobId, nextAt)
		indexAccountProfileCycleDue(ctx, publishAccountScope{TenantID: row.TenantId, AccountID: row.AccountId}, row.JobId, nextAt)
		return false, nil
	}
	if errors.Is(err, errTelegramPublishPermissionPaused) {
		nextAt := gtime.Now().Add(time.Hour)
		_, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobId).
			Data(g.Map{"next_cycle_at": nextAt, "last_dispatch_error": err.Error()}).Update()
		indexProfileCycleDue(ctx, row.JobId, nextAt)
		indexAccountProfileCycleDue(ctx, publishAccountScope{TenantID: row.TenantId, AccountID: row.AccountId}, row.JobId, nextAt)
		return false, updateErr
	}
	if err == nil {
		_, resetErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobId).
			Data(g.Map{"cycle_failure_count": 0, "last_dispatch_error": "", "updated_at": gtime.Now()}).Update()
		if resetErr != nil {
			g.Log().Warningf(ctx, "重置资料循环失败次数失败 job:%d err:%+v", row.JobId, resetErr)
		}
		return true, nil
	}
	if errors.Is(err, errPublishProfileUnavailable) {
		_ = s.deactivateChannelProfile(ctx, row.ChannelId, row.ProfileId)
		return false, s.disableProfileCycleJob(ctx, row.JobId, "资料已下架或不可发布")
	}
	failureCount := row.CycleFailureCount + 1
	message := cycleFailureSummary(err)
	data := g.Map{"next_cycle_at": gtime.Now().Add(profileCycleRetryDelay), "cycle_failure_count": failureCount,
		"last_dispatch_error": message, "updated_at": gtime.Now()}
	if isPermanentProfileCycleError(err) || failureCount >= profileCycleFailureThreshold {
		data["cycle_enabled"] = 0
		data["next_cycle_at"] = nil
		data["last_dispatch_error"] = message + "；连续失败达到阈值，已暂停循环"
	}
	_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobId).Data(data).Update()
	if nextAt, ok := data["next_cycle_at"].(*gtime.Time); ok && nextAt != nil {
		indexProfileCycleDue(ctx, row.JobId, nextAt)
		indexAccountProfileCycleDue(ctx, publishAccountScope{TenantID: row.TenantId, AccountID: row.AccountId}, row.JobId, nextAt)
	} else {
		removeProfileCycleDue(ctx, row.JobId)
	}
	return false, err
}

func profileCycleChannelHasCapacity(ctx context.Context, backlog int) bool {
	limit := maxConfigInt(ctx, "youbanPublish.cycle.channelBacklogLimit", profileCycleChannelBacklogDefault)
	return backlog < limit
}

func cycleFailureSummary(err error) string {
	if err == nil {
		return "循环发布失败"
	}
	message := strings.TrimSpace(err.Error())
	if len([]rune(message)) > 480 {
		message = string([]rune(message)[:477]) + "..."
	}
	return message
}

func isPermanentProfileCycleError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"wrong file identifier", "file_id_invalid", "file reference expired", "http 404"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (s *sSysPublish) disableProfileCycleJob(ctx context.Context, jobId int64, message string) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Data(g.Map{
		"cycle_enabled": 0, "next_cycle_at": nil, "last_dispatch_error": strings.TrimSpace(message), "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) profileCycleChildActive(ctx context.Context, sourceJobId int64) (bool, error) {
	prefix := fmt.Sprintf("cycle_batch:due:%d:", sourceJobId)
	count, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereLike("operation_no", prefix+"%").WhereIn("status", telegramJobActiveStatuses()).Count()
	return count > 0, err
}

func dueProfileCycleOperationNo(sourceJobId int64, attempt int, profileId int64, channelId int64) string {
	return fmt.Sprintf("cycle_batch:due:%d:%d:%d:%d", sourceJobId, attempt, profileId, channelId)
}

func (s *sSysPublish) runProfileCycleStartupRecovery(ctx context.Context) {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	if globalStreamPushEngineEnabled() {
		if err := s.enqueueAccountSchedules(ctx); err != nil {
			g.Log().Warningf(ctx, "启动账户循环调度恢复失败 err:%+v", err)
		}
		return
	}
	// 灰度阶段只恢复旧账户；新账户沿用持久化的到期计划，不在启动时全量改写。
	var channelIds []int64
	model := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Fields("id").Where("cycle_publish_enabled", 1).Where("status", 1).Where("publish_direction", "up")
	model = accountScheduleModel(model, publishChannelTable+".tenant_id", publishChannelTable+".tg_account_id", nil)
	if err := model.Scan(&channelIds); err != nil {
		g.Log().Warningf(ctx, "读取启动循环恢复频道失败：%+v", err)
	} else {
		for _, channelId := range channelIds {
			if err := s.enqueueCycleReschedule(ctx, channelId, 0); err != nil {
				g.Log().Warningf(ctx, "提交启动循环重算失败 channel:%d err:%+v", channelId, err)
			}
		}
	}
	if err := s.runProfileCycleDueScan(ctx); err != nil && ctx.Err() == nil {
		g.Log().Warningf(ctx, "启动资料循环补扫失败：%+v", err)
	}
	// 启动时主动补偿一次资料级 run，避免 Cron 首次加载或刷新延迟造成已到期任务长期 pending。
	if err := s.runProfileCycleOrchestrator(ctx); err != nil && ctx.Err() == nil {
		g.Log().Warningf(ctx, "启动资料级循环调度补偿失败：%+v", err)
	}
}
