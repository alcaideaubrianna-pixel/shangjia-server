package sys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	iservice "hotgo/internal/service"
)

const (
	tgMessageRepairStatusPending = "pending"
	tgMessageRepairStatusRunning = "running"
	tgMessageRepairStatusSuccess = "success"
	tgMessageRepairStatusFailed  = "failed"
)

type tgMessageRepairRun struct {
	Id           int64       `json:"id"`
	TenantId     int64       `json:"tenantId"`
	AccountId    int64       `json:"accountId"`
	ProfileId    int64       `json:"profileId"`
	TaskId       int64       `json:"taskId"`
	Status       string      `json:"status"`
	Stage        string      `json:"stage"`
	Progress     int         `json:"progress"`
	ChannelCount int         `json:"channelCount"`
	ScannedCount int         `json:"scannedCount"`
	MatchedCount int         `json:"matchedCount"`
	ErrorMessage string      `json:"errorMessage"`
	CreatedAt    *gtime.Time `json:"createdAt"`
	UpdatedAt    *gtime.Time `json:"updatedAt"`
	FinishedAt   *gtime.Time `json:"finishedAt"`
}

type tgMessageRepairChannel struct {
	Id           int64  `json:"id"`
	TgAccountId  int64  `json:"tgAccountId"`
	TargetChatId string `json:"targetChatId"`
	BotIdJson    string `json:"botIdJson"`
}

type tgMessageRepairCacheRow struct {
	Id           int64       `json:"id"`
	TenantId     int64       `json:"tenantId"`
	TgAccountId  int64       `json:"tgAccountId"`
	ChannelId    int64       `json:"channelId"`
	TargetChatId string      `json:"targetChatId"`
	TgMessageId  int64       `json:"tgMessageId"`
	MessageText  string      `json:"messageText"`
	MessageDate  *gtime.Time `json:"messageDate"`
	MediaType    string      `json:"mediaType"`
	MediaGroupId string      `json:"mediaGroupId"`
}

