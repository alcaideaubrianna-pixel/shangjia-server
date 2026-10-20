package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	relayconsts "hotgo/addons/youban_publish/consts"
	"html"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type relayPackagePayload struct {
	Caption string               `json:"caption"`
	Display []*telegramMediaItem `json:"display"`
	Verify  []*telegramMediaItem `json:"verify"`
}

type relayPackageMessage struct {
	SourceID int64               `json:"sourceId"`
	Marker   string              `json:"marker"`
	Message  telegramSentMessage `json:"message"`
}

type relayPackageContextKey struct{}

func relayPackageEnabled(ctx context.Context, job telegramJobRecord) bool {
	if job.SendPhase == relayconsts.RelaySendPhasePreparing || job.SendPhase == relayconsts.RelaySendPhaseReady || job.SendPhase == relayconsts.RelaySendPhaseSending || job.SendPhase == relayconsts.RelaySendPhaseConfirmed {
		return true
	}
	if job.SendPhase != "" || !strings.EqualFold(os.Getenv("YOUBAN_RELAY_PACKAGE_ENABLED"), "true") {
		return false
	}
	tenants := strings.TrimSpace(os.Getenv("YOUBAN_RELAY_PACKAGE_TENANTS"))
	if tenants == "" {
		return true
	}
	for _, id := range strings.Split(tenants, ",") {
		if strings.TrimSpace(id) == strconv.FormatInt(job.TenantId, 10) {
			return true
		}
	}
	return false
}

func relayPackageMarker(jobID int64, index int) string {
	return telegramJobPhaseMarker(jobID, fmt.Sprintf("relay-item:%d", index))
}

