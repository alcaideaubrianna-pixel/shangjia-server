package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"github.com/gogf/gf/v2/util/gconv"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	hglock "hotgo/internal/library/hgrds/lock"
)

const materialImportPagesPerRun = 10

func (s *sSysPublish) ExecuteMaterialImportTask(ctx context.Context, taskId int64) error {
	lock := hglock.NewConfig(5*time.Minute, 200*time.Millisecond).
		Mutex(fmt.Sprintf("youban_publish:material_import:%d", taskId))
	if err := lock.Lock(ctx); err != nil {
		return gerror.Wrap(err, "获取资料导入任务锁失败")
	}
	defer s.releaseTelegramChannelLease(context.Background(), lock)

	task, err := s.materialImportTaskByPrimary(ctx, taskId)
	if err != nil {
		return err
	}
	if task.Status == sysin.MaterialImportStatusCanceled || task.Status == sysin.MaterialImportStatusSuccess {
		return nil
	}
	if err = s.materialImportMarkRunning(ctx, task.Id, task.UpdatedBy, materialImportNextStage(task)); err != nil {
		return err
	}
	if err = s.executeMaterialImport(ctx, task); err != nil {
		if isTelegramPermanentAccountAuthError(err) {
			s.handleTgAccountPermanentAuthError(context.Background(), task.TgAccountId, task.UpdatedBy, telegramPermanentAccountAuthMessage(err), err)
		}
		if pause, ok := err.(*collectHistoryPauseError); ok {
			delay := int(pause.delay.Seconds())
			if delay <= 0 {
				delay = 60
			}
			_ = s.materialImportMarkWaiting(ctx, task.Id, task.UpdatedBy, delay, pause.Error(), task.Stage)
			return s.enqueueMaterialImportTask(ctx, task.Id, time.Duration(delay)*time.Second)
		}
		_ = s.materialImportMarkFailed(ctx, task.Id, task.UpdatedBy, err.Error())
		return err
	}
	return nil
}

func (s *sSysPublish) executeMaterialImport(ctx context.Context, task *sysin.MaterialImportTaskModel) error {
	if task == nil || task.Id <= 0 {
		return gerror.New("资料导入任务不存在")
	}
	if err := s.materialImportEnsureNotCanceled(ctx, task.Id); err != nil {
		return err
	}
	switch materialImportNextStage(task) {
	case sysin.MaterialImportStageMedia:
		return s.executeMaterialImportMedia(ctx, task.Id)
	default:
		return s.executeMaterialImportPull(ctx, task)
	}
}

func (s *sSysPublish) executeMaterialImportPull(ctx context.Context, task *sysin.MaterialImportTaskModel) error {
	cache, err := s.tgChannelCacheByChannelId(ctx, task.TenantId, task.TgAccountId, task.SourceChatId)
	if err != nil {
		return err
	}
	peer, err := collectInputPeerChannel(cache)
	if err != nil {
		return err
	}
	run := func(runCtx context.Context, client *telegram.Client) error {
		if _, selfErr := client.Self(runCtx); selfErr != nil {
			return selfErr
		}
		return s.pullMaterialImportPages(runCtx, client, task, peer, cache)
	}
	usedRuntime, err := s.executeAccountCollectOperation(ctx, task.TgAccountId, 50*time.Minute, run)
	if err != nil || usedRuntime {
		return err
	}
	tgAccount, err := s.accountCollectTgAccount(ctx, task.TgAccountId)
	if err != nil {
		return err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return err
	}
	client, err := s.newAccountCollectClient(ctx, conf, tgAccount, tg.NewUpdateDispatcher())
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, 50*time.Minute)
	defer cancel()
	return client.Run(runCtx, func(clientCtx context.Context) error {
		return run(clientCtx, client)
	})
}

