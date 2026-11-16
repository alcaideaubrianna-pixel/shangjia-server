package sys

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	publishconsts "hotgo/addons/youban_publish/consts"
)

// telegramChannelQueueDispatcher bounds local work across polling passes.
// The existing distributed channel lease still guards cross-process sends.
type telegramChannelQueueDispatcher struct {
	sem     chan struct{}
	mu      sync.Mutex
	running map[int64]struct{}
	wg      sync.WaitGroup
	next    int
}

// dispatch resumes after the last admitted channel so a fixed active list is fair.
// Only the scheduling loop calls dispatch; submit protects in-flight state.
func (d *telegramChannelQueueDispatcher) dispatch(ctx context.Context, channels []int64, process func(int64)) {
	if len(channels) == 0 {
		d.next = 0
		return
	}
	start := d.next % len(channels)
	for i := 0; i < len(channels); i++ {
		index := (start + i) % len(channels)
		if d.submit(ctx, channels[index], process) {
			d.next = (index + 1) % len(channels)
		}
	}
}

// runTelegramChannelQueueSchedule owns all submissions and waits only at shutdown.
// Recovery has its own single slot, so a slow recovery cannot stall polling.
func runTelegramChannelQueueSchedule(ctx context.Context, ticks, recoveryTicks <-chan time.Time, pass, recover func(context.Context)) {
	var recovery sync.WaitGroup
	busy := make(chan struct{}, 1)
	defer recovery.Wait()
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if ctx.Err() == nil {
				pass(ctx)
			}
		case <-recoveryTicks:
			if ctx.Err() != nil {
				return
			}
			select {
			case busy <- struct{}{}:
				recovery.Add(1)
				go func() {
					defer recovery.Done()
					defer func() { <-busy }()
					recover(ctx)
				}()
			default:
			}
		}
	}
}

func newTelegramChannelQueueDispatcher(limit int) *telegramChannelQueueDispatcher {
	if limit <= 0 {
		limit = 1
	}
	return &telegramChannelQueueDispatcher{
		sem:     make(chan struct{}, limit),
		running: make(map[int64]struct{}),
	}
}

func (d *telegramChannelQueueDispatcher) submit(ctx context.Context, channelID int64, process func(int64)) bool {
	d.mu.Lock()
	if ctx.Err() != nil || channelID <= 0 {
		d.mu.Unlock()
		return false
	}
	if _, exists := d.running[channelID]; exists {
		d.mu.Unlock()
		return false
	}
	select {
	case d.sem <- struct{}{}:
	default:
		d.mu.Unlock()
		return false
	}
	d.running[channelID] = struct{}{}
	d.wg.Add(1)
	d.mu.Unlock()

	go func() {
		defer d.wg.Done()
		defer func() {
			<-d.sem
			d.mu.Lock()
			delete(d.running, channelID)
			d.mu.Unlock()
		}()
		if ctx.Err() == nil {
			process(channelID)
		}
	}()
	return true
}

func (d *telegramChannelQueueDispatcher) wait() {
	d.wg.Wait()
}

func (s *sSysPublish) runTelegramChannelQueueWorker(ctx context.Context) {
	dispatcher := newTelegramChannelQueueDispatcher(channelQueueWorkerConcurrency())
	ticker := time.NewTicker(500 * time.Millisecond)
	// The Redis active-channel set is only a fast index. Jobs created before
	// the index write, or jobs whose Redis enqueue was interrupted, remain in
	// PostgreSQL with dispatch_status=idle. Recover those channel heads
	// frequently so one lost wake-up cannot leave an entire channel invisible
	// to the worker for minutes.
	recoveryTicker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	defer recoveryTicker.Stop()
	defer dispatcher.wait()
	runTelegramChannelQueueSchedule(ctx, ticker.C, recoveryTicker.C,
		func(ctx context.Context) { s.processTelegramChannelQueuePass(ctx, dispatcher) },
		func(ctx context.Context) {
			if err := s.rehydrateMissingTelegramChannelQueue(ctx, telegramChannelRecoveryBatchSize()); err != nil {
				g.Log().Warningf(ctx, "频道队列低频恢复失败 err:%+v", err)
			}
		})
}

func telegramChannelRecoveryBatchSize() int {
	const (
		defaultSize = 500
		maxSize     = 2000
	)
	raw := strings.TrimSpace(os.Getenv("YOUBAN_PUBLISH_TG_CHANNEL_RECOVERY_BATCH_SIZE"))
	if raw == "" {
		return defaultSize
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultSize
	}
	if value > maxSize {
		return maxSize
	}
	return value
}

