package sys

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/utility/sourcequeue"
)

type deliverySourceFakeQueue struct {
	acked, nacked, renews int
	token                 string
	delay                 time.Duration
	renewErr, ackErr      error
	busy                  bool
	immediateErr          error
	historySlot           bool
}

func (q *deliverySourceFakeQueue) ReserveHistorySlot(context.Context, sourcequeue.Scope, time.Duration) (bool, error) {
	return q.historySlot, nil
}

func (q *deliverySourceFakeQueue) HasImmediateWork(context.Context, sourcequeue.Scope, sourcequeue.Lane) (bool, error) {
	return q.busy, q.immediateErr
}
func (q *deliverySourceFakeQueue) Claim(context.Context, sourcequeue.Lane, time.Duration) (*sourcequeue.Claim, error) {
	return nil, nil
}
func (q *deliverySourceFakeQueue) Ack(_ context.Context, c *sourcequeue.Claim) error {
	q.acked++
	q.token = c.Token
	return q.ackErr
}
func (q *deliverySourceFakeQueue) Nack(_ context.Context, c *sourcequeue.Claim, d time.Duration) error {
	q.nacked++
	q.token = c.Token
	q.delay = d
	return nil
}
func (q *deliverySourceFakeQueue) Renew(context.Context, *sourcequeue.Claim, time.Duration) error {
	q.renews++
	return q.renewErr
}
func (q *deliverySourceFakeQueue) Recover(context.Context, sourcequeue.Lane, int) error { return nil }
func deliverySourceTestClaim(t *testing.T, lane sourcequeue.Lane) *sourcequeue.Claim {
	t.Helper()
	payload, err := json.Marshal(deliverySourcePayload{DeliveryID: 42, TenantID: 78, SourceID: 351})
	if err != nil {
		t.Fatal(err)
	}
	return &sourcequeue.Claim{Scope: sourcequeue.Scope{TenantID: 78, SourceID: 351}, Lane: lane, ID: "42", Payload: payload, Token: "original-token"}
}

func TestDeliverySourcePersistedScope(t *testing.T) {
	row := entity.TgCollectorDelivery{Id: 42, TenantId: 78, EventId: 19627141}
	event := entity.TgCollectorEvent{Id: 19627141, TenantId: 78, SourceId: 351}
	for _, tc := range []struct {
		name      string
		scope     sourcequeue.Scope
		wantError bool
	}{
		{"matches", sourcequeue.Scope{TenantID: 78, SourceID: 351}, false},
		{"other tenant", sourcequeue.Scope{TenantID: 79, SourceID: 351}, true},
		{"other source", sourcequeue.Scope{TenantID: 78, SourceID: 352}, true},
		{"invalid", sourcequeue.Scope{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateDeliverySourceScope(tc.scope, &row, &event); (err != nil) != tc.wantError {
				t.Fatalf("scope=%+v err=%v", tc.scope, err)
			}
		})
	}
	event.TenantId = 79
	if validateDeliverySourceScope(sourcequeue.Scope{TenantID: 78, SourceID: 351}, &row, &event) == nil {
		t.Fatal("event tenant mismatch accepted")
	}
	event.TenantId = 78
	event.Id++
	if validateDeliverySourceScope(sourcequeue.Scope{TenantID: 78, SourceID: 351}, &row, &event) == nil {
		t.Fatal("event id mismatch accepted")
	}
}

func TestDeliverySourceSettlement(t *testing.T) {
	for _, tc := range []struct {
		name       string
		processErr error
		ack, nack  int
	}{{"completed", nil, 1, 0}, {"retry", errors.New("temporary"), 0, 1}, {"dead", asynq.SkipRetry, 1, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			q := &deliverySourceFakeQueue{}
			claim := deliverySourceTestClaim(t, sourcequeue.Realtime)
			err := executeDeliverySourceClaim(context.Background(), q, claim, claim.Lane, time.Second, func(ctx context.Context, id int64, _ ...time.Time) error {
				scope, ok := ctx.Value(deliverySourceClaimKey{}).(sourcequeue.Scope)
				if !ok || scope != claim.Scope || id != 42 {
					t.Fatal("claim identity lost")
				}
				if collectorQueueName(ctx) != deliverySourceLogicalQueue(claim.Lane) {
					t.Fatal("business pool queue name mismatch")
				}
				return tc.processErr
			})
			if q.acked != tc.ack || q.nacked != tc.nack || q.token != "original-token" {
				t.Fatalf("settlement=%+v", q)
			}
			if tc.nack > 0 && (q.delay != 30*time.Second || !errors.Is(err, tc.processErr)) {
				t.Fatalf("retry err=%v delay=%v", err, q.delay)
			}
		})
	}
}

