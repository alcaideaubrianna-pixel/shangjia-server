package sys

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"hotgo/utility/sourcequeue"
)

type collectSourceRuntimeFakeQueue struct {
	renew      func(context.Context, *sourcequeue.Claim, time.Duration) error
	immediate  bool
	probeErr   error
	probeCount atomic.Int32
	ackCount   atomic.Int32
	nackCount  atomic.Int32
	nackDelay  time.Duration
	settleErr  error
}

func (q *collectSourceRuntimeFakeQueue) Claim(context.Context, sourcequeue.Lane, time.Duration) (*sourcequeue.Claim, error) {
	return nil, nil
}
func (q *collectSourceRuntimeFakeQueue) Ack(context.Context, *sourcequeue.Claim) error {
	q.ackCount.Add(1)
	return q.settleErr
}
func (q *collectSourceRuntimeFakeQueue) Nack(_ context.Context, _ *sourcequeue.Claim, delay time.Duration) error {
	q.nackCount.Add(1)
	q.nackDelay = delay
	return q.settleErr
}
func (q *collectSourceRuntimeFakeQueue) Renew(ctx context.Context, c *sourcequeue.Claim, d time.Duration) error {
	if q.renew != nil {
		return q.renew(ctx, c, d)
	}
	return nil
}
func (q *collectSourceRuntimeFakeQueue) Recover(context.Context, sourcequeue.Lane, int) error {
	return nil
}
func (q *collectSourceRuntimeFakeQueue) HasImmediateWork(context.Context, sourcequeue.Scope, sourcequeue.Lane) (bool, error) {
	q.probeCount.Add(1)
	return q.immediate, q.probeErr
}

func TestCollectSourceRuntimeIDRetainsEveryBusinessScope(t *testing.T) {
	base := collectProcessQueuePayload{TenantId: 78, AccountId: 0, SourceId: 351, SourceChatId: "-100123", EventId: 100, RuleId: 7}
	id, err := collectSourceRuntimeID(base)
	if err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*collectProcessQueuePayload){
		"tenant":    func(p *collectProcessQueuePayload) { p.TenantId++ },
		"account":   func(p *collectProcessQueuePayload) { p.AccountId++ },
		"source":    func(p *collectProcessQueuePayload) { p.SourceId++ },
		"chat":      func(p *collectProcessQueuePayload) { p.SourceChatId = "-100456" },
		"event":     func(p *collectProcessQueuePayload) { p.EventId++ },
		"rule":      func(p *collectProcessQueuePayload) { p.RuleId++ },
		"ruleQueue": func(p *collectProcessQueuePayload) { p.RuleQueueKey = "independent-rule" },
		"owner":     func(p *collectProcessQueuePayload) { p.OutboxOwner = "new-durable-owner" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			p := base
			change(&p)
			other, err := collectSourceRuntimeID(p)
			if err != nil || other == id {
				t.Fatalf("business identity collapsed: %s %v", name, err)
			}
		})
	}
	base.Observation = json.RawMessage(`{"trace":"another-trace","time":123}`)
	same, err := collectSourceRuntimeID(base)
	if err != nil || same != id {
		t.Fatalf("observation changed deduplication identity: %v", err)
	}
	// Source-window wakes with EventId=0 must also retain chat and rule.
	base.EventId = 0
	first, _ := collectSourceRuntimeID(base)
	base.SourceChatId = "-100789"
	second, _ := collectSourceRuntimeID(base)
	if first == second {
		t.Fatal("window wake collapsed distinct chats")
	}
	base.RuleId++
	third, _ := collectSourceRuntimeID(base)
	if second == third {
		t.Fatal("window wake collapsed distinct rules")
	}
}

