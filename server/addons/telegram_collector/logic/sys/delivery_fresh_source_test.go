package sys

import (
	"context"
	"strconv"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/utility/sourcequeue"
)

func TestDeliveryFreshSourceFeatureGate(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{{"", false}, {"false", false}, {"0", false}, {"invalid", false}, {"true", true}, {"1", true}} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED", tc.value)
			if got := deliveryFreshEnabled(); got != tc.want {
				t.Fatalf("flag=%q enabled=%v want=%v", tc.value, got, tc.want)
			}
		})
	}
}

func TestDeliveryFreshSourceFirstHandoff(t *testing.T) {
	t.Setenv("YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED", "true")
	base := context.Background()
	realtime := withCollectorSourceLane(base, sourcequeue.Realtime)
	history := withCollectorSourceLane(base, sourcequeue.History)
	recovery := context.WithValue(realtime, deliverySourceRecoveryKey{}, true)
	for _, tc := range []struct {
		name     string
		ctx      context.Context
		attempt  int
		queue    string
		delivery *sysin.CollectorDelivery
		want     bool
	}{
		{"source realtime uses lane identity", realtime, 1, consts.QueueDeliveryRealtime, &sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount}, true},
		{"source history cannot masquerade as fresh", history, 1, consts.QueueCollectFresh, &sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount}, false},
		{"recovery cannot promote", recovery, 1, consts.QueueCollectFresh, &sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount}, false},
		{"second event attempt", realtime, 2, consts.QueueCollectFresh, &sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount}, false},
		{"unclaimed event", realtime, 0, consts.QueueCollectFresh, &sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount}, false},
		{"history payload", realtime, 1, consts.QueueCollectFresh, &sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount, History: true}, false},
		{"nil payload", realtime, 1, consts.QueueCollectFresh, nil, false},
		{"legacy fresh event", base, 1, consts.QueueCollectFresh, &sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount}, true},
		{"legacy realtime event", base, 1, consts.QueueCollectRealtime, &sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount}, true},
		{"legacy history event", base, 1, consts.QueueCollectHistory, &sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstFreshSourceHandoff(tc.ctx, tc.attempt, tc.queue, tc.delivery); got != tc.want {
				t.Fatalf("fresh=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestDeliveryFreshSourceBudgets(t *testing.T) {
	for _, total := range []int{1, 2, 3, 4, 8, 32} {
		fresh, backlog := deliverySourceRealtimeBudgets(total)
		if fresh < 1 || backlog < 0 || fresh+backlog != total {
			t.Fatalf("total=%d fresh=%d backlog=%d: capacity changed", total, fresh, backlog)
		}
		if total == 1 && (fresh != 1 || backlog != 0) {
			t.Fatal("one worker must share both lanes without allocating another worker")
		}
		if total > 1 && backlog < 1 {
			t.Fatalf("total=%d has no backlog drain budget", total)
		}
	}
}

func TestDeliveryFreshSourceEnqueueRouting(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			t.Setenv("YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED", strconv.FormatBool(enabled))
			q := deliverySourceTestRedis(t)
			collector := &sCollector{}
			ctx := withCollectorSourceScope(context.Background(), 78, 351)
			// Keep ordinary pending work untouched while admitting the new handoff.
			for id := int64(1); id <= 3; id++ {
				if err := collector.enqueueDeliveryTask(withCollectorSourceLane(ctx, sourcequeue.Realtime), id, sysin.EventPriorityRealtime); err != nil {
					t.Fatal(err)
				}
			}
			if err := collector.enqueueDeliveryTaskToQueue(withCollectorSourceLane(ctx, sourcequeue.Realtime), 42, sysin.EventPriorityRealtime, true, consts.QueueDeliveryFresh); err != nil {
				t.Fatal(err)
			}
			wantLane := sourcequeue.Realtime
			if enabled {
				wantLane = sourcequeue.Fresh
				claim, err := q.Claim(ctx, sourcequeue.Fresh, time.Minute)
				if err != nil || claim == nil || claim.ID != "42" {
					t.Fatalf("new delivery stuck behind backlog: claim=%+v err=%v", claim, err)
				}
				if err := q.Ack(ctx, claim); err != nil {
					t.Fatal(err)
				}
			}
			for id := int64(1); id <= 3; id++ {
				claim, err := q.Claim(ctx, sourcequeue.Realtime, time.Minute)
				if err != nil || claim == nil || claim.ID != strconv.FormatInt(id, 10) {
					t.Fatalf("old FIFO moved or lost at %d: claim=%+v err=%v", id, claim, err)
				}
				if err := q.Ack(ctx, claim); err != nil {
					t.Fatal(err)
				}
			}
			if !enabled {
				claim, err := q.Claim(ctx, wantLane, time.Minute)
				if err != nil || claim == nil || claim.ID != "42" {
					t.Fatalf("disabled gate changed old FIFO: claim=%+v err=%v", claim, err)
				}
			}
			// An explicit fresh hint must never override a history context.
			history := withCollectorSourceLane(ctx, sourcequeue.History)
			if err := collector.enqueueDeliveryTaskToQueue(history, 43, sysin.EventPriorityRealtime, true, consts.QueueDeliveryFresh); err != nil {
				t.Fatal(err)
			}
			claim, err := q.Claim(ctx, sourcequeue.History, time.Minute)
			if err != nil || claim == nil || claim.ID != "43" {
				t.Fatalf("history task promoted: claim=%+v err=%v", claim, err)
			}
		})
	}
}

