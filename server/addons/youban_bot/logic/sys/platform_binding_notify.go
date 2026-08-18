package sys

import (
	"context"
	"fmt"

	"hotgo/addons/youban_bot/model/input/sysin"
	botservice "hotgo/addons/youban_bot/service"
	"hotgo/internal/consts"
	"hotgo/internal/library/platformbinding"

	"github.com/gogf/gf/v2/frame/g"
)

func init() {
	platformbinding.RegisterApprovedHandler(notifyPlatformBindingApproved)
}

func notifyPlatformBindingApproved(ctx context.Context, event platformbinding.ApprovedEvent) error {
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model("hg_youban_publish_account").Safe().Ctx(ctx).
		Fields("id").Where("tenant_id", event.TenantID).Where("status", 1).WhereNull("deleted_at").Scan(&rows)
	if err != nil {
		return err
	}
	accountIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			accountIDs = append(accountIDs, row.Id)
		}
	}
	if len(accountIDs) == 0 {
		return nil
	}
	platformName := event.AppName
	if platformName == "" {
		platformName = event.AppID
	}
	return botservice.SysBot().NotifyAccounts(ctx, &sysin.NotifyAccountsInp{
		App: consts.AppApi, AccountIds: accountIDs, BotStrategy: "bound",
		Text:      fmt.Sprintf("<b>平台合作申请已通过</b>\n\n平台：%s\n状态：已绑定\n你的已上架资料将按平台规则同步展示。", platformName),
		ParseMode: "HTML",
	})
}