func (s *sSysPublish) MyTgMessageRepairStart(ctx context.Context, in *sysin.TgMessageRepairStartInp) (*sysin.TgMessageRepairModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.startTgMessageRepair(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminTgMessageRepairStart(ctx context.Context, in *sysin.TgMessageRepairStartInp) (*sysin.TgMessageRepairModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.startTgMessageRepair(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) MyTgMessageRepairView(ctx context.Context, in *sysin.TgMessageRepairViewInp) (*sysin.TgMessageRepairModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.tgMessageRepairView(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminTgMessageRepairView(ctx context.Context, in *sysin.TgMessageRepairViewInp) (*sysin.TgMessageRepairModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.tgMessageRepairView(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) startTgMessageRepair(ctx context.Context, in *sysin.TgMessageRepairStartInp, tenantId int64, accountId int64) (*sysin.TgMessageRepairModel, error) {
	if in == nil || (in.ProfileId <= 0 && strings.TrimSpace(in.Uuid) == "") {
		return nil, gerror.New("请选择要修复的资料")
	}
	targetIds, err := s.allowedProfileTargetIds(ctx, []int64{in.ProfileId}, []string{in.Uuid}, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	ids, err := s.allowedProfileIds(ctx, targetIds, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, gerror.New("资料不存在或无权操作")
	}
	runId, err := s.createTgMessageRepairRun(ctx, ids[0], tenantId, accountId)
	if err != nil {
		return nil, err
	}
	return s.tgMessageRepairView(ctx, &sysin.TgMessageRepairViewInp{RunId: runId}, tenantId, accountId)
}

func (s *sSysPublish) tgMessageRepairView(ctx context.Context, in *sysin.TgMessageRepairViewInp, tenantId int64, accountId int64) (*sysin.TgMessageRepairModel, error) {
	if in == nil || in.RunId <= 0 {
		return nil, gerror.New("修复任务ID不能为空")
	}
	mod := g.DB().Model(publishTgMessageRepairRunTable).Safe().Ctx(ctx).
		Where("id", in.RunId).
		Where("tenant_id", tenantId)
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	var item sysin.TgMessageRepairModel
	if err := mod.Scan(&item); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gerror.New("TG消息修复任务不存在")
		}
		return nil, gerror.Wrap(err, "读取TG消息修复任务失败")
	}
	if item.Id <= 0 {
		return nil, gerror.New("TG消息修复任务不存在")
	}
	return &item, nil
}

func (s *sSysPublish) startMissingTelegramMessageRepairForDown(ctx context.Context, ids []int64, tenantId int64, accountId int64) (int64, error) {
	runIds, err := s.startMissingTelegramMessageRepairsForDown(ctx, ids, tenantId, accountId)
	if err != nil || len(runIds) == 0 {
		return 0, err
	}
	return runIds[0], nil
}

func (s *sSysPublish) startMissingTelegramMessageRepairsForDown(ctx context.Context, ids []int64, tenantId int64, accountId int64) ([]int64, error) {
	runIds := make([]int64, 0)
	for _, profileId := range ids {
		if profileId <= 0 {
			continue
		}
		need, err := s.profileNeedsTgMessageRepair(ctx, profileId, tenantId, accountId)
		if err != nil {
			return nil, err
		}
		if !need {
			continue
		}
		runId, err := s.createTgMessageRepairRun(ctx, profileId, tenantId, accountId)
		if err != nil {
			return nil, err
		}
		if runId > 0 {
			runIds = append(runIds, runId)
		}
	}
	return runIds, nil
}

func (s *sSysPublish) profileNeedsTgMessageRepair(ctx context.Context, profileId int64, tenantId int64, accountId int64) (bool, error) {
	task, err := s.tgMessageRepairTask(ctx, profileId, tenantId, accountId)
	if err != nil {
		return false, err
	}
	if task.IsEmpty() || task["status"].Int() != 1 || task["visibility"].String() != consts.ContentVisibilityPublic {
		return false, nil
	}
	count, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("profile_id", profileId).
		Where("status", "sent").
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查TG消息记录失败")
	}
	return count == 0, nil
}

func (s *sSysPublish) createTgMessageRepairRun(ctx context.Context, profileId int64, tenantId int64, accountId int64) (int64, error) {
	task, err := s.tgMessageRepairTask(ctx, profileId, tenantId, accountId)
	if err != nil {
		return 0, err
	}
	if task.IsEmpty() {
		return 0, gerror.New("资料不存在，无法修复TG消息")
	}
	var existing tgMessageRepairRun
	if err = g.DB().Model(publishTgMessageRepairRunTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("profile_id", profileId).
		WhereIn("status", []string{tgMessageRepairStatusPending, tgMessageRepairStatusRunning}).
		OrderDesc("id").
		Scan(&existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		} else {
			return 0, gerror.Wrap(err, "读取TG消息修复任务失败")
		}
	}
	if existing.Id > 0 {
		return existing.Id, nil
	}
	now := gtime.Now()
	runId, err := g.DB().Model(publishTgMessageRepairRunTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":     tenantId,
		"account_id":    task["account_id"].Int64(),
		"profile_id":    profileId,
		"status":        tgMessageRepairStatusPending,
		"stage":         "created",
		"progress":      0,
		"channel_count": 0,
		"scanned_count": 0,
		"matched_count": 0,
		"error_message": "",
		"created_at":    now,
		"updated_at":    now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建TG消息修复任务失败")
	}
	if err = s.enqueueTgMessageRepairRun(ctx, runId, 0); err != nil {
		return 0, gerror.Wrap(err, "加入TG消息修复队列失败")
	}
	return runId, nil
}

func (s *sSysPublish) tgMessageRepairRunProfileId(ctx context.Context, runId int64) (int64, error) {
	if runId <= 0 {
		return 0, nil
	}
	value, err := g.DB().Model(publishTgMessageRepairRunTable).Safe().Ctx(ctx).
		Where("id", runId).
		Fields("profile_id").
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG消息修复资料失败")
	}
	return value.Int64(), nil
}

func (s *sSysPublish) ExecuteTgMessageRepairRun(ctx context.Context, runId int64) (err error) {
	run, locked, err := s.lockTgMessageRepairRun(ctx, runId)
	if err != nil || !locked {
		return err
	}
	defer func() {
		if err != nil {
			_ = s.updateTgMessageRepairRun(ctx, runId, g.Map{
				"status":        tgMessageRepairStatusFailed,
				"stage":         "failed",
				"error_message": err.Error(),
				"finished_at":   gtime.Now(),
				"updated_at":    gtime.Now(),
			})
		}
	}()
	task, err := s.tgMessageRepairTask(ctx, run.ProfileId, run.TenantId, run.AccountId)
	if err != nil {
		return err
	}
	if task.IsEmpty() {
		return gerror.New("资料缺少上架任务，无法修复TG消息")
	}
	channels, err := s.tgMessageRepairChannels(ctx, task)
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		return gerror.New("资料没有可用于匹配的上架频道")
	}
	if err = s.updateTgMessageRepairRun(ctx, runId, g.Map{"stage": "scan", "progress": 10, "channel_count": len(channels), "updated_at": gtime.Now()}); err != nil {
		return err
	}
	scanned := 0
	for _, channel := range channels {
		time.Sleep(400 * time.Millisecond)
		count, scanErr := s.scanTgChannelMessages(ctx, run.TenantId, channel)
		scanned += count
		_ = s.updateTgMessageRepairRun(ctx, runId, g.Map{"scanned_count": scanned, "progress": 55, "updated_at": gtime.Now()})
		if scanErr != nil {
			return scanErr
		}
	}
	if err = s.updateTgMessageRepairRun(ctx, runId, g.Map{"stage": "match", "progress": 70, "updated_at": gtime.Now()}); err != nil {
		return err
	}
	matches, err := s.matchTgRepairMessages(ctx, run.TenantId, task, channels)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		if err = s.finishProfileDownWithoutRepair(ctx, task); err != nil {
			return err
		}
		return s.updateTgMessageRepairRun(ctx, runId, g.Map{
			"status":        tgMessageRepairStatusSuccess,
			"stage":         "skipped",
			"progress":      100,
			"error_message": "未匹配到对应的TG频道消息，已仅同步本地下架状态",
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		})
	}
	if err = s.saveTgRepairMatches(ctx, task, matches); err != nil {
		return err
	}
	if err = s.updateTgMessageRepairRun(ctx, runId, g.Map{"stage": "down", "progress": 85, "matched_count": len(matches), "updated_at": gtime.Now()}); err != nil {
		return err
	}
	if err = s.finishProfileDownAfterRepair(ctx, task); err != nil {
		return err
	}
	return s.updateTgMessageRepairRun(ctx, runId, g.Map{
		"status":        tgMessageRepairStatusSuccess,
		"stage":         "finished",
		"progress":      100,
		"matched_count": len(matches),
		"finished_at":   gtime.Now(),
		"updated_at":    gtime.Now(),
	})
}

func (s *sSysPublish) lockTgMessageRepairRun(ctx context.Context, runId int64) (tgMessageRepairRun, bool, error) {
	var run tgMessageRepairRun
	result, err := g.DB().Model(publishTgMessageRepairRunTable).Safe().Ctx(ctx).
		Where("id", runId).
		WhereIn("status", []string{tgMessageRepairStatusPending, tgMessageRepairStatusFailed}).
		Data(g.Map{"status": tgMessageRepairStatusRunning, "stage": "running", "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return run, false, gerror.Wrap(err, "锁定TG消息修复任务失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return run, false, nil
	}
	if err = g.DB().Model(publishTgMessageRepairRunTable).Safe().Ctx(ctx).Where("id", runId).Scan(&run); err != nil {
		return run, false, gerror.Wrap(err, "读取TG消息修复任务失败")
	}
	return run, true, nil
}

func (s *sSysPublish) updateTgMessageRepairRun(ctx context.Context, runId int64, data g.Map) error {
	_, err := g.DB().Model(publishTgMessageRepairRunTable).Safe().Ctx(ctx).Where("id", runId).Data(data).Update()
	return err
}

func (s *sSysPublish) tgMessageRepairTask(ctx context.Context, profileId int64, tenantId int64, accountId int64) (gdb.Record, error) {
	mod := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Fields("0 AS id,ps.tenant_id,ps.account_id,p.id AS profile_id,p.title,p.profile_no,p.plain_text,p.status,p.visibility,p.published_at,p.created_at,p.updated_at,p.source_type,p.source_key,p.source_created_at,p.source_updated_at").
		Where("ps.tenant_id", tenantId).
		Where("p.id", profileId).
		WhereNull("p.deleted_at")
	if accountId > 0 {
		mod = mod.Where("ps.account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG消息修复资料失败")
	}
	return row, nil
}

func (s *sSysPublish) tgMessageRepairChannels(ctx context.Context, task gdb.Record) ([]tgMessageRepairChannel, error) {
	channelIds, err := s.profileChannelIdsOrDefaults(ctx, task["tenant_id"].Int64(), task["account_id"].Int64(), task["profile_id"].Int64())
	if err != nil {
		return nil, err
	}
	mod := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tg_account_id,target_chat_id,bot_id_json").
		Where("tenant_id", task["tenant_id"].Int64()).
		Where("publish_direction", "up").
		Where("status", 1).
		WhereNull("deleted_at")
	if len(channelIds) > 0 {
		mod = mod.WhereIn("id", channelIds)
	}
	var channels []tgMessageRepairChannel
	if err := mod.OrderAsc("id").Scan(&channels); err != nil {
		return nil, gerror.Wrap(err, "读取TG消息修复频道失败")
	}
	return channels, nil
}

func (s *sSysPublish) scanTgChannelMessages(ctx context.Context, tenantId int64, channel tgMessageRepairChannel) (int, error) {
	return s.scanTgChannelMessagesSince(ctx, tenantId, channel, time.Now().AddDate(0, -6, 0).Unix())
}

func (s *sSysPublish) scanTgChannelMessagesSince(ctx context.Context, tenantId int64, channel tgMessageRepairChannel, cutoff int64) (int, error) {
	if channel.TgAccountId <= 0 {
		return 0, gerror.New("频道未绑定协议号，无法拉取历史消息")
	}
	if err := ensureTgMessageCacheMediaTypeColumn(ctx); err != nil {
		return 0, err
	}
	cache, err := s.tgChannelCacheByChannelId(ctx, tenantId, channel.TgAccountId, channel.TargetChatId)
	if err != nil {
		return 0, err
	}
	channelID, err := strconv.ParseInt(cache.ChannelId, 10, 64)
	if err != nil {
		return 0, gerror.New("频道ID无效，请刷新频道缓存")
	}
	accessHash, err := strconv.ParseInt(cache.AccessHash, 10, 64)
	if err != nil {
		return 0, gerror.New("频道AccessHash无效，请刷新频道缓存")
	}
	latestCachedMessageId, err := s.latestTgMessageCacheId(ctx, tenantId, channel.Id)
	if err != nil {
		return 0, err
	}
	scanned := 0
	err = s.executeTelegramAccountOperation(ctx, channel.TgAccountId, 2*time.Minute, func(ctx context.Context, client *telegram.Client) error {
		offsetID := 0
		for {
			previousOffset := offsetID
			var res tg.MessagesMessagesClass
			var pageErr error
			for attempt := 0; attempt < 4; attempt++ {
				res, pageErr = client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
					Peer:     &tg.InputPeerChannel{ChannelID: channelID, AccessHash: accessHash},
					OffsetID: offsetID,
					Limit:    100,
				})
				if pageErr == nil {
					break
				}
				if !isTgRepairRetryableErr(pageErr) || attempt == 3 {
					return pageErr
				}
				time.Sleep(tgRepairBackoffDelay(attempt, pageErr))
			}
			messages := tgHistoryMessages(res)
			if len(messages) == 0 {
				return nil
			}
			stop := false
			for _, message := range messages {
				if message == nil || message.ID <= 0 {
					continue
				}
				if latestCachedMessageId > 0 && int64(message.ID) <= latestCachedMessageId {
					stop = true
					continue
				}
				if int64(message.Date) < cutoff {
					stop = true
					continue
				}
				if telegramHistoryMessageMediaType(message) == "" {
					continue
				}
				scanned++
				if err = s.upsertTgMessageCache(ctx, tenantId, channel, message); err != nil {
					return err
				}
				if offsetID == 0 || message.ID < offsetID {
					offsetID = message.ID
				}
			}
			if stop || offsetID <= 0 || (previousOffset > 0 && offsetID >= previousOffset) {
				return nil
			}
			time.Sleep(600 * time.Millisecond)
		}
	})
	if err != nil {
		return scanned, gerror.Wrap(err, "拉取TG频道历史消息失败")
	}
	return scanned, nil
}

