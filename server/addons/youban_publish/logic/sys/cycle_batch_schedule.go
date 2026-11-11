package sys

import (
	"context"
	"hotgo/addons/youban_publish/model/input/sysin"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type dueBatchCycleChannel struct {
	Id              int64  `orm:"id"`
	TenantId        int64  `orm:"tenant_id"`
	BatchSize       int    `orm:"cycle_batch_size"`
	BatchTime       string `orm:"cycle_batch_time"`
	BatchCursor     int64  `orm:"cycle_batch_cursor"`
	BatchCursorKind string `orm:"cycle_batch_cursor_kind"`
	RoundEndID      int64  `orm:"cycle_round_end_id"`
}

func (s *sSysPublish) scheduleDueChannelBatchCycles(ctx context.Context, limit int) error {
	for shard := 0; shard < profileCycleShardCount; shard++ {
		if err := s.scheduleDueChannelBatchCyclesShard(ctx, limit, shard); err != nil {
			return err
		}
	}
	return nil
}

// scheduleDueChannelBatchCyclesShard 按账户扫描到期批次配置，复用会员权限和既有批次游标能力。
func (s *sSysPublish) scheduleDueChannelBatchCyclesShard(ctx context.Context, limit, shard int, scopes ...publishAccountScope) error {
	now := gtime.Now()
	var channels []dueBatchCycleChannel
	model := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,cycle_batch_size,cycle_batch_time,cycle_batch_cursor,cycle_batch_cursor_kind,cycle_round_end_id").
		Where("cycle_publish_enabled", 1).Where("cycle_publish_mode", "batch").
		Where("status", 1).Where("publish_direction", "up").Where("cycle_active_run_id", 0).
		Where("cycle_next_run_at IS NULL OR cycle_next_run_at<=?", now).
		WhereNull("deleted_at")
	model = accountScheduleModel(model, publishChannelTable+".tenant_id", publishChannelTable+".tg_account_id", scopes)
	if shard >= 0 {
		model = model.Where("MOD(ABS(HASHTEXT(tenant_id::text || ':' || id::text)), ?) = ?", profileCycleShardCount, shard)
	}
	err := model.OrderAsc("id").Limit(limit).Scan(&channels)
	if err != nil {
		return gerror.Wrap(err, "读取到期批次循环频道失败")
	}
	vipByTenant := make(map[int64]*sysin.TenantVipStatusModel)
	for _, channel := range channels {
		if channel.BatchSize <= 0 {
			continue
		}
		vip, loaded := vipByTenant[channel.TenantId]
		if !loaded {
			var vipErr error
			vip, vipErr = s.tenantVipStatus(ctx, channel.TenantId)
			if vipErr != nil {
				g.Log().Warningf(ctx, "校验批次循环会员状态失败 channel:%d tenant:%d err:%+v", channel.Id, channel.TenantId, vipErr)
				continue
			}
			vipByTenant[channel.TenantId] = vip
		}
		if !tenantVipStatusActive(vip) || !containsString(vip.Features, sysin.TenantVipFeatureBatchCycle) {
			if _, updateErr := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).Where("tenant_id", channel.TenantId).Data(g.Map{
				"cycle_publish_mode": "time", "cycle_batch_cursor": 0, "cycle_active_run_id": 0, "cycle_next_run_at": nil, "updated_at": now,
			}).Update(); updateErr != nil {
				g.Log().Warningf(ctx, "降级过期会员批次循环失败 channel:%d tenant:%d err:%+v", channel.Id, channel.TenantId, updateErr)
			} else if enqueueErr := s.enqueueCycleReschedule(ctx, channel.Id, 0); enqueueErr != nil {
				g.Log().Warningf(ctx, "提交过期会员循环重算失败 channel:%d tenant:%d err:%+v", channel.Id, channel.TenantId, enqueueErr)
			}
			continue
		}
		if err = s.createScheduledBatchCycleRun(ctx, channel, now); err != nil {
			g.Log().Warningf(ctx, "创建频道批次循环失败 channel:%d err:%+v", channel.Id, err)
		}
	}
	return nil
}

