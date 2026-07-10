package sys

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

var collectMediaFloodWaitPattern = regexp.MustCompile(`(?i)FLOOD_WAIT[_ ]?\(?([0-9]+)\)?`)

func (s *sSysPublish) forwardCollectMediaToBackup(ctx context.Context, event gdb.Record, items []collectMediaItem) ([]collectMediaItem, bool, error) {
	backups, err := s.collectEventBackupChannels(ctx, event)
	if err != nil || len(backups) == 0 {
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
	ids, indexMap := collectGotdMessageIds(items)
	if len(ids) == 0 {
		return items, false, nil
	}
	var lastErr error
	for _, backup := range rotateCollectBackupChannels(backups, event["id"].Int64()) {
		backupPeer, peerErr := collectInputPeerChannel(backup)
		if peerErr != nil {
			lastErr = peerErr
			continue
		}
		s.appendCollectEventLogForRecord(ctx, event, "media", "forwarding", "开始转存媒体到备份频道", collectForwardMeta(backup.ChannelId, ids))
		forwarded := make([]int, 0, len(ids))
		usedRuntime, forwardErr := s.forwardCollectMediaByAccountRuntime(ctx, event["tg_account_id"].Int64(), sourcePeer, backupPeer, ids, &forwarded)
		if forwardErr != nil {
			lastErr = forwardErr
			s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "备份频道转存失败，尝试下一个备份频道", backup.ChannelId+": "+forwardErr.Error())
			continue
		}
		if usedRuntime {
			s.appendCollectEventLogForRecord(ctx, event, "media", "forwarded", "账号采集运行时已完成媒体转存", collectForwardMeta(backup.ChannelId, forwarded))
			if err = s.validateCollectBackupCopyRefs(ctx, event, backup.ChannelId, forwarded); err != nil {
				lastErr = err
				s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "备份频道媒体不可被Bot复制，尝试下一个备份频道", backup.ChannelId+": "+err.Error())
				continue
			}
			return applyCollectBackupForwardResult(items, backup.ChannelId, ids, indexMap, forwarded)
		}
		if event["source_type"].String() == sysin.CollectSourceTypeAccount {
			return items, false, newCollectMediaRetryError("账号采集运行时未就绪，等待重试媒体转存", 15*time.Second)
		}
		if forwardedItems, changed, fallbackErr := s.forwardCollectMediaWithStandaloneClient(ctx, event, sourcePeer, backupPeer, backup.ChannelId, ids, indexMap, items); fallbackErr != nil {
			lastErr = fallbackErr
			s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "独立客户端转存失败，尝试下一个备份频道", backup.ChannelId+": "+fallbackErr.Error())
			continue
		} else if changed {
			refs := collectCopyMessageIds(forwardedItems, backup.ChannelId)
			if err = s.validateCollectBackupCopyRefs(ctx, event, backup.ChannelId, refs); err != nil {
				lastErr = err
				s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "备份频道媒体不可被Bot复制，尝试下一个备份频道", backup.ChannelId+": "+err.Error())
				continue
			}
			return forwardedItems, true, nil
		}
	}
	if lastErr != nil {
		if delay, ok := collectMediaFloodWaitDelay(lastErr); ok {
			message := fmt.Sprintf("备份频道转存触发Telegram限流，等待%s后重试", delay)
			s.appendCollectEventLogForRecord(ctx, event, "media", "pending", message, lastErr.Error())
			return items, false, newCollectMediaRetryError(message, delay)
		}
		s.appendCollectEventLogForRecord(ctx, event, "media", "failed", "转存媒体到备份频道失败", lastErr.Error())
		return items, false, lastErr
	}
	return items, false, nil
}

func collectMediaFloodWaitDelay(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	message := err.Error()
	matches := collectMediaFloodWaitPattern.FindStringSubmatch(message)
	if len(matches) < 2 {
		lower := strings.ToLower(message)
		if !strings.Contains(lower, "too many requests") {
			return 0, false
		}
		return time.Minute, true
	}
	seconds, scanErr := strconv.Atoi(matches[1])
	if scanErr != nil || seconds <= 0 {
		return time.Minute, true
	}
	return time.Duration(seconds+2) * time.Second, true
}

