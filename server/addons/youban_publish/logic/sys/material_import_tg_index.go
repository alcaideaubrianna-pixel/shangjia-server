package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) ensureMaterialImportTelegramIndex(ctx context.Context, task *sysin.MaterialImportTaskModel, group *sysin.MaterialImportGroupModel, profileId int64) error {
	if task == nil || group == nil || profileId <= 0 {
		return nil
	}
	ids := materialImportSourceMessageIds(group.SourceMessageIds)
	if len(ids) == 0 {
		return nil
	}
	channel, err := s.materialImportIndexChannel(ctx, task)
	if err != nil {
		_ = s.appendMaterialImportPublishLog(ctx, task, profileId, "index_skipped", err.Error())
		return nil
	}
	botId := firstPositiveId(decodeBotIds(channel.BotIdJson))
	if botId <= 0 {
		_ = s.appendMaterialImportPublishLog(ctx, task, profileId, "index_skipped", "导入源频道未绑定Bot，无法建立下架删除索引")
		return nil
	}
	jobId, err := s.ensureMaterialImportTelegramJob(ctx, task, group, channel, profileId, botId)
	if err != nil {
		return err
	}
	rows, err := s.materialImportCachedMessages(ctx, task, ids)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err = s.ensureMaterialImportTelegramMessage(ctx, task, row, jobId, channel, profileId, botId); err != nil {
			return err
		}
	}
	_ = s.appendMaterialImportPublishLog(ctx, task, profileId, "indexed", fmt.Sprintf("导入源TG消息索引已写入：%d条", len(rows)))
	return nil
}

func (s *sSysPublish) materialImportIndexChannel(ctx context.Context, task *sysin.MaterialImportTaskModel) (tgMessageRepairChannel, error) {
	var channel tgMessageRepairChannel
	sourceChatId := normalizeTelegramChannelChatID(task.SourceChatId)
	if sourceChatId == "" {
		return channel, gerror.New("导入源频道ID为空，无法建立下架删除索引")
	}
	sourceChatKey := strings.TrimPrefix(sourceChatId, "-100")
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tg_account_id,target_chat_id,bot_id_json").
		Where("tenant_id", task.TenantId).
		Where("tg_account_id", task.TgAccountId).
		Where("REPLACE(target_chat_id, '-100', '')", sourceChatKey).
		Where("status", 1).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&channel)
	if err != nil {
		return channel, gerror.Wrap(err, "读取导入源频道配置失败")
	}
	if channel.Id <= 0 {
		return s.ensureMaterialImportIndexChannel(ctx, task, sourceChatKey)
	}
	channel.TargetChatId = normalizeTelegramChannelChatID(channel.TargetChatId)
	return channel, nil
}

func (s *sSysPublish) ensureMaterialImportIndexChannel(ctx context.Context, task *sysin.MaterialImportTaskModel, sourceChatKey string) (tgMessageRepairChannel, error) {
	var channel tgMessageRepairChannel
	cache, err := s.materialImportSourceChannelCache(ctx, task, sourceChatKey)
	if err != nil {
		return channel, err
	}
	botId, err := s.materialImportDefaultBotID(ctx, task.TenantId)
	if err != nil {
		return channel, err
	}
	if botId <= 0 {
		return channel, gerror.New("租户没有可用Bot，无法建立下架删除索引")
	}
	if ok, err := s.materialImportBotHasSourceChannel(ctx, botId, sourceChatKey); err != nil {
		return channel, err
	} else if !ok {
		// The index is still useful, but deletion will only work after the Bot is in the source channel.
	}
	now := gtime.Now()
	botJSON, _ := encodeBotIds([]int64{botId})
	id, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":             task.TenantId,
		"merchant_id":           task.TenantId,
		"tg_account_id":         task.TgAccountId,
		"channel_title":         firstNonEmpty(cache.ChannelTitle, task.SourceTitle, "资料导入源频道"),
		"channel_username":      cache.ChannelUsername,
		"target_chat_id":        sourceChatKey,
		"publish_direction":     "up",
		"cycle_publish_enabled": 0,
		"cycle_publish_days":    4,
		"cycle_publish_time":    "",
		"is_default_selected":   0,
		"publish_visible":       0,
		"bot_id_json":           botJSON,
		"remark":                "资料导入源频道自动配置，用于下架删除源消息",
		"status":                1,
		"created_by":            task.UpdatedBy,
		"updated_by":            task.UpdatedBy,
		"created_at":            now,
		"updated_at":            now,
	}).InsertAndGetId()
	if err != nil {
		return channel, gerror.Wrap(err, "创建导入源频道配置失败")
	}
	return tgMessageRepairChannel{
		Id:           id,
		TgAccountId:  task.TgAccountId,
		TargetChatId: normalizeTelegramChannelChatID(sourceChatKey),
		BotIdJson:    botJSON,
	}, nil
}

func (s *sSysPublish) materialImportSourceChannelCache(ctx context.Context, task *sysin.MaterialImportTaskModel, sourceChatKey string) (*sysin.ChannelCacheModel, error) {
	var cache *sysin.ChannelCacheModel
	err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,tg_account_id,channel_id,channel_title,channel_username,management_role,can_post_messages,can_invite_users,can_add_admins").
		Where("tenant_id", task.TenantId).
		Where("tg_account_id", task.TgAccountId).
		Where("REPLACE(channel_id, '-100', '')", sourceChatKey).
		Scan(&cache)
	if err != nil {
		return nil, gerror.Wrap(err, "读取导入源频道缓存失败")
	}
	if cache == nil || cache.Id <= 0 {
		return nil, gerror.New("导入源频道未刷新缓存，无法自动建立下架删除索引")
	}
	return cache, nil
}

