package sys

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
)

// Channel queue is a logical ready queue. It is deliberately independent of
// the Asynq transport so the migration can be introduced behind one producer.
// The database job remains the source of truth; Redis stores ready ids and
// token-owned processing receipts. 滚动部署期间旧二进制仍可能破坏式出队，
// 本闭环不能为旧消费者补写凭证；必须完成所有 publish worker 替换后验收。
// 旧 dequeue helper 仅保留在 _test.go，禁止重新引入生产消费路径。
const telegramChannelQueuePrefix = "youban:tg:channel:queue:"
const telegramChannelUrgentQueuePrefix = "youban:tg:channel:urgent:"
const telegramChannelHistoryQueuePrefix = "youban:tg:channel:history:"
const telegramChannelActiveSetKey = "youban:tg:channel:active"

const (
	telegramChannelProcessingPrefix = publishconsts.TelegramChannelProcessingPrefix
	telegramChannelProcessingDueKey = publishconsts.TelegramChannelProcessingDueKey
	telegramChannelClaimTTL         = 30 * time.Second
)

func telegramChannelQueueKey(channelID int64) string {
	return fmt.Sprintf("%s%d", telegramChannelQueuePrefix, channelID)
}

func telegramChannelUrgentQueueKey(channelID int64) string {
	return fmt.Sprintf("%s%d", telegramChannelUrgentQueuePrefix, channelID)
}

func telegramChannelHistoryQueueKey(channelID int64) string {
	return fmt.Sprintf("%s%d", telegramChannelHistoryQueuePrefix, channelID)
}

func telegramChannelQueueKeyForPriority(channelID int64, priority int) string {
	if priority <= tgJobPriorityInteractive {
		return telegramChannelUrgentQueueKey(channelID)
	}
	if priority >= tgJobPriorityHistory {
		return telegramChannelHistoryQueueKey(channelID)
	}
	return telegramChannelQueueKey(channelID)
}

const (
	// The score is lane * scale + dueMillis. The previous format multiplied
	// dueMillis by 1,000 and used a lane scale of 1.8e15. At current Unix time
	// that low part was already close to the scale, so normal lane-4 scores
	// crossed the boundary and were falsely treated as legacy scores forever.
	// Keep the lane scale above the current epoch-millisecond range while the
	// complete score remains far below Redis' exact integer range (2^53).
	telegramChannelQueuePriorityScale int64 = 2_000_000_000_000
	telegramChannelQueueSequenceScale int64 = 1_000
)

// telegramChannelQueuePriorityRank maps the existing publish lanes to the
// queue order. Lower ranks are consumed first; manual/interactive work is
// therefore ahead of realtime, normal, bulk and historical work.
func telegramChannelQueuePriorityRank(priority int) int64 {
	switch {
	case priority <= tgJobPriorityInteractive:
		return 0
	case priority <= tgJobPriorityUrgent:
		return 1
	case priority <= tgJobPriorityDefault:
		return 2
	case priority <= tgJobPriorityBulk:
		return 3
	default:
		return 4
	}
}

// telegramChannelQueueScore implements priority > available_at > FIFO.
// Job IDs are monotonically allocated by PostgreSQL and provide a stable FIFO
// tie breaker for jobs made available in the same millisecond. The queue
// worker decodes only the due timestamp for readiness checks.
func telegramChannelQueueScore(priority int, availableAt time.Time, jobID int64) int64 {
	if availableAt.IsZero() {
		availableAt = time.Now()
	}
	dueMillis := availableAt.UnixMilli()
	if dueMillis < 0 {
		dueMillis = 0
	}
	sequence := jobID % telegramChannelQueueSequenceScale
	if sequence < 0 {
		sequence = -sequence
	}
	// Millisecond ordering is sufficient for the queue; the job id remains the
	// durable FIFO tie breaker when jobs share the same availability timestamp.
	return telegramChannelQueuePriorityRank(priority)*telegramChannelQueuePriorityScale + dueMillis + sequence
}

func telegramChannelQueueNeedsMigration(score int64, priority int, urgent bool, history ...bool) bool {
	// The effective lane is represented by the queue key and claim metadata.
	// A media wait may use a temporary history lane without changing the DB
	// priority; callers must pass that effective priority here.
	expectedHistory := priority >= tgJobPriorityHistory
	actualHistory := len(history) > 0 && history[0]
	if actualHistory != expectedHistory {
		return true
	}
	expectedUrgent := priority <= tgJobPriorityInteractive
	if urgent != expectedUrgent {
		return true
	}
	if expectedUrgent || expectedHistory {
		return false
	}
	return score/telegramChannelQueuePriorityScale != telegramChannelQueuePriorityRank(priority)
}

