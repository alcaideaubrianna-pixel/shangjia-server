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
func (s *sSysPublish) preparePublishedProfileEdit(ctx context.Context, profileId, tenantId, accountId int64, operationNo string) (bool, error) {
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
	stateRow, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("publish_task_status,publish_operation_no").
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("profile_id", profileId).
		WhereNull("deleted_at").One()
	if err != nil {
		return false, gerror.Wrap(err, "读取资料发布状态失败")
	}
	if !stateRow.IsEmpty() && stateRow["publish_task_status"].String() != "editing" {
		return false, gerror.New("资料正在上架处理中，请稍后再编辑")
	}
	if !stateRow.IsEmpty() && stateRow["publish_task_status"].String() == "editing" && stateRow["publish_operation_no"].String() != operationNo {
		return false, gerror.New("资料正在编辑处理中，请稍后再编辑")
	}
	status := row[dao.ContentProfile.Columns().Status].Int()
	if status != 1 {
		return false, nil
	}
	claimed, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).Where("account_id", accountId).Where("profile_id", profileId).
		WhereNull("deleted_at").Where("publish_task_status", "").
		Data(g.Map{"publish_task_status": "editing", "publish_operation_no": operationNo}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "锁定资料编辑操作失败")
	}
	if affected, _ := claimed.RowsAffected(); affected == 0 && stateRow.IsEmpty() {
		return false, gerror.New("资料正在编辑处理中，请稍后再编辑")
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

func (s *sSysPublish) releaseProfileEditClaim(ctx context.Context, profileId, tenantId, accountId int64, operationNo string) {
	_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).Where("account_id", accountId).Where("profile_id", profileId).
		Where("publish_task_status", "editing").Where("publish_operation_no", operationNo).
		Data(g.Map{"publish_task_status": "", "publish_operation_no": ""}).Update()
}

func markProfileOnlineForEdit(in *sysin.ProfileSaveInp) {
	in.Status = 1
	in.Visibility = consts.ContentVisibilityPublic
}
