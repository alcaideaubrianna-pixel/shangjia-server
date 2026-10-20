package sys

import (
	"context"
	"encoding/json"
	"fmt"
	relayconsts "hotgo/addons/youban_publish/consts"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/internal/model/entity"
)

func relayPackageToken(pkg *entity.YoubanPublishRelayPackage) string {
	return fmt.Sprintf("relay-package:%d:%d", pkg.Id, pkg.Generation)
}

func (s *sSysPublish) handleRelayPrepareTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (resultErr error) {
	id, err := relayTaskID(task.TaskKey, "relay-prepare:")
	if err != nil {
		return err
	}
	var pkg entity.YoubanPublishRelayPackage
	if err = dao.YoubanPublishRelayPackage.Ctx(ctx).Master().WherePri(id).Scan(&pkg); err != nil {
		return err
	}
	parts := strings.Split(task.TaskKey, ":")
	if len(parts) != 3 {
		return gerror.New("中转资料准备任务参数无效")
	}
	generation, _ := strconv.Atoi(parts[2])
	if pkg.Id == 0 || pkg.TenantId != task.TenantID {
		return gerror.New("中转资料包任务归属无效")
	}
	if pkg.Generation != generation || pkg.Status != relayconsts.RelayStatusPreparing {
		return nil
	}
	var channel entity.YoubanPublishRelayChannel
	if err = dao.YoubanPublishRelayChannel.Ctx(ctx).Master().WherePri(pkg.RelayChannelId).Scan(&channel); err != nil {
		return err
	}
	if channel.Id == 0 || channel.TgAccountId != task.AccountID || channel.TenantId != task.TenantID {
		return gerror.New("中转账号任务与频道归属不匹配")
	}
	job, err := s.telegramJobById(ctx, pkg.JobId)
	if err != nil {
		return err
	}
	if job.Status == telegramJobStatusSuperseded || job.SendPhase == telegramSendPhaseTimeoutCancelled {
		pc := dao.YoubanPublishRelayPackage.Columns()
		_, err := dao.YoubanPublishRelayPackage.Ctx(ctx).Where(pc.Id, pkg.Id).Where(pc.Generation, pkg.Generation).Where(pc.Status, relayconsts.RelayStatusPreparing).Data(do.YoubanPublishRelayPackage{Status: relayconsts.RelayStatusCanceled, LeaseUntil: gtime.Now()}).Update()
		return err
	}
	var payload relayPackagePayload
	if err = json.Unmarshal([]byte(pkg.Payload), &payload); err != nil {
		return err
	}
	botItem, err := s.getBotById(ctx, pkg.BotId, pkg.TenantId)
	if err != nil {
		return err
	}
	// Every error after this point is durably classified; a task retry must not blindly repeat an upload.
	defer func() {
		if resultErr != nil {
			resultErr = s.failRelayPreparation(ctx, &pkg, &channel, resultErr)
		}
	}()
	if err = s.ensureRelayBotWithClient(ctx, client, &channel, botItem); err != nil {
		return err
	}
	bot, err := s.telegramMediaBot(ctx, botItem.BotToken)
	if err != nil {
		return err
	}
	me, err := bot.GetMe(ctx)
	if err != nil {
		return err
	}
	if me.ID != pkg.TelegramUserId {
		return gerror.New("发布Bot身份变化，原资料包不能继续准备")
	}
	stopHeartbeat := s.startRelayPackageHeartbeat(pkg.Id, pkg.Generation)
	defer stopHeartbeat()
	sendCtx := context.WithValue(ctx, relayPackageContextKey{}, true)
	manifest := []relayPackageMessage{}
	token := telegramAttemptMarker(relayPackageToken(&pkg))
	index := 0
	for _, part := range []struct {
		purpose, caption string
		media            []*telegramMediaItem
	}{{"display", payload.Caption, payload.Display}, {"verify", "", payload.Verify}} {
		if part.purpose == "display" && len(part.media) == 0 {
			marker := relayPackageMarker(job.Id, index)
			index++
			messages, err := s.sendTelegramDisplayPart(sendCtx, bot, channel.TargetChatId, part.caption+marker+token, nil)
			if err != nil {
				return err
			}
			if len(messages) != 1 {
				return gerror.New("中转文字展示未完整返回")
			}
			manifest = append(manifest, relayPackageMessage{SourceID: messages[0].MessageId, Marker: marker, Message: *messages[0]})
		} else {
			for _, item := range part.media {
				if item == nil {
					return gerror.New("中转媒体为空")
				}
				item.TenantId = job.TenantId
				item.AccountId = job.AccountId
				item.JobId = job.Id
				item.DeliveryMarker = relayPackageMarker(job.Id, index) + token
				index++
				// Existing unscoped file IDs cannot be trusted for a different publisher.
				item.TgFileId = ""
				item.TgThumbFileId = ""
				if !item.AntiScanEnabled {
					var cached entity.YoubanPublishRelayFile
					fc := dao.YoubanPublishRelayFile.Columns()
					if err = dao.YoubanPublishRelayFile.Ctx(ctx).Where(fc.TenantId, pkg.TenantId).Where(fc.TelegramUserId, pkg.TelegramUserId).Where(fc.AssetKey, relayFileKey(item)).Scan(&cached); err != nil {
						return err
					}
					item.TgFileId = cached.FileId
				}
				item.ForceUpload = item.AntiScanEnabled
			}
			// This helper only reuses collection IDs proven to belong to this exact Bot.
			pinnedJob := job
			pinnedJob.BotId = pkg.BotId
			if err = s.applySameBotCollectFileIDs(ctx, pinnedJob, part.purpose, part.media); err != nil {
				return err
			}
			messages, err := s.sendTelegramMediaSet(sendCtx, bot, channel.TargetChatId, part.purpose, part.caption, part.media)
			if err != nil {
				return err
			}
			if len(messages) != len(part.media) {
				return gerror.New("中转媒体准备返回数量不完整")
			}
			offset := len(manifest)
			for i, msg := range messages {
				item := part.media[i]
				manifest = append(manifest, relayPackageMessage{SourceID: msg.MessageId, Marker: relayPackageMarker(job.Id, offset+i), Message: *msg})
				if !item.AntiScanEnabled && msg.TgFileId != "" {
					fc := dao.YoubanPublishRelayFile.Columns()
					if _, err = dao.YoubanPublishRelayFile.Ctx(ctx).Data(do.YoubanPublishRelayFile{TenantId: pkg.TenantId, TelegramUserId: pkg.TelegramUserId, AssetKey: relayFileKey(item), FileId: msg.TgFileId, FileUniqueId: msg.TgFileUniqueId}).OnConflict(fc.TenantId + "," + fc.TelegramUserId + "," + fc.AssetKey).Save(); err != nil {
						return err
					}
				}
			}
		}
	}
	if len(manifest) == 0 || len(manifest) > 100 {
		return gerror.New("中转整包消息数量必须在1到100条之间")
	}
	for i := range manifest {
		manifest[i].Message.RelaySourceId = manifest[i].SourceID
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if _, err = decodeRelayManifest(string(body)); err != nil {
		return err
	}
	pc := dao.YoubanPublishRelayPackage.Columns()
	result, err := dao.YoubanPublishRelayPackage.Ctx(ctx).Where(pc.Id, pkg.Id).Where(pc.Generation, generation).Where(pc.Status, relayconsts.RelayStatusPreparing).Data(do.YoubanPublishRelayPackage{Status: relayconsts.RelayStatusReady, Messages: string(body), LeaseUntil: gtime.Now(), LastError: ""}).Update()
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return gerror.New("中转资料包租约已变化，停止发布")
	}
	return nil
}

