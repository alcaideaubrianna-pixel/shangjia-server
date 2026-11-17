package sourcequeue

// Scripts use Redis TIME so worker clock skew cannot expire another owner.
// KEYS: ready,sequence,leases,delayed,pending,payload,inflight,generation,state,next_due.
const commonScript = `
local t=redis.call('TIME')
local now=tonumber(t[1])*1000+math.floor(tonumber(t[2])/1000)
local function activate(source)
 if redis.call('EXISTS',KEYS[7])==0 and redis.call('LLEN',KEYS[5])>0 and not redis.call('ZSCORE',KEYS[1],source) then
  local seq=redis.call('INCR',KEYS[2])
  redis.call('ZADD',KEYS[1],seq,source)
 end
end
`
const enqueueScript = commonScript + `
local source,id,payload,delay,member=ARGV[1],ARGV[2],ARGV[3],tonumber(ARGV[4]),ARGV[5]
local state=redis.call('HGET',KEYS[9],id)
redis.call('HSET',KEYS[6],id,payload)
redis.call('HINCRBY',KEYS[8],id,1)
if not state then
 if delay>0 then
  redis.call('HSET',KEYS[9],id,'d')
  redis.call('ZADD',KEYS[4],now+delay,member)
 else
  redis.call('HSET',KEYS[9],id,'p')
  redis.call('RPUSH',KEYS[5],id)
 end
elseif state=='i' then
 redis.call('HSET',KEYS[10],id,now+delay)
elseif state=='d' then
 if delay==0 then
  redis.call('ZREM',KEYS[4],member)
  redis.call('HSET',KEYS[9],id,'p')
  redis.call('RPUSH',KEYS[5],id)
 else
  local due=redis.call('ZSCORE',KEYS[4],member)
  if not due or tonumber(due)>now+delay then redis.call('ZADD',KEYS[4],now+delay,member) end
 end
end
activate(source)
return 1
`
const claimScript = commonScript + `
local source,token,lease=ARGV[1],ARGV[2],tonumber(ARGV[3])
if not redis.call('ZSCORE',KEYS[1],source) then return {} end
if redis.call('EXISTS',KEYS[7])~=0 then redis.call('ZREM',KEYS[1],source);return {} end
local id=redis.call('LINDEX',KEYS[5],0)
if not id then redis.call('ZREM',KEYS[1],source);return {} end
local payload=redis.call('HGET',KEYS[6],id)
local generation=redis.call('HGET',KEYS[8],id)
if not payload or not generation then return redis.error_reply('sourcequeue corrupt task') end
redis.call('ZREM',KEYS[1],source)
redis.call('LPOP',KEYS[5])
redis.call('HSET',KEYS[9],id,'i')
redis.call('HSET',KEYS[7],'id',id,'token',token,'generation',generation,'deadline',now+lease)
redis.call('ZADD',KEYS[3],now+lease,source)
return {id,payload}
`
const finishScript = commonScript + `
local source,id,token,op,ms,member=ARGV[1],ARGV[2],ARGV[3],ARGV[4],tonumber(ARGV[5]),ARGV[6]
if redis.call('HGET',KEYS[7],'id')~=id or redis.call('HGET',KEYS[7],'token')~=token then return 0 end
local deadline=tonumber(redis.call('HGET',KEYS[7],'deadline') or '0')
if deadline<=now then return 0 end
if op=='renew' then
 redis.call('HSET',KEYS[7],'deadline',now+ms)
 redis.call('ZADD',KEYS[3],now+ms,source)
 return 1
end
local nextDue=tonumber(redis.call('HGET',KEYS[10],id) or '0')
local dirty=redis.call('HGET',KEYS[7],'generation')~=redis.call('HGET',KEYS[8],id)
redis.call('DEL',KEYS[7])
redis.call('ZREM',KEYS[3],source)
redis.call('HDEL',KEYS[10],id)
if op=='ack' and not dirty then
 redis.call('HDEL',KEYS[6],id)
 redis.call('HDEL',KEYS[8],id)
 redis.call('HDEL',KEYS[9],id)
elseif op=='ack' and dirty and nextDue>now then
 redis.call('HSET',KEYS[9],id,'d')
 redis.call('ZADD',KEYS[4],nextDue,member)
elseif op=='nack' and ms>0 then
 redis.call('HSET',KEYS[9],id,'d')
 redis.call('ZADD',KEYS[4],now+ms,member)
else
 redis.call('HSET',KEYS[9],id,'p')
 redis.call('RPUSH',KEYS[5],id)
end
activate(source)
return 1
`

// At most 2*limit index entries are read, at most limit are returned. A
// persisted turn prevents starvation between indexes even for limit=1.
const dueScript = `
local t=redis.call('TIME')
local now=tonumber(t[1])*1000+math.floor(tonumber(t[2])/1000)
local n=tonumber(ARGV[1])
local leases=redis.call('ZRANGEBYSCORE',KEYS[1],'-inf',now,'LIMIT',0,n)
local delays=redis.call('ZRANGEBYSCORE',KEYS[2],'-inf',now,'LIMIT',0,n)
local turn=tonumber(redis.call('GET',KEYS[3]) or '0')
local a,b=1,1
local out={}
for i=1,n do
 if a>#leases and b>#delays then break end
 if (turn==0 and a<=#leases) or b>#delays then
  out[#out+1]='lease';out[#out+1]=leases[a];a=a+1;turn=1
 else
  out[#out+1]='delay';out[#out+1]=delays[b];b=b+1;turn=0
 end
end
redis.call('SET',KEYS[3],turn)
return out
`
const recoverScript = commonScript + `
local source,kind,member,id=ARGV[1],ARGV[2],ARGV[3],ARGV[4]
if kind=='lease' then
 local score=redis.call('ZSCORE',KEYS[3],source)
 if not score or tonumber(score)>now then return 0 end
 local deadline=tonumber(redis.call('HGET',KEYS[7],'deadline') or '0')
 if deadline>now then return 0 end
 id=redis.call('HGET',KEYS[7],'id')
 if id then
  redis.call('HSET',KEYS[9],id,'p')
  redis.call('LPUSH',KEYS[5],id)
 end
 redis.call('DEL',KEYS[7])
 if id then redis.call('HDEL',KEYS[10],id) end
 redis.call('ZREM',KEYS[3],source)
else
 local due=redis.call('ZSCORE',KEYS[4],member)
 if not due or tonumber(due)>now then return 0 end
 if redis.call('HGET',KEYS[9],id)=='d' then
  redis.call('HSET',KEYS[9],id,'p')
  redis.call('RPUSH',KEYS[5],id)
 end
 redis.call('ZREM',KEYS[4],member)
end
activate(source)
return 1
`