// createScheduledBatchCycleRun 在频道事务锁内领取有限批次，保存整轮上界并在提交后通知执行队列。
func (s *sSysPublish) createScheduledBatchCycleRun(ctx context.Context, channel dueBatchCycleChannel, now *gtime.Time) error {
	var runID int64
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		locked, err := tx.Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).
			Where("cycle_active_run_id", 0).Where("cycle_publish_mode", "batch").
			Where("cycle_next_run_at IS NULL OR cycle_next_run_at<=?", now).
			Data(g.Map{"cycle_active_run_id": -1, "updated_at": now}).Update()
		if err != nil {
			return err
		}
		affected, _ := locked.RowsAffected()
		if affected == 0 {
			return nil
		}
		// 在取得频道行锁后重新读取游标与轮次边界，不能沿用扫描时的旧快照。
		if err := tx.Model(publishChannelTable).Where("id", channel.Id).Scan(&channel); err != nil {
			return err
		}
		cursor := channel.BatchCursor
		if channel.BatchCursorKind != "profile" {
			cursor, err = migrateBatchCycleCursorTx(ctx, tx, channel.Id, cursor)
			if err != nil {
				return err
			}
		}
		activeRunValue, valueErr := tx.Model(publishCycleRunTable).Safe().Ctx(ctx).
			Where("channel_id", channel.Id).
			WhereIn("status", []string{cycleRunStatusPending, cycleRunStatusRunning, cycleRunStatusDispatching}).
			OrderAsc("id").Value("id")
		if valueErr != nil {
			return valueErr
		}
		activeRunId := activeRunValue.Int64()
		if activeRunId > 0 {
			result, updateErr := tx.Model(publishChannelTable).Safe().Ctx(ctx).
				Where("id", channel.Id).Where("cycle_active_run_id", -1).
				Data(g.Map{"cycle_active_run_id": activeRunId, "updated_at": now}).Update()
			if updateErr != nil {
				return updateErr
			}
			updated, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				return rowsErr
			}
			if updated == 0 {
				return gerror.New("恢复频道循环运行状态失败")
			}
			return nil
		}
		// 固定一轮上界；新增资料不能无限延长本轮，旧资料最终能重新轮到。
		upper := channel.RoundEndID
		if upper <= 0 || cursor >= upper {
			if upper > 0 {
				cursor = 0
			} // 已有profile游标首次迁移时保留进度。
			value, queryErr := s.batchCycleEligibleProfileModel(ctx, channel.TenantId, channel.Id).Fields("p.id").OrderDesc("p.id").Limit(1).Value()
			if queryErr != nil {
				return queryErr
			}
			upper = value.Int64()
			if cursor >= upper {
				cursor = 0
			}
		}
		// 只读取本批数量，避免每次创建批次都COUNT全部剩余资料。
		var candidates []struct {
			ID int64 `orm:"id"`
		}
		readCandidates := func() error {
			candidates = nil
			if upper <= 0 {
				return nil
			}
			return s.batchCycleEligibleProfileModel(ctx, channel.TenantId, channel.Id).WhereGT("p.id", cursor).WhereLTE("p.id", upper).Fields("p.id").OrderAsc("p.id").Limit(channel.BatchSize).Scan(&candidates)
		}
		if err := readCandidates(); err != nil {
			return err
		}
		if len(candidates) == 0 && cursor > 0 {
			// 尾部资料可能全部下架：本轮已结束，直接开启新轮，不能空等一个调度日。
			cursor = 0
			value, err := s.batchCycleEligibleProfileModel(ctx, channel.TenantId, channel.Id).Fields("p.id").OrderDesc("p.id").Limit(1).Value()
			if err != nil {
				return err
			}
			upper = value.Int64()
			if err := readCandidates(); err != nil {
				return err
			}
		}
		if len(candidates) == 0 {
			// 本轮剩余资料可能已下架；结束本轮，下一调度日重新确定上界。
			_, err = tx.Model(publishChannelTable).Where("id", channel.Id).Data(g.Map{
				"cycle_active_run_id": 0, "cycle_batch_cursor": 0, "cycle_round_end_id": 0,
				"cycle_next_run_at": nextBatchCycleAt(channel.BatchTime, now.Time),
			}).Update()
			return err
		}
		total := len(candidates)
		runId, err := tx.Model(publishCycleRunTable).Safe().Ctx(ctx).Data(g.Map{
			"tenant_id": channel.TenantId, "channel_id": channel.Id, "status": cycleRunStatusPending,
			"stage": "batch", "cursor_id": cursor, "upper_profile_id": upper, "total_count": total, "queued_count": 0,
			"scheduled_at": now, "created_at": now, "updated_at": now,
		}).InsertAndGetId()
		if err != nil {
			return err
		}
		runID = runId
		_, err = tx.Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).Data(g.Map{
			"cycle_active_run_id": runId, "cycle_round_end_id": upper, "cycle_next_run_at": nextBatchCycleAt(channel.BatchTime, now.Time), "updated_at": now,
		}).Update()
		return err
	})
	if err != nil || runID <= 0 {
		return err
	}
	// 事务提交后再入队，避免 Redis 成功而数据库回滚产生幽灵批次。
	if err = s.enqueueCycleRun(ctx, runID, 0); err != nil {
		g.Log().Warningf(ctx, "提交定时批次循环入队失败，等待恢复扫描 run:%d channel:%d err:%+v", runID, channel.Id, err)
	}
	return err
}

func migrateBatchCycleCursorTx(ctx context.Context, tx gdb.TX, channelId, legacyCursor int64) (int64, error) {
	cursor := int64(0)
	if legacyCursor > 0 {
		value, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", legacyCursor).Where("channel_id", channelId).Value("profile_id")
		if err != nil {
			return 0, gerror.Wrap(err, "转换频道批次循环游标失败")
		}
		cursor = value.Int64()
	}
	_, err := tx.Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channelId).Where("cycle_active_run_id", -1).Data(g.Map{
		"cycle_batch_cursor": cursor, "cycle_batch_cursor_kind": "profile", "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return 0, gerror.Wrap(err, "保存频道批次循环资料游标失败")
	}
	return cursor, nil
}

func (s *sSysPublish) batchCycleEligibleProfileModel(ctx context.Context, tenantId, channelId int64) *gdb.Model {
	published := "EXISTS (SELECT 1 FROM " + publishTgJobTable + " sj WHERE sj.tenant_id=ps.tenant_id AND sj.profile_id=p.id AND sj.status='sent')"
	return s.fullPushEligibleProfileModel(ctx, tenantId, channelId).
		Where(published).
		Where(cycleProfileMediaAvailableSQL("p.id"))
}

func cycleProfileMediaAvailableSQL(profileField string) string {
	return `NOT EXISTS (
		SELECT 1 FROM ` + publishMediaTable + ` m
		WHERE m.profile_id=` + profileField + ` AND m.status=1 AND m.deleted_at IS NULL
			AND m.processing_status='failed'
			AND COALESCE(m.storage_path,'')=''
			AND m.file_url LIKE 'https://api.telegram.org/file/bot%'
	)`
}

func nextBatchCycleAt(clock string, base time.Time) *gtime.Time {
	hour, minute, ok := parseCycleClock(clock)
	if !ok {
		hour, minute = 0, 0
	}
	next := time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, base.Location())
	if !next.After(base) {
		next = next.AddDate(0, 0, 1)
	}
	return gtime.New(next)
}
