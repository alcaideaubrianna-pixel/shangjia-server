// account_schedule_migrate coalesces legacy account wakeups; never publish jobs.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/redis/go-redis/v9"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/util/guid"
	"google.golang.org/protobuf/encoding/protowire"
	"hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/library/accountschedule"
)

const queue = "youban_publish_account_schedule"
const taskType = "youban_publish:account_schedule"
const prefix = "asynq:{" + queue + "}:"
const pending = prefix + "pending"
const work = prefix + "migration:work:v1"
const quarantine = prefix + "migration:quarantine:v1"
const lock = prefix + "migration:lease:v1"
const paused = prefix + "paused"

// Every mutation fences the migration token. No long-list LREM is used.
const acquireScript = `if redis.call('SET',KEYS[1],ARGV[1],'NX','PX',30000) then return 1 end return 0`
const renewScript = `if redis.call('GET',KEYS[1])~=ARGV[1] then return 0 end redis.call('PEXPIRE',KEYS[1],30000); return 1`
const releaseScript = `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`
const pauseScript = `if redis.call('GET',KEYS[1])~=ARGV[1] then return redis.error_reply('LEASE_LOST') end redis.call('SET',KEYS[2],'1'); return 1`
const snapshotScript = `if redis.call('GET',KEYS[1])~=ARGV[1] then return redis.error_reply('LEASE_LOST') end
redis.call('SET',KEYS[2],'1');return redis.call('LLEN',KEYS[3])+redis.call('LLEN',KEYS[4])`
const stageScript = `if redis.call('GET',KEYS[1])~=ARGV[1] then return redis.error_reply('LEASE_LOST') end
if redis.call('EXISTS',KEYS[4])~=1 then return redis.error_reply('QUEUE_NOT_PAUSED') end
if redis.call('LLEN',KEYS[3])>0 then return redis.call('LLEN',KEYS[3]) end
local n=0; for i=1,tonumber(ARGV[2]) do if not redis.call('LMOVE',KEYS[2],KEYS[3],'RIGHT','LEFT') then break end; n=n+1 end; return n`
const finishScript = `if redis.call('GET',KEYS[1])~=ARGV[1] then return redis.error_reply('LEASE_LOST') end
if redis.call('EXISTS',KEYS[10])~=1 then return redis.error_reply('QUEUE_NOT_PAUSED') end
if redis.call('LINDEX',KEYS[2],-1)~=ARGV[2] then return redis.error_reply('WORK_CHANGED') end
local state=redis.call('HGET',KEYS[3],'state'); local msg=redis.call('HGET',KEYS[3],'msg')
if ARGV[4]=='delete' and state=='pending' and msg==ARGV[3] then
 for i=5,9 do if redis.call('ZSCORE',KEYS[i],ARGV[2]) then return redis.error_reply('TASK_HAS_OTHER_OWNER') end end
 redis.call('UNLINK',KEYS[3]);redis.call('RPOP',KEYS[2]);return 1
end
redis.call('LMOVE',KEYS[2],KEYS[4],'RIGHT','LEFT');return 0`

type migrator struct {
	client       *gredis.Redis
	token        string
	mark         func(context.Context, accountschedule.Scope) error
	receipts     map[accountschedule.Scope]struct{}
	receiptLimit int
}

func (m *migrator) eval(ctx context.Context, script string, keys []string, args ...interface{}) (int, error) {
	v, e := m.client.GroupScript().Eval(ctx, script, int64(len(keys)), keys, args)
	if e != nil {
		return 0, e
	}
	return v.Int(), nil
}
func (m *migrator) heartbeat(ctx context.Context) error {
	n, e := m.eval(ctx, renewScript, []string{lock}, m.token)
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("migration lease lost")
	}
	return nil
}

// rememberWake caches only successful coalescible wakes within this migration process.
// A nil cache retains the conservative per-batch Mark behavior.
func (m *migrator) rememberWake(ctx context.Context, scope accountschedule.Scope, marked map[accountschedule.Scope]bool) error {
	if marked[scope] {
		return nil
	}
	if m.receipts != nil {
		if _, ok := m.receipts[scope]; ok {
			marked[scope] = true
			return nil
		}
		limit := m.receiptLimit
		if limit <= 0 {
			limit = 10000
		}
		if len(m.receipts) >= limit {
			return errors.New("wake receipt limit reached; remaining work preserved")
		}
	}
	if err := m.mark(ctx, scope); err != nil {
		return err
	}
	marked[scope] = true
	if m.receipts != nil {
		m.receipts[scope] = struct{}{}
	}
	return nil
}