func (s *sSysPublish) processTelegramChannelQueuePass(ctx context.Context, dispatcher *telegramChannelQueueDispatcher) {
	if !telegramChannelQueueEnabled() {
		return
	}
	channels, err := activeTelegramChannelIDs(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "频道活跃集合读取失败 err:%+v", err)
		return
	}
	dispatcher.dispatch(ctx, channels, func(channelID int64) {
		processed, processErr := s.processTelegramChannelQueueHead(ctx, channelID)
		if processErr != nil {
			g.Log().Warningf(ctx, "频道队列消费失败 channel:%d err:%+v", channelID, processErr)
			return
		}
		if !processed {
			if removeErr := removeActiveTelegramChannel(ctx, channelID); removeErr != nil {
				g.Log().Warningf(ctx, "频道活跃集合清理失败 channel:%d err:%+v", channelID, removeErr)
			}
		}
	})
}

func channelQueueWorkerConcurrency() int {
	limit := 128
	if raw := os.Getenv("YOUBAN_PUBLISH_CHANNEL_WORKER_CONCURRENCY"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			limit = value
		}
	}
	return limit
}

// telegramChannelCursorNext returns the last channel visited by a pass. It is
// kept separate from the database query so cursor wrap behavior is unit
// testable without a live database.
type telegramChannelQueueChannel struct {
	ChannelID int64 `json:"channel_id"`
}

func telegramChannelCursorNext(current int64, channels []telegramChannelQueueChannel) int64 {
	if len(channels) == 0 {
		return 0
	}
	next := current
	for _, channel := range channels {
		if channel.ChannelID > next {
			next = channel.ChannelID
		}
	}
	return next
}

