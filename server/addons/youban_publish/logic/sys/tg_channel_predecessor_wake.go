package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/utility/telegrambudget"
)

type telegramChannelClaimContextKey struct{}

type telegramChannelClaimContext struct {
	jobID         int64
	predecessorID int64
}

func telegramPredecessorWakeKeys(id int64) (string, string) {
	key := fmt.Sprintf("%s:predecessor:%d", telegramChannelActiveSetKey, id)
	return key + ":wait", key + ":done"
}

func registerTelegramPredecessorWake(ctx context.Context, predecessor, channel, job int64, priority int, score int64) error {
	waitKey, doneKey := telegramPredecessorWakeKeys(predecessor)
	const script = `local score=redis.call('ZSCORE',KEYS[3],ARGV[1])
 if not score or tonumber(score)~=tonumber(ARGV[3]) then return 0 end
 if redis.call('EXISTS',KEYS[2])==1 then
 local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
 local n=tonumber(score); local due=n%2000000000000
 if due>now then redis.call('ZADD',KEYS[3],n-due+now,ARGV[1]) end
 redis.call('SADD',KEYS[4],ARGV[2]); return 1
 end
 if redis.call('HLEN',KEYS[1])<32 or redis.call('HEXISTS',KEYS[1],ARGV[1])==1 then
 redis.call('HSET',KEYS[1],ARGV[1],cjson.encode({KEYS[3],ARGV[2],ARGV[3]}))
 redis.call('EXPIRE',KEYS[1],120)
 end
 return 0`
	_, err := telegramChannelRedisDo(ctx, "EVAL", script, 4, waitKey, doneKey,
		telegramChannelQueueKeyForPriority(channel, priority), telegramChannelActiveSetKey, job, channel, score)
	return err
}

func finishTelegramPredecessorWake(ctx context.Context, predecessor int64) {
	if predecessor <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), min(time.Second, telegrambudget.LeaseOperation(ctx)))
	defer cancel()
	waitKey, doneKey := telegramPredecessorWakeKeys(predecessor)
	const script = `redis.call('SET',KEYS[2],'1','EX',120)
 local entries=redis.call('HGETALL',KEYS[1])
 local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
 for i=1,#entries,2 do
 local job=entries[i]; local item=cjson.decode(entries[i+1])
 local score=redis.call('ZSCORE',item[1],job)
 if score and tonumber(score)==tonumber(item[3]) then
 local n=tonumber(score); local due=n%2000000000000
 if due>now then redis.call('ZADD',item[1],n-due+now,job) end
 redis.call('SADD',KEYS[3],item[2])
 end
 end
 redis.call('DEL',KEYS[1]); return 1`
	if _, err := telegramChannelRedisDo(ctx, "EVAL", script, 3, waitKey, doneKey, telegramChannelActiveSetKey); err != nil {
		g.Log().Warningf(ctx, "唤醒频道前序后继失败 jobId:%d err:%+v", predecessor, err)
	}
}