func TestDeliverySourceRenewFailureCancelsWithoutSettlement(t *testing.T) {
	for _, cause := range []error{sourcequeue.ErrLeaseLost, errors.New("Redis unavailable")} {
		q := &deliverySourceFakeQueue{renewErr: cause}
		claim := deliverySourceTestClaim(t, sourcequeue.Realtime)
		err := executeDeliverySourceClaim(context.Background(), q, claim, claim.Lane, 90*time.Millisecond, func(ctx context.Context, _ int64, _ ...time.Time) error {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
				t.Fatal("renew failure did not cancel work")
				return nil
			}
		})
		if !errors.Is(err, cause) || q.acked != 0 || q.nacked != 0 || q.renews < 1 {
			t.Fatalf("err=%v queue=%+v", err, q)
		}
	}
}

func TestDeliverySourceDeadlineCannotAckLateSuccess(t *testing.T) {
	q := &deliverySourceFakeQueue{}
	claim := deliverySourceTestClaim(t, sourcequeue.History)
	err := executeDeliverySourceClaim(context.Background(), q, claim, claim.Lane, 30*time.Millisecond, func(ctx context.Context, _ int64, _ ...time.Time) error { <-ctx.Done(); return nil })
	if !errors.Is(err, context.DeadlineExceeded) || q.acked != 0 || q.nacked != 0 {
		t.Fatalf("late result settled: err=%v queue=%+v", err, q)
	}
}

func TestDeliverySourceLateAckRetainsToken(t *testing.T) {
	q := &deliverySourceFakeQueue{ackErr: sourcequeue.ErrLeaseLost}
	claim := deliverySourceTestClaim(t, sourcequeue.Realtime)
	err := executeDeliverySourceClaim(context.Background(), q, claim, claim.Lane, time.Second, func(context.Context, int64, ...time.Time) error { return nil })
	if !errors.Is(err, sourcequeue.ErrLeaseLost) || q.token != "original-token" || q.nacked != 0 {
		t.Fatalf("late ACK must retain token: err=%v queue=%+v", err, q)
	}
}

func TestDeliverySourceBadEnvelopeCannotExecute(t *testing.T) {
	claim := deliverySourceTestClaim(t, sourcequeue.Realtime)
	claim.Scope.SourceID++
	q := &deliverySourceFakeQueue{}
	err := executeDeliverySourceClaim(context.Background(), q, claim, claim.Lane, time.Second, func(context.Context, int64, ...time.Time) error { t.Fatal("invalid claim executed"); return nil })
	if err == nil || q.acked != 0 || q.nacked != 0 {
		t.Fatal("invalid claim settled")
	}
}

func TestDeliverySourceBudgetsAndLane(t *testing.T) {
	if workers, legacy := deliverySourceHistoryBudgets(0); workers != 0 || legacy != 0 {
		t.Fatal("paused history allocated execution slots")
	}
	for _, total := range []int{1, 2, 4, 32} {
		workers, legacy := deliverySourceHistoryBudgets(total)
		if workers+legacy != total || workers < 1 || legacy > 1 {
			t.Fatalf("budget=%d workers=%d legacy=%d", total, workers, legacy)
		}
	}
	ctx := context.Background()
	if deliverySourceLane(ctx, sysin.EventPriorityNormal) != sourcequeue.History || deliverySourceLane(ctx, sysin.EventPriorityUrgent) != sourcequeue.Realtime {
		t.Fatal("priority lane mismatch")
	}
	if deliverySourceLane(withCollectorSourceLane(ctx, sourcequeue.History), sysin.EventPriorityUrgent) != sourcequeue.History {
		t.Fatal("history handoff promoted")
	}
}

func TestDeliverySourceHistoryYieldsToRealtime(t *testing.T) {
	t.Setenv("YOUBAN_TELEGRAM_DELIVERY_HISTORY_BUSY_SCOPES", "")
	q := &deliverySourceFakeQueue{busy: true}
	claim := deliverySourceTestClaim(t, sourcequeue.History)
	err := executeDeliverySourceClaim(context.Background(), q, claim, claim.Lane, time.Second, func(context.Context, int64, ...time.Time) error {
		t.Fatal("history ran while realtime busy")
		return nil
	})
	if err != nil || q.nacked != 1 || q.delay != time.Second || q.acked != 0 {
		t.Fatalf("history not yielded: err=%v queue=%+v", err, q)
	}
}
func TestDeliverySourceHistoryRedisFailureDoesNotAck(t *testing.T) {
	cause := errors.New("Redis unavailable")
	q := &deliverySourceFakeQueue{immediateErr: cause}
	claim := deliverySourceTestClaim(t, sourcequeue.History)
	err := executeDeliverySourceClaim(context.Background(), q, claim, claim.Lane, time.Second, func(context.Context, int64, ...time.Time) error {
		t.Fatal("history ran without priority check")
		return nil
	})
	if !errors.Is(err, cause) || q.acked != 0 || q.nacked != 0 {
		t.Fatalf("Redis failure settled: err=%v queue=%+v", err, q)
	}
}