func (s *sSysPublish) validateCollectBackupCopyRefs(ctx context.Context, event gdb.Record, backupChannelId string, messageIds []int) error {
	messageIds = positiveUniqueInts(messageIds)
	if len(messageIds) == 0 {
		return gerror.New("备份频道未返回可复制消息ID")
	}
	bots, err := s.collectBackupCopyValidationBots(ctx, event, backupChannelId)
	if err != nil {
		return err
	}
	if len(bots) == 0 {
		return gerror.New("备份频道未配置可校验Bot")
	}
	chatId := normalizeTelegramChannelChatID(backupChannelId)
	for _, botToken := range bots {
		bot, err := s.telegramBot(ctx, botToken)
		if err != nil {
			return err
		}
		for _, messageId := range messageIds {
			copied, err := bot.CopyMessage(ctx, &tgbot.CopyMessageParams{
				ChatID:              chatId,
				FromChatID:          chatId,
				MessageID:           messageId,
				DisableNotification: true,
			})
			if err != nil {
				return gerror.Wrapf(err, "备份频道消息不可复制 chat:%s message:%d", chatId, messageId)
			}
			if copied == nil || copied.ID <= 0 {
				return gerror.Newf("备份频道消息复制校验无返回 chat:%s message:%d", chatId, messageId)
			}
			if _, err = bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: chatId, MessageID: copied.ID}); err != nil {
				g.Log().Warningf(ctx, "清理备份频道复制校验消息失败 chat:%s message:%d err:%+v", chatId, copied.ID, err)
			}
		}
	}
	return nil
}

func (s *sSysPublish) collectBackupCopyValidationBots(ctx context.Context, event gdb.Record, backupChannelId string) ([]string, error) {
	botIds, err := s.collectBackupCopyValidationBotIds(ctx, event, backupChannelId)
	if err != nil {
		return nil, err
	}
	tokens := make([]string, 0, len(botIds))
	seen := make(map[string]struct{}, len(botIds))
	for _, botId := range botIds {
		bot, err := s.getBotById(ctx, botId, event["tenant_id"].Int64())
		if err != nil {
			return nil, err
		}
		token := strings.TrimSpace(bot.BotToken)
		if token == "" {
			return nil, gerror.Newf("Bot Token未配置 bot:%d", botId)
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func (s *sSysPublish) collectBackupCopyValidationBotIds(ctx context.Context, event gdb.Record, backupChannelId string) ([]int64, error) {
	chatIds := collectBackupChannelChatCandidates(backupChannelId)
	var rows []struct {
		BotIdJson string `json:"botIdJson"`
	}
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("bot_id_json").
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("publish_direction", "backup").
		Where("status", 1).
		WhereIn("target_chat_id", chatIds).
		WhereNull("deleted_at").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取备份频道Bot配置失败")
	}
	ids := make([]int64, 0)
	for _, row := range rows {
		ids = append(ids, decodeBotIds(row.BotIdJson)...)
	}
	ids = positiveUniqueInt64s(ids)
	if len(ids) > 0 {
		return ids, nil
	}
	bots, err := s.enabledBots(ctx, event["tenant_id"].Int64())
	if err != nil {
		return nil, err
	}
	for _, bot := range bots {
		if bot != nil {
			ids = append(ids, bot.Id)
		}
	}
	return positiveUniqueInt64s(ids), nil
}

func collectBackupChannelChatCandidates(channelId string) []string {
	normalized := normalizeTelegramChannelChatID(channelId)
	positive := strings.TrimPrefix(normalized, "-100")
	candidates := []string{strings.TrimSpace(channelId), normalized, positive}
	seen := make(map[string]struct{}, len(candidates))
	list := make([]string, 0, len(candidates))
	for _, item := range candidates {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		list = append(list, item)
	}
	return list
}

func collectCopyMessageIds(items []collectMediaItem, backupChannelId string) []int {
	chatId := normalizeTelegramChannelChatID(backupChannelId)
	ids := make([]int, 0, len(items))
	for _, item := range items {
		ref, ok := telegramCopyMediaRefFromFileId(item.FileId)
		if !ok || ref.ChatId != chatId || ref.MessageId <= 0 {
			continue
		}
		ids = append(ids, ref.MessageId)
	}
	return positiveUniqueInts(ids)
}

func positiveUniqueInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	list := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		list = append(list, value)
	}
	sort.Ints(list)
	return list
}

func positiveUniqueInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	list := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		list = append(list, value)
	}
	return list
}

func (s *sSysPublish) forwardCollectMediaWithStandaloneClient(ctx context.Context, event gdb.Record, sourcePeer *tg.InputPeerChannel, backupPeer *tg.InputPeerChannel, backupChannelId string, ids []int, indexMap map[int][]int, items []collectMediaItem) ([]collectMediaItem, bool, error) {
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
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	forwarded := make([]int, 0, len(ids))
	err = runCollectBackupForwardWithTimeout(runCtx, 90*time.Second, func(runCtx context.Context) error {
		return client.Run(runCtx, func(runCtx context.Context) error {
			updates, err := invokeCollectForwardWithoutUpdates(runCtx, client, sourcePeer, backupPeer, ids)
			if err != nil {
				return gerror.Wrap(err, "转发采集媒体到备份频道失败")
			}
			forwarded = collectForwardedMessageIds(updates)
			sort.Ints(forwarded)
			if len(forwarded) < len(ids) {
				return gerror.New("转发采集媒体到备份频道成功，但未读取到完整消息ID")
			}
			return nil
		})
	})
	if err != nil {
		return items, false, err
	}
	return applyCollectBackupForwardResult(items, backupChannelId, ids, indexMap, forwarded)
}

