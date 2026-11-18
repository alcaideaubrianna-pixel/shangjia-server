// Package sourcequeue implements durable, source-isolated Redis queues.
// Consumers must be idempotent: delivery is at least once, including when a
// Redis response is lost. A lease fences queue operations, not external writes.
package sourcequeue

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/internal/library/cache"
)

type Scope struct {
	TenantID int64
	SourceID int64
}
type Lane string

const (
	Realtime Lane = "realtime"
	History  Lane = "history"
)

// ErrLeaseLost means the owner expired, was replaced, or already acknowledged.
// Consumers must cancel work and must not acknowledge using another token.
var ErrLeaseLost = gerror.New("来源队列租约已失效")

type Queue struct {
	client *gredis.Redis
	prefix string
}
type Claim struct {
	Scope   Scope
	Lane    Lane
	ID      string
	Payload []byte
	Token   string
}

// New uses only the application's configured Redis adapter; it creates no new
// Redis configuration or background goroutine. The caller owns recovery loops.
func New(namespace string) (*Queue, error) {
	if strings.TrimSpace(namespace) == "" {
		return nil, gerror.New("来源队列命名空间不能为空")
	}
	client, err := cache.RedisClient()
	if err != nil {
		return nil, err
	}
	return newQueue(namespace, client)
}

// newQueue is also the test seam: tests inject an isolated localhost client.
func newQueue(namespace string, client *gredis.Redis) (*Queue, error) {
	if strings.TrimSpace(namespace) == "" || client == nil {
		return nil, gerror.New("来源队列命名空间或客户端无效")
	}
	sum := sha256.Sum256([]byte(namespace))
	return &Queue{client: client, prefix: "sourcequeue:{" + hex.EncodeToString(sum[:]) + "}:v1:"}, nil
}

func validate(scope Scope, lane Lane) error {
	if scope.TenantID <= 0 || scope.SourceID <= 0 {
		return gerror.New("来源队列作用域无效")
	}
	return validateLane(lane)
}
func validateLane(lane Lane) error {
	if lane != Realtime && lane != History {
		return gerror.New("来源队列通道无效")
	}
	return nil
}
func millis(d time.Duration, positive bool) (int64, error) {
	if d < 0 || (positive && d == 0) {
		return 0, gerror.New("来源队列时间参数无效")
	}
	n := d / time.Millisecond
	if d%time.Millisecond != 0 {
		n++
	}
	return int64(n), nil
}
func scopeID(s Scope) string {
	return strconv.FormatInt(s.TenantID, 10) + ":" + strconv.FormatInt(s.SourceID, 10)
}
func parseScope(s string) (Scope, error) {
	p := strings.Split(s, ":")
	if len(p) != 2 {
		return Scope{}, gerror.New("来源队列索引损坏")
	}
	a, e := strconv.ParseInt(p[0], 10, 64)
	if e != nil {
		return Scope{}, e
	}
	b, e := strconv.ParseInt(p[1], 10, 64)
	if e != nil {
		return Scope{}, e
	}
	v := Scope{a, b}
	if e = validate(v, Realtime); e != nil {
		return Scope{}, e
	}
	return v, nil
}

// All keys are declared explicitly to EVAL and share the namespace hash tag.
func (q *Queue) keys(s Scope, lane Lane) []string {
	p := q.prefix + string(lane) + ":"
	source := p + "source:" + scopeID(s) + ":"
	return []string{p + "ready", p + "sequence", p + "leases", p + "delayed", source + "pending", source + "payload", source + "inflight", source + "generation", source + "state", source + "next_due"}
}
func (q *Queue) eval(ctx context.Context, script string, keys []string, args ...interface{}) (*gvar.Var, error) {
	if q == nil || q.client == nil {
		return nil, gerror.New("来源队列未初始化")
	}
	v, e := q.client.GroupScript().Eval(ctx, script, int64(len(keys)), keys, args)
	if e != nil {
		return nil, gerror.Wrap(e, "来源队列 Redis 操作失败")
	}
	return v, nil
}
func (q *Queue) Enqueue(ctx context.Context, s Scope, lane Lane, id string, payload []byte) error {
	return q.EnqueueAfter(ctx, s, lane, id, payload, 0)
}

// EnqueueAfter coalesces the ID until Ack. Payload is latest-write-wins. An
// enqueue during a claim advances its generation so the old Ack cannot lose it.
// A duplicate delayed enqueue may bring its due time forward, never postpone it.
func (q *Queue) EnqueueAfter(ctx context.Context, s Scope, lane Lane, id string, payload []byte, delay time.Duration) error {
	return q.enqueueAfter(ctx, s, lane, id, payload, delay, 0)
}