func (s *sSysPublish) relayPackageByJob(ctx context.Context, jobID int64) (*entity.YoubanPublishRelayPackage, error) {
	var row entity.YoubanPublishRelayPackage
	err := dao.YoubanPublishRelayPackage.Ctx(ctx).Master().Where(dao.YoubanPublishRelayPackage.Columns().JobId, jobID).Scan(&row)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func decodeRelayManifest(value string) ([]relayPackageMessage, error) {
	var messages []relayPackageMessage
	if err := json.Unmarshal([]byte(value), &messages); err != nil {
		return nil, gerror.Wrap(err, "读取中转资料包清单失败")
	}
	if len(messages) == 0 || len(messages) > 100 {
		return nil, gerror.New("中转资料包消息数量必须为1到100条")
	}
	seen := map[string]bool{}
	var previous int64
	for _, m := range messages {
		if m.SourceID <= previous || m.Marker == "" || seen[m.Marker] || (m.Message.Purpose != "display" && m.Message.Purpose != "verify") {
			return nil, gerror.New("中转资料包顺序或消息归属不完整")
		}
		previous = m.SourceID
		seen[m.Marker] = true
	}
	return messages, nil
}

func relayPackageCopiedMessages(manifest []relayPackageMessage, ids []models.MessageID) ([]*telegramSentMessage, error) {
	if len(manifest) != len(ids) {
		return nil, gerror.Newf("中转整包复制不完整，期望%d条，实际%d条，禁止按位置猜测归属", len(manifest), len(ids))
	}
	result := make([]*telegramSentMessage, 0, len(ids))
	seen := map[int]bool{}
	for i, id := range ids {
		if id.ID <= 0 || seen[id.ID] {
			return nil, gerror.New("中转整包复制返回无效消息编号")
		}
		seen[id.ID] = true
		m := manifest[i].Message
		m.MessageId = int64(id.ID)
		m.MediaGroupId = ""
		result = append(result, &m)
	}
	return result, nil
}

func (s *sSysPublish) resolveRelayPublisher(ctx context.Context, job telegramJobRecord) (*sysin.BotModel, *models.User, error) {
	var item *sysin.BotModel
	var err error
	if job.BotId > 0 {
		item, err = s.getBotById(ctx, job.BotId, job.TenantId)
	} else {
		var bots []*sysin.BotModel
		bots, err = s.enabledBots(ctx, job.TenantId)
		if err == nil && len(bots) == 0 {
			bots, err = s.enabledBots(ctx, 0)
		}
		if len(bots) > 0 {
			item = bots[0]
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if item == nil {
		return nil, nil, gerror.New("未配置可用发布Bot")
	}
	bot, err := s.telegramMediaBot(ctx, item.BotToken)
	if err != nil {
		return nil, nil, err
	}
	me, err := bot.GetMe(ctx)
	return item, me, err
}

// Called before cleanup_previous_publish. Preparing a relay package never removes live messages.
func (s *sSysPublish) sendRelayPackageJob(ctx context.Context, job telegramJobRecord) (resultErr error) {
	deliveryAttemptStarted := false
	defer func() {
		if resultErr != nil && !deliveryAttemptStarted {
			resultErr = telegramPreparationError(resultErr)
		}
	}()

	if job.SendPhase == "" {
		if err := s.updateTelegramJobSendPhase(ctx, job.Id, relayconsts.RelaySendPhasePreparing); err != nil {
			return err
		}
		job.SendPhase = relayconsts.RelaySendPhasePreparing
	}
	pkg, err := s.relayPackageByJob(ctx, job.Id)
	if err != nil {
		return err
	}
	if pkg.Id == 0 {
		item, me, err := s.resolveRelayPublisher(ctx, job)
		if err != nil {
			return err
		}
		if job.BotId == 0 {
			if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("bot_id", 0).Data(g.Map{"bot_id": item.Id}).Update(); err != nil {
				return err
			}
			job.BotId = item.Id
		}
		caption, err := s.telegramJobCaption(ctx, job)
		if err != nil {
			return err
		}
		display, err := s.telegramJobMedia(ctx, job, "display")
		if err != nil {
			return err
		}
		display, err = s.selectTelegramDisplayMediaForTenant(ctx, job, display)
		if err != nil {
			return err
		}
		verify, err := s.telegramJobMedia(ctx, job, "verify")
		if err != nil {
			return err
		}
		caption, err = s.applyTelegramJobContentProtection(ctx, job, caption, display, verify)
		if err != nil {
			return err
		}
		if len(display) == 0 && strings.TrimSpace(caption) == "" {
			return gerror.New("展示资料和文案不能同时为空")
		}
		count := len(display) + len(verify)
		if len(display) == 0 {
			count++
		}
		if count > 100 {
			return gerror.New("资料超过100条，无法一次整包发布，请减少媒体数量")
		}
		payload, err := json.Marshal(relayPackagePayload{Caption: caption, Display: display, Verify: verify})
		if err != nil {
			return err
		}
		_, err = dao.YoubanPublishRelayPackage.Ctx(ctx).Data(do.YoubanPublishRelayPackage{TenantId: job.TenantId, JobId: job.Id, BotId: item.Id, TelegramUserId: me.ID, Payload: string(payload), Status: relayconsts.RelayStatusPending}).Insert()
		if err != nil {
			return err
		}
		pkg, err = s.relayPackageByJob(ctx, job.Id)
		if err != nil {
			return err
		}
	}
	if pkg.Status != relayconsts.RelayStatusReady && pkg.Status != relayconsts.RelayStatusCopying && pkg.Status != relayconsts.RelayStatusConfirmed {

		if err = s.dispatchRelayPackage(ctx, pkg); err != nil {
			return err
		}
		return s.deferRelayPackageJob(ctx, job)
	}
	manifest, err := decodeRelayManifest(pkg.Messages)
	if err != nil {
		return err
	}
	// The real Bot is pinned durably. Shared/default Bot changes cannot change an in-flight publisher.
	job.BotId = pkg.BotId
	botItem, err := s.getBotById(ctx, pkg.BotId, pkg.TenantId)
	if err != nil {
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
		return gerror.New("发布Bot身份已变化，禁止使用旧中转资料包")
	}
	restored, err := s.restoreConfirmedTelegramResponse(ctx, job, "package", nil)
	if err != nil {
		return err
	}
	if restored {
		return s.finalizeRelayPackageJob(ctx, job, pkg)
	}
	if job.SendPhase == relayconsts.RelaySendPhaseSending {
		return telegramDeliveryUncertainError(gerror.New("整包复制已开始，必须先核实结果"))
	}
	if job.SendPhase != relayconsts.RelaySendPhaseReady {
		allowed, err := s.prepareProfileChannelPublish(ctx, job)
		if err != nil {
			return err
		}
		if !allowed {
			return errTelegramJobSuperseded
		}
		if err = s.updateTelegramJobSendPhase(ctx, job.Id, relayconsts.RelaySendPhaseReady); err != nil {
			return err
		}
		job.SendPhase = relayconsts.RelaySendPhaseReady
	}
	// Check the actual Bot immediately before using a cached source channel.
	var channel entity.YoubanPublishRelayChannel
	if err = dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(pkg.RelayChannelId).Scan(&channel); err != nil {
		return err
	}
	member, err := bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{ChatID: channel.TargetChatId, UserID: pkg.TelegramUserId})
	if err != nil {
		if scope, _ := relayFailureScope(err); scope != "channel" {
			return err
		}
	}
	if err != nil || !telegramBotCanSendMessage(member) {
		if _, err = dao.YoubanPublishRelayPackage.Ctx(ctx).WherePri(pkg.Id).Where(dao.YoubanPublishRelayPackage.Columns().Generation, pkg.Generation).Data(do.YoubanPublishRelayPackage{Status: relayconsts.RelayStatusPending, Generation: pkg.Generation + 1}).Update(); err != nil {
			return err
		}
		return s.deferRelayPackageJob(ctx, job)
	}
	if err = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPublishing); err != nil {
		return err
	}
	if err = s.upsertPublishJobRecord(ctx, job, "sending", ""); err != nil {
		return err
	}
	allowed, err := s.canSendTelegramJob(ctx, job)
	if err != nil {
		return err
	}
	if !allowed {
		return errTelegramJobSuperseded
	}
	// Package marker is present on every source message, including album followers.
	deliveryAttemptStarted = true
	attempt, err := s.prepareRelayPackageAttempt(ctx, job, pkg, manifest)
	if err != nil {
		return err
	}
	ids := make([]int, len(manifest))
	for i, m := range manifest {
		ids[i] = int(m.SourceID)
	}
	stop := s.startTelegramAttemptHeartbeat(attempt.Id)
	defer stop()
	copied, sendErr := bot.CopyMessages(ctx, &tgbot.CopyMessagesParams{ChatID: job.TargetChatId, FromChatID: channel.TargetChatId, MessageIDs: ids})
	if sendErr != nil {
		if isTelegramAmbiguousDeliveryError(sendErr) {
			if err = s.waitTelegramDeliveryWebhook(context.WithoutCancel(ctx), attempt, sendErr); err != nil {
				return err
			}
			return errTelegramAwaitingWebhook
		}
		s.abandonTelegramDeliveryAttempt(ctx, attempt, nil, sendErr)
		// A definite rejection is safe to retry; keep the staged package and return through normal rate handling.
		_ = s.updateTelegramJobSendPhase(ctx, job.Id, relayconsts.RelaySendPhaseReady)
		if strings.Contains(strings.ToLower(sendErr.Error()), "message to copy not found") {
			// Only a definite source rejection may rebuild on a different relay.
			cc := dao.YoubanPublishRelayChannel.Columns()
			if _, err = dao.YoubanPublishRelayChannel.Ctx(ctx).Where(cc.Id, channel.Id).Data(do.YoubanPublishRelayChannel{HealthStatus: relayChannelHealthCooldown, CooldownUntil: gtime.Now().Add(time.Minute), LastError: sendErr.Error()}).Update(); err != nil {
				return err
			}
			pc := dao.YoubanPublishRelayPackage.Columns()
			if _, err = dao.YoubanPublishRelayPackage.Ctx(ctx).Where(pc.Id, pkg.Id).Where(pc.Generation, pkg.Generation).Data(do.YoubanPublishRelayPackage{Status: relayconsts.RelayStatusPending, Generation: pkg.Generation + 1, LeaseUntil: gtime.Now()}).Update(); err != nil {
				return err
			}
			return s.deferRelayPackageJob(ctx, job)
		}
		return sendErr
	}
	messages, mapErr := relayPackageCopiedMessages(manifest, copied)
	if mapErr != nil {
		// IDs cannot be positionally assigned when Telegram skips a source. Keep raw evidence; do not resend.
		raw := make([]*telegramSentMessage, 0, len(copied))
		for _, id := range copied {
			raw = append(raw, &telegramSentMessage{MessageId: int64(id.ID), Purpose: "package"})
		}
		_ = s.recordTelegramAttemptMessages(context.WithoutCancel(ctx), attempt, raw, "partial_response")
		if err = s.waitTelegramDeliveryWebhook(context.WithoutCancel(ctx), attempt, mapErr); err != nil {
			return err
		}
		return errTelegramAwaitingWebhook
	}
	if err = s.confirmTelegramAPISuccess(ctx, job, attempt, messages); err != nil {
		return gerror.Wrap(errTelegramRemoteConfirmed, err.Error())
	}
	return s.finalizeRelayPackageJob(ctx, job, pkg)
}

func (s *sSysPublish) finalizeRelayPackageJob(ctx context.Context, job telegramJobRecord, pkg *entity.YoubanPublishRelayPackage) error {
	var payload relayPackagePayload
	if err := json.Unmarshal([]byte(pkg.Payload), &payload); err != nil {
		return err
	}
	if _, err := dao.YoubanPublishRelayPackage.Ctx(ctx).WherePri(pkg.Id).Data(do.YoubanPublishRelayPackage{Status: relayconsts.RelayStatusConfirmed}).Update(); err != nil {
		return err
	}
	if len(payload.Verify) > 0 {
		if err := s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseVerifyConfirmed); err != nil {
			return err
		}
	}
	if err := s.finalizeTelegramMediaDelivery(ctx, job, len(payload.Verify) > 0, false); err != nil {
		return err
	}
	return nil
}