// enqueueTelegramChannelJob adds a job id exactly once. Lower scores are read
// first; the lane is encoded ahead of the creation sequence by the caller.
func enqueueTelegramChannelJob(ctx context.Context, channelID, jobID int64, priority int, availableAt time.Time) error {
	if channelID <= 0 || jobID <= 0 {
		return fmt.Errorf("invalid telegram channel queue identity channel:%d job:%d", channelID, jobID)
	}
	key := telegramChannelQueueKeyForPriority(channelID, priority)
	member := strconv.FormatInt(jobID, 10)
	score := telegramChannelQueueScore(priority, availableAt, jobID)
	otherKeys := make([]string, 0, 2)
	for _, candidate := range []string{telegramChannelUrgentQueueKey(channelID), telegramChannelQueueKey(channelID), telegramChannelHistoryQueueKey(channelID)} {
		if candidate != key {
			otherKeys = append(otherKeys, candidate)
		}
	}
	// Keep the queue migration atomic while limiting the script to one channel
	// and one job. This replaces four Redis round trips with one EVAL and keeps
	// the PostgreSQL Outbox as the durable source of truth.
	const enqueueChannelJobScript = `redis.call('ZADD',KEYS[1],ARGV[1],ARGV[2]); redis.call('ZREM',KEYS[3],ARGV[2]); redis.call('ZREM',KEYS[4],ARGV[2]); redis.call('SADD',KEYS[2],ARGV[3]); return 1`
	durable, _ := ctx.Value(telegramChannelOutboxRetryKey{}).(bool)
	attempts, err := runTelegramChannelEnqueueRetries(ctx, durable, func() error {
		startedAt := time.Now()
		_, enqueueErr := g.Redis().Do(ctx, "EVAL", enqueueChannelJobScript, 4, key, telegramChannelActiveSetKey, otherKeys[0], otherKeys[1], strconv.FormatInt(score, 10), member, strconv.FormatInt(channelID, 10))
		result := "success"
		if enqueueErr != nil {
			result = "error"
		}
		observeRedisLua(ctx, "telegram_channel_enqueue", result, time.Since(startedAt))
		return enqueueErr
	})
	if err == nil {
		return nil
	}
	g.Log().Warning(ctx, "频道队列写入失败", g.Map{
		"event": "channel_queue_enqueue_failed", "channelId": channelID, "jobId": jobID,
		"priority": priority, "availableAt": availableAt, "score": score, "queue": key, "attempts": attempts, "durableOutbox": durable, "error": err.Error(),
	})
	return err
}

func activeTelegramChannelIDs(ctx context.Context) ([]int64, error) {
	value, err := g.Redis().Do(ctx, "SMEMBERS", telegramChannelActiveSetKey)
	if err != nil {
		return nil, err
	}
	var result []int64
	for _, item := range value.Array() {
		id, parseErr := strconv.ParseInt(fmt.Sprint(item), 10, 64)
		if parseErr == nil && id > 0 {
			result = append(result, id)
		}
	}
	return result, nil
}

func removeActiveTelegramChannel(ctx context.Context, channelID int64) error {
	if channelID <= 0 {
		return nil
	}
	// 判空与移除必须在同一原子操作内，不能覆盖并发入队的唤醒。
	const script = `if redis.call('EXISTS',KEYS[4])==0 and redis.call('EXISTS',KEYS[6])==0 and redis.call('ZCARD',KEYS[1])+redis.call('ZCARD',KEYS[2])+redis.call('ZCARD',KEYS[3])==0 then return redis.call('SREM',KEYS[5],ARGV[1]) end return 0`
	_, err := telegramChannelRedisDo(ctx, "EVAL", script, 6, telegramChannelUrgentQueueKey(channelID), telegramChannelQueueKey(channelID), telegramChannelHistoryQueueKey(channelID), telegramChannelProcessingKey(channelID), telegramChannelActiveSetKey, telegramChannelOrderOwnerKey(channelID), strconv.FormatInt(channelID, 10))
	return err
}

type telegramChannelQueueClaim struct {
	ChannelID     int64
	Token         string
	Lane          string
	JobID         int64
	Score         int64
	Urgent        bool
	History       bool
	PreserveOrder bool
}