// batch uses bounded metadata/finish pipelines. All wakeups succeed before any deletion.
// A partial network failure may leave some tasks finished; remaining work is resumed.
func (m *migrator) batch(ctx context.Context, size int) (migrated, isolated int, err error) {
	if size < 1 || size > 200 {
		return 0, 0, errors.New("invalid batch size")
	}
	if err = m.heartbeat(ctx); err != nil {
		return
	}
	if _, err = m.eval(ctx, stageScript, []string{lock, pending, work, paused}, m.token, size); err != nil {
		return
	}
	raw, ok := m.client.Client().(redis.UniversalClient)
	if !ok {
		return 0, 0, errors.New("Redis adapter does not support pipelines")
	}
	ids, err := raw.LRange(ctx, work, int64(-size), -1).Result()
	if err != nil {
		return
	}
	// Work is LEFT-pushed and RIGHT-consumed. Preserve this exact order in the finish pipeline.
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	metadata := raw.Pipeline()
	reads := make([]*redis.SliceCmd, len(ids))
	for i, id := range ids {
		reads[i] = metadata.HMGet(ctx, prefix+"t:"+id, "state", "msg")
	}
	if _, err = metadata.Exec(ctx); err != nil {
		return
	}
	type item struct{ id, msg, action string }
	items := make([]item, 0, len(ids))
	marked := make(map[accountschedule.Scope]bool, len(ids))
	for i, id := range ids {
		if err = ctx.Err(); err != nil {
			return
		}
		values, e := reads[i].Result()
		if e != nil {
			err = e
			return
		}
		if len(values) != 2 {
			err = errors.New("invalid HMGET response")
			return
		}
		state, _ := values[0].(string)
		msg, _ := values[1].(string)
		scope, parseErr := decodeTask([]byte(msg), id)
		action := "quarantine"
		if state == "pending" && parseErr == nil {
			if err = m.rememberWake(ctx, scope, marked); err != nil {
				return
			}
			action = "delete"
		}
		items = append(items, item{id, msg, action})
	}
	if err = ctx.Err(); err != nil {
		return
	}
	finish := raw.Pipeline()
	results := make([]*redis.Cmd, len(items))
	for i, it := range items {
		results[i] = finish.Eval(ctx, finishScript, []string{lock, work, prefix + "t:" + it.id, quarantine, prefix + "lease", prefix + "retry", prefix + "scheduled", prefix + "archived", prefix + "completed", paused}, m.token, it.id, it.msg, it.action)
	}
	_, execErr := finish.Exec(ctx)
	// Inspect every response. Later commands may also execute after one Lua error.
	for _, result := range results {
		n, e := result.Int()
		if e != nil {
			err = errors.Join(err, e)
			continue
		}
		if n == 1 {
			migrated++
		} else {
			isolated++
		}
	}
	err = errors.Join(err, execErr)
	return
}

// Decode only Asynq v0.25 TaskMessage identity fields without importing internal protobufs.
func decodeTask(raw []byte, id string) (accountschedule.Scope, error) {
	var scope accountschedule.Scope
	fields := map[protowire.Number][]byte{}
	for len(raw) > 0 {
		n, t, k := protowire.ConsumeTag(raw)
		if k < 0 {
			return scope, errors.New("malformed protobuf")
		}
		raw = raw[k:]
		if n == 1 || n == 2 || n == 3 || n == 4 || n == 14 {
			if t != protowire.BytesType {
				return scope, errors.New("identity wire type")
			}
			b, c := protowire.ConsumeBytes(raw)
			if c < 0 {
				return scope, errors.New("malformed bytes")
			}
			if _, ok := fields[n]; ok {
				return scope, errors.New("duplicate identity field")
			}
			fields[n] = b
			raw = raw[c:]
		} else {
			c := protowire.ConsumeFieldValue(n, t, raw)
			if c < 0 {
				return scope, errors.New("malformed field")
			}
			raw = raw[c:]
		}
	}
	if string(fields[1]) != taskType || string(fields[3]) != id || string(fields[4]) != queue || len(fields[14]) != 0 {
		return scope, errors.New("not a legacy account wake")
	}
	decoder := json.NewDecoder(bytes.NewReader(fields[2]))
	token, e := decoder.Token()
	if e != nil || token != json.Delim('{') {
		return scope, errors.New("invalid scope")
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, e := decoder.Token()
		if e != nil {
			return scope, e
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return scope, errors.New("duplicate scope field")
		}
		seen[name] = true
		var value int64
		if e = decoder.Decode(&value); e != nil {
			return scope, e
		}
		switch name {
		case "tenantId":
			scope.TenantID = value
		case "accountId":
			scope.AccountID = value
		case "sourceId":
			if value != 0 {
				return scope, errors.New("source scoped wake")
			}
		default:
			return scope, errors.New("unknown scope field")
		}
	}
	if _, e = decoder.Token(); e != nil {
		return scope, e
	}
	var tail interface{}
	if e = decoder.Decode(&tail); e != io.EOF {
		return scope, errors.New("trailing scope data")
	}
	if scope.TenantID <= 0 || scope.AccountID <= 0 {
		return scope, errors.New("invalid scope ids")
	}
	return scope, nil
}