// Releasing the publish Worker does not consume the business retry counter.
func (s *sSysPublish) deferRelayPackageJob(ctx context.Context, job telegramJobRecord) error {
	_, err := s.transitionTelegramJob(ctx, telegramJobTransition{JobId: job.Id, From: []string{telegramJobStatusSending}, To: telegramJobStatusRetryWait, RetryDelay: 2 * time.Second, Additional: g.Map{"error_message": "中转资料准备中"}})
	if err != nil {
		return err
	}
	return errTelegramMediaFallbackQueued
}

func (s *sSysPublish) dispatchRelayPackage(ctx context.Context, pkg *entity.YoubanPublishRelayPackage) error {
	c := dao.YoubanPublishRelayPackage.Columns()
	if pkg.Status == relayconsts.RelayStatusPreparing && pkg.LeaseUntil != nil && pkg.LeaseUntil.After(gtime.Now()) {
		// Repair a DB-commit / task-submit crash with the same deterministic task key.
		return s.submitRelayPrepare(ctx, pkg)
	}
	if pkg.Status == relayconsts.RelayStatusPreparing {
		if _, err := dao.YoubanPublishRelayPackage.Ctx(ctx).Where(c.Id, pkg.Id).Where(c.Generation, pkg.Generation).Data(do.YoubanPublishRelayPackage{Status: relayconsts.RelayStatusPending, Generation: pkg.Generation + 1}).Update(); err != nil {
			return err
		}
		pkg.Status = relayconsts.RelayStatusPending
		pkg.Generation++
	}
	if pkg.NextRetryAt != nil && pkg.NextRetryAt.After(gtime.Now()) {
		return nil
	}
	selected, err := s.reserveRelayPackageChannel(ctx, pkg)
	if err != nil {
		return err
	}
	if selected.Id == 0 {
		return nil
	}
	pkg.RelayChannelId = selected.Id
	pkg.Status = relayconsts.RelayStatusPreparing
	return s.submitRelayPrepare(ctx, pkg)
}