func removeTelegramChannelJob(ctx context.Context, channelID, jobID int64) error {
	if channelID <= 0 || jobID <= 0 {
		return fmt.Errorf("invalid telegram channel queue identity channel:%d job:%d", channelID, jobID)
	}
	member := strconv.FormatInt(jobID, 10)
	for _, key := range []string{telegramChannelUrgentQueueKey(channelID), telegramChannelQueueKey(channelID), telegramChannelHistoryQueueKey(channelID)} {
		if _, err := g.Redis().Do(ctx, "ZREM", key, member); err != nil {
			return err
		}
	}
	return nil
}

func telegramChannelProcessingKey(channelID int64) string {
	return telegramChannelProcessingPrefix + strconv.FormatInt(channelID, 10)
}

// claimTelegramChannelJob 将一个就绪任务移入持久 processing；hash 不设 TTL，
// 避免进程崩溃后凭证先过期导致无法恢复。deadline 由 Redis 时钟决定。
func claimTelegramChannelJob(ctx context.Context, channelID int64) (telegramChannelQueueClaim, error) {
	startedAt := time.Now()
	fields := g.Map{"channelId": channelID, "queue": telegramChannelProcessingKey(channelID), "stage": "claim", "leaseMs": telegramChannelClaimTTL.Milliseconds()}
	if channelID <= 0 {
		err := fmt.Errorf("频道编号无效：%d", channelID)
		fields["reason"] = "invalid_channel"
		fields["result"] = "error"
		fields["durationMs"] = time.Since(startedAt).Milliseconds()
		logTelegramQueueEvent(ctx, "claim_error", fields, err)
		return telegramChannelQueueClaim{}, err
	}
	token, err := telegramChannelClaimToken()
	if err != nil {
		fields["reason"] = "token_generation_failed"
		fields["result"] = "error"
		fields["durationMs"] = time.Since(startedAt).Milliseconds()
		logTelegramQueueEvent(ctx, "claim_error", fields, err)
		return telegramChannelQueueClaim{}, err
	}
	fields["claimId"] = telegramQueueClaimID(token)
	const script = `if redis.call('EXISTS',KEYS[4])==1 then return {} end
 local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
 local owner=redis.call('GET',KEYS[7])
 if owner and redis.call('PTTL',KEYS[7])==-1 then redis.call('PEXPIRE',KEYS[7],ARGV[3]) end
 local ownerLane=0
 if owner then
 for i=1,3 do if redis.call('ZSCORE',KEYS[i],owner) then ownerLane=i; break end end
 end
 for i=1,3 do local v
 if owner then
 if ownerLane==i then v={owner,redis.call('ZSCORE',KEYS[i],owner)}
 elseif ownerLane==0 and i==1 then v={owner,tostring(now)}
 else v={} end
 else
 v={}
 for rank=0,4 do
 local base=rank*2000000000000
 v=redis.call('ZRANGEBYSCORE',KEYS[i],base,base+now,'WITHSCORES','LIMIT',0,1)
 if #v>0 then break end
 end
 if #v==0 then
 v=redis.call('ZRANGEBYSCORE',KEYS[i],10000000000000,'+inf','WITHSCORES','LIMIT',0,1)
 end
 end
 if #v>0 and tonumber(v[2])%2000000000000<=now then
 redis.call('ZREM',KEYS[i],v[1]); local deadline=now+tonumber(ARGV[3])
 redis.call('HSET',KEYS[4],'token',ARGV[1],'job',v[1],'lane',KEYS[i],'score',v[2],'deadline',deadline)
 redis.call('ZADD',KEYS[5],deadline,ARGV[2]); redis.call('SADD',KEYS[6],ARGV[2])
 return {v[1],v[2],KEYS[i]} end end return {}`
	value, err := telegramChannelRedisDo(ctx, "EVAL", script, 7, telegramChannelUrgentQueueKey(channelID), telegramChannelQueueKey(channelID), telegramChannelHistoryQueueKey(channelID), telegramChannelProcessingKey(channelID), telegramChannelProcessingDueKey, telegramChannelActiveSetKey, telegramChannelOrderOwnerKey(channelID), token, strconv.FormatInt(channelID, 10), telegramChannelClaimTTL.Milliseconds())
	if err != nil {
		fields["reason"] = "redis_error"
		fields["result"] = "error"
		fields["durationMs"] = time.Since(startedAt).Milliseconds()
		err = telegramQueueRedactError(err, token)
		logTelegramQueueEvent(ctx, "claim_error", fields, err)
		return telegramChannelQueueClaim{}, fmt.Errorf("领取频道任务失败：%w", err)
	}
	v := value.Array()
	if len(v) < 3 {
		return telegramChannelQueueClaim{}, nil
	}
	lane := gconv.String(v[2])
	fields["jobId"] = gconv.Int64(v[0])
	fields["queue"] = lane
	fields["score"] = gconv.Int64(v[1])
	fields["reason"] = "claimed"
	fields["result"] = "success"
	fields["durationMs"] = time.Since(startedAt).Milliseconds()
	logTelegramQueueEvent(ctx, "claim_success", fields, nil)
	return telegramChannelQueueClaim{JobID: gconv.Int64(v[0]), Score: gconv.Int64(v[1]), ChannelID: channelID, Token: token, Lane: lane, Urgent: strings.HasPrefix(lane, telegramChannelUrgentQueuePrefix), History: strings.HasPrefix(lane, telegramChannelHistoryQueuePrefix)}, nil
}