// EnqueueHistoryPriority moves a pending task to the front or inserts a missing
// task. Active leases and delayed retry deadlines are preserved.
func (q *Queue) EnqueueHistoryPriority(ctx context.Context, s Scope, id string, payload []byte) error {
	return q.enqueueAfter(ctx, s, History, id, payload, 0, 1)
}

func (q *Queue) enqueueAfter(ctx context.Context, s Scope, lane Lane, id string, payload []byte, delay time.Duration, priority int) error {
	if e := validate(s, lane); e != nil {
		return e
	}
	if id == "" {
		return gerror.New("来源队列任务编号不能为空")
	}
	ms, e := millis(delay, false)
	if e != nil {
		return e
	}
	_, e = q.eval(ctx, enqueueScript, q.keys(s, lane), scopeID(s), id, string(payload), ms, delayMember(s, id), priority)
	return e
}
func delayMember(s Scope, id string) string {
	return scopeID(s) + "/" + base64.RawURLEncoding.EncodeToString([]byte(id))
}

// Claim makes a bounded optimistic attempt to acquire one source. Contention
// may return (nil,nil); callers should poll with backoff. Recover is explicit.
func (q *Queue) Claim(ctx context.Context, lane Lane, lease time.Duration) (*Claim, error) {
	if e := validateLane(lane); e != nil {
		return nil, e
	}
	ms, e := millis(lease, true)
	if e != nil {
		return nil, e
	}
	token := make([]byte, 24)
	if _, e = rand.Read(token); e != nil {
		return nil, e
	}
	owner := hex.EncodeToString(token)
	for i := 0; i < 8; i++ {
		keys := q.keys(Scope{}, lane)
		v, e := q.eval(ctx, `return redis.call('ZRANGE',KEYS[1],0,0)`, keys[:1])
		if e != nil {
			return nil, e
		}
		sources := v.Strings()
		if len(sources) == 0 {
			return nil, nil
		}
		s, e := parseScope(sources[0])
		if e != nil {
			return nil, e
		}
		v, e = q.eval(ctx, claimScript, q.keys(s, lane), scopeID(s), owner, ms)
		if e != nil {
			return nil, e
		}
		a := v.Strings()
		if len(a) == 0 {
			continue
		}
		if len(a) != 2 {
			return nil, gerror.New("来源队列领取结果无效")
		}
		return &Claim{Scope: s, Lane: lane, ID: a[0], Payload: []byte(a[1]), Token: owner}, nil
	}
	return nil, nil
}
func (q *Queue) finish(ctx context.Context, c *Claim, op string, d time.Duration) error {
	if c == nil || c.ID == "" || c.Token == "" {
		return gerror.New("来源队列领取凭据无效")
	}
	if e := validate(c.Scope, c.Lane); e != nil {
		return e
	}
	ms, e := millis(d, op == "renew")
	if e != nil {
		return e
	}
	v, e := q.eval(ctx, finishScript, q.keys(c.Scope, c.Lane), scopeID(c.Scope), c.ID, c.Token, op, ms, delayMember(c.Scope, c.ID))
	if e != nil {
		return e
	}
	if v.Int() != 1 {
		return ErrLeaseLost
	}
	return nil
}
func (q *Queue) Ack(ctx context.Context, c *Claim) error { return q.finish(ctx, c, "ack", 0) }
func (q *Queue) Nack(ctx context.Context, c *Claim, delay time.Duration) error {
	return q.finish(ctx, c, "nack", delay)
}
func (q *Queue) Renew(ctx context.Context, c *Claim, lease time.Duration) error {
	return q.finish(ctx, c, "renew", lease)
}