func (s *sSysPublish) reserveRelayPackageChannel(ctx context.Context, pkg *entity.YoubanPublishRelayPackage) (selected entity.YoubanPublishRelayChannel, resultErr error) {
	c := dao.YoubanPublishRelayPackage.Columns()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Serialize only the short selection/reservation, never Telegram I/O or account rows.
		if g.DB().GetConfig().Type == "pgsql" {
			if _, err := tx.Exec("SET LOCAL lock_timeout = '2s'"); err != nil {
				return err
			}
			if _, err := tx.Exec("SET LOCAL statement_timeout = '5s'"); err != nil {
				return err
			}
			if _, err := tx.Exec("SELECT pg_advisory_xact_lock(?)", pkg.TenantId^int64(0x72656c6179)); err != nil {
				return err
			}
		}
		cc := dao.YoubanPublishRelayChannel.Columns()
		rows, err := tx.Model(dao.YoubanPublishRelayChannel.Table()).Where(cc.TenantId, pkg.TenantId).Where(cc.Status, relayChannelStatusEnabled).
			Where("("+cc.HealthStatus+"=? OR ("+cc.HealthStatus+"=? AND "+cc.CooldownUntil+"<=?))", relayChannelHealthReady, relayChannelHealthCooldown, gtime.Now()).
			Where("("+cc.CooldownUntil+" IS NULL OR "+cc.CooldownUntil+"<=?)", gtime.Now()).Order(cc.LastUsedAt + " ASC," + cc.Id + " ASC").Limit(100).LockUpdate().All()
		if err != nil {
			return err
		}
		loads, err := tx.Model(dao.YoubanPublishRelayPackage.Table()).Fields(c.RelayChannelId+",COUNT(*) AS total").Where(c.TenantId, pkg.TenantId).Where(c.Status, relayconsts.RelayStatusPreparing).WhereGT(c.LeaseUntil, gtime.Now()).Group(c.RelayChannelId).All()
		if err != nil {
			return err
		}
		counts := map[int64]int{}
		for _, load := range loads {
			counts[load[c.RelayChannelId].Int64()] = load["total"].Int()
		}
		best := int(^uint(0) >> 1)
		for _, row := range rows {
			id := row[cc.Id].Int64()
			n := counts[id]
			capacity := row[cc.Capacity].Int()
			if capacity < 1 {
				capacity = 2
			}
			score := n * 1000 / capacity
			if n < capacity && score < best {
				best = score
				if err = row.Struct(&selected); err != nil {
					return err
				}
			}
		}
		if selected.Id == 0 {
			return nil
		}
		result, err := tx.Model(dao.YoubanPublishRelayPackage.Table()).Where(c.Id, pkg.Id).Where(c.Generation, pkg.Generation).WhereIn(c.Status, []string{relayconsts.RelayStatusPending, relayconsts.RelayStatusRetryWait}).Data(do.YoubanPublishRelayPackage{RelayChannelId: selected.Id, Status: relayconsts.RelayStatusPreparing, LeaseUntil: gtime.Now().Add(5 * time.Minute), TaskId: 0}).Update()
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			selected.Id = 0
			return nil
		}
		_, err = tx.Model(dao.YoubanPublishRelayChannel.Table()).Where(cc.Id, selected.Id).Data(do.YoubanPublishRelayChannel{LastUsedAt: gtime.Now()}).Update()
		return err
	})
	return selected, err
}

