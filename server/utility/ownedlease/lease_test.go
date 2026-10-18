package ownedlease

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gredis"
)

func TestLeaseCompareReleaseProtectsSuccessor(t *testing.T) {
	server := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: server.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	ctx := context.Background()
	first := New("test:lease", "same-owner", time.Minute, client)
	if ok, err := first.TryAcquire(ctx); err != nil || !ok {
		t.Fatal(ok, err)
	}
	observed, err := Inspect(ctx, "test:lease", client)
	if err != nil {
		t.Fatal(err)
	}
	server.FastForward(time.Minute)
	second := New("test:lease", "same-owner", time.Minute, client)
	if ok, err := second.TryAcquire(ctx); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if removed, err := ReleaseObserved(ctx, "test:lease", observed, client); err != nil || removed {
		t.Fatal("removed successor", removed, err)
	}
	if err := first.Unlock(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := Inspect(ctx, "test:lease", client)
	if err != nil || current.Owner != "same-owner" || current.Token == observed.Token {
		t.Fatal(current.Owner, err)
	}
	if err := second.Unlock(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseLegacyTokenIsNotReclaimed(t *testing.T) {
	server := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: server.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	if err := server.Set("legacy", "old-random-mutex-token"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Inspect(context.Background(), "legacy", client)
	if err != nil || snapshot.Token != "" {
		t.Fatal("legacy token cannot prove owner", err)
	}
	if removed, err := ReleaseObserved(context.Background(), "legacy", snapshot, client); err != nil || removed {
		t.Fatal(removed, err)
	}
	if token, _ := server.Get("legacy"); token != "old-random-mutex-token" {
		t.Fatal("legacy lease removed")
	}
}
