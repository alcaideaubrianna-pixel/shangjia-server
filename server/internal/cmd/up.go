// Package cmd
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package cmd

import (
	"context"
	"strconv"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcmd"

	"hotgo/addons/youban_publish/install"
	publishsys "hotgo/addons/youban_publish/logic/sys"
	"hotgo/internal/cmd/upgrade/fix"
)

var (
	Up = &gcmd.Command{
		Name:        "up",
		Brief:       "处理hotgo版本升级更新带来的兼容问题",
		Description: ``,
		Func: func(ctx context.Context, parser *gcmd.Parser) (err error) {
			args := parser.GetOptAll()
			g.Log().Debugf(ctx, "up args:%v", args)
			if len(args) == 0 {
				err = gerror.New("up args cannot be empty.")
				return
			}

			method, ok := args["m"]
			if !ok {
				err = gerror.New("up method cannot be empty.")
				return
			}

			switch method {
			case "fix":
				err = handleUpgradeFix(ctx, args)
			case "publishMigration":
				g.Log().Info(ctx, "开始执行上架系统独立迁移脚本")
				err = install.ExecMaintenanceSql(ctx, install.MigrationSqlFiles())
				if err == nil {
					g.Log().Info(ctx, "上架系统独立迁移脚本执行完成")
				}
			default:
				err = gerror.Newf("up method[%v] does not exist", method)
			}

			if err == nil {
				g.Log().Info(ctx, "up exec successful!")
			}
			return
		},
	}
)

// handleUpgradeFix 处理修复脚本
func handleUpgradeFix(ctx context.Context, args map[string]string) (err error) {
	a1, ok := args["a1"]
	if !ok {
		err = gerror.New("fix args cannot be empty.")
		return
	}

	switch a1 {
	case "menuTree":
		fix.UpdateAdminMenuTree(ctx)
	case "mediaPHashBucket":
		err = fix.BackfillYoubanPublishMediaPHashBucket(ctx)
	case "mediaPHashMissing":
		err = fix.BackfillYoubanPublishMediaMissingPHash(ctx)
	case "noteIndex":
		err = fix.BackfillYoubanPublishNoteIndex(ctx)
	case "publishHeavyIndexes":
		err = fix.ApplyYoubanPublishHeavyIndexes(ctx)
	case "mediaPHashProfileIndexes":
		err = fix.ApplyYoubanPublishMediaPHashProfileIndexes(ctx)
	case "publishProfileMedia":
		err = fix.BackfillYoubanPublishProfileMedia(ctx)
	case "publishChannelProfile":
		err = fix.BackfillYoubanPublishChannelProfiles(ctx)
	case "publishProfileTaskCleanup":
		err = fix.CleanupYoubanPublishProfileTasks(ctx)
	case "collectEventRequeue":
		err = fix.RequeueYoubanPublishCollectEvents(ctx)
	case "collectMediaRetryState":
		err = fix.ApplyYoubanPublishCollectMediaRetryState(ctx)
	case "collectMediaQueueRebalance":
		_, err = publishsys.RebalanceCollectMediaQueue(ctx)
	case "collectProfileMediaRepair":
		profileIds, parseErr := fix.ParseMaterialImportRepairGroupIDs(args["a2"])
		if parseErr != nil {
			return gerror.New("collectProfileMediaRepair 的 a2 必须是逗号分隔的有效资料ID")
		}
		err = fix.RepairYoubanPublishCollectProfileMedia(ctx, profileIds)
	case "collectProfileMediaDedupe":
		err = fix.DedupeYoubanPublishCollectProfileMedia(ctx)
	case "collectMediaCDNRepair":
		mediaIds, parseErr := fix.ParseMaterialImportRepairGroupIDs(args["a2"])
		if parseErr != nil {
			return gerror.New("collectMediaCDNRepair 的 a2 必须是逗号分隔的有效媒体ID")
		}
		err = fix.RepairYoubanPublishCollectMediaCDN(ctx, mediaIds)
	case "collectReviewMediaRepair":
		reviewIds, parseErr := fix.ParseMaterialImportRepairGroupIDs(args["a2"])
		if parseErr != nil || len(reviewIds) == 0 {
			return gerror.New("collectReviewMediaRepair 需要通过 a2 传入逗号分隔的审核ID")
		}
		err = publishsys.RepairCollectReviewMedia(ctx, reviewIds)
	case "collectRuleRemoveUniqueNo":
		err = fix.RemoveYoubanPublishCollectRuleUniqueNo(ctx)
	case "collectDedupeIndexes":
		err = fix.ApplyYoubanPublishCollectDedupeIndexes(ctx)
	case "collectMaterialRebuild":
		err = fix.RebuildYoubanPublishCollectMaterialGroups(ctx)
	case "collectRelationNormalize":
		err = fix.NormalizeYoubanPublishCollectRelations(ctx)
	case "collectMediaMetaNormalize":
		err = fix.NormalizeYoubanPublishCollectMediaMeta(ctx)
	case "materialImportMediaNormalize":
		err = fix.NormalizeYoubanPublishMaterialImportMedia(ctx)
	case "materialImportMediaRepair":
		accountId, parseErr := strconv.ParseInt(args["a2"], 10, 64)
		if parseErr != nil || accountId <= 0 {
			return gerror.New("materialImportMediaRepair 需要通过 a2 传入有效的上架账号ID")
		}
		groupIds, parseErr := fix.ParseMaterialImportRepairGroupIDs(args["a3"])
		if parseErr != nil {
			return gerror.New("materialImportMediaRepair 的 a3 必须是逗号分隔的有效分组ID")
		}
		err = fix.RepairYoubanPublishMaterialImportMissingMedia(ctx, accountId, groupIds)
	default:
		err = gerror.Newf("fix a1 is invalid, a1:%v", a1)
	}
	return
}