func ensureTgMessageCacheMediaTypeColumn(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		_, err := g.DB().Exec(ctx, `ALTER TABLE "hg_youban_publish_tg_message_cache" ADD COLUMN IF NOT EXISTS "media_type" varchar(32) NOT NULL DEFAULT ''`)
		if err != nil {
			return gerror.Wrap(err, "检查TG消息缓存媒体类型字段失败")
		}
		return nil
	}
	_, err := g.DB().Exec(ctx, "ALTER TABLE `hg_youban_publish_tg_message_cache` ADD COLUMN `media_type` varchar(32) NOT NULL DEFAULT '' COMMENT '媒体类型' AFTER `message_text`")
	if err != nil && !isIgnorableImportTaskServerIPColumnError(err) {
		return gerror.Wrap(err, "检查TG消息缓存媒体类型字段失败")
	}
	return nil
}

func (s *sSysPublish) latestTgMessageCacheId(ctx context.Context, tenantId int64, channelId int64) (int64, error) {
	value, err := g.DB().Model(publishTgMessageCacheTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("channel_id", channelId).
		Fields("MAX(tg_message_id)").
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG消息缓存游标失败")
	}
	return value.Int64(), nil
}

func isTgRepairRetryableErr(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "flood_wait") ||
		strings.Contains(message, "too many requests") ||
		strings.Contains(message, "timeout") ||
		strings.Contains(message, "eof") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "broken pipe")
}

