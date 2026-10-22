package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/grand"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	hglock "hotgo/internal/library/hgrds/lock"
	"hotgo/utility/scantrace"
)

const scanMediaGroupTTL = 2 * time.Minute

const scanMediaBatchQuietPeriod = 300 * time.Millisecond

const scanMediaBatchPollInterval = 50 * time.Millisecond

type scanMediaGroupPending struct {
	BotId     int64                       `json:"botId"`
	ChatId    string                      `json:"chatId"`
	UserId    string                      `json:"userId"`
	CreatedAt int64                       `json:"createdAt"`
	UpdatedAt int64                       `json:"updatedAt"`
	Active    int                         `json:"active"`
	TraceId   string                      `json:"traceId"`
	Items     []*sysin.BotMediaSearchItem `json:"items"`
}

func (s *sSysBot) beginScanMediaGroupItem(ctx context.Context, botId int64, userId string, batchId string, msg *models.Message) error {
	if msg == nil {
		return nil
	}
	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	lock := hglock.NewConfig(10*time.Second, 50*time.Millisecond).Mutex(scanMediaGroupLockKey(botId, userId, batchId))
	if err := lock.Lock(lockCtx); err != nil {
		return err
	}
	defer func() { _ = lock.Unlock(context.Background()) }()

	key := scanMediaGroupKey(botId, userId, batchId)
	pending := &scanMediaGroupPending{}
	value, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return fmt.Errorf("读取扫图批次失败: %w", err)
	}
	isFirst := true
	if value != nil && !value.IsNil() {
		if err := json.Unmarshal([]byte(value.String()), pending); err != nil {
			return fmt.Errorf("解析扫图批次失败: %w", err)
		}
		if pending.BotId != botId {
			return fmt.Errorf("扫图批次机器人不匹配")
		}
		isFirst = false
	}
	if isFirst {
		pending = &scanMediaGroupPending{BotId: botId, ChatId: fmt.Sprintf("%d", msg.Chat.ID), UserId: userId, CreatedAt: time.Now().Unix(), TraceId: scanTraceId(ctx)}
	}
	if pending.TraceId == "" {
		pending.TraceId = scanTraceId(ctx)
	}
	pending.Active++
	pending.UpdatedAt = time.Now().UnixMilli()
	data, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	if err = cache.Instance().Set(ctx, key, string(data), scanMediaGroupTTL); err != nil {
		return err
	}
	if isFirst {
		go s.finishScanMediaGroup(context.WithoutCancel(ctx), botId, userId, batchId)
	}
	return nil
}

func (s *sSysBot) completeScanMediaGroupItem(ctx context.Context, botId int64, userId string, batchId string, items []*sysin.BotMediaSearchItem) error {
	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	lock := hglock.NewConfig(10*time.Second, 50*time.Millisecond).Mutex(scanMediaGroupLockKey(botId, userId, batchId))
	if err := lock.Lock(lockCtx); err != nil {
		return err
	}
	defer func() { _ = lock.Unlock(context.Background()) }()

	key := scanMediaGroupKey(botId, userId, batchId)
	value, err := cache.Instance().Get(ctx, key)
	if err != nil || value == nil || value.IsNil() {
		if err != nil {
			return err
		}
		return fmt.Errorf("扫图批次已过期或不存在")
	}
	var pending scanMediaGroupPending
	if err = json.Unmarshal([]byte(value.String()), &pending); err != nil {
		return err
	}
	pending.Items = append(pending.Items, items...)
	if pending.Active > 0 {
		pending.Active--
	}
	pending.UpdatedAt = time.Now().UnixMilli()
	data, err := json.Marshal(&pending)
	if err != nil {
		return err
	}
	return cache.Instance().Set(ctx, key, string(data), scanMediaGroupTTL)
}

func scanMediaBatchSessionKey(botId int64, userId string) string {
	return fmt.Sprintf("youban_bot:scan_batch:%d:%s", botId, strings.TrimSpace(userId))
}

func (s *sSysBot) scanMediaBatch(ctx context.Context, botId int64, userId string) (string, error) {
	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	lock := hglock.NewConfig(10*time.Second, 50*time.Millisecond).Mutex(scanMediaBatchSessionKey(botId, userId) + ":lock")
	if err := lock.Lock(lockCtx); err != nil {
		return "", err
	}
	defer func() { _ = lock.Unlock(context.Background()) }()

	key := scanMediaBatchSessionKey(botId, userId)
	if value, err := cache.Instance().Get(ctx, key); err == nil && value != nil && !value.IsNil() {
		batchId := strings.TrimSpace(value.String())
		if batchId != "" && !s.scanMediaGroupResolved(ctx, botId, userId, batchId) {
			return batchId, nil
		}
	}
	batchId := strings.ToUpper(grand.S(12))
	if err := cache.Instance().Set(ctx, key, batchId, scanMediaGroupTTL); err != nil {
		return "", err
	}
	return batchId, nil
}

