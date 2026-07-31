package install

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cron"
	"hotgo/internal/model/entity"
)

const (
	tenantVipCloseOrderCronName      = "youbanPublishVipCloseOrder"
	tenantVipCloseOrderCronPattern   = "@every 1m"
	cycleSchedulerCronName           = "youbanPublishCycleScheduler"
	cycleSchedulerDevelopCronPattern = "@every 10m"
	cycleSchedulerProductCronPattern = "@every 1h"
)

func cycleSchedulerCronPattern(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "not-set", "develop", "testing":
		return cycleSchedulerDevelopCronPattern
	default:
		return cycleSchedulerProductCronPattern
	}
}

func ensureVipOrderCloseCron(ctx context.Context) error {
	columns := dao.SysCron.Columns()
	now := gtime.Now()
	row, err := dao.SysCron.Ctx(ctx).Where(columns.Name, tenantVipCloseOrderCronName).Where(columns.Params, "").One()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return nil
		}
		return gerror.Wrap(err, "读取上架VIP关单定时任务失败")
	}

	data := g.Map{
		columns.GroupId:   1,
		columns.Title:     "上架VIP超时自动关单",
		columns.Name:      tenantVipCloseOrderCronName,
		columns.Params:    "",
		columns.Pattern:   tenantVipCloseOrderCronPattern,
		columns.Policy:    consts.CronPolicySingle,
		columns.Count:     0,
		columns.Sort:      20,
		columns.Remark:    "关闭超过10分钟未支付的上架VIP订单",
		columns.Status:    consts.StatusEnabled,
		columns.UpdatedAt: now,
	}
	if row.IsEmpty() {
		data[columns.CreatedAt] = now
		_, err = dao.SysCron.Ctx(ctx).Data(data).Insert()
	} else {
		_, err = dao.SysCron.Ctx(ctx).Where(columns.Id, row[columns.Id].Int64()).Data(data).Update()
	}
	if err != nil {
		return gerror.Wrap(err, "初始化上架VIP关单定时任务失败")
	}

	var cronRow *entity.SysCron
	if err = dao.SysCron.Ctx(ctx).Where(columns.Name, tenantVipCloseOrderCronName).Where(columns.Params, "").Scan(&cronRow); err != nil {
		return gerror.Wrap(err, "读取上架VIP关单定时任务失败")
	}
	return cron.RefreshStatus(cronRow)
}

func ensureCycleSchedulerCron(ctx context.Context) error {
	columns := dao.SysCron.Columns()
	now := gtime.Now()
	row, err := dao.SysCron.Ctx(ctx).Where(columns.Name, cycleSchedulerCronName).Where(columns.Params, "").One()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return nil
		}
		return gerror.Wrap(err, "读取上架循环调度定时任务失败")
	}
	data := g.Map{
		columns.GroupId:   1,
		columns.Title:     "上架频道循环调度",
		columns.Name:      cycleSchedulerCronName,
		columns.Params:    "",
		columns.Pattern:   cycleSchedulerCronPattern(g.Cfg().MustGet(ctx, "system.mode", "develop").String()),
		columns.Policy:    consts.CronPolicySingle,
		columns.Count:     0,
		columns.Sort:      21,
		columns.Remark:    "扫描到期的频道循环上架计划并加入Telegram队列",
		columns.Status:    consts.StatusEnabled,
		columns.UpdatedAt: now,
	}
	if row.IsEmpty() {
		data[columns.CreatedAt] = now
		_, err = dao.SysCron.Ctx(ctx).Data(data).Insert()
	} else {
		_, err = dao.SysCron.Ctx(ctx).Where(columns.Id, row[columns.Id].Int64()).Data(data).Update()
	}
	if err != nil {
		return gerror.Wrap(err, "初始化上架循环调度定时任务失败")
	}
	var cronRow *entity.SysCron
	if err = dao.SysCron.Ctx(ctx).Where(columns.Name, cycleSchedulerCronName).Where(columns.Params, "").Scan(&cronRow); err != nil {
		return gerror.Wrap(err, "读取上架循环调度定时任务失败")
	}
	return cron.RefreshStatus(cronRow)
}
