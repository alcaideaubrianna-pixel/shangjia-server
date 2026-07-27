package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/gotd/td/tg"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type materialImportMessageUnit struct {
	GroupedId string
	UniqueKey string
	RawText   string
	MessageAt *gtime.Time
	MessageId int
	Messages  []int
	Media     []collectMediaItem
}

func (s *sSysPublish) upsertMaterialImportUnits(ctx context.Context, task *sysin.MaterialImportTaskModel, messages []*tg.Message) error {
	return s.upsertMaterialImportUnitBlocks(ctx, task, materialImportBuildUnits(task, messages))
}

func (s *sSysPublish) upsertMaterialImportUnitBlocks(ctx context.Context, task *sysin.MaterialImportTaskModel, units []*materialImportMessageUnit) error {
	for _, unit := range units {
		if unit == nil || materialImportIgnoredNotice(unit.RawText) {
			continue
		}
		if len(unit.Media) == 0 && strings.TrimSpace(unit.RawText) == "" {
			continue
		}
		if strings.TrimSpace(unit.RawText) == "" {
			if materialImportAllMediaType(unit.Media, "video") {
				if err := s.appendMaterialImportVerifyUnit(ctx, task, unit); err != nil {
					return err
				}
			}
			continue
		}
		if err := s.upsertMaterialImportDisplayUnit(ctx, task, unit); err != nil {
			return err
		}
	}
	return nil
}

func materialImportSplitLeadingUnits(units []*materialImportMessageUnit) (processable []*materialImportMessageUnit, pending []*materialImportMessageUnit) {
	if len(units) == 0 {
		return nil, nil
	}
	firstTextIndex := -1
	for index, unit := range units {
		if unit != nil && strings.TrimSpace(unit.RawText) != "" {
			firstTextIndex = index
			break
		}
	}
	if firstTextIndex < 0 {
		return nil, units
	}
	if firstTextIndex == 0 {
		return units, nil
	}
	return units[firstTextIndex:], units[:firstTextIndex]
}

func materialImportMergeAdjacentUnits(units []*materialImportMessageUnit) []*materialImportMessageUnit {
	if len(units) == 0 {
		return nil
	}
	merged := make([]*materialImportMessageUnit, 0, len(units))
	for _, unit := range units {
		if unit == nil {
			continue
		}
		if len(merged) > 0 {
			prev := merged[len(merged)-1]
			if prev != nil && prev.GroupedId != "" && prev.GroupedId == unit.GroupedId {
				if strings.TrimSpace(prev.RawText) == "" {
					prev.RawText = unit.RawText
				}
				if strings.TrimSpace(prev.UniqueKey) == "" {
					prev.UniqueKey = unit.UniqueKey
				}
				if prev.MessageAt == nil || (unit.MessageAt != nil && unit.MessageAt.Before(prev.MessageAt)) {
					prev.MessageAt = unit.MessageAt
				}
				prev.MessageId = materialImportMinMessageID(prev.MessageId, unit.MessageId)
				prev.Messages = append(prev.Messages, unit.Messages...)
				prev.Media = append(prev.Media, unit.Media...)
				continue
			}
		}
		merged = append(merged, unit)
	}
	return merged
}

func materialImportMinMessageID(left int, right int) int {
	if left <= 0 {
		return right
	}
	if right <= 0 {
		return left
	}
	if right < left {
		return right
	}
	return left
}