// settleTelegramChannelClaim 只确认本代令牌；并发生产者写入的 ready 不会被 ACK 删除。
// requeue 使用 NX 并检查全部 lane，避免旧消费者覆盖更新的唤醒/优先级。
func settleTelegramChannelClaim(ctx context.Context, claim telegramChannelQueueClaim, requeue bool, priority int, at time.Time) (bool, error) {
	startedAt := time.Now()
	fields := g.Map{"channelId": claim.ChannelID, "jobId": claim.JobID, "queue": claim.Lane, "stage": "settle", "claimId": telegramQueueClaimID(claim.Token), "action": "ack"}
	if requeue {
		fields["action"] = "requeue"
		fields["queue"] = telegramChannelQueueKeyForPriority(claim.ChannelID, priority)
		fields["priority"] = priority
		fields["availableAtMs"] = at.UnixMilli()
	}
	const script = `local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
 if redis.call('HGET',KEYS[4],'token')~=ARGV[1] or tonumber(redis.call('HGET',KEYS[4],'deadline') or '0')<=now then return 0 end
 if ARGV[6]=='1' then
 local owner=redis.call('GET',KEYS[8]); if owner and owner~=ARGV[2] then return 0 end
 redis.call('SET',KEYS[8],ARGV[2],'PX',ARGV[7])
 elseif ARGV[3]=='0' and redis.call('GET',KEYS[8])==ARGV[2] then redis.call('DEL',KEYS[8]) end
 if ARGV[3]=='1' and not redis.call('ZSCORE',KEYS[1],ARGV[2]) and not redis.call('ZSCORE',KEYS[2],ARGV[2]) and not redis.call('ZSCORE',KEYS[3],ARGV[2]) then
 local score=tonumber(ARGV[4])
 if redis.call('HGET',KEYS[4],'order_woken')==ARGV[2] then score=score-score%2000000000000+now end
 redis.call('ZADD',KEYS[7],score,ARGV[2]) end
 redis.call('DEL',KEYS[4]); redis.call('ZREM',KEYS[5],ARGV[5]); redis.call('SADD',KEYS[6],ARGV[5]); return 1`
	mode := "0"
	if requeue {
		mode = "1"
	}
	preserve := "0"
	if requeue && claim.PreserveOrder {
		preserve = "1"
	}
	fields["preserveOrder"] = claim.PreserveOrder
	value, err := telegramChannelRedisDo(ctx, "EVAL", script, 8, telegramChannelUrgentQueueKey(claim.ChannelID), telegramChannelQueueKey(claim.ChannelID), telegramChannelHistoryQueueKey(claim.ChannelID), telegramChannelProcessingKey(claim.ChannelID), telegramChannelProcessingDueKey, telegramChannelActiveSetKey, telegramChannelQueueKeyForPriority(claim.ChannelID, priority), telegramChannelOrderOwnerKey(claim.ChannelID), claim.Token, strconv.FormatInt(claim.JobID, 10), mode, telegramChannelQueueScore(priority, at, claim.JobID), strconv.FormatInt(claim.ChannelID, 10), preserve, telegramChannelClaimTTL.Milliseconds())
	fields["durationMs"] = time.Since(startedAt).Milliseconds()
	if err != nil {
		fields["reason"] = "redis_error"
		fields["result"] = "error"
		err = telegramQueueRedactError(err, claim.Token)
		logTelegramQueueEvent(ctx, "settle_error", fields, err)
		return false, fmt.Errorf("确认频道任务领取失败：%w", err)
	}
	accepted := value.Int() == 1
	fields["result"] = "success"
	fields["reason"] = "settled"
	event := "settle_success"
	if !accepted {
		event = "settle_rejected"
		fields["result"] = "rejected"
		fields["reason"] = "token_mismatch_or_expired"
	}
	logTelegramQueueEvent(ctx, event, fields, nil)
	return accepted, nil
}

