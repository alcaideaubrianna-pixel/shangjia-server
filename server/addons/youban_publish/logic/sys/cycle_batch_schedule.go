package sys

import (
	"context"
	"errors"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/utility/dbtime"
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
			// 权限未确认时只跳过本轮，不持久化改写频道模式。此前将 batch
			// 永久改成 time 会留下脏配置，且 VIP 恢复后不会自动恢复批次循环。
			nextRunAt := nextBatchCycleAt(channel.BatchTime, now.Time)
			if _, updateErr := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).Where("tenant_id", channel.TenantId).Data(g.Map{
				"cycle_active_run_id": 0, "cycle_next_run_at": nextRunAt, "updated_at": now,
			}).Update(); updateErr != nil {
				g.Log().Warningf(ctx, "跳过无批次权限频道失败 channel:%d tenant:%d err:%+v", channel.Id, channel.TenantId, updateErr)
			} else {
				g.Log().Warningf(ctx, "批次循环权限未确认，本轮跳过且保留batch配置 channel:%d tenant:%d next:%s", channel.Id, channel.TenantId, nextRunAt.String())
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
		locked, err := tx.Model(publishChannelTable).Ctx(ctx).Where("id", channel.Id).Where("tenant_id", channel.TenantId).
			Where("cycle_active_run_id", 0).Where("cycle_publish_mode", "batch").
			Where("cycle_publish_enabled", 1).Where("status", 1).Where("publish_direction", "up").
			Where("cycle_next_run_at IS NULL OR cycle_next_run_at<=?", now).
			Data(g.Map{"cycle_active_run_id": -1}).Update()
		if err != nil {
			return err
		}
		affected, err := locked.RowsAffected()
		if err != nil || affected == 0 {
			return err
		}
		if err = tx.Model(publishChannelTable).Where("id", channel.Id).Scan(&channel); err != nil {
			return err
		}
		// Recover ownership before starting a new day, including legacy runs. Their
		// producers are retired by ExecuteCycleRun without selecting a fresh batch.
		active, err := tx.Model(publishCycleRunTable).Ctx(ctx).Where("tenant_id", channel.TenantId).Where("channel_id", channel.Id).
			WhereIn("status", []string{cycleRunStatusPending, cycleRunStatusRunning, cycleRunStatusDispatching}).OrderAsc("id").Value("id")
		if err != nil {
			return err
		}
		if active.Int64() > 0 {
			runID = active.Int64()
			_, err = tx.Model(publishChannelTable).Where("id", channel.Id).Data(g.Map{"cycle_active_run_id": runID}).Update()
			return err
		}
		start, end := batchCycleLocalDay(now.Time)
		existing, err := tx.Model(publishCycleRunTable).Ctx(ctx).Where("tenant_id", channel.TenantId).Where("channel_id", channel.Id).
			WhereGTE("scheduled_at", start).WhereLT("scheduled_at", end).OrderDesc("id").Value("id")
		if err != nil {
			return err
		}
		if existing.Int64() > 0 || channel.BatchSize <= 0 {
			_, err = tx.Model(publishChannelTable).Where("id", channel.Id).Data(g.Map{"cycle_active_run_id": 0, "cycle_next_run_at": nextBatchCycleAt(channel.BatchTime, now.Time)}).Update()
			return err
		}
		candidates, err := batchRotationCandidatesTx(ctx, tx, channel.TenantId, channel.Id, channel.BatchSize)
		if errors.Is(err, errBatchRotationScanBudget) {
			// Commit the readmodel scan's bounded CAS progress without consuming
			// today's snapshot allowance. Retry this channel shortly.
			_, updateErr := tx.Model(publishChannelTable).Where("id", channel.Id).
				Where("cycle_active_run_id", -1).
				Data(g.Map{"cycle_active_run_id": 0, "cycle_next_run_at": now.Add(30 * time.Second)}).Update()
			return updateErr
		}
		if err != nil {
			return err
		}
		if err = reserveBatchRotationCandidatesTx(ctx, tx, channel.TenantId, channel.Id, candidates); err != nil {
			return err
		}
		items, err := newCycleBatchSnapshot(channel.TenantId, channel.Id, channel.BatchSize, candidates)
		if err != nil {
			return err
		}
		// A negative legacy upper bound is the durable snapshot protocol marker;
		// stage is free to progress through the existing producing/dispatching states.
		runID, err = tx.Model(publishCycleRunTable).Ctx(ctx).Data(g.Map{
			"tenant_id": channel.TenantId, "channel_id": channel.Id, "status": cycleRunStatusPending,
			"stage": "batch", "cursor_id": 0, "upper_profile_id": cycleBatchSnapshotVersion,
			"total_count": len(items), "queued_count": 0, "scheduled_at": now, "scheduled_day": start.Format("2006-01-02"),
		}).InsertAndGetId()
		if err != nil {
			return err
		}
		if err = persistCycleBatchSnapshotTx(ctx, tx, runID, items); err != nil {
			return err
		}
		_, err = tx.Model(publishChannelTable).Where("id", channel.Id).Data(g.Map{
			"cycle_active_run_id": runID, "cycle_batch_cursor": 0, "cycle_batch_cursor_kind": "batch_order",
			"cycle_round_end_id": 0, "cycle_next_run_at": nextBatchCycleAt(channel.BatchTime, now.Time),
		}).Update()
		return err
	})
	if err != nil || runID <= 0 {
		return err
	}
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
	base = base.In(dbtime.Location())
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