// Recover inspects at most limit due entries (hard cap 1000). Selection
// alternates lease/delay indexes when both have work; limit=1 also recovers
// lease-only backlogs. Concurrent calls recheck deadlines atomically.
// Call periodically for EACH lane. No background worker is started by New.
func (q *Queue) Recover(ctx context.Context, lane Lane, limit int) error {
	if e := validateLane(lane); e != nil {
		return e
	}
	if limit <= 0 || limit > 1000 {
		return gerror.New("来源队列恢复批次必须为1到1000")
	}
	keys := q.keys(Scope{}, lane)
	v, e := q.eval(ctx, dueScript, []string{keys[2], keys[3], keys[1] + ":recover-turn"}, limit)
	if e != nil {
		return e
	}
	entries := v.Strings()
	for i := 0; i < len(entries); i += 2 {
		if i+1 >= len(entries) {
			return gerror.New("来源队列恢复索引无效")
		}
		kind, member := entries[i], entries[i+1]
		source := member
		id := ""
		if kind == "delay" {
			parts := strings.SplitN(member, "/", 2)
			if len(parts) != 2 {
				return gerror.New("来源队列延迟索引损坏")
			}
			source = parts[0]
			b, e := base64.RawURLEncoding.DecodeString(parts[1])
			if e != nil {
				return e
			}
			id = string(b)
		}
		s, e := parseScope(source)
		if e != nil {
			return e
		}
		if _, e = q.eval(ctx, recoverScript, q.keys(s, lane), source, kind, member, id); e != nil {
			return e
		}
	}
	return nil
}

// String does not expose connection credentials.
func (q *Queue) String() string {
	if q == nil {
		return "sourcequeue<nil>"
	}
	return fmt.Sprintf("sourcequeue<%s>", q.prefix)
}

// HasImmediateWork checks one explicit source without enumerating sources.
// Delayed tasks are excluded so a media wait does not suspend history forever.
func (q *Queue) HasImmediateWork(ctx context.Context, scope Scope, lane Lane) (bool, error) {
	if err := validate(scope, lane); err != nil {
		return false, err
	}
	keys := q.keys(scope, lane)
	value, err := q.eval(ctx, `return redis.call('LLEN',KEYS[1])+redis.call('EXISTS',KEYS[2])`, []string{keys[4], keys[6]})
	if err != nil {
		return false, err
	}
	return value.Int64() > 0, nil
}

// ReserveHistorySlot admits one historical execution per scope and interval
// across all workers sharing the queue.
func (q *Queue) ReserveHistorySlot(ctx context.Context, scope Scope, interval time.Duration) (bool, error) {
	if err := validate(scope, History); err != nil {
		return false, err
	}
	ttl, err := millis(interval, true)
	if err != nil {
		return false, err
	}
	key := q.keys(scope, History)[8] + ":budget"
	value, err := q.eval(ctx, `if redis.call('SET',KEYS[1],1,'NX','PX',ARGV[1]) then return 1 end return 0`, []string{key}, ttl)
	if err != nil {
		return false, err
	}
	return value.Int() == 1, nil
}

// HasTask checks one explicit task's state, including pending, inflight and
// delayed tasks. It does not enumerate sources or inspect the recovery indexes.
// This is a point-in-time observation, not an atomic cross-lane reservation;
// consumers still require business-level idempotency during recovery races.
func (q *Queue) HasTask(ctx context.Context, scope Scope, lane Lane, id string) (bool, error) {
	if err := validate(scope, lane); err != nil {
		return false, err
	}
	if id == "" {
		return false, gerror.New("来源队列任务编号不能为空")
	}
	keys := q.keys(scope, lane)
	value, err := q.eval(ctx, `return redis.call('HEXISTS',KEYS[1],ARGV[1])`, []string{keys[8]}, id)
	if err != nil {
		return false, err
	}
	return value.Int64() == 1, nil
}

// CancelScope atomically cancels both lanes of this namespace for one source.
// It invalidates queue ownership; callers must separately stop/delete the source
// in their durable business configuration to prevent concurrent new producers.
// Old delayed members are removed lazily by bounded Recover, without enumeration.
func (q *Queue) CancelScope(ctx context.Context, scope Scope) error {
	if err := validate(scope, Realtime); err != nil {
		return err
	}
	keys := append(q.keys(scope, Realtime), q.keys(scope, History)...)
	_, err := q.eval(ctx, `
for offset=0,10,10 do
 redis.call('ZREM',KEYS[offset+1],ARGV[1])
 redis.call('ZREM',KEYS[offset+3],ARGV[1])
 redis.call('UNLINK',KEYS[offset+5],KEYS[offset+6],KEYS[offset+7],KEYS[offset+8],KEYS[offset+9],KEYS[offset+10])
end
return 1
`, keys, scopeID(scope))
	return err
}

// Shared namespaces keep source cancellation aligned with the three producers.
const (
	CollectorEventNamespace    = "telegram_collector:event"
	CollectorDeliveryNamespace = "telegram_collector:delivery"
	CollectProcessNamespace    = "youban_publish:collect:process:source:v1"
)
