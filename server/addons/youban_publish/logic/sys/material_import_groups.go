package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
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
	mediaCount := len(normalizeCollectMediaItems(items))
	existing, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("source_unique_key", unit.UniqueKey).One()
	if err != nil {
		return gerror.Wrap(err, "读取资料导入分组失败")
	}
	processedText, err := s.applyMaterialImportCollectRules(ctx, task, unit.RawText, mediaCount)
	if err != nil {
		return err
	}
	now := gtime.Now()
	data := g.Map{
		"task_id":            task.Id,
		"tenant_id":          task.TenantId,
		"account_id":         task.AccountId,
		"source_chat_id":     task.SourceChatId,
		"source_grouped_id":  unit.GroupedId,
		"source_unique_key":  unit.UniqueKey,
		"raw_text":           firstNonEmpty(processedText, existing["raw_text"].String()),
		"profile_text":       firstNonEmpty(processedText, existing["profile_text"].String()),
		"media_total":        mediaCount,
		"status":             sysin.MaterialImportStatusPending,
		"message_at":         unit.MessageAt,
		"updated_at":         now,
		"source_message_ids": collectMaterialMessageIDs(existing["source_message_ids"].String(), unit.Messages),
	}
	parsedText := firstNonEmpty(processedText, existing["raw_text"].String())
	title, profileNo, nickname := materialImportTitle(parsedText)
	data["title"] = title
	data["profile_no"] = profileNo
	data["nickname"] = nickname
	if existing.IsEmpty() {
		data["created_at"] = now
		groupId, insertErr := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Data(data).InsertAndGetId()
		if insertErr != nil {
			return gerror.Wrap(insertErr, "创建资料导入分组失败")
		}
		return s.replaceMaterialImportGroupMedia(ctx, groupId, task.Id, task.TenantId, task.AccountId, items)
	}
	existingItems, err := s.materialImportGroupMediaItems(ctx, existing["id"].Int64())
	if err != nil {
		return err
	}
	items = mergeCollectMediaItems(existingItems, items)
	data["media_total"] = len(items)
	_, err = pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", existing["id"].Int64()).Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新资料导入分组失败")
	}
	return s.replaceMaterialImportGroupMedia(ctx, existing["id"].Int64(), task.Id, task.TenantId, task.AccountId, items)
}

func (s *sSysPublish) applyMaterialImportCollectRules(ctx context.Context, task *sysin.MaterialImportTaskModel, rawText string, mediaCount int) (string, error) {
	rawText = strings.TrimSpace(rawText)
	if task == nil || task.TenantId <= 0 || task.AccountId <= 0 || strings.TrimSpace(task.SourceChatId) == "" {
		return rawText, nil
	}
	sourceKey := strings.TrimPrefix(strings.TrimSpace(task.SourceChatId), "-100")
	source, err := g.DB().Model(publishCollectSourceTable).Safe().Ctx(ctx).
		Where("tenant_id", task.TenantId).
		Where("account_id", task.AccountId).
		Where("tg_account_id", task.TgAccountId).
		Where("REPLACE(source_chat_id, '-100', '') = ?", sourceKey).
		Where("status", 1).
		WhereNull("deleted_at").
		OrderAsc("id").
		One()
	if err != nil {
		return rawText, gerror.Wrap(err, "读取TG导入来源采集配置失败")
	}
	if source.IsEmpty() {
		return rawText, nil
	}
	event := gdb.Record{
		"id":                gvar.New(int64(0)),
		"tenant_id":         gvar.New(task.TenantId),
		"account_id":        gvar.New(task.AccountId),
		"source_id":         gvar.New(source["id"].Int64()),
		"raw_text":          gvar.New(rawText),
		"media_count":       gvar.New(mediaCount),
		"source_message_id": gvar.New(int64(0)),
	}
	rules, err := s.collectEventRules(ctx, event, task.TenantId, task.AccountId)
	if err != nil {
		return rawText, err
	}
	for _, rule := range rules {
		if !precheckCollectRule(event, rule).Matched {
			continue
		}
		return buildCollectRuleDecision(event, nil, rule).Text, nil
	}
	return rawText, nil
}