func tgRepairBackoffDelay(attempt int, err error) time.Duration {
	if err == nil {
		return time.Second
	}
	message := strings.ToLower(err.Error())
	if idx := strings.Index(message, "flood_wait_"); idx >= 0 {
		value := 0
		if _, scanErr := fmt.Sscanf(message[idx+11:], "%d", &value); scanErr == nil && value > 0 {
			return time.Duration(value+2) * time.Second
		}
	}
	if attempt < 0 {
		attempt = 0
	}
	return time.Duration(attempt+1) * time.Second
}

func tgHistoryMessages(res tg.MessagesMessagesClass) []*tg.Message {
	items := make([]*tg.Message, 0)
	appendMessages := func(messages []tg.MessageClass) {
		for _, item := range messages {
			if message, ok := item.(*tg.Message); ok {
				items = append(items, message)
			}
		}
	}
	switch data := res.(type) {
	case *tg.MessagesMessages:
		appendMessages(data.Messages)
	case *tg.MessagesMessagesSlice:
		appendMessages(data.Messages)
	case *tg.MessagesChannelMessages:
		appendMessages(data.Messages)
	}
	return items
}

func telegramHistoryMessageMediaType(message *tg.Message) string {
	if message == nil || message.Media == nil {
		if strings.TrimSpace(message.Message) != "" {
			return "text"
		}
		return ""
	}
	switch media := message.Media.(type) {
	case *tg.MessageMediaPhoto:
		return "photo"
	case *tg.MessageMediaDocument:
		if doc, ok := media.Document.(*tg.Document); ok {
			for _, attr := range doc.Attributes {
				switch attr.(type) {
				case *tg.DocumentAttributeVideo:
					return "video"
				case *tg.DocumentAttributeAudio:
					return "audio"
				}
			}
		}
		return "document"
	default:
		return ""
	}
}