func (s *sSysPublish) submitRelayPrepare(ctx context.Context, pkg *entity.YoubanPublishRelayPackage) error {
	var channel entity.YoubanPublishRelayChannel
	if err := dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(pkg.RelayChannelId).Scan(&channel); err != nil {
		return err
	}
	if channel.Id == 0 {
		return gerror.New("中转频道不存在")
	}
	id, err := collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{TenantID: pkg.TenantId, AccountID: channel.TgAccountId, TaskType: collectorin.AccountTaskTypeRelayPrepare, TaskKey: fmt.Sprintf("relay-prepare:%d:%d", pkg.Id, pkg.Generation), Priority: collectorin.EventPriorityUrgent, MaxAttempts: 1})
	if err != nil {
		return err
	}
	_, err = dao.YoubanPublishRelayPackage.Ctx(ctx).WherePri(pkg.Id).Where(dao.YoubanPublishRelayPackage.Columns().Generation, pkg.Generation).Data(do.YoubanPublishRelayPackage{TaskId: id}).Update()
	return err
}

func relayFileKey(item *telegramMediaItem) string {
	// Protection is deliberately not cached across jobs: its seed/history is part of existing behavior.
	source := item.AssetHash
	if source == "" {
		source = item.StoragePath + ":" + item.FileUrl
		if item.StoragePath == "" && item.FileUrl == "" {
			source = fmt.Sprintf("media:%d", item.Id)
		}
	}
	sum := sha256.Sum256([]byte(item.MediaType + ":" + source + ":" + item.PosterStoragePath + ":" + item.PosterUrl))
	return hex.EncodeToString(sum[:])
}

