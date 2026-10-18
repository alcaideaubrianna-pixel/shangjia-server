// Package ownedlease provides fixed Redis leases with inspectable business owners.
// Tokens remain compatible with the existing string-valued Redis mutexes.
package ownedlease

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

type Lease struct {
	key    string
	token  string
	ttl    time.Duration
	client *gredis.Redis
}

type Snapshot struct {
	Owner string `json:"owner"`
	Nonce string `json:"nonce"`
	Token string `json:"-"`
}

func New(key, owner string, ttl time.Duration, clients ...*gredis.Redis) *Lease {
	value, _ := json.Marshal(Snapshot{Owner: owner, Nonce: guid.S()})
	return &Lease{key: key, token: string(value), ttl: ttl, client: redisClient(clients)}
}

func redisClient(clients []*gredis.Redis) *gredis.Redis {
	if len(clients) > 0 && clients[0] != nil {
		return clients[0]
	}
	return g.Redis()
}

func (l *Lease) TryAcquire(ctx context.Context) (bool, error) {
	if l.ttl < time.Millisecond {
		return false, gerror.New("租约有效期必须大于零")
	}
	value, err := l.client.GroupScript().Eval(ctx,
		`return redis.call('set',KEYS[1],ARGV[1],'PX',ARGV[2],'NX') or ''`,
		1, []string{l.key}, []interface{}{l.token, l.ttl.Milliseconds()})
	if err != nil {
		return false, err
	}
	return value.String() == "OK", nil
}

func Inspect(ctx context.Context, key string, clients ...*gredis.Redis) (Snapshot, error) {
	value, err := redisClient(clients).GroupString().Get(ctx, key)
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if value == nil || json.Unmarshal([]byte(value.String()), &snapshot) != nil || snapshot.Nonce == "" {
		// Legacy mutex tokens have no business owner and cannot be reclaimed.
		return Snapshot{}, nil
	}
	snapshot.Token = value.String()
	return snapshot, nil
}

// ReleaseObserved cannot remove a successor's lease, including an ABA owner.
func ReleaseObserved(ctx context.Context, key string, snapshot Snapshot, clients ...*gredis.Redis) (bool, error) {
	if snapshot.Token == "" {
		return false, nil
	}
	value, err := redisClient(clients).GroupScript().Eval(ctx,
		`if redis.call('get',KEYS[1]) == ARGV[1] then return redis.call('del',KEYS[1]) end return 0`,
		1, []string{key}, []interface{}{snapshot.Token})
	if err != nil {
		return false, err
	}
	return value.Int() == 1, nil
}

func (l *Lease) Unlock(ctx context.Context) error {
	_, err := ReleaseObserved(ctx, l.key, Snapshot{Token: l.token}, l.client)
	return err
}