func (s *sSysPublish) upsertTgMessageCache(ctx context.Context, tenantId int64, channel tgMessageRepairChannel, message *tg.Message) error {
	mediaType := telegramHistoryMessageMediaType(message)
	if mediaType == "" {
		return nil
	}
	messageDate := gtime.NewFromTime(time.Unix(int64(message.Date), 0))
	mediaGroupId := ""
	if groupedId, ok := message.GetGroupedID(); ok && groupedId != 0 {
		mediaGroupId = strconv.FormatInt(groupedId, 10)
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id":      tenantId,
		"tg_account_id":  channel.TgAccountId,
		"channel_id":     channel.Id,
		"target_chat_id": channel.TargetChatId,
		"tg_message_id":  message.ID,
		"message_text":   message.Message,
		"media_type":     mediaType,
		"message_date":   messageDate,
		"media_group_id": mediaGroupId,
		"created_at":     now,
		"updated_at":     now,
	}
	if _, err := g.DB().Model(publishTgMessageCacheTable).Safe().Ctx(ctx).
		Data(data).
		OnConflict("tenant_id,channel_id,tg_message_id").
		OnDuplicateEx("id,tenant_id,channel_id,tg_message_id,created_at").
		Save(); err != nil {
		return gerror.Wrap(err, "写入TG消息缓存失败")
	}
	return nil
}

