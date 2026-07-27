package fix

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const publishChannelProfileBackfillBatchSize = 2000

type publishChannelProfileJob struct {
	Id          int64  `orm:"id"`
	TenantId    int64  `orm:"tenant_id"`
	AccountId   int64  `orm:"account_id"`
	ProfileId   int64  `orm:"profile_id"`
	ChannelId   int64  `orm:"channel_id"`
	OperationNo string `orm:"operation_no"`
}

// BackfillYoubanPublishChannelProfiles rebuilds the current channel/profile
// projection outside the publish runtime.
func BackfillYoubanPublishChannelProfiles(ctx context.Context) error {
	processed := 0
	for {
		jobs, err := pendingPublishChannelProfileJobs(ctx, publishChannelProfileBackfillBatchSize)
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			break
		}
		seen := make(map[string]struct{}, len(jobs))
		for _, job := range jobs {
			key := fmt.Sprintf("%d:%d", job.ChannelId, job.ProfileId)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			if err = upsertPublishChannelProfile(ctx, job); err != nil {
				return err
			}
			processed++
		}
		g.Log().Infof(ctx, "频道当前上架资料索引回填进度：processed=%d", processed)
	}
	g.Log().Infof(ctx, "频道当前上架资料索引回填完成：processed=%d", processed)
	return nil
}

func pendingPublishChannelProfileJobs(ctx context.Context, limit int) ([]publishChannelProfileJob, error) {
	var jobs []publishChannelProfileJob
	err := g.DB().Model("hg_youban_publish_tg_job j").Safe().Ctx(ctx).
		Fields("j.id,j.tenant_id,j.account_id,j.profile_id,j.channel_id,j.operation_no").
		Where("j.status", "sent").
		Where("j.profile_id > 0").
		Where("LOWER(j.operation_no) NOT LIKE 'down:%'").
		Where("NOT EXISTS (SELECT 1 FROM hg_youban_publish_channel_profile cp WHERE cp.channel_id=j.channel_id AND cp.profile_id=j.profile_id AND cp.status='active')").
		OrderDesc("j.id").Limit(limit).Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取待回填频道上架资料失败")
	}
	return jobs, nil
}

func upsertPublishChannelProfile(ctx context.Context, job publishChannelProfileJob) error {
	now := gtime.Now()
	data := g.Map{
		"tenant_id": job.TenantId, "account_id": job.AccountId, "channel_id": job.ChannelId,
		"profile_id": job.ProfileId, "last_job_id": job.Id,
		"status": "active", "updated_at": now,
	}
	result, err := g.DB().Model("hg_youban_publish_channel_profile").Safe().Ctx(ctx).
		Where("channel_id", job.ChannelId).Where("profile_id", job.ProfileId).
		Where("last_job_id <= ?", job.Id).Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新频道上架资料索引失败")
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return nil
	}
	data["created_at"] = now
	if _, err = g.DB().Model("hg_youban_publish_channel_profile").Safe().Ctx(ctx).Data(data).Insert(); err == nil {
		return nil
	}
	if !isPublishChannelProfileDuplicate(err) {
		return gerror.Wrap(err, "创建频道上架资料索引失败")
	}
	_, err = g.DB().Model("hg_youban_publish_channel_profile").Safe().Ctx(ctx).
		Where("channel_id", job.ChannelId).Where("profile_id", job.ProfileId).
		Where("last_job_id <= ?", job.Id).Data(data).Update()
	return gerror.Wrap(err, "并发更新频道上架资料索引失败")
}

func isPublishChannelProfileDuplicate(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") || strings.Contains(message, "unique constraint") || strings.Contains(message, "1062")
}