func (s *sSysPublish) forwardCollectMediaByAccountRuntime(ctx context.Context, tgAccountId int64, sourcePeer *tg.InputPeerChannel, backupPeer *tg.InputPeerChannel, ids []int, forwarded *[]int) (bool, error) {
	return s.executeAccountCollectOperation(ctx, tgAccountId, 90*time.Second, func(runCtx context.Context, client *telegram.Client) error {
		updates, err := invokeCollectForwardWithoutUpdates(runCtx, client, sourcePeer, backupPeer, ids)
		if err != nil {
			return gerror.Wrap(err, "转发采集媒体到备份频道失败")
		}
		*forwarded = collectForwardedMessageIds(updates)
		sort.Ints(*forwarded)
		if len(*forwarded) < len(ids) {
			return gerror.New("转发采集媒体到备份频道成功，但未读取到完整消息ID")
		}
		return nil
	})
}

func invokeCollectForwardWithoutUpdates(ctx context.Context, client *telegram.Client, sourcePeer *tg.InputPeerChannel, backupPeer *tg.InputPeerChannel, ids []int) (tg.UpdatesClass, error) {
	req := &tg.MessagesForwardMessagesRequest{
		FromPeer: sourcePeer,
		ToPeer:   backupPeer,
		ID:       ids,
		RandomID: collectForwardRandomIds(ids),
		Silent:   true,
	}
	return client.API().MessagesForwardMessages(ctx, req)
}

func applyCollectBackupForwardResult(items []collectMediaItem, backupChannelId string, ids []int, indexMap map[int][]int, forwarded []int) ([]collectMediaItem, bool, error) {
	for i, sourceId := range ids {
		itemIndexes := indexMap[sourceId]
		if len(itemIndexes) == 0 || i >= len(forwarded) {
			continue
		}
		copyFileId := telegramCopyMediaFileId(backupChannelId, forwarded[i])
		for _, itemIndex := range itemIndexes {
			items[itemIndex].FileId = copyFileId
			items[itemIndex].FileUrl = ""
			items[itemIndex].StoragePath = ""
		}
	}
	return items, true, nil
}

func runCollectBackupForwardWithTimeout(ctx context.Context, timeout time.Duration, run func(context.Context) error) error {
	if timeout <= 0 {
		return run(ctx)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- run(runCtx)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		cancel()
		return gerror.New("转发采集媒体到备份频道超时")
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}

func (s *sSysPublish) collectEventBackupChannels(ctx context.Context, event gdb.Record) ([]*sysin.ChannelCacheModel, error) {
	channelIds, err := s.collectEventBackupChannelIds(ctx, event)
	if err != nil || len(channelIds) == 0 {
		return nil, err
	}
	channels := make([]*sysin.ChannelCacheModel, 0, len(channelIds))
	for _, channelId := range channelIds {
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
			return nil, gerror.Wrap(err, "读取备份频道配置失败")
		}
		if strings.TrimSpace(channel.TargetChatId) == "" {
			continue
		}
		cache, cacheErr := s.tgChannelCacheByChannelId(ctx, event["tenant_id"].Int64(), event["tg_account_id"].Int64(), channel.TargetChatId)
		if cacheErr != nil {
			return nil, gerror.Wrap(cacheErr, "读取备份频道缓存失败")
		}
		channels = append(channels, cache)
	}
	return channels, nil
}

func rotateCollectBackupChannels(channels []*sysin.ChannelCacheModel, eventId int64) []*sysin.ChannelCacheModel {
	if len(channels) <= 1 {
		return channels
	}
	start := int(eventId % int64(len(channels)))
	rotated := make([]*sysin.ChannelCacheModel, 0, len(channels))
	rotated = append(rotated, channels[start:]...)
	rotated = append(rotated, channels[:start]...)
	return rotated
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
	candidateRows, _ := s.precheckCollectEventRules(event, rows)
	ids := make([]int64, 0)
	for _, row := range candidateRows {
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
	sort.Ints(ids)
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

func collectForwardMeta(channelId string, ids []int) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, strconv.Itoa(id))
	}
	return "channel=" + strings.TrimSpace(channelId) + " ids=" + strings.Join(values, ",")
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
