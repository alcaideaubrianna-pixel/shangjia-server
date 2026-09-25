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
	Id          int64  `orm:"id"`
	TenantId    int64  `orm:"tenant_id"`
	BatchSize   int    `orm:"cycle_batch_size"`
	BatchTime   string `orm:"cycle_batch_time"`
	BatchCursor int64  `orm:"cycle_batch_cursor"`
}

func (s *sSysPublish) scheduleDueChannelBatchCycles(ctx context.Context, limit int) error {
	now := gtime.Now()
	var channels []dueBatchCycleChannel
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,cycle_batch_size,cycle_batch_time,cycle_batch_cursor").
		Where("cycle_publish_enabled", 1).Where("cycle_publish_mode", "batch").
		Where("status", 1).Where("publish_direction", "up").Where("cycle_active_run_id", 0).
		Where("cycle_next_run_at IS NULL OR cycle_next_run_at<=?", now).
		WhereNull("deleted_at").OrderAsc("id").Limit(limit).Scan(&channels)
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

func (s *sSysPublish) createScheduledBatchCycleRun(ctx context.Context, channel dueBatchCycleChannel, now *gtime.Time) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
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
		cursor := channel.BatchCursor
		var activeRunId int64
		activeRunId, err = tx.Model(publishCycleRunTable).Safe().Ctx(ctx).
			Where("channel_id", channel.Id).
			Where("cursor_id", cursor).
			WhereIn("status", []string{cycleRunStatusPending, cycleRunStatusRunning, cycleRunStatusDispatching}).
			OrderAsc("id").Value("id")
		if err != nil {
			return err
		}
		if activeRunId > 0 {
			_, _ = tx.Model(publishChannelTable).Safe().Ctx(ctx).
				Where("id", channel.Id).Where("cycle_active_run_id", -1).
				Data(g.Map{"cycle_active_run_id": activeRunId, "updated_at": now}).Update()
			return nil
		}
		count, err := s.batchCycleCandidateCount(ctx, channel.TenantId, channel.Id, cursor)
		if err != nil {
			return err
		}
		if count == 0 && cursor > 0 {
			cursor = 0
			count, err = s.batchCycleCandidateCount(ctx, channel.TenantId, channel.Id, 0)
		}
		if err != nil {
			return err
		}
		if count == 0 {
			_, err = tx.Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).Data(g.Map{
				"cycle_active_run_id": 0, "cycle_batch_cursor": cursor, "cycle_next_run_at": nextBatchCycleAt(channel.BatchTime, now.Time), "updated_at": now,
			}).Update()
			return err
		}
		total := channel.BatchSize
		if count < total {
			total = count
		}
		runId, err := tx.Model(publishCycleRunTable).Safe().Ctx(ctx).Data(g.Map{
			"tenant_id": channel.TenantId, "channel_id": channel.Id, "status": cycleRunStatusPending,
			"stage": "batch", "cursor_id": cursor, "total_count": total, "queued_count": 0,
			"scheduled_at": now, "created_at": now, "updated_at": now,
		}).InsertAndGetId()
		if err != nil {
			return err
		}
		_, err = tx.Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).Data(g.Map{
			"cycle_active_run_id": runId, "cycle_next_run_at": nextBatchCycleAt(channel.BatchTime, now.Time), "updated_at": now,
		}).Update()
		if err == nil {
			err = s.enqueueCycleRun(ctx, runId, 0)
		}
		return err
	})
}

func (s *sSysPublish) batchCycleCandidateCount(ctx context.Context, tenantId, channelId, cursor int64) (int, error) {
	sql := `(SELECT MIN(r.id) AS id
		FROM hg_youban_publish_success_record r
		WHERE r.tenant_id=? AND r.channel_id=? AND r.status='success'
			AND ` + cycleProfileMediaAvailableSQL("r.profile_id") + `
		GROUP BY r.profile_id) q`
	return g.DB().Model(sql, tenantId, channelId).Safe().Ctx(ctx).WhereGT("id", cursor).Count()
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