func (s *sSysPublish) pullMaterialImportPages(ctx context.Context, client *telegram.Client, task *sysin.MaterialImportTaskModel, peer *tg.InputPeerChannel, cache *sysin.ChannelCacheModel) error {
	offsetID := int(task.PullOffsetId)
	latestCachedMessageID, err := s.materialImportLatestCachedMessageID(ctx, task.TenantId, task.TgAccountId, cache)
	if err != nil {
		return err
	}
	cutoff := gtime.NewFromTime(time.Now().Add(-time.Duration(task.PullLimitDays) * 24 * time.Hour))
	shouldFinish := false
	pendingUnits := make([]*materialImportMessageUnit, 0, 16)
	for page := 0; page < materialImportPagesPerRun; page++ {
		if err := s.materialImportEnsureNotCanceled(ctx, task.Id); err != nil {
			return err
		}
		messages, err := materialImportHistoryPage(ctx, client, peer, offsetID)
		if err != nil {
			return err
		}
		if len(messages) == 0 {
			shouldFinish = true
			break
		}
		if err = s.materialImportAddPulledMessages(ctx, task.Id, len(messages)); err != nil {
			return err
		}
		nextOffset, stop, units, err := s.ingestMaterialImportMessages(ctx, task, messages, cutoff, cache, latestCachedMessageID)
		if err != nil {
			return err
		}
		if nextOffset > 0 {
			offsetID = nextOffset
		}
		pageUnits, carryUnits := materialImportSplitLeadingUnits(units)
		if len(pageUnits) > 0 || len(pendingUnits) > 0 {
			pageUnits = materialImportMergeAdjacentUnits(append(pageUnits, pendingUnits...))
			if len(pageUnits) > 0 {
				if err := s.upsertMaterialImportUnitBlocks(ctx, task, pageUnits); err != nil {
					return err
				}
				if err := s.refreshMaterialImportTaskStats(ctx, task.Id); err != nil {
					return err
				}
			}
		}
		pendingUnits = carryUnits
		if stop || len(messages) < materialImportPageLimit {
			shouldFinish = true
			break
		}
		time.Sleep(1200 * time.Millisecond)
	}
	if !shouldFinish {
		_, err := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).Where("id", task.Id).Data(g.Map{
			"pull_offset_id": offsetID,
			"updated_at":     gtime.Now(),
		}).Update()
		if err != nil {
			return gerror.Wrap(err, "更新资料导入游标失败")
		}
		return s.enqueueMaterialImportTask(ctx, task.Id, 3*time.Second)
	}
	_, err = pdao.YoubanPublishMaterialImportTask.Ctx(ctx).Where("id", task.Id).Data(g.Map{
		"stage":          sysin.MaterialImportStageMedia,
		"pull_offset_id": offsetID,
		"updated_at":     gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新资料导入阶段失败")
	}
	return s.enqueueMaterialImportTask(ctx, task.Id, 0)
}

func materialImportHistoryPage(ctx context.Context, client *telegram.Client, peer *tg.InputPeerChannel, offsetID int) ([]*tg.Message, error) {
	for attempt := 0; attempt < 3; attempt++ {
		res, err := client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			OffsetID: offsetID,
			Limit:    materialImportPageLimit,
		})
		if err == nil {
			return tgHistoryMessages(res), nil
		}
		delay := tgRepairBackoffDelay(attempt, err)
		if collectHistoryIsFloodWait(err) {
			return nil, &collectHistoryPauseError{delay: delay, err: err}
		}
		if !isTgRepairRetryableErr(err) || attempt == 2 {
			return nil, gerror.Wrap(err, "拉取频道消息失败")
		}
		time.Sleep(delay)
	}
	return nil, nil
}

func (s *sSysPublish) ingestMaterialImportMessages(ctx context.Context, task *sysin.MaterialImportTaskModel, messages []*tg.Message, cutoff *gtime.Time, cache *sysin.ChannelCacheModel, latestCachedMessageID int64) (int, bool, []*materialImportMessageUnit, error) {
	ordered := collectHistoryMessagesInSendOrder(messages)
	nextOffset := int(task.PullOffsetId)
	stop := false
	validMessages := make([]*tg.Message, 0, len(ordered))
	channelID, _ := materialImportCacheChannelID(cache)
	for _, msg := range ordered {
		if msg == nil || msg.ID <= 0 {
			continue
		}
		if nextOffset == 0 || msg.ID < nextOffset {
			nextOffset = msg.ID
		}
		messageAt := gtime.NewFromTime(time.Unix(int64(msg.Date), 0))
		if cutoff != nil && messageAt.Before(cutoff) {
			stop = true
			continue
		}
		validMessages = append(validMessages, msg)
		if cache != nil {
			if err := s.upsertTgMessageCache(ctx, task.TenantId, tgMessageRepairChannel{
				Id:           channelID,
				TgAccountId:  task.TgAccountId,
				TargetChatId: cache.ChannelId,
			}, msg); err != nil {
				return nextOffset, stop, nil, err
			}
		}
	}
	return nextOffset, stop, materialImportBuildUnits(task, validMessages), nil
}

func (s *sSysPublish) materialImportLatestCachedMessageID(ctx context.Context, tenantId int64, tgAccountId int64, cache *sysin.ChannelCacheModel) (int64, error) {
	channelID, err := materialImportCacheChannelID(cache)
	if err != nil || channelID <= 0 {
		return 0, nil
	}
	value, err := pdao.YoubanPublishTgMessageCache.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		Where("channel_id", channelID).
		Fields("MAX(tg_message_id)").
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取资料导入消息缓存失败")
	}
	return value.Int64(), nil
}

