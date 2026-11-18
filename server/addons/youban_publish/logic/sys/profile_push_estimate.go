package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/utility/dbtime"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const profilePushEstimateTimeout = 3 * time.Second

type profilePushChannelSchedule struct {
	sysin.ProfilePushChannelModel
	ActiveRunID int64 `orm:"cycle_active_run_id"`
}

const profilePushQueueSnapshotLimit = 10000
const profilePushBatchItemTable = "hg_youban_publish_cycle_batch_item"

type profilePushQueueSnapshot struct {
	Items       []profilePushQueueItem `json:"items"`
	Active      bool                   `json:"active"`
	ScheduledAt *gtime.Time            `json:"scheduledAt"`
}

// MATERIALIZED bounds work before primary-key eligibility checks. In particular,
// the planner cannot turn the prefix into a scan of all content profiles.
func profilePushQueueSnapshotSQL() string {
	// OFFSET 0 prevents PostgreSQL converting the correlated primary-key
	// eligibility probe into a hashed subplan over all content profiles.
	eligibility := strings.Replace(batchRotationEligibilitySQL(), "p.deleted_at IS NULL)", "p.deleted_at IS NULL OFFSET 0)", 1)
	return `WITH page AS MATERIALIZED (
		SELECT cp.profile_id,cp.account_id,cp.batch_order,cp.tenant_id,cp.channel_id
		FROM ` + publishChannelProfileTable + ` cp
		WHERE cp.tenant_id=? AND cp.channel_id=? AND cp.status='active' AND cp.batch_order>0 AND cp.cycle_override_enabled=false
		ORDER BY cp.batch_order,cp.profile_id LIMIT ?
	), selected AS MATERIALIZED (
		SELECT profile_id,account_id,batch_order,ordinal,job_id,item_state,tenant_id,channel_id
		FROM ` + profilePushBatchItemTable + `
		WHERE run_id=? AND tenant_id=? AND channel_id=? ORDER BY ordinal LIMIT ?
	)
	SELECT cp.profile_id,cp.account_id,cp.batch_order,
		CASE WHEN (si.job_id=0 AND si.item_state IN ('selected','admitted'))
			OR (si.job_id>0 AND (j.id IS NULL OR j.status NOT IN (?,?,?)))
			THEN si.ordinal ELSE 0 END AS ordinal,
		(` + eligibility + `) AS eligible
	FROM page cp LEFT JOIN selected si ON si.profile_id=cp.profile_id
		AND si.account_id=cp.account_id
	LEFT JOIN ` + publishTgJobTable + ` j ON j.id=si.job_id
		AND j.tenant_id=si.tenant_id AND j.account_id=si.account_id
		AND j.channel_id=si.channel_id AND j.profile_id=si.profile_id
	ORDER BY cp.batch_order,cp.profile_id`
}

// No DDL or legacy fallback is permitted on a read request. Require a valid
// channel queue index as well as the new columns and persisted snapshot table.
func profilePushBatchSchemaSQL() string {
	return `SELECT
		EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid=to_regclass(?)
		 AND i.indexrelid=to_regclass('idx_ybp_batch_inherited_rotation')
		 AND lower(regexp_replace(pg_get_expr(i.indpred,i.indrelid), '::(text|character varying)|[()[:space:]]', '', 'g'))='status=''active''andbatch_order>0andcycle_override_enabled=false'
		 AND i.indisvalid AND i.indisready AND i.indnkeyatts=5
		 AND ARRAY(SELECT a.attname::text FROM unnest(i.indkey::smallint[]) WITH ORDINALITY k(attnum,n)
		 JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.attnum
		 WHERE k.n<=5 ORDER BY k.n)=ARRAY['tenant_id','channel_id','status','batch_order','profile_id']) AS queue_index,
		(SELECT count(*) FROM pg_attribute WHERE attrelid=to_regclass(?)
		 AND attname IN ('run_id','ordinal','tenant_id','account_id','channel_id','profile_id','batch_order','job_id','item_state')
		 AND NOT attisdropped)=9 AS snapshot_columns,
		EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid=to_regclass(?)
		 AND i.indisvalid AND i.indisready AND i.indpred IS NULL AND i.indnkeyatts>=2
		 AND ARRAY(SELECT a.attname::text FROM unnest(i.indkey::smallint[]) WITH ORDINALITY k(attnum,n)
		 JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.attnum
		 WHERE k.n<=2 ORDER BY k.n)=ARRAY['run_id','ordinal']) AS snapshot_index,
		EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid=to_regclass(?)
		 AND i.indexrelid=to_regclass('idx_ybp_batch_unmigrated')
		 AND i.indisvalid AND i.indisready AND i.indnkeyatts=2
		 AND lower(regexp_replace(pg_get_expr(i.indpred,i.indrelid), '::(text|character varying)|[()[:space:]]', '', 'g'))='status=''active''andbatch_order<=0'
		 AND ARRAY(SELECT a.attname::text FROM unnest(i.indkey::smallint[]) WITH ORDINALITY k(attnum,n)
		 JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.attnum
		 WHERE k.n<=2 ORDER BY k.n)=ARRAY['tenant_id','channel_id']) AS unmigrated_index`
}