// Install a disposable local Redis adapter explicitly; never use an inherited
// client (another integration fixture may already have cached g.Redis()).
func deliverySourceTestRedis(t *testing.T) *sourcequeue.Queue {
	t.Helper()
	redis := miniredis.RunT(t)
	oldConfig := g.Cfg().GetAdapter()
	config, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + redis.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(config)
	shared := g.Redis()
	oldAdapter := shared.GetAdapter()
	local, err := gredis.New(&gredis.Config{Address: redis.Addr()})
	if err != nil {
		g.Cfg().SetAdapter(oldConfig)
		t.Fatal(err)
	}
	shared.SetAdapter(local.GetAdapter())
	t.Cleanup(func() {
		shared.SetAdapter(oldAdapter)
		_ = local.Close(context.Background())
		g.Cfg().SetAdapter(oldConfig)
	})
	q, err := sourcequeue.New(collectorDeliverySourceNamespace)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestDeliverySourceRecoveryPreservesExistingLane(t *testing.T) {
	for _, existingLane := range []sourcequeue.Lane{sourcequeue.Realtime, sourcequeue.History} {
		t.Run(string(existingLane), func(t *testing.T) {
			q := deliverySourceTestRedis(t)
			collector := &sCollector{}
			ctx := withCollectorSourceScope(context.Background(), 78, 351)
			ctx = withCollectorSourceLane(ctx, existingLane)
			if err := collector.enqueueDeliveryTask(ctx, 42, sysin.EventPriorityRealtime); err != nil {
				t.Fatal(err)
			}
			// Include inflight work: recovery must not advance its generation. The
			// original ACK must finish it with no replacement task in either lane.
			claim, err := q.Claim(ctx, existingLane, time.Minute)
			if err != nil || claim == nil {
				t.Fatalf("claim=%+v err=%v", claim, err)
			}
			opposite := sourcequeue.History
			if existingLane == sourcequeue.History {
				opposite = sourcequeue.Realtime
			}
			recovery := context.WithValue(withCollectorSourceLane(ctx, opposite), deliverySourceRecoveryKey{}, true)
			if err := collector.enqueueDeliveryTaskWithOptions(recovery, 42, sysin.EventPriorityRealtime, false); err != nil {
				t.Fatal(err)
			}
			if err := q.Ack(ctx, claim); err != nil {
				t.Fatal(err)
			}
			for _, lane := range []sourcequeue.Lane{sourcequeue.Realtime, sourcequeue.History} {
				next, err := q.Claim(ctx, lane, time.Minute)
				if err != nil || next != nil {
					t.Fatalf("recovery duplicated inflight task: lane=%s claim=%+v err=%v", lane, next, err)
				}
			}
		})
	}
}

func TestDeliverySourceIngressStillWakesInflightTask(t *testing.T) {
	q := deliverySourceTestRedis(t)
	collector := &sCollector{}
	ctx := withCollectorSourceLane(withCollectorSourceScope(context.Background(), 78, 351), sourcequeue.Realtime)
	if err := collector.enqueueDeliveryTask(ctx, 42, sysin.EventPriorityRealtime); err != nil {
		t.Fatal(err)
	}
	claim, err := q.Claim(ctx, sourcequeue.Realtime, time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if err := collector.enqueueDeliveryTask(ctx, 42, sysin.EventPriorityRealtime); err != nil {
		t.Fatal(err)
	}
	if err := q.Ack(ctx, claim); err != nil {
		t.Fatal(err)
	}
	next, err := q.Claim(ctx, sourcequeue.Realtime, time.Minute)
	if err != nil || next == nil || next.ID != "42" {
		t.Fatalf("inflight ingress notification lost: claim=%+v err=%v", next, err)
	}
}

func TestDeliverySourceHistoryUsesScopedBudgetWhenRealtimeBusy(t *testing.T) {
	claim := deliverySourceTestClaim(t, sourcequeue.History)
	t.Setenv("YOUBAN_TELEGRAM_DELIVERY_HISTORY_BUSY_SCOPES", strconv.FormatInt(claim.Scope.TenantID, 10)+":"+strconv.FormatInt(claim.Scope.SourceID, 10))
	t.Setenv("YOUBAN_TELEGRAM_DELIVERY_HISTORY_BUSY_INTERVAL", "2s")
	queue := &deliverySourceFakeQueue{busy: true, historySlot: true}
	processed := 0
	run := func() error {
		return executeDeliverySourceClaim(context.Background(), queue, claim, claim.Lane, time.Second, func(context.Context, int64, ...time.Time) error {
			processed++
			return nil
		})
	}
	if err := run(); err != nil || processed != 1 || queue.acked != 1 {
		t.Fatalf("reserved history slot did not run: processed=%d queue=%+v err=%v", processed, queue, err)
	}
	queue.historySlot = false
	if err := run(); err != nil || processed != 1 || queue.nacked != 1 {
		t.Fatalf("exhausted budget did not defer: processed=%d queue=%+v err=%v", processed, queue, err)
	}
}
