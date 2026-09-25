package fix

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// ResetYoubanPublishAccountLastActive initializes the activity window for
// existing tenant administrators after introducing last_active_at.
//
// This is intentionally an explicit repair command rather than part of the
// normal migration: the operation must be run once against the production
// database after deployment, and it should reset all existing administrators
// to the deployment time so nobody is suspended based on an unknown history.
func ResetYoubanPublishAccountLastActive(ctx context.Context) error {
	base := func() *gdb.Model {
		return g.DB().Model("hg_youban_publish_account").Safe().Ctx(ctx).Where("account_type", "admin")
	}
	total, err := base().Count()
	if err != nil {
		return gerror.Wrap(err, "统计上架管理员账号失败")
	}
	missing, err := base().WhereNull("last_active_at").Count()
	if err != nil {
		return gerror.Wrap(err, "统计未初始化账号活跃时间失败")
	}

	result, err := base().Data(g.Map{
		"last_active_at": gdb.Raw("CURRENT_TIMESTAMP"),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "初始化上架管理员账号活跃时间失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "读取活跃时间初始化结果失败")
	}

	remaining, err := base().WhereNull("last_active_at").Count()
	if err != nil {
		return gerror.Wrap(err, "校验账号活跃时间初始化结果失败")
	}
	g.Log().Infof(ctx, "上架管理员活跃时间初始化完成 total=%d missingBefore=%d affected=%d remainingNull=%d", total, missing, affected, remaining)
	return nil
}