func (s *sSysPublish) matchTgRepairMessages(ctx context.Context, tenantId int64, task gdb.Record, channels []tgMessageRepairChannel) ([]tgMessageRepairCacheRow, error) {
	keywords := tgRepairMatchKeywords(task)
	if len(keywords) == 0 {
		return nil, gerror.New("资料缺少标题，无法匹配TG消息")
	}
	channelIds := make([]int64, 0, len(channels))
	for _, channel := range channels {
		channelIds = append(channelIds, channel.Id)
	}
	mod := g.DB().Model(publishTgMessageCacheTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("channel_id", channelIds).
		Where("media_type <>", "").
		OrderDesc("message_date").
		Limit(5000)
	var rows []tgMessageRepairCacheRow
	if err := mod.Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取TG消息缓存失败")
	}
	matches := make([]tgMessageRepairCacheRow, 0)
	for _, row := range rows {
		text := strings.ToLower(strings.TrimSpace(row.MessageText))
		if text == "" {
			continue
		}
		for _, keyword := range keywords {
			if strings.Contains(text, keyword) {
				matches = append(matches, row)
				break
			}
		}
	}
	return s.expandTgRepairMediaMatches(ctx, tenantId, matches)
}

func (s *sSysPublish) expandTgRepairMediaMatches(ctx context.Context, tenantId int64, matches []tgMessageRepairCacheRow) ([]tgMessageRepairCacheRow, error) {
	if len(matches) == 0 {
		return matches, nil
	}
	expanded := make([]tgMessageRepairCacheRow, 0, len(matches))
	seen := make(map[string]struct{})
	addRows := func(rows []tgMessageRepairCacheRow) {
		for _, row := range rows {
			key := fmt.Sprintf("%d:%d", row.ChannelId, row.TgMessageId)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			expanded = append(expanded, row)
		}
	}
	for _, match := range matches {
		if match.MediaGroupId == "" {
			addRows([]tgMessageRepairCacheRow{match})
			continue
		}
		var groupRows []tgMessageRepairCacheRow
		err := g.DB().Model(publishTgMessageCacheTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("channel_id", match.ChannelId).
			Where("media_group_id", match.MediaGroupId).
			Where("media_type <>", "").
			OrderAsc("tg_message_id").
			Scan(&groupRows)
		if err != nil {
			return nil, gerror.Wrap(err, "读取TG媒体组缓存失败")
		}
		addRows(groupRows)
		if len(groupRows) > 0 {
			if video, err := s.nextVerifyVideoAfterMediaGroup(ctx, tenantId, groupRows[len(groupRows)-1]); err != nil {
				return nil, err
			} else if video.TgMessageId > 0 {
				addRows([]tgMessageRepairCacheRow{video})
			}
		}
	}
	return expanded, nil
}

func (s *sSysPublish) nextVerifyVideoAfterMediaGroup(ctx context.Context, tenantId int64, last tgMessageRepairCacheRow) (tgMessageRepairCacheRow, error) {
	var row tgMessageRepairCacheRow
	if last.MessageDate == nil {
		return row, nil
	}
	err := g.DB().Model(publishTgMessageCacheTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("channel_id", last.ChannelId).
		Where("media_type", "video").
		WhereGT("tg_message_id", last.TgMessageId).
		WhereGTE("message_date", last.MessageDate).
		WhereLTE("message_date", last.MessageDate.Add(3*time.Minute)).
		OrderAsc("tg_message_id").
		Limit(1).
		Scan(&row)
	if err != nil {
		return row, gerror.Wrap(err, "读取TG媒体组后续视频失败")
	}
	return row, nil
}

func tgRepairMatchKeywords(task gdb.Record) []string {
	values := []string{
		task["title"].String(),
		task["profile_no"].String(),
		task["source_key"].String(),
	}
	keywords := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		keywords = append(keywords, value)
	}
	return keywords
}

func (s *sSysPublish) saveTgRepairMatches(ctx context.Context, task gdb.Record, matches []tgMessageRepairCacheRow) error {
	jobs := make(map[int64]int64)
	for _, item := range matches {
		if item.ChannelId <= 0 || item.TgMessageId <= 0 {
			continue
		}
		jobId, ok := jobs[item.ChannelId]
		if !ok {
			channel, err := s.tgRepairChannelById(ctx, task["tenant_id"].Int64(), item.ChannelId)
			if err != nil {
				return err
			}
			jobId, err = s.ensureTgRepairJob(ctx, task, channel, item.TargetChatId)
			if err != nil {
				return err
			}
			jobs[item.ChannelId] = jobId
		}
		if err := s.ensureTgRepairMessage(ctx, task, jobId, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) tgRepairChannelById(ctx context.Context, tenantId int64, channelId int64) (tgMessageRepairChannel, error) {
	var channel tgMessageRepairChannel
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tg_account_id,target_chat_id,bot_id_json").
		Where("tenant_id", tenantId).
		Where("id", channelId).
		WhereNull("deleted_at").
		Scan(&channel); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return channel, gerror.New("TG修复频道不存在")
		}
		return channel, gerror.Wrap(err, "读取TG修复频道失败")
	}
	if channel.Id <= 0 {
		return channel, gerror.New("TG修复频道不存在")
	}
	return channel, nil
}

func (s *sSysPublish) ensureTgRepairJob(ctx context.Context, task gdb.Record, channel tgMessageRepairChannel, targetChatId string) (int64, error) {
	var existing struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id").
		Where("profile_id", task["profile_id"].Int64()).
		Where("channel_id", channel.Id).
		Scan(&existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		} else {
			return 0, gerror.Wrap(err, "读取TG修复任务失败")
		}
	}
	if existing.Id > 0 {
		now := gtime.Now()
		data := telegramJobStateUpdateData("sent", 0, now)
		data["error_message"] = ""
		data["sent_at"] = now
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", existing.Id).Data(data).Update()
		return existing.Id, nil
	}
	botId := firstPositiveId(decodeBotIds(channel.BotIdJson))
	if botId <= 0 {
		return 0, gerror.New("上架频道缺少Bot配置，无法删除历史消息")
	}
	now := gtime.Now()
	data := telegramJobStateUpdateData("sent", 0, now)
	data["tenant_id"] = task["tenant_id"].Int64()
	data["merchant_id"] = task["tenant_id"].Int64()
	data["account_id"] = task["account_id"].Int64()
	data["profile_id"] = task["profile_id"].Int64()
	data["channel_id"] = channel.Id
	data["bot_id"] = botId
	data["target_chat_id"] = targetChatId
	data["retry_count"] = 0
	data["error_message"] = ""
	data["sent_at"] = now
	data["created_at"] = now
	return g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
}