func (s *sSysPublish) failRelayPreparation(ctx context.Context, pkg *entity.YoubanPublishRelayPackage, channel *entity.YoubanPublishRelayChannel, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	scope, delay := relayFailureScope(cause)
	status := relayconsts.RelayStatusRetryWait
	// A private staging retry can leave an orphan, but can never duplicate a target publish.
	// Generation fencing prevents a late staging task from becoming the active package.
	next := gtime.Now().Add(delay)
	cc := dao.YoubanPublishRelayChannel.Columns()
	if scope == "channel" || scope == "account" {
		q := dao.YoubanPublishRelayChannel.Ctx(ctx).Where(cc.TenantId, pkg.TenantId)
		if scope == "account" {
			q = q.Where(cc.TgAccountId, channel.TgAccountId)
		} else {
			q = q.Where(cc.Id, channel.Id)
		}
		if _, err := q.Data(do.YoubanPublishRelayChannel{HealthStatus: relayChannelHealthCooldown, CooldownUntil: next, LastError: cause.Error()}).Update(); err != nil {
			return err
		}
		next = gtime.Now() // This job may immediately choose another healthy candidate.
	}
	pc := dao.YoubanPublishRelayPackage.Columns()
	_, err := dao.YoubanPublishRelayPackage.Ctx(ctx).Where(pc.Id, pkg.Id).Where(pc.Generation, pkg.Generation).Where(pc.Status, relayconsts.RelayStatusPreparing).Data(do.YoubanPublishRelayPackage{
		Status: status, Generation: pkg.Generation + 1, LeaseUntil: gtime.Now(), NextRetryAt: next, LastError: cause.Error(), Messages: "",
	}).Update()
	if err != nil {
		return err
	}
	g.Log().Warning(ctx, "中转资料准备失败", g.Map{"jobId": pkg.JobId, "relayChannelId": channel.Id, "scope": scope, "wait": delay.String(), relayconsts.RelayStatusError: cause.Error()})
	if scope == "channel" || scope == "account" {
		pkg.Status = status
		pkg.Generation++
		pkg.NextRetryAt = next
		return s.dispatchRelayPackage(ctx, pkg)
	}
	return nil
}

func (s *sSysPublish) prepareRelayPackageAttempt(ctx context.Context, job telegramJobRecord, pkg *entity.YoubanPublishRelayPackage, manifest []relayPackageMessage) (telegramDeliveryAttempt, error) {
	attempt, err := s.prepareTelegramDeliveryAttempt(ctx, job, "package", len(manifest), true)
	if err != nil {
		return attempt, err
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		_, err := tx.Model(dao.YoubanPublishRelayPackage.Table()).Where(dao.YoubanPublishRelayPackage.Columns().Id, pkg.Id).Data(do.YoubanPublishRelayPackage{Status: relayconsts.RelayStatusCopying, AttemptId: attempt.Id}).Update()
		return err
	})
	return attempt, err
}

func (s *sSysPublish) startRelayPackageHeartbeat(id int64, generation int) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				updateCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				c := dao.YoubanPublishRelayPackage.Columns()
				_, err := dao.YoubanPublishRelayPackage.Ctx(updateCtx).Where(c.Id, id).Where(c.Generation, generation).Where(c.Status, relayconsts.RelayStatusPreparing).Data(do.YoubanPublishRelayPackage{LeaseUntil: gtime.Now().Add(5 * time.Minute)}).Update()
				stop()
				if err != nil {
					g.Log().Warning(ctx, "续期中转准备租约失败", g.Map{"packageId": id, relayconsts.RelayStatusError: err.Error()})
				}
			}
		}
	}()
	return func() { cancel(); <-done }
}
