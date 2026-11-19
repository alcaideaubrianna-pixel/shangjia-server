package sys

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/utility/dbtime"
	"strings"
)

// cycleChannelAllowed 任务准备和发送前复用同一频道配置拦截；手工上架与删除不受循环开关影响。
// cycleProfileScope identifies the tenant-owned profile policy at a cycle boundary.
type cycleProfileScope struct {
	ProfileID int64
	AccountID int64
}

func cycleChannelAllowed(ctx context.Context, tenantID, channelID int64, operation string, profiles ...cycleProfileScope) (bool, error) {
	if !isCycleBatchOperation(operation) {
		return true, nil
	}
	var channel channelCycleRecord
	err := g.DB().Model(publishChannelTable).Ctx(ctx).Where("tenant_id", tenantID).Where("id", channelID).Scan(&channel)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if channel.Id <= 0 || channel.Enabled != 1 || channel.Status != 1 || channel.PublishDirection != "up" {
		return false, nil
	}
	mode := strings.ToLower(strings.TrimSpace(channel.Mode))
	independent := false
	independentDays := 0
	if len(profiles) > 0 && profiles[0].ProfileID > 0 {
		scope := profiles[0]
		value, lookupErr := g.DB().Model(publishProfileStateTable).Ctx(ctx).
			Where("tenant_id", tenantID).Where("account_id", scope.AccountID).
			Where("profile_id", scope.ProfileID).WhereNull("deleted_at").
			Fields("cycle_override_enabled,cycle_override_days").One()
		if lookupErr != nil {
			return false, lookupErr
		}
		independent = value["cycle_override_enabled"].Bool()
		independentDays = value["cycle_override_days"].Int()
	}
	if strings.HasPrefix(operation, "profile_cycle:") {
		if independent {
			if independentDays <= 0 {
				return false, nil
			}
			independentDays = new(sSysPublish).effectiveProfileCycleOverrideDays(ctx, tenantID, independentDays)
			scope := profiles[0]
			var last struct {
				SentAt *gtime.Time `orm:"sent_at"`
			}
			lookupErr := g.DB().Model(publishChannelProfileTable+" cp").Ctx(ctx).
				InnerJoin(publishTgJobTable+" j", "j.id=cp.last_job_id").
				Fields("j.sent_at").Where("cp.tenant_id", tenantID).
				Where("cp.account_id", scope.AccountID).Where("cp.profile_id", scope.ProfileID).
				Where("cp.channel_id", channelID).Where("cp.status", "active").
				Where("j.tenant_id", tenantID).Where("j.account_id", scope.AccountID).
				Where("j.profile_id", scope.ProfileID).Where("j.channel_id", channelID).
				WhereNotNull("j.sent_at").Scan(&last)
			if lookupErr != nil {
				if errors.Is(lookupErr, sql.ErrNoRows) {
					return false, nil
				}
				return false, lookupErr
			}
			// A queued child created under the old interval must not send before
			// the freshly saved independent interval, even before projection catches up.
			if last.SentAt == nil {
				return false, nil
			}
			nextAt := nextProfileCycleAt(ctx, independentDays, "", dbtime.FromTimestamp(last.SentAt), scope.ProfileID, channelID)
			return nextAt != nil && !nextAt.After(gtime.Now()), nil
		}
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
		// 恢复期间旧批次 run 可能已被清理；对应 job 已失去调度归属，
		// 必须按失效循环任务收口，不能让它永久停留 pending 阻塞频道。
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return run.Id > 0 && !independent && (!scheduledBatchCycleRun(run) || mode == "batch"), nil
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
		result.CanDeleteMessages = 0
	}
	raw := mergeChannelBotPermissionState(value.String(), result)
	_, err = tx.Model(publishChannelTable).Where("tenant_id", job.TenantId).Where("id", job.ChannelId).Data(g.Map{"bot_permission_status_json": raw}).Update()
	return err
}
