package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) upsertMaterialImportUnitBlocks(ctx context.Context, task *sysin.MaterialImportTaskModel, units []*collectMaterialUnit) error {
	for _, unit := range pairCollectMaterialUnits(units) {
		if unit == nil {
			continue
		}
		if err := s.upsertMaterialImportDisplayUnit(ctx, task, unit); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) upsertMaterialImportDisplayUnit(ctx context.Context, task *sysin.MaterialImportTaskModel, unit *collectMaterialUnit) error {
	items := collectMediaItemsWithPurpose(unit.Media, collectMaterialRoleDisplay)
	mediaJSON, mediaCount := collectMessageMediaJSON(items)
	existing, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("source_unique_key", unit.UniqueKey).One()
	if err != nil {
		return gerror.Wrap(err, "读取资料导入分组失败")
	}
	now := gtime.Now()
	data := g.Map{
		"task_id":            task.Id,
		"tenant_id":          task.TenantId,
		"account_id":         task.AccountId,
		"source_chat_id":     task.SourceChatId,
		"source_grouped_id":  unit.GroupedId,
		"source_unique_key":  unit.UniqueKey,
		"raw_text":           firstNonEmpty(unit.RawText, existing["raw_text"].String()),
		"profile_text":       firstNonEmpty(unit.RawText, existing["profile_text"].String()),
		"media_json":         mediaJSON,
		"media_total":        mediaCount,
		"status":             sysin.MaterialImportStatusPending,
		"message_at":         unit.MessageAt,
		"updated_at":         now,
		"source_message_ids": collectMaterialMessageIDs(existing["source_message_ids"].String(), unit.Messages),
	}
	parsedText := firstNonEmpty(unit.RawText, existing["raw_text"].String())
	title, profileNo, nickname := materialImportTitle(parsedText)
	data["title"] = title
	data["profile_no"] = profileNo
	data["nickname"] = nickname
	if existing.IsEmpty() {
		data["created_at"] = now
		_, err = pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Data(data).Insert()
		return gerror.Wrap(err, "创建资料导入分组失败")
	}
	if strings.TrimSpace(existing["media_json"].String()) != "" {
		data["media_json"], data["media_total"] = mergeCollectMediaJSON(existing["media_json"].String(), mediaJSON)
	}
	_, err = pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", existing["id"].Int64()).Data(data).Update()
	return gerror.Wrap(err, "更新资料导入分组失败")
}