// rehydrateMissingTelegramChannelQueue 优先从 Redis 领取凭证和旧队列重建唤醒。
// SCAN 游标持久推进，避免固定前 N 个频道；禁止在实时循环扫描业务大表。
func (s *sSysPublish) rehydrateMissingTelegramChannelQueue(ctx context.Context, limit int) (returnedErr error) {
	startedAt := time.Now()
	logCtx := ctx
	scanned, recovered, failed, pending, dueCount, activeRestored := 0, 0, 0, 0, 0, 0
	stage, reason, result := "clock", "round_completed", "success"
	cursor, nextCursor, summaryToken, cursorPayload := "", "", "", ""
	defer func() {
		returnedErr = telegramQueueRedactError(returnedErr, summaryToken, cursorPayload)
		logErr := returnedErr
		if logErr != nil {
			failed++
			result = "error"
			reason = stage + "_failed"

		}
		// 无错误、无恢复且未丢失游标租约时不输出空扫描 Info。
		if logErr == nil && failed == 0 && recovered == 0 && activeRestored == 0 && result != "rejected" {
			return
		}
		logTelegramQueueEvent(logCtx, "rehydrate_summary", g.Map{
			"stage": stage, "reason": reason, "result": result, "durationMs": time.Since(startedAt).Milliseconds(),
			"scanned": scanned, "recovered": recovered, "failed": failed, "pending": pending,
			"cursor": cursor, "nextCursor": nextCursor, "limit": limit, "dueCount": dueCount, "activeRestored": activeRestored,
		}, logErr)
	}()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	// 索引只定位已过期频道；每条恢复脚本仍检查 Redis 时钟，续期不会被旧快照覆盖。
	clock, err := telegramChannelRedisDo(ctx, "TIME")
	if err != nil {
		return err
	}
	clockParts := clock.Array()
	if len(clockParts) != 2 {
		result = "rejected"
		reason = "invalid_clock_response"
		return nil
	}
	nowMillis := gconv.Int64(clockParts[0])*1000 + gconv.Int64(clockParts[1])/1000
	stage = "due_candidates"
	due, err := telegramChannelRedisDo(ctx, "ZRANGEBYSCORE", telegramChannelProcessingDueKey, "-inf", nowMillis, "LIMIT", 0, limit)
	if err != nil {
		return err
	}
	dueCount = len(due.Array())
	stage = "due_recovery"
	for _, v := range due.Array() {
		if err := ctx.Err(); err != nil {
			return err
		}
		id := gconv.Int64(v)
		if id <= 0 {
			continue
		}
		if changed, recoverErr := recoverExpiredTelegramChannelClaim(ctx, id); recoverErr != nil {
			failed++
		} else if changed {
			recovered++
		}
	}
	stage = "cursor_token"
	lockToken, err := telegramChannelClaimToken()
	if err != nil {
		return err
	}
	summaryToken = lockToken
	stage = "cursor_lock"
	locked, lockErr := telegramChannelRedisDo(ctx, "SET", telegramChannelRehydrateLockKey, lockToken, "NX", "PX", 8000)
	if lockErr != nil {
		return lockErr
	}
	if locked.String() != "OK" {
		result = "skipped"
		reason = "cursor_lock_busy"
		return nil
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer stop()
		_, _ = telegramChannelRedisDo(cleanupCtx, "EVAL", "if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0", 1, telegramChannelRehydrateLockKey, lockToken)
	}()
	stage = "cursor_read"
	cursorValue, err := telegramChannelRedisDo(ctx, "GET", telegramChannelRehydrateCursorKey)
	if err != nil {
		return err
	}
	state := telegramChannelRehydratePage{Cursor: "0"}
	if raw := cursorValue.String(); raw != "" {
		if strings.HasPrefix(raw, "{") {
			if err := json.Unmarshal([]byte(raw), &state); err != nil {
				return err
			}
		} else {
			state.Cursor = raw
		}
	}
	cursor = state.Cursor
	nextCursor = state.Cursor
	pending = len(state.Pending)
	if len(state.Pending) == 0 {
		stage = "scan"
		page, scanErr := telegramChannelRedisDo(ctx, "SCAN", state.Cursor, "MATCH", "youban:tg:channel:*", "COUNT", limit)
		if scanErr != nil {
			return scanErr
		}
		values := page.Array()
		if len(values) != 2 {
			result = "rejected"
			reason = "invalid_scan_response"
			return nil
		}
		state.Cursor = gconv.String(values[0])
		state.Pending = gconv.Strings(values[1])
	}
	nextCursor = state.Cursor
	pending = len(state.Pending)
	stage = "scan_restore"
	// COUNT 只是 Redis 扫描提示；超额返回保存在 cursor 状态，下一轮继续，
	// 每轮实际处理最多 limit 个键，不丢弃一页剩余成员。
	count := len(state.Pending)
	if count > limit {
		count = limit
	}
	for _, key := range state.Pending[:count] {
		if err := ctx.Err(); err != nil {
			return err
		}
		scanned++
		prefix := ""
		for _, candidate := range []string{telegramChannelProcessingPrefix, telegramChannelUrgentQueuePrefix, telegramChannelHistoryQueuePrefix, telegramChannelQueuePrefix} {
			if strings.HasPrefix(key, candidate) {
				prefix = candidate
				break
			}
		}
		if prefix == "" {
			continue
		}
		id, parseErr := strconv.ParseInt(strings.TrimPrefix(key, prefix), 10, 64)
		if parseErr != nil || id <= 0 {
			continue
		}
		if prefix == telegramChannelProcessingPrefix {
			if changed, recoverErr := recoverExpiredTelegramChannelClaim(ctx, id); recoverErr != nil {
				failed++
				logTelegramQueueEvent(ctx, "rehydrate_bad_key", g.Map{"channelId": id, "queue": key, "stage": "scan_processing", "reason": "processing_recovery_failed", "result": "error"}, recoverErr)
				continue
			} else if changed {
				recovered++
			}
		}
		// SCAN 结果可能已被消费；原子检查后才重建 active，避免空频道自激。
		const restore = `if redis.call('EXISTS',KEYS[4])==1 or redis.call('ZCARD',KEYS[1])+redis.call('ZCARD',KEYS[2])+redis.call('ZCARD',KEYS[3])>0 then return redis.call('SADD',KEYS[5],ARGV[1]) end return 0`
		if restored, err := telegramChannelRedisDo(ctx, "EVAL", restore, 5, telegramChannelUrgentQueueKey(id), telegramChannelQueueKey(id), telegramChannelHistoryQueueKey(id), telegramChannelProcessingKey(id), telegramChannelActiveSetKey, strconv.FormatInt(id, 10)); err != nil {
			failed++
			logTelegramQueueEvent(ctx, "rehydrate_bad_key", g.Map{"channelId": id, "queue": key, "stage": "scan_restore", "reason": "active_restore_failed", "result": "error"}, err)
			continue
		} else {
			activeRestored += restored.Int()
		}
	}
	state.Pending = state.Pending[count:]
	pending = len(state.Pending)
	stage = "cursor_encode"
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	cursorPayload = string(encoded)
	stage = "cursor_commit"
	committed, err := commitTelegramChannelRehydrateCursor(ctx, lockToken, string(encoded))
	if err == nil && !committed {
		result = "rejected"
		reason = "cursor_lease_lost"
	}
	if err == nil && committed && failed > 0 {
		result = "partial"
		reason = "channel_errors_isolated"
	}
	return err
}

const telegramChannelRehydrateCursorKey = publishconsts.TelegramChannelRehydrateCursorKey
const telegramChannelRehydrateLockKey = publishconsts.TelegramChannelRehydrateLockKey

type telegramChannelRehydratePage struct {
	Cursor  string   `json:"cursor"`
	Pending []string `json:"pending,omitempty"`
}

func commitTelegramChannelRehydrateCursor(ctx context.Context, token, state string) (bool, error) {
	value, err := telegramChannelRedisDo(ctx, "EVAL", "if redis.call('GET',KEYS[1])==ARGV[1] then redis.call('SET',KEYS[2],ARGV[2]); return 1 end return 0", 2, telegramChannelRehydrateLockKey, telegramChannelRehydrateCursorKey, token, state)
	if err != nil {
		return false, err
	}
	return value.Int() == 1, nil
}