func profilePushBatchSchemaReady(ctx context.Context) error {
	row, err := g.DB().GetOne(ctx, profilePushBatchSchemaSQL(),
		publishChannelProfileTable, profilePushBatchItemTable, profilePushBatchItemTable, publishChannelProfileTable)
	if err != nil {
		return gerror.Wrap(err, "连续轮播预估结构检查失败")
	}
	if !row["queue_index"].Bool() || !row["snapshot_columns"].Bool() || !row["snapshot_index"].Bool() || !row["unmigrated_index"].Bool() {
		return gerror.New("连续轮播预估等待迁移：需要 batch_order 频道索引、idx_ybp_batch_unmigrated 零序号索引及每日批次快照表和索引")
	}
	return nil
}

func normalizedProfilePushCycleMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "time"
	}
	return mode
}

func profilePushNextBatchAt(planned *gtime.Time, now *gtime.Time) *gtime.Time {
	planned = dbtime.FromTimestamp(planned)
	if planned != nil && !planned.Before(now) {
		return planned
	}
	// The scheduler treats both NULL and overdue plans as immediately due.
	// Later batches still follow the configured daily clock.
	return now
}

func (s *sSysPublish) estimateProfileBatchPushAt(ctx context.Context, profile *sysin.ProfileModel, channel *profilePushChannelSchedule, now *gtime.Time) (*gtime.Time, error) {
	if channel.CycleBatchSize <= 0 {
		return nil, nil
	}
	key := fmt.Sprintf("%srotation:v1:%d:%d:%d", consts.ProfilePushEstimateCacheKeyPrefix,
		profile.TenantId, channel.ChannelId, channel.ActiveRunID)
	value, err := cache.Instance().GetOrSetFuncLock(ctx, key, func(ctx context.Context) (interface{}, error) {
		if err := profilePushBatchSchemaReady(ctx); err != nil {
			return nil, err
		}
		var snapshot profilePushQueueSnapshot
		var run struct {
			ID          int64       `orm:"id"`
			TotalCount  int64       `orm:"total_count"`
			ScheduledAt *gtime.Time `orm:"scheduled_at"`
		}
		if channel.ActiveRunID > 0 {
			if err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).
				Fields("id,total_count,scheduled_at").Where("id", channel.ActiveRunID).
				Where("tenant_id", profile.TenantId).Where("channel_id", channel.ChannelId).
				WhereIn("status", []string{cycleRunStatusPending, cycleRunStatusRunning, cycleRunStatusDispatching}).
				Scan(&run); !telegramOptionalAttemptScanSucceeded(err) {
				return nil, err
			}
		}
		snapshot.Active = run.ID > 0
		snapshot.ScheduledAt = dbtime.FromTimestamp(run.ScheduledAt)
		if snapshot.Active {
			count, err := g.DB().GetValue(ctx, `SELECT count(*) FROM (
				SELECT 1 FROM `+profilePushBatchItemTable+`
				WHERE run_id=? AND tenant_id=? AND channel_id=? ORDER BY ordinal LIMIT ?
			) selected`, run.ID, profile.TenantId, channel.ChannelId, profilePushQueueSnapshotLimit+1)
			if err != nil {
				return nil, err
			}
			if count.Int64() != run.TotalCount || count.Int64() == 0 {
				return nil, gerror.New("连续轮播预估等待迁移：活动批次缺少完整的持久化成员快照")
			}
			if count.Int64() > profilePushQueueSnapshotLimit {
				return nil, gerror.New("当前批次超过有界预估上限，暂无法精确预估")
			}
		}
		unmigrated, err := g.DB().GetValue(ctx, `SELECT EXISTS (
			SELECT 1 FROM `+publishChannelProfileTable+`
			WHERE tenant_id=? AND channel_id=? AND status='active' AND batch_order<=0 AND cycle_override_enabled=false LIMIT 1)`,
			profile.TenantId, channel.ChannelId)
		if err != nil {
			return nil, err
		}
		if unmigrated.Bool() {
			return nil, gerror.New("连续轮播预估等待迁移：频道仍有未分配 batch_order 的成员")
		}
		rows, err := g.DB().GetAll(ctx, profilePushQueueSnapshotSQL(), profile.TenantId, channel.ChannelId,
			profilePushQueueSnapshotLimit+1, run.ID, profile.TenantId, channel.ChannelId, profilePushQueueSnapshotLimit+1,
			JobStatusSent, JobStatusFailed, JobStatusSuperseded)
		if err != nil {
			return nil, gerror.Wrap(err, "读取连续轮播频道快照失败")
		}
		if len(rows) > profilePushQueueSnapshotLimit {
			return nil, gerror.New("频道队列超过有界预估上限，暂无法精确预估")
		}
		if err := rows.Structs(&snapshot.Items); err != nil {
			return nil, err
		}
		return snapshot, nil
	}, 15*time.Second)
	if err != nil {
		return nil, err
	}
	var snapshot profilePushQueueSnapshot
	if err := value.Scan(&snapshot); err != nil {
		return nil, err
	}
	rank := profilePushQueueRank(snapshot.Items, profile.Id, profile.AccountId)
	var active int64
	if snapshot.Active {
		active = 1
	}
	offset, ok := profilePushBatchDayOffset(channel.CycleBatchSize, rank, active)
	if !ok {
		return nil, nil
	}
	position := profilePushBatchPosition(rank, active)
	channel.BatchAheadCount = &position.AheadCount
	channel.BatchInCurrentRun = position.InCurrentRun
	channel.BatchWaitingNextRound = false
	channel.BatchRoundRemaining = nil
	return profilePushBatchEstimateAt(channel.CycleBatchTime, channel.NextBatchAt, now, snapshot.ScheduledAt, snapshot.Active, offset), nil
}

func profilePushBatchEstimateAt(clock string, planned, now, activeAt *gtime.Time, active bool, offset int64) *gtime.Time {
	if offset < 0 {
		if activeAt != nil && activeAt.After(now) {
			return activeAt
		}
		return now
	}
	next := profilePushNextBatchAt(planned, now)
	if active {
		// A nonmember cannot enter today's immutable run, even at rank zero.
		base := now
		if activeAt != nil && activeAt.After(base) {
			base = activeAt
		}
		tomorrow := profilePushBatchSendingDay(clock, base, 1)
		if next.Before(tomorrow) {
			next = tomorrow
		}
	}
	return profilePushBatchSendingDay(clock, next, offset)
}

// Calendar arithmetic always uses Beijing time, including a stale/UTC DB plan.
func profilePushBatchSendingDay(clock string, first *gtime.Time, offset int64) *gtime.Time {
	base := first.Time.In(dbtime.Location())
	if offset <= 0 {
		return gtime.New(base)
	}
	hour, minute, ok := parseCycleClock(clock)
	if !ok {
		hour, minute = 0, 0
	}
	return gtime.New(time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, dbtime.Location()).AddDate(0, 0, int(offset)))
}
