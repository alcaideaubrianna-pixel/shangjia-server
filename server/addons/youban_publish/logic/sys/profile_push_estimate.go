package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/utility/dbtime"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const profilePushEstimateTimeout = 3 * time.Second

type profilePushChannelSchedule struct {
	sysin.ProfilePushChannelModel
	BatchCursor     int64  `orm:"cycle_batch_cursor"`
	BatchCursorKind string `orm:"cycle_batch_cursor_kind"`
	RoundEndID      int64  `orm:"cycle_round_end_id"`
	ActiveRunID     int64  `orm:"cycle_active_run_id"`
}

type profilePushBatchRank struct {
	BeforeTarget   int64 `json:"beforeTarget" orm:"before_target"`
	AheadInRound   int64 `json:"aheadInRound" orm:"ahead_in_round"`
	RoundLeft      int64 `json:"roundLeft" orm:"round_left"`
	Eligible       int64 `json:"eligible" orm:"eligible"`
	InCurrentRound int64 `json:"inCurrentRound" orm:"in_current_round"`
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

// profilePushBatchDayOffset follows the scheduler's fixed round upper bound.
// New profiles above that bound join only after the current round is exhausted.
func profilePushBatchDayOffset(batchSize int, rank profilePushBatchRank, activeBatchSize int64) (int64, bool) {
	if batchSize <= 0 || rank.Eligible == 0 {
		return 0, false
	}
	inCurrentRound := rank.InCurrentRound > 0
	size := int64(batchSize)
	if inCurrentRound {
		if activeBatchSize > 0 && rank.AheadInRound < activeBatchSize {
			return -1, true // already belongs to the running batch
		}
		ahead := rank.AheadInRound - activeBatchSize
		if ahead < 0 {
			ahead = 0
		}
		return ahead / size, true
	}
	left := rank.RoundLeft - activeBatchSize
	if left < 0 {
		left = 0
	}
	return (left+size-1)/size + rank.BeforeTarget/size, true
}

func (s *sSysPublish) estimateProfileBatchPushAt(ctx context.Context, profile *sysin.ProfileModel, channel *profilePushChannelSchedule, now *gtime.Time) (*gtime.Time, error) {
	if channel.CycleBatchSize <= 0 {
		return nil, nil
	}
	cursor, upper := channel.BatchCursor, channel.RoundEndID
	if cursor > 0 && channel.BatchCursorKind != "profile" {
		// Interpret a legacy Job cursor without running its mutating migration.
		value, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", cursor).Where("tenant_id", profile.TenantId).
			Where("channel_id", channel.ChannelId).Value("profile_id")
		if err != nil {
			return nil, err
		}
		cursor = value.Int64()
	}
	if upper > 0 && cursor >= upper {
		cursor, upper = 0, 0
	}
	key := fmt.Sprintf("%s%d:%d:%d:%d:%d:%d", consts.ProfilePushEstimateCacheKeyPrefix,
		profile.TenantId, profile.AccountId, profile.Id, channel.ChannelId, cursor, upper)
	value, err := cache.Instance().GetOrSetFuncLock(ctx, key, func(ctx context.Context) (interface{}, error) {
		// Reuse the actual scheduler's account/channel/publication/media filters.
		// This is one bounded tenant/channel query, not a global profile count.
		model := s.batchCycleEligibleProfileModel(ctx, profile.TenantId, channel.ChannelId)
		// Match channelCyclePage's (created_at,id) ordering. Imported profiles
		// need not have creation timestamps ordered by their numeric IDs.
		profileTuple := "(p.created_at,p.id)"
		tupleFor := func(id int64) string {
			return fmt.Sprintf("(SELECT anchor.created_at,anchor.id FROM %s anchor WHERE anchor.id=%d)", dao.ContentProfile.Table(), id)
		}
		targetTuple := tupleFor(profile.Id)
		roundCondition := "1=1"
		if cursor > 0 {
			roundCondition += " AND " + profileTuple + ">" + tupleFor(cursor)
		}
		if upper > 0 {
			roundCondition += " AND " + profileTuple + "<=" + tupleFor(upper)
			model = model.Where("(" + profileTuple + "<=" + tupleFor(upper) + " OR " + profileTuple + "<=" + targetTuple + ")")
		} else if cursor == 0 {
			// A fresh round needs only the target's prefix, never its full tail.
			model = model.Where(profileTuple + "<=" + targetTuple)
		}
		fields := fmt.Sprintf(
			"COUNT(DISTINCT CASE WHEN %s<%s THEN p.id END) AS before_target,"+
				"COUNT(DISTINCT CASE WHEN %s AND %s<%s THEN p.id END) AS ahead_in_round,"+
				"COUNT(DISTINCT CASE WHEN %s THEN p.id END) AS round_left,"+
				"COUNT(DISTINCT CASE WHEN p.id=%d THEN p.id END) AS eligible,"+
				"COUNT(DISTINCT CASE WHEN %s AND p.id=%d THEN p.id END) AS in_current_round",
			profileTuple, targetTuple, roundCondition, profileTuple, targetTuple, roundCondition,
			profile.Id, roundCondition, profile.Id)
		var rank profilePushBatchRank
		if err := model.Fields(fields).Scan(&rank); err != nil {
			return nil, err
		}
		return rank, nil
	}, time.Minute)
	if err != nil {
		return nil, err
	}
	var rank profilePushBatchRank
	if err := value.Scan(&rank); err != nil {
		return nil, err
	}
	var activeBatchSize int64
	var activeAt *gtime.Time
	if channel.ActiveRunID > 0 {
		var run struct {
			TotalCount  int         `orm:"total_count"`
			ScheduledAt *gtime.Time `orm:"scheduled_at"`
		}
		if err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).
			Fields("total_count,scheduled_at").Where("id", channel.ActiveRunID).
			Where("tenant_id", profile.TenantId).Where("channel_id", channel.ChannelId).
			WhereIn("status", []string{cycleRunStatusPending, cycleRunStatusRunning, cycleRunStatusDispatching}).
			Scan(&run); !telegramOptionalAttemptScanSucceeded(err) {
			return nil, err
		}
		activeBatchSize, activeAt = int64(run.TotalCount), dbtime.FromTimestamp(run.ScheduledAt)
	}
	offset, ok := profilePushBatchDayOffset(channel.CycleBatchSize, rank, activeBatchSize)
	if !ok {
		return nil, nil
	}
	// Reuse the same cached rank and active-run snapshot as the date estimate.
	// Do not count a second time just to explain the displayed wait.
	position := profilePushBatchPosition(rank, activeBatchSize)
	channel.BatchAheadCount = &position.AheadCount
	channel.BatchInCurrentRun = position.InCurrentRun
	channel.BatchWaitingNextRound = position.WaitingNextRound
	if position.WaitingNextRound {
		channel.BatchRoundRemaining = &position.RoundRemaining
	}
	if offset < 0 {
		if activeAt != nil && activeAt.After(now) {
			return activeAt, nil
		}
		return now, nil
	}
	next := profilePushNextBatchAt(channel.NextBatchAt, now)
	if offset == 0 {
		return next, nil
	}
	// Use calendar days and the configured batch clock, including overdue runs.
	return nextBatchCycleAt(channel.CycleBatchTime, next.Time).AddDate(0, 0, int(offset-1)), nil
}