func relayManifestMatch(manifest []relayPackageMessage, text string, id int64) (*telegramSentMessage, bool) {
	for _, entry := range manifest {
		if strings.Contains(text, entry.Marker) {
			m := entry.Message
			m.MessageId = id
			m.RelaySourceId = entry.SourceID
			return &m, true
		}
	}
	return nil, false
}

func sortedRelayMessages(messages []*telegramSentMessage) {
	sort.Slice(messages, func(i, j int) bool { return messages[i].MessageId < messages[j].MessageId })
}

var relayCaptionHTML = regexp.MustCompile(`<[^>]*>`)

// Reserve BOTH per-item identity and attempt markers; the legacy limiter only preserves the last marker.
func relayCaptionWithMarkers(caption, markers string) string {
	if markers == "" {
		return caption
	}
	if len(utf16.Encode([]rune(caption+markers))) <= telegramMediaCaptionLimit {
		return caption + markers
	}
	plain := html.UnescapeString(relayCaptionHTML.ReplaceAllString(caption, ""))
	runes := []rune(plain)
	max := telegramMediaCaptionLimit - len(utf16.Encode([]rune(markers))) - 1
	for len(runes) > 0 && len(utf16.Encode(runes)) > max {
		runes = runes[:len(runes)-1]
	}
	return html.EscapeString(string(runes)+"…") + markers
}

// Bound Webhook lookup to one package query instead of one query per historical Attempt.
func (s *sSysPublish) relayPackageAttemptMarkers(ctx context.Context, attempts []telegramDeliveryAttempt) (map[int64]string, error) {
	ids := []int64{}
	seen := map[int64]bool{}
	for _, attempt := range attempts {
		if attempt.Phase == "package" && !seen[attempt.JobId] {
			ids = append(ids, attempt.JobId)
			seen[attempt.JobId] = true
		}
	}
	markers := map[int64]string{}
	if len(ids) == 0 {
		return markers, nil
	}
	c := dao.YoubanPublishRelayPackage.Columns()
	var packages []*entity.YoubanPublishRelayPackage
	if err := dao.YoubanPublishRelayPackage.Ctx(ctx).Master().Fields(c.Id, c.Generation, c.AttemptId).WhereIn(c.JobId, ids).Scan(&packages); err != nil {
		return nil, err
	}
	for _, pkg := range packages {
		if pkg.AttemptId > 0 {
			markers[pkg.AttemptId] = telegramAttemptMarker(relayPackageToken(pkg))
		}
	}
	return markers, nil
}
