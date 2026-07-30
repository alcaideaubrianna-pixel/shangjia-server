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
		>> 补全历史采集资料媒体，无法恢复的资料会被删除  [go run main.go up -m=fix -a1=collectProfileMediaRepair]
		>> 清理历史采集资料重复媒体  [go run main.go up -m=fix -a1=collectProfileMediaDedupe]
		>> 删除采集规则唯一编号字段  [go run main.go up -m=fix -a1=collectRuleRemoveUniqueNo]
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
)

func init() {
	if err := Main.AddCommand(All, Http, Queue, Cron, Auth, Tools, Up, Help); err != nil {
		panic(err)
	}
}
