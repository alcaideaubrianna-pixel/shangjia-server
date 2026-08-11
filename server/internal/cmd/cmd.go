// Package cmd
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package cmd

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcmd"
	"hotgo/utility/runrole"
	"hotgo/utility/simple"
)

var (
	Main = &gcmd.Command{
		Description: `默认启动所有服务`,
		Func: func(ctx context.Context, parser *gcmd.Parser) (err error) {
			return All.Func(ctx, parser)
		},
	}

	Help = &gcmd.Command{
		Name:  "help",
		Brief: "查看帮助",
		Description: `
		命令提示符
		---------------------------------------------------------------------------------
		启动服务
		>> 所有服务  [go run main.go]   热编译  [gf run main.go]
		>> API服务   [go run main.go web]
		>> Worker服务 [go run main.go worker]
		>> Account服务 [go run main.go account]
		>> Scheduler服务 [go run main.go scheduler]
		>> Runtime兼容服务 [go run main.go runtime]
		>> HTTP服务  [go run main.go http]
		>> 消息队列  [go run main.go queue]
		>> 定时任务  [go run main.go cron]
		>> 查看帮助  [go run main.go help]

		---------------------------------------------------------------------------------
		工具
		>> 释放casbin权限，用于清理无效的权限设置  [go run main.go tools -m=casbin -a1=refresh]
		>> 清理图片感知哈希完全重复的资料  [go run main.go tools -m=content -a1=dedupePHash -startId=0 -limit=10000]
		>> 打印所有打包的资源文件列表  [go run main.go tools -m=gres -a1=dump]
		>> 打印指定打包的资源文件内容  [go run main.go tools -m=gres -a1=content -a2=resource/template/home/index.html]
		---------------------------------------------------------------------------------
		升级更新
		>> 修复菜单关系树  [go run main.go up -m=fix -a1=menuTree]
		>> 执行上架系统独立迁移脚本  [go run main.go up -m=publishMigration]
		>> 回填上架媒体感知哈希分桶  [go run main.go up -m=fix -a1=mediaPHashBucket]
		>> 补全上架媒体缺失感知哈希  [go run main.go up -m=fix -a1=mediaPHashMissing]
		>> 回填上架资料索引  [go run main.go up -m=fix -a1=noteIndex]
		>> 创建上架系统大表索引  [go run main.go up -m=fix -a1=publishHeavyIndexes]
		>> 回填上架资料当前媒体  [go run main.go up -m=fix -a1=publishProfileMedia]
		>> 回填频道当前上架资料索引  [go run main.go up -m=fix -a1=publishChannelProfile]
		>> 清理普通资料历史Task  [go run main.go up -m=fix -a1=publishProfileTaskCleanup]
		>> 补全历史采集资料媒体，无法恢复的资料会被删除  [go run main.go up -m=fix -a1=collectProfileMediaRepair -a2=<资料ID,可选>]
		>> 补全TG导入资料缺失媒体  [go run main.go up -m=fix -a1=materialImportMediaRepair -a2=<上架账号ID> -a3=<分组ID,可选>]
		>> 清理历史采集资料重复媒体  [go run main.go up -m=fix -a1=collectProfileMediaDedupe]
		>> 回填历史采集媒体 CDN 地址  [go run main.go up -m=fix -a1=collectMediaCDNRepair]
		>> 修复指定采集审核媒体地址  [go run main.go up -m=fix -a1=collectReviewMediaRepair -a2=<审核ID>]
		>> 删除采集规则唯一编号字段  [go run main.go up -m=fix -a1=collectRuleRemoveUniqueNo]
		>> 创建采集去重查询索引  [go run main.go up -m=fix -a1=collectDedupeIndexes]
		>> 统一采集资料未上架状态  [go run main.go up -m=fix -a1=collectProfileOfflineState]
		---------------------------------------------------------------------------------
		更多
       	github地址：https://github.com/bufanyun/hotgo
		文档地址：https://github.com/bufanyun/hotgo/tree/v2.0/docs/guide-zh-CN	
		HotGo框架交流1群：190966648
    `,
	}

	All = &gcmd.Command{
		Name:        "all",
		Brief:       "start all server",
		Description: "this is the command entry for starting all server",
		Func: func(ctx context.Context, parser *gcmd.Parser) (err error) {
			runrole.Set(runrole.All)
			g.Log().Debug(ctx, "starting all server")

			// 需要启动的服务
			var allServers = []*gcmd.Command{Http, Queue, Cron}

			for _, server := range allServers {
				var cmd = server
				simple.SafeGo(ctx, func(ctx context.Context) {
					if err := cmd.Func(ctx, parser); err != nil {
						g.Log().Fatalf(ctx, "%v start fail:%v", cmd.Name, err)
					}
				})
			}

			// 信号监听
			signalListen(ctx, signalHandlerForOverall)

			<-serverCloseSignal
			serverWg.Wait()
			g.Log().Debug(ctx, "all service successfully closed ..")
			return
		},
	}

	Web = roleCommand("web", "启动 API 服务", runrole.Web, Http)

	Worker = roleCommand("worker", "启动 Worker 服务", runrole.Worker, Http, Queue)

	CollectorWorker = roleCommand("collector-worker", "启动 Telegram 采集 Worker 服务", runrole.CollectorWorker, Http)

	MediaWorker = roleCommand("media-worker", "启动 Telegram 媒体 Worker 服务", runrole.MediaWorker, Http)

	PublishWorker = roleCommand("publish-worker", "启动 Telegram 发布 Worker 服务", runrole.PushWorker, Http)

	Account = roleCommand("account", "启动 Telegram 账号与 Bot 常驻服务", runrole.Account, Http)

	Scheduler = roleCommand("scheduler", "启动 Cron 与调度服务", runrole.Scheduler, Http, Cron)

	Runtime = roleCommand("runtime", "启动 Runtime 服务", runrole.Runtime, Http, Cron)
)

func roleCommand(name string, brief string, role string, commands ...*gcmd.Command) *gcmd.Command {
	return &gcmd.Command{
		Name:  name,
		Usage: name,
		Brief: brief,
		Func: func(ctx context.Context, parser *gcmd.Parser) error {
			runrole.Set(role)
			for _, command := range commands {
				current := command
				simple.SafeGo(ctx, func(ctx context.Context) {
					if err := current.Func(ctx, parser); err != nil {
						g.Log().Fatalf(ctx, "%s start fail:%+v", current.Name, err)
					}
				})
			}
			signalListen(ctx, signalHandlerForOverall)
			<-serverCloseSignal
			serverWg.Wait()
			return nil
		},
	}
}

func init() {
	if err := Main.AddCommand(All, Web, Worker, CollectorWorker, MediaWorker, PublishWorker, Account, Scheduler, Runtime, Http, Queue, Cron, Auth, Tools, Up, Help); err != nil {
		panic(err)
	}
}
