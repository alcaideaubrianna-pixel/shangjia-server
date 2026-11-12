package sys

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
	"strings"
)

// cycleChannelAllowed 任务准备和发送前复用同一频道配置拦截；手工上架与删除不受循环开关影响。
func cycleChannelAllowed(ctx context.Context, tenantID, channelID int64, operation string) (bool, error) {
	if !isCycleBatchOperation(operation) {
		return true, nil
	}
	var channel channelCycleRecord
	err := g.DB().Model(publishChannelTable).Ctx(ctx).Where("tenant_id", tenantID).Where("id", channelID).Scan(&channel)
	if err != nil {
		return false, err
	}
	if channel.Id <= 0 || channel.Enabled != 1 || channel.Status != 1 || channel.PublishDirection != "up" {
		return false, nil
	}
	mode := strings.ToLower(strings.TrimSpace(channel.Mode))
	if strings.HasPrefix(operation, "profile_cycle:") {
		return mode == "" || mode == "time", nil
	}
	// 手工触发的整频道循环沿用原语义；只有定时批次要求当前仍为batch。
	var runID int64
	// 新批次任务使用 cycle_batch:due:<runID>:...，兼容迁移期旧的
	// cycle_batch:<runID>:... 格式，不能把 due 当作整数解析。
	if _, err := fmt.Sscanf(operation, "cycle_batch:due:%d:", &runID); err != nil {
		if _, legacyErr := fmt.Sscanf(operation, "cycle_batch:%d:", &runID); legacyErr != nil {
			return false, err
		}
	}
	var run cycleRunRecord
	if err := g.DB().Model(publishCycleRunTable).Ctx(ctx).Where("id", runID).Where("tenant_id", tenantID).Where("channel_id", channelID).Scan(&run); err != nil {
		return false, err
	}
	return run.Id > 0 && (!scheduledBatchCycleRun(run) || mode == "batch"), nil
}

// isDefinitiveTelegramPermissionError 只把明确的发送权限丢失立即熔断，不把限流、超时或媒体错误当成失权。
func isDefinitiveTelegramPermissionError(err error) bool {
	if !isTelegramPermissionCircuitError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, word := range []string{"bot was kicked", "bot is not a member", "not enough rights to send", "chat_write_forbidden", "chat_admin_required", "user_banned_in_channel", "user banned in channel"} {
		if strings.Contains(message, word) {
			return true
		}
	}
	return false
}

// projectChannelPermissionFailure 复用已有权限JSON合并能力，只重置本次失败Bot，不覆盖其他Bot状态。
func (s *sSysPublish) projectChannelPermissionFailure(ctx context.Context, tx gdb.TX, job telegramJobRecord, cause error) error {
	value, err := tx.Model(publishChannelTable).Where("tenant_id", job.TenantId).Where("id", job.ChannelId).LockUpdate().Fields("bot_permission_status_json").Value()
	if err != nil {
		return err
	}
	result := &sysin.ChannelCheckBotModel{BotId: job.BotId, CanSendMessage: 0, Status: "error", Message: cause.Error()}
	if previous := channelBotPermissionStateForBot(value.String(), job.BotId); previous != nil {
		result.BotName = previous.BotName
		result.BotUsername = previous.BotUsername
		result.CanDeleteMessages = previous.CanDeleteMessages
		result.InChannel = previous.InChannel
	}
	if strings.Contains(strings.ToLower(cause.Error()), "kicked") || strings.Contains(strings.ToLower(cause.Error()), "not a member") {
		result.InChannel = 0
	}
	raw := mergeChannelBotPermissionState(value.String(), result)
	_, err = tx.Model(publishChannelTable).Where("tenant_id", job.TenantId).Where("id", job.ChannelId).Data(g.Map{"bot_permission_status_json": raw}).Update()
	return err
}
