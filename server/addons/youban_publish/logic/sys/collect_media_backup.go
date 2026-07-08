package sys

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/tg"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) forwardCollectMediaToBackup(ctx context.Context, event gdb.Record, items []collectMediaItem) ([]collectMediaItem, bool, error) {
	backup, ok, err := s.collectEventBackupChannel(ctx, event)
	if err != nil || !ok {
		return items, false, err
	}
	source, err := s.tgChannelCacheByChannelId(ctx, event["tenant_id"].Int64(), event["tg_account_id"].Int64(), event["source_chat_id"].String())
	if err != nil {
		return items, false, gerror.Wrap(err, "读取采集来源频道缓存失败")
	}
	sourcePeer, err := collectInputPeerChannel(source)
	if err != nil {
		return items, false, err
	}
	backupPeer, err := collectInputPeerChannel(backup)
	if err != nil {
		return items, false, err
	}
	ids, indexMap := collectGotdMessageIds(items)
	if len(ids) == 0 {
		return items, false, nil
	}
	account, err := s.accountCollectTgAccount(ctx, event["tg_account_id"].Int64())
	if err != nil {
		return items, false, err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return items, false, err
	}
	client, err := s.newAccountCollectClient(ctx, conf, account, tg.NewUpdateDispatcher())
	if err != nil {
		return items, false, err
	}
	forwarded := make([]int, 0, len(ids))
	err = client.Run(ctx, func(runCtx context.Context) error {
		updates, err := client.API().MessagesForwardMessages(runCtx, &tg.MessagesForwardMessagesRequest{
			FromPeer:          sourcePeer,
			ToPeer:            backupPeer,
			ID:                ids,
			RandomID:          collectForwardRandomIds(ids),
			DropAuthor:        true,
			DropMediaCaptions: true,
			Silent:            true,
		})
		if err != nil {
			return gerror.Wrap(err, "转发采集媒体到备份频道失败")
		}
		forwarded = collectForwardedMessageIds(updates)
		if len(forwarded) < len(ids) {
			return gerror.New("转发采集媒体到备份频道成功，但未读取到完整消息ID")
		}
		return nil
	})
	if err != nil {
		return items, false, err
	}
	for i, sourceId := range ids {
		itemIndexes := indexMap[sourceId]
		if len(itemIndexes) == 0 || i >= len(forwarded) {
			continue
		}
		copyFileId := telegramCopyMediaFileId(backup.ChannelId, forwarded[i])
		for _, itemIndex := range itemIndexes {
			items[itemIndex].FileId = copyFileId
			items[itemIndex].FileUrl = ""
			items[itemIndex].StoragePath = ""
		}
	}
	return items, true, nil
}

func (s *sSysPublish) collectEventBackupChannel(ctx context.Context, event gdb.Record) (*sysin.ChannelCacheModel, bool, error) {
	channelIds, err := s.collectEventBackupChannelIds(ctx, event)
	if err != nil || len(channelIds) == 0 {
		return nil, false, err
	}
	channelId := channelIds[int(event["id"].Int64()%int64(len(channelIds)))]
	var channel struct {
		TargetChatId string `json:"targetChatId"`
	}
	if err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("target_chat_id").
		Where("id", channelId).
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("publish_direction", "backup").
		Where("status", 1).
		WhereNull("deleted_at").
		Scan(&channel); err != nil {
		return nil, false, gerror.Wrap(err, "读取备份频道配置失败")
	}
	if strings.TrimSpace(channel.TargetChatId) == "" {
		return nil, false, nil
	}
	cache, err := s.tgChannelCacheByChannelId(ctx, event["tenant_id"].Int64(), event["tg_account_id"].Int64(), channel.TargetChatId)
	if err != nil {
		return nil, false, gerror.Wrap(err, "读取备份频道缓存失败")
	}
	return cache, true, nil
}