func (s *sSysPublish) ensureTgRepairMessage(ctx context.Context, task gdb.Record, jobId int64, item tgMessageRepairCacheRow) error {
	count, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("job_id", jobId).
		Where("tg_message_id", item.TgMessageId).
		Where("status", "sent").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG修复消息失败")
	}
	if count > 0 {
		return nil
	}
	var job struct {
		BotId int64 `json:"botId"`
	}
	_ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Fields("bot_id").Where("id", jobId).Scan(&job)
	now := gtime.Now()
	_, err = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":         jobId,
		"tenant_id":      task["tenant_id"].Int64(),
		"account_id":     task["account_id"].Int64(),
		"profile_id":     task["profile_id"].Int64(),
		"bot_id":         job.BotId,
		"target_chat_id": item.TargetChatId,
		"tg_message_id":  item.TgMessageId,
		"media_group_id": item.MediaGroupId,
		"media_id":       0,
		"purpose":        "repair",
		"tg_file_id":     "",
		"status":         "sent",
		"sent_at":        item.MessageDate,
		"created_at":     now,
		"updated_at":     now,
	}).Insert()
	if err != nil {
		return gerror.Wrap(err, "保存TG修复消息失败")
	}
	return nil
}

func (s *sSysPublish) finishProfileDownAfterRepair(ctx context.Context, task gdb.Record) error {
	tenantId := task["tenant_id"].Int64()
	profileId := task["profile_id"].Int64()
	if _, err := s.syncProfilePublishState(ctx, profileId, 2, consts.ContentVisibilityPrivate, nil); err != nil {
		return gerror.Wrap(err, "更新资料下架状态失败")
	}
	if err := s.syncProfileNoteIndex(ctx, profileId); err != nil {
		return err
	}
	downAt := gtime.Now().String()
	if err := s.handleProfilesDown(ctx, []int64{profileId}, tenantId, downAt, newTelegramOperationNo("down", profileId)); err != nil {
		return err
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) finishProfileDownWithoutRepair(ctx context.Context, task gdb.Record) error {
	profileId := task["profile_id"].Int64()
	if _, err := s.syncProfilePublishState(ctx, profileId, 2, consts.ContentVisibilityPrivate, nil); err != nil {
		return gerror.Wrap(err, "更新资料下架状态失败")
	}
	if err := s.syncProfileNoteIndex(ctx, profileId); err != nil {
		return err
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func tgRepairRawJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