func materialImportBuildUnits(task *sysin.MaterialImportTaskModel, messages []*tg.Message) []*materialImportMessageUnit {
	units := make([]*materialImportMessageUnit, 0, len(messages))
	unitByKey := map[string]*materialImportMessageUnit{}
	for _, msg := range messages {
		if msg == nil || msg.ID <= 0 {
			continue
		}
		groupedId := gotdMessageGroupedId(msg)
		key := fmt.Sprintf("msg:%d", msg.ID)
		if groupedId != "" {
			key = "group:" + groupedId
		}
		unit := unitByKey[key]
		if unit == nil {
			unit = &materialImportMessageUnit{
				GroupedId: groupedId,
				UniqueKey: materialImportUnitUniqueKey(task, groupedId, msg.ID),
				MessageAt: gtime.NewFromTime(time.Unix(int64(msg.Date), 0)),
				MessageId: msg.ID,
			}
			unitByKey[key] = unit
			units = append(units, unit)
		}
		if unit.RawText == "" {
			unit.RawText = strings.TrimSpace(msg.Message)
		}
		unit.Messages = append(unit.Messages, msg.ID)
		unit.Media = append(unit.Media, gotdCollectMedia(msg, task.SourceChatId)...)
	}
	return units
}

func materialImportUnitUniqueKey(task *sysin.MaterialImportTaskModel, groupedId string, messageId int) string {
	if groupedId != "" {
		return fmt.Sprintf("task:%d:%d:%s:group:%s", task.Id, task.TgAccountId, task.SourceChatId, groupedId)
	}
	return fmt.Sprintf("task:%d:%d:%s:%d", task.Id, task.TgAccountId, task.SourceChatId, messageId)
}

func (s *sSysPublish) upsertMaterialImportDisplayUnit(ctx context.Context, task *sysin.MaterialImportTaskModel, unit *materialImportMessageUnit) error {
	items := materialImportMediaItemsWithPurpose(unit.Media, "display")
	mediaJson, mediaCount := collectMessageMediaJSON(items)
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
		"media_json":         mediaJson,
		"media_total":        mediaCount,
		"status":             sysin.MaterialImportStatusPending,
		"message_at":         unit.MessageAt,
		"updated_at":         now,
		"source_message_ids": materialImportUnitMessageIds(existing["source_message_ids"].String(), unit.Messages),
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
		data["media_json"], data["media_total"] = mergeCollectMediaJSON(existing["media_json"].String(), mediaJson)
	}
	_, err = pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", existing["id"].Int64()).Data(data).Update()
	return gerror.Wrap(err, "更新资料导入分组失败")
}

func (s *sSysPublish) appendMaterialImportVerifyUnit(ctx context.Context, task *sysin.MaterialImportTaskModel, unit *materialImportMessageUnit) error {
	mediaJson, _ := collectMessageMediaJSON(materialImportMediaItemsWithPurpose(unit.Media, "verify"))
	if strings.TrimSpace(mediaJson) == "" || strings.TrimSpace(mediaJson) == "[]" {
		return nil
	}
	messageAt := unit.MessageAt
	if messageAt == nil {
		messageAt = gtime.Now()
	}
	row, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).
		Where("task_id", task.Id).
		Where("source_chat_id", task.SourceChatId).
		Where("COALESCE(raw_text,'') <> ''").
		WhereLTE("message_at", messageAt).
		OrderDesc("message_at").
		OrderDesc("id").
		One()
	if err != nil {
		return gerror.Wrap(err, "读取验证视频归属资料失败")
	}
	if row.IsEmpty() {
		return nil
	}
	nextMediaJson, nextMediaCount := mergeCollectMediaJSON(row["media_json"].String(), mediaJson)
	_, err = pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", row["id"].Int64()).Data(g.Map{
		"media_json":         nextMediaJson,
		"media_total":        nextMediaCount,
		"media_done":         0,
		"media_failed":       0,
		"source_message_ids": materialImportUnitMessageIds(row["source_message_ids"].String(), unit.Messages),
		"status":             sysin.MaterialImportStatusPending,
		"profile_id":         0,
		"task_profile_id":    0,
		"error_message":      "",
		"updated_at":         gtime.Now(),
	}).Update()
	return gerror.Wrap(err, "合并验证视频到导入资料失败")
}

func materialImportUnitMessageIds(existing string, ids []int) string {
	for _, id := range ids {
		existing = materialImportMessageIds(existing, gconv.Int(id))
	}
	return existing
}
