package profilecycle

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	red "github.com/go-redis/redis/v8"
	"github.com/gogf/gf/v2/container/gvar"
)

type liveDueRedis struct{ client *red.Client }

func (r liveDueRedis) Do(ctx context.Context, command string, args ...any) (*gvar.Var, error) {
	values := make([]interface{}, 0, len(args))
	for _, arg := range args {
		values = append(values, arg)
	}
	cmd := r.client.Do(ctx, append([]interface{}{command}, values...)...)
	value, err := cmd.Result()
	if err != nil {
		return nil, err
	}
	return gvar.New(value), nil
}

func TestRedisDueIndexLiveReplay(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := red.NewClient(&red.Options{Addr: server.Addr()})
	defer client.Close()
	index, err := NewRedisDueIndex(liveDueRedis{client}, "replay:profile-cycle-due")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < 10000; i++ {
		if err = index.Add(context.Background(), "run-"+strconv.Itoa(i), now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := index.PopDue(context.Background(), now.Add(499*time.Second), 500)
	if err != nil || len(keys) != 500 {
		t.Fatalf("keys=%d err=%v", len(keys), err)
	}
}
