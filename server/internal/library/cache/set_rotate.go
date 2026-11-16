package cache

import (
	"context"
	"fmt"
	"strings"
)

// One scan page per call; COUNT is a Redis hint, not an output bound. Buffer
// overflow so compact sets and large hash buckets never drop unseen members.
// Lua serializes cursor movement across workers. Stable members are revisited
// after a full scan; additions/removals follow Redis SCAN semantics.
const setRotateScript = `
if redis.call('EXISTS',KEYS[1]) == 0 then
 redis.call('DEL',KEYS[2],KEYS[3])
 return {}
end
local limit = tonumber(ARGV[1])
if redis.call('LLEN',KEYS[3]) == 0 then
 local page = redis.call('SSCAN',KEYS[1],redis.call('GET',KEYS[2]) or '0','COUNT',limit)
 redis.call('SET',KEYS[2],page[1])
 for _, member in ipairs(page[2]) do redis.call('RPUSH',KEYS[3],member) end
end
local result = {}
for i=1,limit do
 local member = redis.call('LPOP',KEYS[3])
 if not member then break end
 if redis.call('SISMEMBER',KEYS[1],member) == 1 then table.insert(result,member) end
end
return result
`

// SetRotate returns at most 256 members using a shared, persistent SSCAN
// cursor. An empty page does not prove the set is empty. Errors never trigger
// a database fallback. The caller owns processing retries after selection.
func SetRotate(ctx context.Context, key string, limit int) ([]string, error) {
	if key == "" || limit <= 0 {
		return nil, nil
	}
	if limit > 256 {
		limit = 256
	}
	cursor, pending, err := setRotateKeys(key)
	if err != nil {
		return nil, err
	}
	client, err := RedisClient()
	if err != nil {
		return nil, err
	}
	value, err := client.Do(ctx, "EVAL", setRotateScript, 3, key, cursor, pending, limit)
	if err != nil || value == nil || value.IsNil() {
		return nil, err
	}
	return value.Strings(), nil
}

// Preserve the original Redis Cluster hash slot, including an existing tag.
func setRotateKeys(key string) (string, string, error) {
	tag := key
	if strings.ContainsAny(key, "{}") {
		start := strings.IndexByte(key, '{')
		if start < 0 {
			return "", "", fmt.Errorf("SetRotate key has unmatched braces")
		}
		end := strings.IndexByte(key[start+1:], '}')
		if end <= 0 || strings.ContainsAny(key[:start], "{}") || strings.ContainsAny(key[start+1:start+1+end], "{}") || strings.ContainsAny(key[start+end+2:], "{}") {
			return "", "", fmt.Errorf("SetRotate key must have one nonempty, balanced hash tag")
		}
		tag = key[start+1 : start+1+end]
	}
	prefix := "{" + tag + "}:" + key + ":rotate:"
	return prefix + "cursor", prefix + "pending", nil
}