func (s *sSysPublish) collectEventBackupChannelIds(ctx context.Context, event gdb.Record) ([]int64, error) {
	if err := ensureCollectRuleColumns(ctx); err != nil {
		return nil, err
	}
	var bindRows []struct {
		RuleId int64 `json:"ruleId"`
	}
	if err := g.DB().Model(pdao.YoubanPublishCollectSourceRule.Table()).Safe().Ctx(ctx).
		Fields("rule_id").
		Where("source_id", event["source_id"].Int64()).
		Where("status", 1).
		OrderAsc("sort").
		Scan(&bindRows); err != nil {
		return nil, gerror.Wrap(err, "读取采集源绑定规则失败")
	}
	mod := g.DB().Model(pdao.YoubanPublishCollectRule.Table()).Safe().Ctx(ctx).
		Fields("backup_channel_id,backup_channel_id_json").
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		Where("status", 1).
		Where("(backup_channel_id > 0 OR COALESCE(backup_channel_id_json, '') NOT IN ('', '[]', 'null'))").
		WhereNull("deleted_at")
	if len(bindRows) > 0 {
		ids := make([]int64, 0, len(bindRows))
		for _, row := range bindRows {
			ids = append(ids, row.RuleId)
		}
		mod = mod.WhereIn("id", ids)
	} else {
		mod = mod.Where("global_enabled", 1)
	}
	rows, err := mod.OrderAsc("sort").OrderAsc("id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集备份频道失败")
	}
	ids := make([]int64, 0)
	for _, row := range rows {
		ids = append(ids, decodeInt64JSON(row["backup_channel_id_json"].String())...)
		if row["backup_channel_id"].Int64() > 0 {
			ids = append(ids, row["backup_channel_id"].Int64())
		}
	}
	return uniqueIds(ids), nil
}

func collectInputPeerChannel(channel *sysin.ChannelCacheModel) (*tg.InputPeerChannel, error) {
	if channel == nil {
		return nil, gerror.New("频道缓存为空")
	}
	channelId, err := strconv.ParseInt(strings.TrimSpace(channel.ChannelId), 10, 64)
	if err != nil || channelId <= 0 {
		return nil, gerror.New("频道ID无效")
	}
	accessHash, err := strconv.ParseInt(strings.TrimSpace(channel.AccessHash), 10, 64)
	if err != nil {
		return nil, gerror.New("频道AccessHash无效")
	}
	return &tg.InputPeerChannel{ChannelID: channelId, AccessHash: accessHash}, nil
}

func collectGotdMessageIds(items []collectMediaItem) ([]int, map[int][]int) {
	ids := make([]int, 0, len(items))
	indexes := make(map[int][]int, len(items))
	seen := map[int]struct{}{}
	for index, item := range items {
		_, messageId, ok := parseGotdCollectFileId(item.FileId)
		if !ok {
			continue
		}
		indexes[messageId] = append(indexes[messageId], index)
		if _, exists := seen[messageId]; exists {
			continue
		}
		seen[messageId] = struct{}{}
		ids = append(ids, messageId)
	}
	return ids, indexes
}

func parseGotdCollectFileId(fileId string) (string, int, bool) {
	fileId = strings.TrimSpace(fileId)
	if !strings.HasPrefix(fileId, "gotd:") {
		return "", 0, false
	}
	raw := strings.TrimPrefix(fileId, "gotd:")
	index := strings.LastIndex(raw, ":")
	if index <= 0 || index >= len(raw)-1 {
		return "", 0, false
	}
	messageId, err := strconv.Atoi(raw[index+1:])
	if err != nil || messageId <= 0 {
		return "", 0, false
	}
	return raw[:index], messageId, true
}

func collectForwardRandomIds(ids []int) []int64 {
	now := time.Now().UnixNano()
	randomIds := make([]int64, 0, len(ids))
	for index, id := range ids {
		randomIds = append(randomIds, now+int64(id)+int64(index+1)*1000)
	}
	return randomIds
}

func collectForwardedMessageIds(updates tg.UpdatesClass) []int {
	list := make([]int, 0)
	for _, update := range collectUpdatesList(updates) {
		switch item := update.(type) {
		case *tg.UpdateNewChannelMessage:
			if msg, ok := item.Message.(*tg.Message); ok && msg.ID > 0 {
				list = append(list, msg.ID)
			}
		case *tg.UpdateNewMessage:
			if msg, ok := item.Message.(*tg.Message); ok && msg.ID > 0 {
				list = append(list, msg.ID)
			}
		}
	}
	return list
}

func collectUpdatesList(updates tg.UpdatesClass) []tg.UpdateClass {
	switch data := updates.(type) {
	case *tg.Updates:
		return data.GetUpdates()
	case *tg.UpdatesCombined:
		return data.GetUpdates()
	default:
		return nil
	}
}
