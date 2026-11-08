package profilecycle

import (
	"context"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
)

type fakeDueRedis struct {
	command string
	args    []any
	result  *gvar.Var
}

func (f *fakeDueRedis) Do(_ context.Context, command string, args ...any) (*gvar.Var, error) {
	f.command, f.args = command, args
	return f.result, nil
}

func TestRedisDueIndexCommands(t *testing.T) {
	fake := &fakeDueRedis{result: gvar.New([]any{"run-1", "run-2"})}
	index, err := NewRedisDueIndex(fake, DueIndexKey)
	if err != nil {
		t.Fatal(err)
	}
	if err = index.Add(context.Background(), "run-1", time.UnixMilli(123)); err != nil || fake.command != "ZADD" {
		t.Fatalf("add command=%s err=%v", fake.command, err)
	}
	keys, err := index.PopDue(context.Background(), time.UnixMilli(456), 20)
	if err != nil || len(keys) != 2 || fake.command != "ZRANGEBYSCORE" {
		t.Fatalf("pop keys=%v command=%s err=%v", keys, fake.command, err)
	}
	if err = index.Remove(context.Background(), "run-1"); err != nil || fake.command != "ZREM" {
		t.Fatalf("remove command=%s err=%v", fake.command, err)
	}
}
