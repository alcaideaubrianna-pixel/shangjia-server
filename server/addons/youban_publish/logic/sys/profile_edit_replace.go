package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
)

// preparePublishedProfileEdit makes the edit lifecycle deterministic. The old
// Telegram messages and pending jobs are removed before the replacement is
// saved, so an asynchronous down task cannot delete the replacement messages.
// It returns true when the profile was online and must be published again.
func (s *sSysPublish) preparePublishedProfileEdit(ctx context.Context, profileId, tenantId, accountId int64) (bool, error) {
	if profileId <= 0 || tenantId <= 0 || accountId <= 0 {
		return false, gerror.New("资料编辑参数不完整")
	}
	row, err := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Fields("p."+dao.ContentProfile.Columns().Status).
		Where("p."+dao.ContentProfile.Columns().Id, profileId).
		Where("ps.tenant_id", tenantId).
		Where("ps.account_id", accountId).
		WhereNull("p." + dao.ContentProfile.Columns().DeletedAt).One()
	if err != nil {
		return false, gerror.Wrap(err, "读取资料编辑状态失败")
	}
	if row.IsEmpty() {
		return false, gerror.New("资料不存在")
	}
	status := row[dao.ContentProfile.Columns().Status].Int()
	if status != 1 {
		return false, nil
	}
	if err := s.setProfilesOffline(ctx, []int64{profileId}, tenantId); err != nil {
		return false, err
	}
	jobs, err := s.telegramJobsWithActiveMessages(ctx, tenantId, []int64{profileId}, "")
	if err != nil {
		return false, err
	}
	for _, job := range jobs {
		if err = s.deleteTelegramMessageSet(ctx, job.telegramJobRecord(), "编辑资料"); err != nil {
			return false, gerror.Wrap(err, "清理编辑前TG消息失败")
		}
	}
	if err = s.supersedeProfilePendingTelegramJobs(ctx, []int64{profileId}, tenantId); err != nil {
		return false, err
	}
	return true, nil
}

func markProfileOnlineForEdit(in *sysin.ProfileSaveInp) {
	in.Status = 1
	in.Visibility = consts.ContentVisibilityPublic
}
