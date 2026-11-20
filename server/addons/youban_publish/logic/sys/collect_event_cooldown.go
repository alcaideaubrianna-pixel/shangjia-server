package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

type collectManualReplayContextKey struct{}

type collectEventCooldown struct {
	Version string `json:"version"`
	Message string `json:"message"`
	RetryAt int64  `json:"retryAt"`
}

func collectEventCooldownKey(event gdb.Record) string {
	return fmt.Sprintf("%s%d:%d:%d:%d", publishconsts.CollectEventCooldownKeyPrefix,
		event["tenant_id"].Int64(), event["account_id"].Int64(), event["source_id"].Int64(), event["id"].Int64())
}

// Recovery may touch updated_at; that timestamp is not a content version.
func collectEventCooldownVersion(event gdb.Record) string {
	return collectHash(fmt.Sprintf("%q:%q:%d:%q:%d:%q", event["text_hash"].String(), event["raw_text"].String(),
		event["media_count"].Int(), event["material_role"].String(), event["material_parent_event_id"].Int64(), event["material_group_status"].String()))
}

func collectEventCooldownRemaining(event gdb.Record, entry collectEventCooldown, now time.Time) time.Duration {
	if event["status"].String() != sysin.CollectEventStatusMediaReady ||
		entry.Version != collectEventCooldownVersion(event) || entry.Message != event["error_message"].String() {
		return 0
	}
	remaining := time.UnixMilli(entry.RetryAt).Sub(now)
	if remaining <= 0 {
		return 0
	}
	// Never let a malformed cache value create an unbounded suspension.
	return min(remaining, collectConfigurationRetryDelay)
}

func collectEventConfigurationCooldown(ctx context.Context, event gdb.Record) error {
	return checkCollectEventConfigurationCooldown(ctx, event, time.Now(), func(ctx context.Context, key string) (*gvar.Var, error) {
		if !cache.Initialized() {
			return nil, newCollectProcessRetryError(collectProcessRedisRetryDelay, "等待事件冷却缓存恢复")
		}
		return cache.Instance().Get(ctx, key)
	})
}

func checkCollectEventConfigurationCooldown(ctx context.Context, event gdb.Record, now time.Time, read func(context.Context, string) (*gvar.Var, error)) error {
	if id, ok := ctx.Value(collectManualReplayContextKey{}).(int64); ok && id == event["id"].Int64() {
		return nil
	}
	message := event["error_message"].String()
	if event["status"].String() != sysin.CollectEventStatusMediaReady ||
		(!strings.Contains(message, "采集规则未配置目标频道") && !strings.Contains(message, "存在无权操作或不可用的推送频道")) {
		return nil
	}
	value, err := read(ctx, collectEventCooldownKey(event))
	if err != nil {
		return gerror.Wrap(err, "读取采集事件冷却状态失败")
	}
	if value == nil || value.IsNil() {
		return nil
	}
	var entry collectEventCooldown
	if err = value.Scan(&entry); err != nil {
		return gerror.Wrap(err, "解析采集事件冷却状态失败")
	}
	if remaining := collectEventCooldownRemaining(event, entry, now); remaining > 0 {
		g.Log().Info(ctx, "采集事件跨通道冷却，跳过重复业务处理", g.Map{
			"event": "collect.event.cooldown_wait", "collectEventId": event["id"].Int64(),
			"tenantId": event["tenant_id"].Int64(), "accountId": event["account_id"].Int64(),
			"sourceId": event["source_id"].Int64(), "retryAtUnixMs": entry.RetryAt, "remainingMs": remaining.Milliseconds(),
		})
		return newCollectProcessRetryError(remaining, "采集事件配置错误冷却中，等待统一重试时间")
	}
	return nil
}

// Called under the existing event lock, only after the failure is persisted.
// Duplicate wakeups read this deadline and never extend it.
func saveCollectEventConfigurationCooldown(ctx context.Context, event gdb.Record, message string, delay time.Duration) error {
	if !cache.Initialized() {
		return gerror.New("采集事件冷却缓存未初始化")
	}
	entry := collectEventCooldown{Version: collectEventCooldownVersion(event), Message: message, RetryAt: time.Now().Add(delay).UnixMilli()}
	return gerror.Wrap(cache.Instance().Set(ctx, collectEventCooldownKey(event), entry, delay), "保存采集事件冷却状态失败")
}