func scanMediaGroupKey(botId int64, userId string, groupId string) string {
	return fmt.Sprintf("youban_bot:scan_group:%d:%s:%s", botId, strings.TrimSpace(userId), strings.TrimSpace(groupId))
}

func scanMediaGroupAckKey(botId int64, userId string, groupId string) string {
	return scanMediaGroupKey(botId, userId, groupId) + ":ack"
}

func scanMediaGroupResolvedKey(botId int64, userId string, groupId string) string {
	return scanMediaGroupKey(botId, userId, groupId) + ":resolved"
}

func scanMediaGroupLockKey(botId int64, userId string, groupId string) string {
	return scanMediaGroupKey(botId, userId, groupId) + ":lock"
}

// acknowledgeScanMedia gives single-media scans the same immediate feedback
// as albums. The message id makes retries/webhook redelivery idempotent.
func (s *sSysBot) acknowledgeScanMedia(ctx context.Context, botId int64, userId string, msg *models.Message) (string, error) {
	if msg == nil {
		return "", nil
	}
	key := fmt.Sprintf("youban_bot:scan_ack:%d:%s:%d", botId, strings.TrimSpace(userId), msg.ID)
	value, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return "", err
	}
	if value != nil && !value.IsNil() {
		if traceId := normalizeScanTraceId(value.String()); traceId != "" {
			return traceId, nil
		}
	}
	traceId := newScanTraceId()
	g.Log().Info(ctx, "Bot扫图确认创建", g.Map{"traceId": traceId, "botId": botId, "userId": userId, "messageId": msg.ID})
	if err := cache.Instance().Set(ctx, key, traceId, scanMediaGroupTTL); err != nil {
		return "", err
	}
	if err := s.sendMessageOnly(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), scanAcknowledgement(traceId)); err != nil {
		_, _ = cache.Instance().Remove(ctx, key)
		return "", err
	}
	return traceId, nil
}

// acknowledgeScanMediaGroup gives immediate feedback before Telegram file
// resolution or fingerprint calculation starts. The distributed lock prevents
// concurrent album updates from sending the acknowledgement more than once.
func (s *sSysBot) acknowledgeScanMediaGroup(ctx context.Context, botId int64, userId string, batchId string, msg *models.Message) (string, error) {
	if msg == nil || strings.TrimSpace(batchId) == "" {
		return "", nil
	}
	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	lock := hglock.NewConfig(10*time.Second, 50*time.Millisecond).Mutex(scanMediaGroupLockKey(botId, userId, batchId))
	if err := lock.Lock(lockCtx); err != nil {
		return "", err
	}
	defer func() { _ = lock.Unlock(context.Background()) }()
	ackKey := scanMediaGroupAckKey(botId, userId, batchId)
	value, err := cache.Instance().Get(ctx, ackKey)
	if err != nil {
		return "", err
	}
	if value != nil && !value.IsNil() {
		if traceId := normalizeScanTraceId(value.String()); traceId != "" {
			return traceId, nil
		}
	}
	traceId := newScanTraceId()
	g.Log().Info(ctx, "Bot扫图批次确认创建", g.Map{"traceId": traceId, "botId": botId, "userId": userId, "batchId": batchId, "messageId": msg.ID})
	if err := cache.Instance().Set(ctx, ackKey, traceId, scanMediaGroupTTL); err != nil {
		return "", err
	}
	if err := s.sendMessageOnly(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), scanAcknowledgement(traceId)); err != nil {
		_, _ = cache.Instance().Remove(ctx, ackKey)
		return "", err
	}
	return traceId, nil
}

type scanTraceContextKey struct{}

func newScanTraceId() string {
	return strings.ToUpper(grand.S(8))
}

func normalizeScanTraceId(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 8 {
		return ""
	}
	return value
}

func withScanTrace(ctx context.Context, traceId string) context.Context {
	traceId = ensureScanTraceId(traceId)
	ctx = scantrace.With(ctx, g.Map{"traceId": traceId})
	return context.WithValue(ctx, scanTraceContextKey{}, traceId)
}

func scanTraceId(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(scanTraceContextKey{}).(string)
	return normalizeScanTraceId(value)
}

func scanAcknowledgement(traceId string) string {
	return fmt.Sprintf("已收到，正在查询，请稍候…\n（追踪号 %s）", ensureScanTraceId(traceId))
}

func ensureScanTraceId(traceId string) string {
	if traceId = normalizeScanTraceId(traceId); traceId != "" {
		return traceId
	}
	return newScanTraceId()
}