func renewTelegramChannelClaim(ctx context.Context, claim telegramChannelQueueClaim) (bool, error) {
	startedAt := time.Now()
	fields := g.Map{"channelId": claim.ChannelID, "jobId": claim.JobID, "queue": claim.Lane, "stage": "renew", "claimId": telegramQueueClaimID(claim.Token), "leaseMs": telegramChannelClaimTTL.Milliseconds()}
	const script = `local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
 if redis.call('HGET',KEYS[1],'token')~=ARGV[1] or tonumber(redis.call('HGET',KEYS[1],'deadline') or '0')<=now then return 0 end
 local deadline=now+tonumber(ARGV[3]); redis.call('HSET',KEYS[1],'deadline',deadline); redis.call('ZADD',KEYS[2],deadline,ARGV[2]); return 1`
	value, err := telegramChannelRedisDo(ctx, "EVAL", script, 2, telegramChannelProcessingKey(claim.ChannelID), telegramChannelProcessingDueKey, claim.Token, strconv.FormatInt(claim.ChannelID, 10), telegramChannelClaimTTL.Milliseconds())
	if err != nil {
		fields["reason"] = "redis_error"
		fields["result"] = "error"
		fields["durationMs"] = time.Since(startedAt).Milliseconds()
		err = telegramQueueRedactError(err, claim.Token)
		logTelegramQueueEvent(ctx, "renew_error", fields, err)
		return false, fmt.Errorf("续期频道任务领取失败：%w", err)
	}
	owned := value.Int() == 1
	if !owned {
		fields["reason"] = "token_mismatch_or_expired"
		fields["result"] = "lost"
		fields["durationMs"] = time.Since(startedAt).Milliseconds()
		logTelegramQueueEvent(ctx, "renew_lost", fields, nil)
	}
	return owned, nil
}

// recoverExpiredTelegramChannelClaim 每次仅恢复一个频道、一个任务；不访问业务表。
func recoverExpiredTelegramChannelClaim(ctx context.Context, channelID int64) (bool, error) {
	startedAt := time.Now()
	fields := g.Map{"channelId": channelID, "queue": telegramChannelProcessingKey(channelID), "stage": "expired_recovery"}
	const script = `local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
 local deadline=redis.call('HGET',KEYS[4],'deadline')
 if not deadline then redis.call('ZREM',KEYS[5],ARGV[1]); return 0 end
 if tonumber(deadline)>now then return 0 end
 local job=redis.call('HGET',KEYS[4],'job'); local lane=redis.call('HGET',KEYS[4],'lane'); local score=redis.call('HGET',KEYS[4],'score')
 if lane~=KEYS[1] and lane~=KEYS[2] and lane~=KEYS[3] then return redis.error_reply('频道领取队列不匹配') end
 if not redis.call('ZSCORE',KEYS[1],job) and not redis.call('ZSCORE',KEYS[2],job) and not redis.call('ZSCORE',KEYS[3],job) then redis.call('ZADD',lane,score,job) end
 redis.call('DEL',KEYS[4]); redis.call('ZREM',KEYS[5],ARGV[1]); redis.call('SADD',KEYS[6],ARGV[1]); return 1`
	value, err := telegramChannelRedisDo(ctx, "EVAL", script, 6, telegramChannelUrgentQueueKey(channelID), telegramChannelQueueKey(channelID), telegramChannelHistoryQueueKey(channelID), telegramChannelProcessingKey(channelID), telegramChannelProcessingDueKey, telegramChannelActiveSetKey, strconv.FormatInt(channelID, 10))
	if err != nil {
		fields["reason"] = "redis_error"
		fields["result"] = "error"
		fields["durationMs"] = time.Since(startedAt).Milliseconds()
		logTelegramQueueEvent(ctx, "expired_recovery_error", fields, err)
		return false, fmt.Errorf("恢复过期频道领取失败：%w", err)
	}
	recovered := value.Int() == 1
	if recovered {
		fields["reason"] = "expired_claim_requeued"
		fields["result"] = "success"
		fields["durationMs"] = time.Since(startedAt).Milliseconds()
		logTelegramQueueEvent(ctx, "expired_recovery_success", fields, nil)
	}
	return recovered, nil
}

// 频道队列是 Redis 基础设施适配器，需要 ZSET/Lua；业务缓存不应调用此入口。
func telegramChannelRedisDo(ctx context.Context, command string, args ...interface{}) (*gvar.Var, error) {
	client, err := cache.RedisClient()
	if err != nil {
		return nil, err
	}
	return client.Do(ctx, command, args...)
}

func telegramChannelClaimToken() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("创建频道领取令牌失败：%w", err)
	}
	return hex.EncodeToString(random[:]), nil
}
