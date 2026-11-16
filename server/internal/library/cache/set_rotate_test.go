package cache

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

func TestSetRotateRedis(t *testing.T) {
	server := miniredis.RunT(t)
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + server.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	old := g.Cfg().GetAdapter()
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(old); _ = g.Redis().Close(context.Background()) })
	ctx := context.Background()
	t.Run("overflow and stable members", func(t *testing.T) {
		key := "rotate:overflow"
		seen := map[string]bool{}
		for i := 0; i < 73; i++ {
			server.SAdd(key, fmt.Sprint(i))
		}
		server.SAdd(key, "bad:scope")
		for i := 0; i < 100; i++ {
			members, err := SetRotate(ctx, key, 3)
			if err != nil {
				t.Fatal(err)
			}
			if len(members) > 3 {
				t.Fatal("unbounded output")
			}
			for _, member := range members {
				seen[member] = true
			}
		}
		if len(seen) != 74 {
			t.Fatalf("starved members: %d/74", len(seen))
		}
	})
	t.Run("buffer overflow and removed members", func(t *testing.T) {
		key := "rotate:buffer"
		server.SAdd(key, "live")
		cursor, pending, err := setRotateKeys(key)
		if err != nil {
			t.Fatal(err)
		}
		server.Set(cursor, "0")
		server.Push(pending, "gone", "live", "gone2")
		first, err := SetRotate(ctx, key, 1)
		if err != nil || len(first) != 0 {
			t.Fatal(first, err)
		}
		second, err := SetRotate(ctx, key, 1)
		if err != nil || len(second) != 1 || second[0] != "live" {
			t.Fatal(second, err)
		}
	})
	t.Run("concurrent cursor consumers", func(t *testing.T) {
		key := "rotate:concurrent"
		for i := 0; i < 80; i++ {
			server.SAdd(key, fmt.Sprint(i))
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		seen := map[string]bool{}
		for worker := 0; worker < 4; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 30; j++ {
					members, err := SetRotate(ctx, key, 2)
					if err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					for _, m := range members {
						seen[m] = true
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if len(seen) != 80 {
			t.Fatalf("concurrent scan starved members: %d", len(seen))
		}
	})
	t.Run("wrong type returns error", func(t *testing.T) {
		server.Set("rotate:wrong", "string")
		if _, err := SetRotate(ctx, "rotate:wrong", 2); err == nil {
			t.Fatal("wrong type silently accepted")
		}
	})
}

func TestSetRotateKeysPreserveSlot(t *testing.T) {
	for _, key := range []string{"plain:key", "a{tenant}b"} {
		cursor, pending, err := setRotateKeys(key)
		if err != nil {
			t.Fatal(err)
		}
		tag := key
		if a := strings.IndexByte(key, '{'); a >= 0 {
			b := strings.IndexByte(key[a+1:], '}')
			tag = key[a+1 : a+1+b]
		}
		for _, derived := range []string{cursor, pending} {
			if !strings.HasPrefix(derived, "{"+tag+"}:") {
				t.Fatalf("hash tag not preserved: %q", derived)
			}
		}
	}
	for _, key := range []string{"{}", "a{}b{later}", "{unclosed", "a{{nested}b", "a}b", "a{x}b{y}"} {
		if _, _, err := setRotateKeys(key); err == nil {
			t.Fatalf("malformed key accepted: %q", key)
		}
	}
}
