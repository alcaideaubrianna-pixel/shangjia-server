// Package accountschedule coalesces durable account wakeups, independently of publish jobs.
package accountschedule

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/util/guid"
)

type State string

const (
	Idle    State = "idle"
	Ready   State = "ready"
	Running State = "running"
)

type Wake uint8

const (
	Ensure Wake = iota
	Changed
)

type Outcome uint8

const (
	Success Outcome = iota
	Failure
	Deferred
	More
)

type Scope struct{ TenantID, AccountID int64 }

func (s Scope) Member() string { return fmt.Sprintf("%d:%d", s.TenantID, s.AccountID) }

type Claim struct {
	Scope      Scope
	Token      string
	Generation int64
	Turn       int64
}
type Queue struct {
	client *gredis.Redis
	keys   []string
}

func New(client *gredis.Redis, prefix string) *Queue {
	return &Queue{client: client, keys: []string{prefix + ":ready", prefix + ":running", prefix + ":state", prefix + ":wake"}}
}

// All scripts touch one account and O(log(accounts)) indexes. Redis TIME avoids host clock skew.
const clock = `local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000); `
const markScript = clock + `
local m=ARGV[1]; local raw=redis.call('HGET',KEYS[3],m)
if raw then
 local s=cjson.decode(raw)
 if s.state=='idle' then s.g=s.g+1; s.state='ready'; redis.call('HSET',KEYS[3],m,cjson.encode(s)); redis.call('ZADD',KEYS[1],now,m)
 elseif ARGV[2]=='1' then s.g=s.g+1; redis.call('HSET',KEYS[3],m,cjson.encode(s)) end
 return s.state
end
redis.call('HSET',KEYS[3],m,cjson.encode({g=1,state='ready',failures=0}))
redis.call('ZADD',KEYS[1],now,m); return 'ready'`
const scheduleScript = `
local prior=redis.call('ZSCORE',KEYS[4],ARGV[1])
if not prior or tonumber(ARGV[2])<tonumber(prior) then redis.call('ZADD',KEYS[4],ARGV[2],ARGV[1]) end
return 1`
const claimScript = clock + `
local wake=redis.call('ZRANGEBYSCORE',KEYS[4],'-inf',now,'LIMIT',0,1)
if #wake>0 then
 local m=wake[1]; redis.call('ZREM',KEYS[4],m)
 local raw=redis.call('HGET',KEYS[3],m)
 if not raw then
 redis.call('HSET',KEYS[3],m,cjson.encode({g=1,state='ready',failures=0}));redis.call('ZADD',KEYS[1],now,m)
 else local s=cjson.decode(raw)
 if s.state=='idle' then s.state='ready';s.g=s.g+1;redis.call('ZADD',KEYS[1],now,m)
 elseif s.state=='running' then s.g=s.g+1 end
 redis.call('HSET',KEYS[3],m,cjson.encode(s)) end
end

local expired=redis.call('ZRANGEBYSCORE',KEYS[2],'-inf',now,'LIMIT',0,1)
if #expired>0 then
 local m=expired[1]; local raw=redis.call('HGET',KEYS[3],m)
 redis.call('ZREM',KEYS[2],m)
 if raw then local s=cjson.decode(raw); s.state='ready'; s.token=nil
 redis.call('HSET',KEYS[3],m,cjson.encode(s)); redis.call('ZADD',KEYS[1],now,m) end
end
local due=redis.call('ZRANGEBYSCORE',KEYS[1],'-inf',now,'LIMIT',0,1)
if #due==0 then return {} end
local m=due[1]; redis.call('ZREM',KEYS[1],m)
local raw=redis.call('HGET',KEYS[3],m); if not raw then return {} end
local s=cjson.decode(raw); s.state='running'; s.token=ARGV[1]; s.turn=(s.turn or 0)+1
redis.call('HSET',KEYS[3],m,cjson.encode(s)); redis.call('ZADD',KEYS[2],now+tonumber(ARGV[2]),m)
return {m,s.token,tostring(s.g),tostring(s.turn)}`
const ackScript = clock + `
local m=ARGV[1]; local raw=redis.call('HGET',KEYS[3],m); if not raw then return 0 end
local s=cjson.decode(raw)
local expiry=redis.call('ZSCORE',KEYS[2],m)
if s.state~='running' or s.token~=ARGV[2] or not expiry or tonumber(expiry)<=now then return 0 end
redis.call('ZREM',KEYS[2],m); s.token=nil
if ARGV[4]=='3' then
 s.state='ready'; s.failures=0; redis.call('HSET',KEYS[3],m,cjson.encode(s)); redis.call('ZADD',KEYS[1],now+10,m)
elseif ARGV[4]=='2' then
 s.state='ready'; redis.call('HSET',KEYS[3],m,cjson.encode(s)); redis.call('ZADD',KEYS[1],now+1000,m)
elseif ARGV[4]=='1' then
 s.failures=math.min((s.failures or 0)+1,8); s.state='ready'
 local delay=math.min(1000*2^(s.failures-1),60000)
 redis.call('HSET',KEYS[3],m,cjson.encode(s)); redis.call('ZADD',KEYS[1],now+delay,m)
elseif s.g>tonumber(ARGV[3]) then
 s.failures=0; s.state='ready'; redis.call('HSET',KEYS[3],m,cjson.encode(s)); redis.call('ZADD',KEYS[1],now+1,m)
else s.state='idle'; s.failures=0; redis.call('HSET',KEYS[3],m,cjson.encode(s)); redis.call('ZADD',KEYS[1],now+30000,m) end
return 1`

