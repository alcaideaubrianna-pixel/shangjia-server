package sys

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gcache"
)

func TestAccountTaskCandidateIDsPreservesPriorityAndBounds(t *testing.T) {
	got := accountTaskCandidateIDs([]string{"9", "bad", "0", "2", "9", "-1", "9223372036854775808", "3"})
	if !reflect.DeepEqual(got, []int64{9, 2, 3}) {
		t.Fatalf("candidates=%v", got)
	}
	members := make([]string, 200)
	for i := range members {
		members[i] = fmt.Sprint(i + 1)
	}
	got = accountTaskCandidateIDs(members)
	if len(got) != 80 || got[79] != 80 {
		t.Fatalf("unbounded candidates: %v", got)
	}
}

func TestAccountTaskWorkWaitWakeFallbackAndCancel(t *testing.T) {
	t.Run("producer wake", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		checks := 0
		waitAccountTaskWork(ctx, 5*time.Second, func() bool { checks++; return true })
		if ctx.Err() != nil || checks != 1 {
			t.Fatalf("wake lost: checks=%d err=%v", checks, ctx.Err())
		}
	})
	t.Run("lost wake fallback", func(t *testing.T) {
		start := time.Now()
		waitAccountTaskWork(context.Background(), 20*time.Millisecond, func() bool { return false })
		if time.Since(start) > time.Second {
			t.Fatal("fallback did not run")
		}
	})
	t.Run("shutdown", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		waitAccountTaskWork(ctx, time.Minute, func() bool { return false })
		if time.Since(start) > time.Second {
			t.Fatal("cancel ignored")
		}
	})
}

func TestAccountTaskIdleProbeDoesNotOverwriteProducerWake(t *testing.T) {
	for _, phase := range []string{"Redis candidates", "PG fallback"} {
		t.Run(phase, func(t *testing.T) {
			store := gcache.New()
			defer store.Close(context.Background())
			ctx := context.Background()
			proceed, finish := reserveAccountTaskIdleProbe(ctx, store, "account:empty", 8*time.Second)
			if !proceed {
				t.Fatal("first reader blocked")
			}
			// Submit persists a task and invalidates while the candidate read is open.
			if _, err := store.Remove(ctx, "account:empty"); err != nil {
				t.Fatal(err)
			}
			finish(true) // The older read returns empty after the producer invalidation.
			value, err := store.Get(ctx, "account:empty")
			if err != nil || (value != nil && !value.IsNil()) {
				t.Fatalf("wake overwritten: %v %v", value, err)
			}
		})
	}
}

func TestAccountTaskIdleProbeCleanupAndCoalescing(t *testing.T) {
	store := gcache.New()
	defer store.Close(context.Background())
	ctx := context.Background()
	proceed, finish := reserveAccountTaskIdleProbe(ctx, store, "account:empty", time.Second)
	if !proceed {
		t.Fatal("first reader blocked")
	}
	second, _ := reserveAccountTaskIdleProbe(ctx, store, "account:empty", time.Second)
	if second {
		t.Fatal("parallel empty probe admitted")
	}
	finish(false) // Candidates found or query failed: next attempt must be allowed.
	next, cleanup := reserveAccountTaskIdleProbe(ctx, store, "account:empty", time.Second)
	if !next {
		t.Fatal("nonempty/error probe left idle marker")
	}
	cleanup(false)
}