func TestDeliveryFreshSourceContextRouting(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			t.Setenv("YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED", strconv.FormatBool(enabled))
			ctx := withCollectorSourceLane(context.Background(), sourcequeue.Fresh)
			want := sourcequeue.Realtime
			if enabled {
				want = sourcequeue.Fresh
			}
			if got := deliverySourceEnqueueLane(ctx, sysin.EventPriorityRealtime); got != want {
				t.Fatalf("fresh context lane=%s want=%s", got, want)
			}
			if got := deliverySourceEnqueueLane(withCollectorSourceLane(ctx, sourcequeue.History), sysin.EventPriorityRealtime); got != sourcequeue.History {
				t.Fatalf("history lane became %s", got)
			}
		})
	}
}

func TestDeliveryFreshSourceRecoveryDoesNotDuplicateInflight(t *testing.T) {
	for _, opposite := range []sourcequeue.Lane{sourcequeue.Realtime, sourcequeue.History} {
		t.Run(string(opposite), func(t *testing.T) {
			t.Setenv("YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED", "true")
			q := deliverySourceTestRedis(t)
			collector := &sCollector{}
			ctx := withCollectorSourceScope(context.Background(), 78, 351)
			fresh := withCollectorSourceLane(ctx, sourcequeue.Fresh)
			if err := collector.enqueueDeliveryTask(fresh, 42, sysin.EventPriorityRealtime); err != nil {
				t.Fatal(err)
			}
			claim, err := q.Claim(ctx, sourcequeue.Fresh, time.Minute)
			if err != nil || claim == nil {
				t.Fatalf("fresh claim=%+v err=%v", claim, err)
			}
			// Disabling production cannot make recovery overlook Fresh tasks
			// already accepted by a previous instance.
			t.Setenv("YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED", "false")
			recovery := context.WithValue(withCollectorSourceLane(ctx, opposite), deliverySourceRecoveryKey{}, true)
			if err := collector.enqueueDeliveryTaskWithOptions(recovery, 42, sysin.EventPriorityRealtime, false); err != nil {
				t.Fatal(err)
			}
			if err := q.Ack(ctx, claim); err != nil {
				t.Fatal(err)
			}
			for _, lane := range []sourcequeue.Lane{sourcequeue.Fresh, sourcequeue.Realtime, sourcequeue.History} {
				next, err := q.Claim(ctx, lane, time.Minute)
				if err != nil || next != nil {
					t.Fatalf("recovery changed generation or copied task into %s: claim=%+v err=%v", lane, next, err)
				}
			}
		})
	}
}