func (q *Queue) Mark(ctx context.Context, s Scope, w Wake) error {
	if s.TenantID <= 0 || s.AccountID <= 0 {
		return gerror.New("账户调度标识不合法")
	}
	_, err := q.client.GroupScript().Eval(ctx, markScript, 4, q.keys, []interface{}{s.Member(), int(w)})
	return err
}
func (q *Queue) Claim(ctx context.Context, lease time.Duration) (*Claim, error) {
	if lease < time.Millisecond {
		return nil, gerror.New("账户调度租约无效")
	}
	v, err := q.client.GroupScript().Eval(ctx, claimScript, 4, q.keys, []interface{}{guid.S(), lease.Milliseconds()})
	if err != nil {
		return nil, err
	}
	a := v.Strings()
	if len(a) == 0 {
		return nil, nil
	}
	if len(a) != 4 {
		return nil, gerror.New("账户调度领取结果无效")
	}
	p := strings.Split(a[0], ":")
	if len(p) != 2 {
		return nil, gerror.New("账户调度成员无效")
	}
	tenant, e1 := strconv.ParseInt(p[0], 10, 64)
	account, e2 := strconv.ParseInt(p[1], 10, 64)
	generation, e3 := strconv.ParseInt(a[2], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return nil, gerror.New("账户调度领取标识无效")
	}
	turn, e4 := strconv.ParseInt(a[3], 10, 64)
	if e4 != nil {
		return nil, e4
	}
	return &Claim{Scope: Scope{tenant, account}, Token: a[1], Generation: generation, Turn: turn}, nil
}
func (q *Queue) Ack(ctx context.Context, c Claim, outcome Outcome) (bool, error) {
	v, err := q.client.GroupScript().Eval(ctx, ackScript, 4, q.keys, []interface{}{c.Scope.Member(), c.Token, c.Generation, int(outcome)})
	if err != nil {
		return false, err
	}
	return v.Int() == 1, nil
}

// Schedule keeps the earliest durable-plan wakeup without dirtying a currently running generation.
func (q *Queue) Schedule(ctx context.Context, scope Scope, at time.Time) error {
	if scope.TenantID <= 0 || scope.AccountID <= 0 {
		return gerror.New("账户调度标识不合法")
	}
	_, err := q.client.GroupScript().Eval(ctx, scheduleScript, 4, q.keys, []interface{}{scope.Member(), at.UnixMilli()})
	return err
}