func (s *sSysPublish) materialImportDefaultBotID(ctx context.Context, tenantId int64) (int64, error) {
	value, err := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		Fields("id").
		Where("tenant_id", tenantId).
		Where("status", 1).
		WhereNull("deleted_at").
		OrderAsc("id").
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取默认Bot失败")
	}
	return value.Int64(), nil
}

func (s *sSysPublish) materialImportBotHasSourceChannel(ctx context.Context, botId int64, sourceChatKey string) (bool, error) {
	count, err := g.DB().Model("hg_youban_bot_channel_cache").Safe().Ctx(ctx).
		Where("bot_id", botId).
		Where("REPLACE(chat_id, '-100', '')", sourceChatKey).
		Count()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return false, nil
		}
		return false, gerror.Wrap(err, "检查Bot导入源频道缓存失败")
	}
	return count > 0, nil
}

func (s *sSysPublish) ensureMaterialImportTelegramJob(ctx context.Context, task *sysin.MaterialImportTaskModel, group *sysin.MaterialImportGroupModel, channel tgMessageRepairChannel, profileId int64, botId int64) (int64, error) {
	operationNo := "material_import:" + gconv.String(group.Id)
	var existing struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id").
		Where("profile_id", profileId).
		Where("operation_no", operationNo).
		Where("channel_id", channel.Id).
		Scan(&existing); err != nil {
		return 0, gerror.Wrap(err, "读取导入TG消息索引任务失败")
	}
	now := gtime.Now()
	data := g.Map{
		"task_id":        nil,
		"tenant_id":      task.TenantId,
		"merchant_id":    task.TenantId,
		"account_id":     task.AccountId,
		"profile_id":     profileId,
		"channel_id":     channel.Id,
		"bot_id":         botId,
		"target_chat_id": channel.TargetChatId,
		"status":         "sent",
		"error_message":  "",
		"sent_at":        group.MessageAt,
		"updated_at":     now,
	}
	if existing.Id > 0 {
		_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", existing.Id).Data(data).Update()
		return existing.Id, gerror.Wrap(err, "更新导入TG消息索引任务失败")
	}
	data["operation_no"] = operationNo
	data["retry_count"] = 0
	data["created_at"] = now
	id, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	return id, gerror.Wrap(err, "创建导入TG消息索引任务失败")
}

func (s *sSysPublish) materialImportCachedMessages(ctx context.Context, task *sysin.MaterialImportTaskModel, ids []int64) ([]tgMessageRepairCacheRow, error) {
	var rows []tgMessageRepairCacheRow
	channelID := gconv.Int64(strings.TrimPrefix(normalizeTelegramChannelChatID(task.SourceChatId), "-100"))
	err := g.DB().Model(publishTgMessageCacheTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,tg_account_id,channel_id,target_chat_id,tg_message_id,message_text,message_date,media_type,media_group_id").
		Where("tenant_id", task.TenantId).
		Where("tg_account_id", task.TgAccountId).
		Where("channel_id", channelID).
		WhereIn("tg_message_id", ids).
		OrderAsc("tg_message_id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取导入源TG消息缓存失败")
	}
	return rows, nil
}

func (s *sSysPublish) ensureMaterialImportTelegramMessage(ctx context.Context, task *sysin.MaterialImportTaskModel, row tgMessageRepairCacheRow, jobId int64, channel tgMessageRepairChannel, profileId int64, botId int64) error {
	if row.TgMessageId <= 0 {
		return nil
	}
	count, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("job_id", jobId).
		Where("tg_message_id", row.TgMessageId).
		Where("status", "sent").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查导入TG消息索引失败")
	}
	now := gtime.Now()
	data := g.Map{
		"task_id":        nil,
		"tenant_id":      task.TenantId,
		"account_id":     task.AccountId,
		"profile_id":     profileId,
		"bot_id":         botId,
		"target_chat_id": channel.TargetChatId,
		"media_group_id": row.MediaGroupId,
		"purpose":        "import",
		"status":         "sent",
		"sent_at":        row.MessageDate,
		"updated_at":     now,
	}
	if count > 0 {
		_, err = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
			Where("job_id", jobId).
			Where("tg_message_id", row.TgMessageId).
			Data(data).
			Update()
		return gerror.Wrap(err, "更新导入TG消息索引失败")
	}
	data["job_id"] = jobId
	data["tg_message_id"] = row.TgMessageId
	data["media_id"] = 0
	data["tg_file_id"] = ""
	data["created_at"] = now
	_, err = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).Data(data).Insert()
	return gerror.Wrap(err, "保存导入TG消息索引失败")
}

func materialImportSourceMessageIds(raw string) []int64 {
	ids := make([]int64, 0)
	for _, item := range strings.Split(raw, ",") {
		if id := gconv.Int64(strings.TrimSpace(item)); id > 0 {
			ids = append(ids, id)
		}
	}
	return uniqueIds(ids)
}
