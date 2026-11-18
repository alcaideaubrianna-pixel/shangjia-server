package sys

import (
	"context"
	"errors"
	"hotgo/utility/dbtime"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	publishCycleBatchItemTable = "hg_youban_publish_cycle_batch_item"
	cycleBatchSnapshotVersion  = -1
)

type cycleBatchItemState string

const (
	cycleBatchItemSelected cycleBatchItemState = "selected"
	cycleBatchItemAdmitted cycleBatchItemState = "admitted"
	cycleBatchItemDeferred cycleBatchItemState = "deferred"
	cycleBatchItemFailed   cycleBatchItemState = "failed"
	cycleBatchItemSkipped  cycleBatchItemState = "skipped"
)

type cycleBatchSnapshotItem struct {
	RunID        int64               `orm:"run_id"`
	Ordinal      int64               `orm:"ordinal"`
	TenantID     int64               `orm:"tenant_id"`
	AccountID    int64               `orm:"account_id"`
	ChannelID    int64               `orm:"channel_id"`
	ProfileID    int64               `orm:"profile_id"`
	BatchOrder   int64               `orm:"batch_order"`
	JobID        int64               `orm:"job_id"`
	ItemState    cycleBatchItemState `orm:"item_state"`
	ErrorMessage string              `orm:"error_message"`
}

func cycleBatchSnapshotRun(run cycleRunRecord) bool {
	return scheduledBatchCycleRun(run) && run.UpperProfileID == cycleBatchSnapshotVersion
}

func batchCycleLocalDay(now time.Time) (time.Time, time.Time) {
	now = now.In(dbtime.Location())
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 0, 1)
}

// Candidate order is authoritative: neither profile IDs nor timestamps are cursors.
func newCycleBatchSnapshot(tenantID, channelID int64, limit int, candidates []batchRotationCandidate) ([]cycleBatchSnapshotItem, error) {
	items := make([]cycleBatchSnapshotItem, 0, len(candidates))
	seen := make(map[int64]bool, len(candidates))
	for _, candidate := range candidates {
		if len(items) >= limit {
			break
		}
		if candidate.ProfileID <= 0 || candidate.AccountID < 0 || seen[candidate.ProfileID] {
			return nil, gerror.New("频道轮播候选身份重复或无效")
		}
		seen[candidate.ProfileID] = true
		items = append(items, cycleBatchSnapshotItem{
			Ordinal: int64(len(items) + 1), TenantID: tenantID, AccountID: candidate.AccountID,
			ChannelID: channelID, ProfileID: candidate.ProfileID, BatchOrder: candidate.Order,
			ItemState: cycleBatchItemSelected,
		})
	}
	return items, nil
}

func persistCycleBatchSnapshotTx(ctx context.Context, tx gdb.TX, runID int64, items []cycleBatchSnapshotItem) error {
	for start := 0; start < len(items); start += cycleBatchPageSize {
		end := start + cycleBatchPageSize
		if end > len(items) {
			end = len(items)
		}
		for i := start; i < end; i++ {
			items[i].RunID = runID
		}
		if _, err := tx.Model(publishCycleBatchItemTable).Ctx(ctx).Data(items[start:end]).Insert(); err != nil {
			return gerror.Wrap(err, "保存频道轮播每日快照失败")
		}
	}
	return nil
}

func cycleBatchSnapshotPage(ctx context.Context, run cycleRunRecord, limit int) ([]cycleBatchSnapshotItem, error) {
	return cycleBatchSnapshotPageModel(g.DB().Model(publishCycleBatchItemTable).Ctx(ctx), run, limit)
}

func cycleBatchSnapshotPageModel(model *gdb.Model, run cycleRunRecord, limit int) ([]cycleBatchSnapshotItem, error) {
	var items []cycleBatchSnapshotItem
	err := model.
		Where("run_id", run.Id).Where("tenant_id", run.TenantId).Where("channel_id", run.ChannelId).
		WhereGT("ordinal", run.CursorId).OrderAsc("ordinal").Limit(limit).Scan(&items)
	return items, err
}

// executeCycleBatchSnapshot consumes only immutable ordinals. A failed media
// preflight consumes its selected slot; it never blocks the next profile.
func (s *sSysPublish) executeCycleBatchSnapshot(ctx context.Context, run cycleRunRecord) error {
	items, err := cycleBatchSnapshotPage(ctx, run, cycleBatchPageSize)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return s.beginChannelCycleDispatch(ctx, run)
	}
	for _, item := range items {
		if item.ItemState != cycleBatchItemSelected {
			continue
		}
		if err = s.submitCycleBatchSnapshotItem(ctx, item); err != nil {
			// Persistence/identity errors must retry this same ordinal, not skip it.
			return err
		}
	}
	// Count admissions from durable items, never from successful deliveries or
	// this attempt's local counters (a crash may replay the page).
	admitted, err := g.DB().Model(publishCycleBatchItemTable).Ctx(ctx).
		Where("run_id", run.Id).Where("item_state", cycleBatchItemAdmitted).Count()
	if err != nil {
		return err
	}
	result, err := g.DB().Model(publishCycleRunTable).Ctx(ctx).
		Where("id", run.Id).Where("status", cycleRunStatusRunning).Where("cursor_id", run.CursorId).
		Data(g.Map{"cursor_id": items[len(items)-1].Ordinal, "queued_count": admitted,
			"status": cycleRunStatusPending, "stage": "producing", "updated_at": gtime.Now()}).Update()
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return err
	}
	return s.enqueueCycleRun(ctx, run.Id, time.Second)
}

func cycleBatchPreparationState(err error) cycleBatchItemState {
	switch {
	case err == nil:
		return cycleBatchItemAdmitted
	case errors.Is(err, errProfileCycleMediaPending), errors.Is(err, errTelegramPublishPermissionPaused):
		return cycleBatchItemDeferred
	case errors.Is(err, errPublishProfileUnavailable):
		return cycleBatchItemSkipped
	default:
		return cycleBatchItemFailed
	}
}

func (s *sSysPublish) submitCycleBatchSnapshotItem(ctx context.Context, item cycleBatchSnapshotItem) error {
	// Replays first recover the exact operation/previously attached admission.
	// An unrelated active job is attached too, but is never replaced or reset.
	if err := recoverCycleBatchAdmission(ctx, &item); err != nil {
		return err
	}
	var submitErr error
	if item.JobID == 0 {
		submitErr = s.submitProfilePublish(context.WithValue(ctx, cycleBatchAdmissionContextKey{}, item),
			item.ProfileID, item.TenantID, item.AccountID, 0,
			cyclePublishOperationNo(item.RunID, item.ProfileID, item.ChannelID), []int64{item.ChannelID}, true)
		if err := recoverCycleBatchAdmission(ctx, &item); err != nil {
			return err
		}
	}
	state := cycleBatchPreparationState(submitErr)
	message := ""
	if submitErr != nil {
		message = submitErr.Error()
	}
	if item.JobID > 0 {
		state = cycleBatchItemAdmitted
	} else if submitErr == nil {
		state = cycleBatchItemDeferred
		message = "发布准备尚未产生任务，等待既有媒体或队列恢复"
	}
	_, err := g.DB().Model(publishCycleBatchItemTable).Ctx(ctx).
		Where("run_id", item.RunID).Where("ordinal", item.Ordinal).
		Where("item_state", cycleBatchItemSelected).
		Data(g.Map{"item_state": state, "error_message": message}).Update()
	return err
}
