package sys

import (
	"context"
	"strconv"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

// publishRecordListFromLog reads the denormalized publish log.  All display
// fields are snapshots, so this path never joins the live job/profile tables.
func (s *sSysPublish) publishRecordListFromLog(ctx context.Context, in *sysin.PublishRecordListInp, tenantId, accountId int64) (list []*sysin.PublishRecordModel, total int, err error) {
	mod := g.DB().Model(publishRecordLogTable+" l").Safe().Ctx(ctx).Where("l.tenant_id", tenantId)
	if accountId > 0 {
		mod = mod.Where("l.account_id", accountId)
	} else if in.AccountId > 0 {
		mod = mod.Where("l.account_id", in.AccountId)
	}
	if in.ProfileId > 0 {
		mod = mod.Where("l.profile_id", in.ProfileId)
	}
	if in.TaskId > 0 {
		mod = mod.Where("l.task_id", in.TaskId)
	}
	if in.Action != "" {
		mod = mod.Where("l.action", in.Action)
	}
	switch in.Status {
	case "success", "sent":
		mod = mod.Where("l.status", "success")
	case "failed":
		mod = mod.WhereIn("l.status", []string{"failed", "failed_retry"})
	case "pending":
		mod = mod.WhereIn("l.status", []string{"pending", "sending"})
	case "":
	default:
		mod = mod.Where("l.status", in.Status)
	}
	if in.Keyword != "" {
		like := "%" + in.Keyword + "%"
		if id, e := strconv.ParseInt(in.Keyword, 10, 64); e == nil && id > 0 {
			mod = mod.Where("(l.title LIKE ? OR l.message LIKE ? OR l.channel_title LIKE ? OR l.bot_name LIKE ? OR l.bot_username LIKE ? OR l.account_name LIKE ? OR l.target_chat_id LIKE ? OR l.id=? OR l.job_id=? OR l.task_id=? OR l.profile_id=?)", like, like, like, like, like, like, like, id, id, id, id)
		} else {
			mod = mod.Where("(l.title LIKE ? OR l.message LIKE ? OR l.channel_title LIKE ? OR l.bot_name LIKE ? OR l.bot_username LIKE ? OR l.account_name LIKE ? OR l.target_chat_id LIKE ?)", like, like, like, like, like, like, like)
		}
	}
	total, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取发送记录总数失败")
	}
	normalizePublishRecordPage(in, total)
	err = mod.Fields("l.id,l.job_id,l.task_id,l.tenant_id,l.account_id,l.profile_id,l.channel_id,l.bot_id,l.operation_no,l.target_chat_id,l.action,l.status,l.message,l.created_at,l.title,l.account_name,l.tenant_name,l.bot_name,l.bot_username,l.channel_title,l.channel_username").Page(in.Page, in.PerPage).OrderDesc("l.id").Scan(&list)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取发送记录失败")
	}
	normalizeCollectPublishRecordActions(list)
	return list, total, nil
}
