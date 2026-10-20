package sys

import (
	"context"
	"encoding/json"
	relayconsts "hotgo/addons/youban_publish/consts"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
)

func (s *sSysPublish) relayPackageWebhookMessage(ctx context.Context, attempt telegramDeliveryAttempt, msg *models.Message) (*telegramSentMessage, error) {
	pkg, err := s.relayPackageByJob(ctx, attempt.JobId)
	if err != nil {
		return nil, err
	}
	if pkg.Id == 0 || pkg.TelegramUserId <= 0 || pkg.AttemptId != attempt.Id {
		return nil, gerror.New("中转包不存在")
	}
	manifest, err := decodeRelayManifest(pkg.Messages)
	if err != nil {
		return nil, err
	}
	text := firstNonEmpty(msg.Caption, msg.Text)
	if !strings.Contains(text, telegramAttemptMarker(relayPackageToken(pkg))) {
		return nil, nil
	}
	sent, ok := relayManifestMatch(manifest, text, int64(msg.ID))
	if !ok {
		return nil, nil
	}
	sent.MediaGroupId = msg.MediaGroupID
	sent.TgFileId = telegramMessageFileId(msg)
	sent.TgFileUniqueId = telegramMessageFileUniqueId(msg)
	return sent, nil
}

func validateRelayDelivery(manifest []relayPackageMessage, messages []*telegramSentMessage) error {
	if len(manifest) != len(messages) {
		return gerror.Wrap(errTelegramAttemptAuditIncomplete, "中转整包投递证据数量不足")
	}
	expected := map[int64]telegramSentMessage{}
	for _, m := range manifest {
		if m.SourceID <= 0 || (m.Message.Purpose != "display" && m.Message.Purpose != "verify") {
			return gerror.Wrap(errTelegramAttemptAuditIncomplete, "中转整包清单无效")
		}
		if _, duplicate := expected[m.SourceID]; duplicate {
			return gerror.Wrap(errTelegramAttemptAuditIncomplete, "中转整包来源重复")
		}
		expected[m.SourceID] = m.Message
	}
	seenTargets := map[int64]bool{}
	for _, m := range messages {
		if m == nil {
			return gerror.Wrap(errTelegramAttemptAuditIncomplete, "中转整包投递证据为空")
		}
		original, exists := expected[m.RelaySourceId]
		if !exists || m.MessageId <= 0 || seenTargets[m.MessageId] || m.Purpose != original.Purpose || m.MediaId != original.MediaId {
			return gerror.Wrap(errTelegramAttemptAuditIncomplete, "中转整包投递证据归属不完整")
		}
		seenTargets[m.MessageId] = true
		delete(expected, m.RelaySourceId)
	}
	if len(expected) > 0 {
		return gerror.Wrap(errTelegramAttemptAuditIncomplete, "中转整包投递证据重复")
	}
	return nil
}

func (s *sSysPublish) restoreUnknownRelayPackage(ctx context.Context, job telegramJobRecord) (bool, error) {
	restored, err := s.restoreConfirmedTelegramResponse(ctx, job, "package", nil)
	if err != nil || !restored {
		return restored, err
	}
	changed, err := s.transitionTelegramJob(ctx, telegramJobTransition{JobId: job.Id, From: []string{telegramJobStatusConfirming}, To: telegramJobStatusSending, Additional: g.Map{"error_message": ""}})
	if err != nil || !changed {
		return true, err
	}
	pkg, err := s.relayPackageByJob(ctx, job.Id)
	if err != nil {
		return true, err
	}
	if err = s.finalizeRelayPackageJob(ctx, job, pkg); err != nil {
		return true, err
	}
	return true, s.completeTelegramJobAndWakeChannel(ctx, job)
}

// Never takes over with a second send when history is empty or partial.
func (s *sSysPublish) reconcileRelayPackageWithClient(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask, job telegramJobRecord, attempt telegramDeliveryAttempt) error {
	if job.Status != telegramJobStatusConfirming {
		return nil
	}
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return err
	}
	if channel.TgAccountId != task.AccountID {
		return gerror.New("整包对账协议号与目标频道不一致")
	}
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return err
	}
	pkg, err := s.relayPackageByJob(ctx, job.Id)
	if err != nil {
		return err
	}
	manifest, err := decodeRelayManifest(pkg.Messages)
	if err != nil {
		return err
	}
	// Bounded history window; absence is uncertainty, never proof of non-delivery.
	history, err := client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: peer, Limit: 100})
	if err != nil {
		return err
	}
	messages := []*telegramSentMessage{}
	marker := telegramAttemptMarker(relayPackageToken(pkg))
	for _, msg := range tgHistoryMessages(history) {
		if msg == nil || !strings.Contains(msg.Message, marker) {
			continue
		}
		m, ok := relayManifestMatch(manifest, msg.Message, int64(msg.ID))
		if ok {
			messages = append(messages, m)
		}
	}
	if err = validateRelayDelivery(manifest, messages); err != nil {
		if !accountTaskFinalAttempt(task) {
			return err
		}
		return s.failTelegramMediaFallbackJob(ctx, job, gerror.Newf("整包复制结果未完整核实，禁止自动补发，已找到%d/%d条，请核对频道", len(messages), len(manifest)))
	}
	sortedRelayMessages(messages)
	if err = s.confirmTelegramDeliveryAttempt(ctx, attempt, messages, false); err != nil {
		return err
	}
	if err = s.saveTelegramSentMessages(ctx, job, messages); err != nil {
		return err
	}
	changed, err := s.transitionTelegramJob(ctx, telegramJobTransition{JobId: job.Id, From: []string{telegramJobStatusConfirming}, To: telegramJobStatusSending, Additional: g.Map{"send_phase": relayconsts.RelaySendPhaseConfirmed, "error_message": ""}})
	if err != nil || !changed {
		return err
	}
	if err = s.finalizeRelayPackageJob(ctx, job, pkg); err != nil {
		return err
	}
	return s.completeTelegramJobAndWakeChannel(ctx, job)
}

func (s *sSysPublish) validateRestoredRelayMessages(ctx context.Context, jobID int64, messages []*telegramSentMessage) error {
	pkg, err := s.relayPackageByJob(ctx, jobID)
	if err != nil {
		return err
	}
	manifest, err := decodeRelayManifest(pkg.Messages)
	if err != nil {
		return err
	}
	return validateRelayDelivery(manifest, messages)
}

func (s *sSysPublish) relayWebhookComplete(ctx context.Context, attempt telegramDeliveryAttempt) bool {
	var rows []telegramAttemptMessageRecord
	if err := g.DB().Model(publishTgAttemptMessageTable).Safe().Ctx(ctx).Where("attempt_id", attempt.Id).Scan(&rows); err != nil {
		return false
	}
	messages := []*telegramSentMessage{}
	for _, row := range rows {
		var m telegramSentMessage
		if err := json.Unmarshal([]byte(row.MessagePayload), &m); err != nil {
			return false
		}
		messages = append(messages, &m)
	}
	return s.validateRestoredRelayMessages(ctx, attempt.JobId, messages) == nil
}