func TestDeliveryFreshSourceIngressPreservesInflightNotification(t *testing.T) {
	t.Setenv("YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED", "true")
	q := deliverySourceTestRedis(t)
	collector := &sCollector{}
	ctx := withCollectorSourceLane(withCollectorSourceScope(context.Background(), 78, 351), sourcequeue.Realtime)
	enqueue := func() {
		t.Helper()
		if err := collector.enqueueDeliveryTaskToQueue(ctx, 42, sysin.EventPriorityRealtime, true, consts.QueueDeliveryFresh); err != nil {
			t.Fatal(err)
		}
	}
	enqueue()
	claim, err := q.Claim(ctx, sourcequeue.Fresh, time.Minute)
	if err != nil || claim == nil || claim.ID != "42" {
		t.Fatalf("initial fresh claim=%+v err=%v", claim, err)
	}
	// A new ingress notification differs from a recovery probe. Even when it
	// names the same delivery, ACK must preserve work received during execution.
	enqueue()
	if err := q.Ack(ctx, claim); err != nil {
		t.Fatal(err)
	}
	next, err := q.Claim(ctx, sourcequeue.Fresh, time.Minute)
	if err != nil || next == nil || next.ID != "42" {
		t.Fatalf("inflight fresh notification lost: claim=%+v err=%v", next, err)
	}
	if next.Token == claim.Token {
		t.Fatal("replayed notification reused the previous execution token")
	}
	if err := q.Ack(ctx, next); err != nil {
		t.Fatal(err)
	}
	for _, lane := range []sourcequeue.Lane{sourcequeue.Fresh, sourcequeue.Realtime, sourcequeue.History} {
		left, err := q.Claim(ctx, lane, time.Minute)
		if err != nil || left != nil {
			t.Fatalf("notification settled into unexpected duplicate: lane=%s claim=%+v err=%v", lane, left, err)
		}
	}
}

func TestDeliveryFreshSourceHintCannotPromoteExistingTask(t *testing.T) {
	for _, existingLane := range []sourcequeue.Lane{sourcequeue.Realtime, sourcequeue.History} {
		for _, inflight := range []bool{false, true} {
			t.Run(string(existingLane)+"/inflight="+strconv.FormatBool(inflight), func(t *testing.T) {
				t.Setenv("YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED", "true")
				q := deliverySourceTestRedis(t)
				collector := &sCollector{}
				ctx := withCollectorSourceScope(context.Background(), 78, 351)
				if err := collector.enqueueDeliveryTask(withCollectorSourceLane(ctx, existingLane), 42, sysin.EventPriorityRealtime); err != nil {
					t.Fatal(err)
				}
				var original *sourcequeue.Claim
				if inflight {
					var err error
					original, err = q.Claim(ctx, existingLane, time.Minute)
					if err != nil || original == nil {
						t.Fatalf("original claim=%+v err=%v", original, err)
					}
				}
				// A repeated first-handoff request cannot move an old delivery
				// into the reserved pool while its old lane still owns it.
				freshHint := withCollectorSourceLane(ctx, sourcequeue.Realtime)
				if err := collector.enqueueDeliveryTaskToQueue(freshHint, 42, sysin.EventPriorityRealtime, true, consts.QueueDeliveryFresh); err != nil {
					t.Fatal(err)
				}
				if fresh, err := q.Claim(ctx, sourcequeue.Fresh, time.Minute); err != nil || fresh != nil {
					t.Fatalf("old task promoted to fresh: claim=%+v err=%v", fresh, err)
				}
				if original == nil {
					var err error
					original, err = q.Claim(ctx, existingLane, time.Minute)
					if err != nil || original == nil || original.ID != "42" {
						t.Fatalf("old pending task lost: claim=%+v err=%v", original, err)
					}
				}
				if err := q.Ack(ctx, original); err != nil {
					t.Fatal(err)
				}
				for _, lane := range []sourcequeue.Lane{sourcequeue.Fresh, sourcequeue.Realtime, sourcequeue.History} {
					next, err := q.Claim(ctx, lane, time.Minute)
					if err != nil || next != nil {
						t.Fatalf("cross-lane fresh hint duplicated task: lane=%s claim=%+v err=%v", lane, next, err)
					}
				}
			})
		}
	}
}