func collectSourceRuntimeTestClaim(t *testing.T, history bool) *sourcequeue.Claim {
	t.Helper()
	p := collectProcessQueuePayload{TenantId: 78, SourceId: 351, AccountId: 0, EventId: 100, SourceChatId: "-100123", RuleId: 7, History: history, Observation: json.RawMessage(`{"carrier":"retained"}`)}
	b, err := json.Marshal(collectSourceRuntimeEnvelope{Kind: collectSourceRuntimeProcess, Process: p, EnqueuedAtUnixMs: time.Now().UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	return &sourcequeue.Claim{Scope: sourcequeue.Scope{TenantID: 78, SourceID: 351}, Lane: collectSourceRuntimeLane(p), ID: "test-process", Payload: b, Token: "test-owner"}
}

func TestCollectSourceRuntimeDecodeScopeLaneKindAndObservation(t *testing.T) {
	c := collectSourceRuntimeTestClaim(t, false)
	e, err := decodeCollectSourceRuntimeClaim(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(e.Process.Observation) != `{"carrier":"retained"}` {
		t.Fatal("carrier lost")
	}
	c.Scope.SourceID++
	if _, err := decodeCollectSourceRuntimeClaim(c); err == nil {
		t.Fatal("foreign source accepted")
	}
	c.Scope.SourceID--
	c.Lane = sourcequeue.History
	if _, err := decodeCollectSourceRuntimeClaim(c); err == nil {
		t.Fatal("incorrect lane accepted")
	}
	c.Lane = sourcequeue.Realtime
	e.Kind = "unknown"
	c.Payload, _ = json.Marshal(e)
	if _, err := decodeCollectSourceRuntimeClaim(c); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestCollectSourceRuntimeLostLeaseCancelsOnlyCurrentJob(t *testing.T) {
	q := &collectSourceRuntimeFakeQueue{renew: func(context.Context, *sourcequeue.Claim, time.Duration) error { return sourcequeue.ErrLeaseLost }}
	parent, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	c := collectSourceRuntimeTestClaim(t, false)
	workErr, leaseErr := awaitCollectSourceRuntimeClaim(parent, q, c, 6*time.Millisecond, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	if !errors.Is(leaseErr, sourcequeue.ErrLeaseLost) || !errors.Is(workErr, context.Canceled) {
		t.Fatalf("lost owner continued: work=%v lease=%v", workErr, leaseErr)
	}
	if parent.Err() != nil {
		t.Fatal("one job lease loss cancelled unrelated jobs")
	}
	if q.ackCount.Load() != 0 || q.nackCount.Load() != 0 {
		t.Fatal("uncertain owner settled a job")
	}
}

func TestCollectSourceRuntimeAwaitDeadlineAndPending(t *testing.T) {
	q := &collectSourceRuntimeFakeQueue{}
	for _, history := range []bool{false, true} {
		c := collectSourceRuntimeTestClaim(t, history)
		pending := newCollectProcessRetryError(17*time.Second, "媒体待就绪")
		workErr, leaseErr := awaitCollectSourceRuntimeClaim(context.Background(), q, c, 30*time.Second, func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("missing per-job deadline")
			}
			remaining := time.Until(deadline)
			if history && remaining > 5*time.Second {
				t.Error("history exceeded short execution budget")
			}
			if !history && remaining < time.Minute {
				t.Error("realtime inherited history budget")
			}
			return pending
		})
		delay, reason := collectSourceRuntimeRetry(workErr)
		if leaseErr != nil || delay != 17*time.Second || reason != "job_dependency_wait" {
			t.Fatalf("pending became success: %v %v %v", workErr, leaseErr, delay)
		}
	}
}

func TestCollectSourceRuntimeHistoryExecutesWithoutRealtimeProbe(t *testing.T) {
	for _, tc := range []struct {
		name      string
		immediate bool
		probeErr  error
	}{
		{name: "realtime_always_pending", immediate: true},
		{name: "realtime_probe_unavailable", immediate: true, probeErr: errors.New("redis probe unavailable")},
	} {
		for _, failAck := range []bool{false, true} {
			name := tc.name + "/ack_success"
			if failAck {
				name = tc.name + "/ack_failure"
			}
			t.Run(name, func(t *testing.T) {
				q := &collectSourceRuntimeFakeQueue{immediate: tc.immediate, probeErr: tc.probeErr}
				if failAck {
					q.settleErr = errors.New("redis ack unavailable")
				}
				called := 0
				executeCollectSourceRuntimeClaimWithHandler(context.Background(), q, collectSourceRuntimeTestClaim(t, true), 30*time.Second, time.Millisecond,
					func(ctx context.Context, envelope collectSourceRuntimeEnvelope) error {
						called++
						if !envelope.Process.History || envelope.Process.EventId != 100 || envelope.Process.TenantId != 78 || envelope.Process.SourceId != 351 || envelope.Process.AccountId != 0 || string(envelope.Process.Observation) != `{"carrier":"retained"}` {
							t.Errorf("history handler lost original scope/carrier: %+v", envelope)
						}
						if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Second {
							t.Error("history handler lost bounded execution budget")
						}
						return nil
					})
				if called != 1 || q.probeCount.Load() != 0 || q.ackCount.Load() != 1 || q.nackCount.Load() != 0 {
					t.Fatalf("history vetoed or settled twice: called=%d probes=%d ack=%d nack=%d", called, q.probeCount.Load(), q.ackCount.Load(), q.nackCount.Load())
				}
			})
		}
	}
}

func TestCollectSourceRuntimeMalformedClaimIsRetriedNotAcknowledged(t *testing.T) {
	q := &collectSourceRuntimeFakeQueue{}
	c := collectSourceRuntimeTestClaim(t, false)
	c.Payload = []byte(`not json`)
	(&sSysPublish{}).executeCollectSourceRuntimeClaim(context.Background(), q, c, 30*time.Second, 0)
	if q.ackCount.Load() != 0 || q.nackCount.Load() != 1 {
		t.Fatal("malformed notification was dropped")
	}
}

func TestCollectSourceRuntimeLegacyQueuesAreExplicitAndBounded(t *testing.T) {
	for _, names := range [][]string{{"*"}, {"unrelated_publish"}, make([]string, 17)} {
		if _, err := collectSourceLegacyQueues(names); err == nil {
			t.Fatal("unsafe migration queue selection accepted")
		}
	}
	if queues, err := collectSourceLegacyQueues(nil); err != nil || len(queues) != 0 {
		t.Fatal("migration enabled by default")
	}
	if queues, err := collectSourceLegacyQueues([]string{tgQueueNameCollectProcessSourceRecoveryPrefix + "78:0:351"}); err != nil || len(queues) != 1 {
		t.Fatal("explicit source migration unavailable")
	}
}

func TestCollectSourceRuntimeHistoryDependencyAndLockRetry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		delay   time.Duration
		message string
	}{
		{name: "media_dependency", delay: 17 * time.Second, message: "媒体待就绪"},
		{name: "business_lock_conflict", delay: 2 * time.Second, message: "采集资料提交锁被占用，稍后重试"},
	} {
		for _, failNack := range []bool{false, true} {
			name := tc.name + "/nack_success"
			if failNack {
				name = tc.name + "/nack_failure"
			}
			t.Run(name, func(t *testing.T) {
				q := &collectSourceRuntimeFakeQueue{immediate: true, probeErr: errors.New("unused realtime probe")}
				if failNack {
					q.settleErr = errors.New("redis nack unavailable")
				}
				called := 0
				executeCollectSourceRuntimeClaimWithHandler(context.Background(), q, collectSourceRuntimeTestClaim(t, true), 30*time.Second, 0,
					func(context.Context, collectSourceRuntimeEnvelope) error {
						called++
						return newCollectProcessRetryError(tc.delay, tc.message)
					})
				if called != 1 || q.probeCount.Load() != 0 || q.ackCount.Load() != 0 || q.nackCount.Load() != 1 || q.nackDelay != max(collectProcessMinimumDelay, tc.delay) {
					t.Fatalf("business retry lost or additionally confirmed: called=%d probes=%d ack=%d nack=%d delay=%v", called, q.probeCount.Load(), q.ackCount.Load(), q.nackCount.Load(), q.nackDelay)
				}
			})
		}
	}
}

func TestCollectSourceRuntimeCancelledWorkCannotBecomeAck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := &collectSourceRuntimeFakeQueue{}
	workErr, leaseErr := awaitCollectSourceRuntimeClaim(ctx, q, collectSourceRuntimeTestClaim(t, false), time.Second, func(context.Context) error { return nil })
	if !errors.Is(workErr, context.Canceled) || leaseErr != nil {
		t.Fatalf("cancelled job reported success: work=%v lease=%v", workErr, leaseErr)
	}
}
