package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type cycleBatchAdmissionContextKey struct{}

// attachCycleBatchAdmission must be called by the existing publishing entrypoint
// after job preparation and before enqueue. It also observes reused active jobs.
func attachCycleBatchAdmission(ctx context.Context, jobID, profileID int64) error {
	item, ok := ctx.Value(cycleBatchAdmissionContextKey{}).(cycleBatchSnapshotItem)
	if !ok {
		return nil
	}
	if profileID != item.ProfileID || jobID <= 0 {
		return gerror.New("轮播任务资料归属不匹配")
	}
	job, err := g.DB().Model(publishTgJobTable).Ctx(ctx).
		Where("id", jobID).Where("tenant_id", item.TenantID).Where("account_id", item.AccountID).
		Where("channel_id", item.ChannelID).Where("profile_id", item.ProfileID).Value("id")
	if err != nil {
		return err
	}
	if job.Int64() != jobID {
		return gerror.New("轮播任务身份不匹配")
	}
	result, err := g.DB().Model(publishCycleBatchItemTable).Ctx(ctx).
		Where("run_id", item.RunID).Where("ordinal", item.Ordinal).
		Where("tenant_id", item.TenantID).Where("account_id", item.AccountID).
		Where("channel_id", item.ChannelID).Where("profile_id", item.ProfileID).
		Where("job_id = 0 OR job_id = ?", jobID).
		Data(g.Map{"job_id": jobID, "item_state": cycleBatchItemAdmitted, "error_message": ""}).Update()
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return gerror.New("轮播快照任务绑定冲突")
	}
	return nil
}

func recoverCycleBatchAdmission(ctx context.Context, item *cycleBatchSnapshotItem) error {
	value, err := g.DB().Model(publishCycleBatchItemTable).Ctx(ctx).
		Where("run_id", item.RunID).Where("ordinal", item.Ordinal).Value("job_id")
	if err != nil {
		return err
	}
	if value.Int64() > 0 {
		item.JobID = value.Int64()
		return nil
	}
	base := g.DB().Model(publishTgJobTable).Ctx(ctx).
		Where("tenant_id", item.TenantID).Where("account_id", item.AccountID).
		Where("profile_id", item.ProfileID).Where("channel_id", item.ChannelID)
	exact, err := base.Clone().Where("operation_no", cyclePublishOperationNo(item.RunID, item.ProfileID, item.ChannelID)).
		OrderDesc("id").Value("id")
	if err != nil {
		return err
	}
	jobID := exact.Int64()
	if jobID == 0 {
		active, queryErr := base.Clone().WhereIn("status", telegramJobActiveStatuses()).OrderDesc("id").Value("id")
		if queryErr != nil {
			return queryErr
		}
		jobID = active.Int64()
	}
	if jobID > 0 {
		if err = attachCycleBatchAdmission(context.WithValue(ctx, cycleBatchAdmissionContextKey{}, *item), jobID, item.ProfileID); err != nil {
			return err
		}
		item.JobID = jobID
	}
	return nil
}

type cycleBatchDeliveryItem struct {
	Ordinal   int64               `orm:"ordinal"`
	ItemState cycleBatchItemState `orm:"item_state"`
	JobID     int64               `orm:"job_id"`
	ActualID  int64               `orm:"actual_id"`
	JobStatus JobStatus           `orm:"job_status"`
}

// aggregateCycleBatchDelivery intentionally treats unknown/missing job states as
// unfinished. Prefix aggregation cannot see reused jobs from other operations.
func aggregateCycleBatchDelivery(items []cycleBatchDeliveryItem) (pending, sent, failed int) {
	for _, item := range items {
		if item.JobID > 0 {
			if item.ActualID != item.JobID || (item.JobStatus != JobStatusSent && item.JobStatus != JobStatusFailed && item.JobStatus != JobStatusSuperseded) {
				pending++
			} else if item.JobStatus == JobStatusSent {
				sent++
			} else {
				failed++
			}
		} else if item.ItemState == cycleBatchItemSelected || item.ItemState == cycleBatchItemAdmitted {
			pending++
		} else {
			failed++
		}
	}
	return
}

func (s *sSysPublish) finalizeCycleBatchSnapshot(ctx context.Context, run cycleRunRecord) error {
	var cursor int64
	var pending, sent, failed, total int
	for {
		var items []cycleBatchDeliveryItem
		err := g.DB().Model(publishCycleBatchItemTable+" i").Ctx(ctx).
			LeftJoin(publishTgJobTable+" j", "j.id=i.job_id AND j.tenant_id=i.tenant_id AND j.account_id=i.account_id AND j.profile_id=i.profile_id AND j.channel_id=i.channel_id").
			Fields("i.ordinal,i.item_state,i.job_id,COALESCE(j.id,0) AS actual_id,COALESCE(j.status,'') AS job_status").
			Where("i.run_id", run.Id).WhereGT("i.ordinal", cursor).OrderAsc("i.ordinal").Limit(cycleBatchPageSize).Scan(&items)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			break
		}
		p, s, f := aggregateCycleBatchDelivery(items)
		pending += p
		sent += s
		failed += f
		total += len(items)
		cursor = items[len(items)-1].Ordinal
	}
	if pending > 0 {
		return nil
	}
	if total != run.TotalCount {
		return gerror.New("轮播快照数量不一致，拒绝提前结束")
	}
	status, message := cycleRunStatusFinished, ""
	if failed > 0 {
		status, message = cycleRunStatusFailed, "部分资料未交付，发送重试仍由既有队列负责"
		if sent > 0 {
			status = cycleRunStatusPartial
		}
	}
	s.finishChannelCycleRun(ctx, run, status, message)
	return nil
}