func run() error {
	apply := flag.Bool("apply", false, "actually migrate; default is read-only")
	coalesce := flag.Bool("coalesce-wakes", false, "remember successful wakes within this process (max 10000 scopes)")
	legacyStopped := flag.Bool("confirm-legacy-producers-stopped", false, "operator verified legacy Asynq wake producers have stopped")
	confirmed := flag.Bool("confirm-new-scheduler", false, "operator verified new background scheduler is running")
	batch := flag.Int("batch", 100, "maximum tasks per batch, 1..200")
	maxItems := flag.Int("max-items", 1000, "bounded maximum tasks to inspect/migrate")
	pause := flag.Duration("pause", 200*time.Millisecond, "delay between batches, at least 100ms")
	flag.Parse()
	if *batch < 1 || *batch > 200 || *maxItems < 1 || *pause < 100*time.Millisecond {
		return errors.New("invalid batch/max-items/pause")
	}
	if *apply && !*confirmed {
		return errors.New("--apply requires --confirm-new-scheduler")
	}
	if *coalesce && !*legacyStopped {
		return errors.New("--coalesce-wakes requires --confirm-legacy-producers-stopped")
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		return errors.New("REDIS_ADDR is required")
	}
	db := 3
	if s := os.Getenv("REDIS_DB"); s != "" {
		n, e := strconv.Atoi(s)
		if e != nil || n < 0 {
			return errors.New("invalid REDIS_DB")
		}
		db = n
	}
	c, e := gredis.New(&gredis.Config{Address: addr, Db: db, User: os.Getenv("REDIS_USER"), Pass: os.Getenv("REDISCLI_AUTH"), DialTimeout: 3 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second})
	if e != nil {
		return e
	}
	defer c.Close(context.Background())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	q := accountschedule.New(c, consts.AccountScheduleRedisPrefix)
	m := &migrator{client: c, token: guid.S(), mark: func(ctx context.Context, s accountschedule.Scope) error {
		return q.Mark(ctx, s, accountschedule.Changed)
	}}
	if *coalesce {
		m.receipts = make(map[accountschedule.Scope]struct{})
		m.receiptLimit = 10000
	}
	if !*apply {
		// Bounded snapshots only. Never create leases/temporary keys in dry-run.
		for _, key := range []string{pending, work, quarantine} {
			v, e := c.Do(ctx, "LLEN", key)
			if e != nil {
				return e
			}
			fmt.Printf("%s count=%d\n", key, v.Int64())
		}
		v, e := c.Do(ctx, "LRANGE", pending, -min(*maxItems, *batch), -1)
		if e != nil {
			return e
		}
		valid, unknown := 0, 0
		for _, id := range v.Strings() {
			msg, e := c.Do(ctx, "HGET", prefix+"t:"+id, "msg")
			if e != nil {
				return e
			}
			if _, e = decodeTask(msg.Bytes(), id); e == nil {
				valid++
			} else {
				unknown++
			}
		}
		fmt.Printf("dry-run sampled=%d recognized=%d unknown=%d (sample, not total distinct accounts)\n", valid+unknown, valid, unknown)
		return nil
	}
	n, e := m.eval(ctx, acquireScript, []string{lock}, m.token)
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("another migration owns the lease")
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = m.eval(releaseCtx, releaseScript, []string{lock}, m.token)
	}()
	snapshotCtx, cancelSnapshot := context.WithTimeout(ctx, 5*time.Second)
	initial, e := m.eval(snapshotCtx, snapshotScript, []string{lock, paused, pending, work}, m.token)
	cancelSnapshot()
	if e != nil {
		return e
	}
	limit := min(*maxItems, initial)
	fmt.Printf("snapshot=%d processing_limit=%d coalesced_wakes=%t\n", initial, limit, *coalesce)

	total := 0
	for total < limit {
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		done, isolated, e := m.batch(bounded, min(*batch, limit-total))
		cancel()
		total += done + isolated
		fmt.Printf("migrated=%d quarantined=%d processed=%d\n", done, isolated, total)
		if e != nil {
			return e
		}
		if done+isolated == 0 || total >= limit {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(*pause):
		}
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migration stopped:", err)
		os.Exit(1)
	}
}
