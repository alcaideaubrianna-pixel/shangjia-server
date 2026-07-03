package main

import (
	"context"
	"flag"
	"fmt"

	_ "github.com/gogf/gf/contrib/drivers/mysql/v2"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/os/gctx"

	pdao "hotgo/addons/youban_publish/internal/dao"
	_ "hotgo/addons/youban_publish/logic"
	"hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
	"hotgo/internal/consts"
	"hotgo/internal/global"
	"hotgo/internal/library/contexts"
	_ "hotgo/internal/logic"
	"hotgo/internal/model"
)

func main() {
	username := flag.String("account", "test1", "上架账号 username")
	variants := flag.Int("variants", 3, "创建测试资料数量")
	submitNow := flag.Int("submit-now", 1, "是否立即提交")
	includeScheduled := flag.Int("scheduled", 1, "是否创建定时测试")
	scheduledDelay := flag.Int("scheduled-delay", 90, "定时测试延迟秒数")
	flag.Parse()

	ctx := gctx.GetInitCtx()
	global.Init(ctx)

	account, err := publishAccountByUsername(ctx, *username)
	if err != nil {
		panic(err)
	}
	ctx = context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{
		Module: consts.AppApi,
		User: &model.Identity{
			Id:       account.Id,
			App:      consts.AppApi,
			DeptType: account.AccountType,
			Username: account.Username,
			RealName: account.Nickname,
		},
	})
	contexts.SetUser(ctx, contexts.GetUser(ctx))
	res, err := publishService.SysPublish().MyDevPublishChainTest(ctx, &sysin.DevPublishChainTestInp{
		Variants:              *variants,
		SubmitNow:             *submitNow,
		IncludeScheduled:      *includeScheduled,
		ScheduledDelaySeconds: *scheduledDelay,
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("channelIds: %v\n", res.ChannelIds)
	for _, item := range res.Items {
		fmt.Printf("task=%d profile=%d submitted=%v publishAt=%s title=%s media=%v\n",
			item.TaskId, item.ProfileId, item.Submitted, item.PublishAt, item.Title, item.MediaIds)
	}
}

func publishAccountByUsername(ctx context.Context, username string) (*sysin.AccountModel, error) {
	var account *sysin.AccountModel
	columns := pdao.YoubanPublishAccount.Columns()
	err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(columns.Username, username).
		Where(columns.Status, 1).
		WhereNull(columns.DeletedAt).
		OrderAsc("id").
		Scan(&account)
	if err != nil {
		return nil, err
	}
	if account == nil || account.Id <= 0 {
		return nil, fmt.Errorf("上架账号不存在：%s", username)
	}
	return account, nil
}