func materialImportCacheChannelID(cache *sysin.ChannelCacheModel) (int64, error) {
	if cache == nil || strings.TrimSpace(cache.ChannelId) == "" {
		return 0, nil
	}
	channelID := gconv.Int64(cache.ChannelId)
	if channelID <= 0 {
		return 0, gerror.New("频道ID无效，请刷新频道缓存")
	}
	return channelID, nil
}

func materialImportMediaItemsWithPurpose(items []collectMediaItem, purpose string) []collectMediaItem {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		purpose = "display"
	}
	for i := range items {
		if strings.TrimSpace(items[i].Purpose) == "" {
			items[i].Purpose = purpose
		}
	}
	return items
}

func materialImportAllMediaType(items []collectMediaItem, mediaType string) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item.Type) != mediaType {
			return false
		}
	}
	return true
}

func materialImportMessageIds(existing string, id int) string {
	ids := make([]int, 0)
	for _, item := range strings.Split(existing, ",") {
		if value := gconv.Int(strings.TrimSpace(item)); value > 0 {
			ids = append(ids, value)
		}
	}
	ids = append(ids, id)
	ids = positiveUniqueInts(ids)
	sort.Ints(ids)
	parts := make([]string, 0, len(ids))
	for _, item := range ids {
		parts = append(parts, gconv.String(item))
	}
	return strings.Join(parts, ",")
}

func materialImportLatestSourceMessageID(value string) int {
	latest := 0
	for _, item := range strings.Split(value, ",") {
		messageID := gconv.Int(strings.TrimSpace(item))
		if messageID > latest {
			latest = messageID
		}
	}
	return latest
}

func materialImportHasVerifyMedia(mediaJSON string) bool {
	items := make([]collectMediaItem, 0)
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Purpose), "verify") {
			return true
		}
	}
	return false
}

var materialImportUserIdRegexp = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_-]*[0-9][A-Za-z0-9_-]*`)

func materialImportTitle(text string) (title string, profileNo string, nickname string) {
	explicitTitle := ""
	userId := ""
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for _, line := range lines {
		item := strings.TrimSpace(line)
		if item == "" {
			continue
		}
		if value, ok := materialImportPrefixedValue(item, "标题"); ok {
			explicitTitle = firstNonEmpty(explicitTitle, value)
			continue
		}
		if value, ok := materialImportPrefixedValue(item, "编号"); ok {
			profileNo = firstNonEmpty(profileNo, value)
			continue
		}
		if value, ok := materialImportPrefixedValue(item, "昵称"); ok {
			nickname = firstNonEmpty(nickname, value)
			continue
		}
		if idx := materialImportInlineFieldIndex(item, "昵称"); idx > 0 {
			userId = firstNonEmpty(userId, materialImportUserId(item[:idx]))
			if value, ok := materialImportPrefixedValue(item[idx:], "昵称"); ok {
				nickname = firstNonEmpty(nickname, value)
			}
			continue
		}
		userId = firstNonEmpty(userId, materialImportUserId(item))
	}
	title = firstNonEmpty(profileNo, explicitTitle, nickname, userId)
	return strings.TrimSpace(title), strings.TrimSpace(profileNo), strings.TrimSpace(nickname)
}

func materialImportPrefixedValue(text string, prefix string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	prefix = strings.TrimSpace(prefix)
	if !strings.HasPrefix(text, prefix) {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(text, prefix))
	for _, sep := range []string{"：", ":"} {
		if strings.HasPrefix(rest, sep) {
			return strings.TrimSpace(strings.TrimPrefix(rest, sep)), true
		}
	}
	if len(text) > len(prefix) && strings.TrimSpace(text[len(prefix):]) != text[len(prefix):] {
		return rest, rest != ""
	}
	return "", false
}

func materialImportUserId(text string) string {
	return materialImportUserIdRegexp.FindString(strings.TrimSpace(text))
}

func materialImportInlineFieldIndex(text string, prefix string) int {
	text = strings.TrimSpace(text)
	prefix = strings.TrimSpace(prefix)
	if text == "" || prefix == "" {
		return -1
	}
	for _, sep := range []string{":", "："} {
		needle := prefix + sep
		if idx := strings.Index(text, needle); idx >= 0 {
			return idx
		}
	}
	return -1
}

func materialImportNextStage(task *sysin.MaterialImportTaskModel) string {
	stage := strings.TrimSpace(task.Stage)
	if stage == sysin.MaterialImportStageMedia {
		return stage
	}
	return sysin.MaterialImportStagePulling
}