func (s *sSysBot) scanMediaGroupResolved(ctx context.Context, botId int64, userId string, groupId string) bool {
	value, err := cache.Instance().Get(ctx, scanMediaGroupResolvedKey(botId, userId, groupId))
	return err == nil && value != nil && !value.IsNil()
}

func (s *sSysBot) claimScanMediaGroupResolved(ctx context.Context, botId int64, userId string, groupId string) (bool, error) {
	return claimScanMediaGroupOnce(ctx, scanMediaGroupResolvedKey(botId, userId, groupId), cache.Instance().SetIfNotExist)
}

func claimScanMediaGroupOnce(ctx context.Context, key string, setIfNotExist func(context.Context, interface{}, interface{}, time.Duration) (bool, error)) (bool, error) {
	return setIfNotExist(ctx, key, 1, scanMediaGroupTTL)
}

func (s *sSysBot) finishScanMediaGroup(ctx context.Context, botId int64, userId string, groupId string) {
	ctx = scantrace.With(ctx, g.Map{"botId": botId, "userId": userId, "batchId": groupId})
	finish := scantrace.Begin(ctx, "album.finalize")
	var finalErr error
	defer func() { finish(finalErr) }()
	key := scanMediaGroupKey(botId, userId, groupId)
	var pending scanMediaGroupPending
	for {
		time.Sleep(scanMediaBatchPollInterval)
		lock := hglock.NewConfig(10*time.Second, 50*time.Millisecond).Mutex(scanMediaGroupLockKey(botId, userId, groupId))
		if err := lock.Lock(ctx); err != nil {
			finalErr = fmt.Errorf("锁定扫图批次失败: %w", err)
			g.Log().Warningf(ctx, "锁定扫图媒体批次失败 botId:%d userId:%s batchId:%s err:%+v", botId, userId, groupId, err)
			return
		}
		value, err := cache.Instance().Get(ctx, key)
		if err != nil || value == nil || value.IsNil() {
			finalErr = fmt.Errorf("扫图批次已过期或不存在")
			if err != nil {
				finalErr = fmt.Errorf("读取扫图批次失败: %w", err)
			}
			_ = lock.Unlock(ctx)
			return
		}
		if err = json.Unmarshal([]byte(value.String()), &pending); err != nil {
			finalErr = fmt.Errorf("解析扫图批次失败: %w", err)
			_ = lock.Unlock(ctx)
			return
		}
		if pending.Active > 0 || (pending.UpdatedAt > 0 && time.Since(time.UnixMilli(pending.UpdatedAt)) < scanMediaBatchQuietPeriod) {
			_ = lock.Unlock(ctx)
			continue
		}
		claimed, claimErr := s.claimScanMediaGroupResolved(ctx, botId, userId, groupId)
		if claimErr != nil {
			finalErr = fmt.Errorf("领取扫图批次失败: %w", claimErr)
			_ = lock.Unlock(ctx)
			g.Log().Warningf(ctx, "领取扫图媒体组失败 botId:%d userId:%s groupId:%s err:%+v", botId, userId, groupId, claimErr)
			return
		}
		if claimed {
			if _, removeErr := cache.Instance().Remove(ctx, key); removeErr != nil {
				g.Log().Warningf(ctx, "清理扫图批次失败 traceId:%s batchId:%s err:%+v", scanTraceId(ctx), groupId, removeErr)
			}
		}
		_ = lock.Unlock(ctx)
		if !claimed || len(pending.Items) == 0 {
			g.Log().Info(ctx, "Bot扫图批次结束", g.Map{"traceId": scanTraceId(ctx), "batchId": groupId, "claimed": claimed, "itemCount": len(pending.Items)})
			return
		}
		break
	}
	ctx = withScanTrace(ctx, pending.TraceId)
	account, err := s.boundProfileAccountByUser(ctx, parseTelegramUserId(userId))
	if err != nil {
		finalErr = err
		observeScanRequest(ctx, botId, "failed")
		_ = s.replyBotError(ctx, botId, pending.ChatId, "扫图搜索", err)
		return
	}
	startedAt := time.Now()
	if err = s.searchScanMediaAndReply(ctx, botId, pending.ChatId, account, pending.Items); err != nil {
		finalErr = err
		observeScanStage(ctx, botId, "group_total", startedAt, err)
		observeScanRequest(ctx, botId, "failed")
		g.Log().Warningf(ctx, "处理扫图媒体组失败 traceId:%s botId:%d userId:%s err:%+v", pending.TraceId, botId, userId, err)
		return
	}
	observeScanStage(ctx, botId, "group_total", startedAt, nil)
	observeScanRequest(ctx, botId, "fingerprint")
}

func parseTelegramUserId(value string) int64 {
	var id int64
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &id)
	return id
}
