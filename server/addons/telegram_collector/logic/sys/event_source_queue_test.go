package sys

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/collectobserve"
	"hotgo/utility/sourcequeue"
)

// No Redis, DB, runtime startup or package-level configuration is involved.
type fakeEventSourceQueue struct {
	acks     []*sourcequeue.Claim
	nacks    []*sourcequeue.Claim
	delays   []time.Duration
	ackErr   error
	nackErr  error
	renewErr error
}

var _ collectorEventSourceQueue = (*fakeEventSourceQueue)(nil)

func (*fakeEventSourceQueue) Claim(context.Context, sourcequeue.Lane, time.Duration) (*sourcequeue.Claim, error) {
	return nil, errors.New("unexpected claim: test supplies its own receipt")
}

func (q *fakeEventSourceQueue) Ack(_ context.Context, c *sourcequeue.Claim) error {
	q.acks = append(q.acks, c)
	return q.ackErr
}

func (q *fakeEventSourceQueue) Nack(_ context.Context, c *sourcequeue.Claim, delay time.Duration) error {
	q.nacks = append(q.nacks, c)
	q.delays = append(q.delays, delay)
	return q.nackErr
}

func (q *fakeEventSourceQueue) Renew(context.Context, *sourcequeue.Claim, time.Duration) error {
	return q.renewErr
}

func newEventSourceTestClaim(t *testing.T, ctx context.Context, lane sourcequeue.Lane) *sourcequeue.Claim {
	t.Helper()
	payload, err := json.Marshal(collectorSourceEventPayload{
		EventID: 9123, TenantID: 78, SourceID: 351,
		Carrier: collectobserve.Capture(ctx, time.Now()),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &sourcequeue.Claim{
		Scope: sourcequeue.Scope{TenantID: 78, SourceID: 351},
		Lane:  lane, ID: "9123", Payload: payload, Token: "original-claim-token",
	}
}

func TestSourceEventClaimRejectsInvalidOwnership(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*sourcequeue.Claim)
	}{
		{"tenant mismatch", func(c *sourcequeue.Claim) { c.Scope.TenantID++ }},
		{"source mismatch", func(c *sourcequeue.Claim) { c.Scope.SourceID++ }},
		{"invalid source", func(c *sourcequeue.Claim) { c.Scope.SourceID = 0 }},
		{"lane mismatch", func(c *sourcequeue.Claim) { c.Lane = sourcequeue.History }},
		{"event identity mismatch", func(c *sourcequeue.Claim) { c.ID = "9124" }},
		{"missing token", func(c *sourcequeue.Claim) { c.Token = "" }},
		{"malformed payload", func(c *sourcequeue.Claim) { c.Payload = []byte("invalid") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claim := newEventSourceTestClaim(t, context.Background(), sourcequeue.Realtime)
			tc.mutate(claim)
			q := new(fakeEventSourceQueue)
			err := executeSourceEventClaim(context.Background(), q, claim, sourcequeue.Realtime, func(context.Context, int64) error {
				t.Fatal("invalid ownership reached processing")
				return nil
			})
			if err == nil || len(q.acks) != 0 || len(q.nacks) != 0 {
				t.Fatalf("invalid claim must remain unconfirmed: err=%v ack=%d nack=%d", err, len(q.acks), len(q.nacks))
			}
		})
	}
}

func TestSourceEventHistoryLaneNeverUpgradedByPriority(t *testing.T) {
	ctx := withCollectorSourceLane(context.Background(), sourcequeue.History)
	for _, origin := range []eventEnqueueOrigin{eventOriginIngress, eventOriginRecovery, eventOriginReplay} {
		for _, priority := range []int{sysin.EventPriorityNormal, sysin.EventPriorityRealtime, sysin.EventPriorityUrgent} {
			if got := collectorEventSourceLane(ctx, priority, origin); got != sourcequeue.History {
				t.Fatalf("history upgraded: origin=%s priority=%d lane=%s", origin, priority, got)
			}
		}
	}
	for _, origin := range []eventEnqueueOrigin{eventOriginRecovery, eventOriginReplay} {
		if got := collectorEventSourceLane(context.Background(), sysin.EventPriorityUrgent, origin); got != sourcequeue.History {
			t.Fatalf("recovery/replay upgraded: origin=%s lane=%s", origin, got)
		}
	}
}

func TestSourceEventClaimPropagatesScopeLaneAndAckToken(t *testing.T) {
	for _, lane := range []sourcequeue.Lane{sourcequeue.Realtime, sourcequeue.History} {
		t.Run(string(lane), func(t *testing.T) {
			claim := newEventSourceTestClaim(t, context.Background(), lane)
			q := new(fakeEventSourceQueue)
			processed := 0
			err := executeSourceEventClaim(context.Background(), q, claim, lane, func(ctx context.Context, id int64) error {
				processed++
				scope, ok := collectorSourceScopeFromContext(ctx)
				if !ok || scope != claim.Scope || id != 9123 {
					t.Fatalf("lost identity: scope=%+v id=%d", scope, id)
				}
				gotLane, ok := collectorSourceLaneFromContext(ctx)
				if !ok || gotLane != lane {
					t.Fatalf("lost execution lane: got=%s want=%s", gotLane, lane)
				}
				return nil
			})
			if err != nil || processed != 1 || len(q.acks) != 1 || len(q.nacks) != 0 {
				t.Fatalf("err=%v processed=%d ack=%d nack=%d", err, processed, len(q.acks), len(q.nacks))
			}
			if q.acks[0] != claim || q.acks[0].Token != "original-claim-token" {
				t.Fatal("ACK must use the original fenced receipt")
			}
		})
	}
}

func TestSourceEventProcessFailureNacksOriginalClaim(t *testing.T) {
	failure := errors.New("business persistence failed")
	for _, nackErr := range []error{nil, errors.New("Redis NACK unavailable")} {
		q := &fakeEventSourceQueue{nackErr: nackErr}
		claim := newEventSourceTestClaim(t, context.Background(), sourcequeue.Realtime)
		err := executeSourceEventClaim(context.Background(), q, claim, sourcequeue.Realtime, func(context.Context, int64) error { return failure })
		if !errors.Is(err, failure) || (nackErr != nil && !errors.Is(err, nackErr)) {
			t.Fatalf("lost processing or queue error: %v", err)
		}
		if len(q.acks) != 0 || len(q.nacks) != 1 || q.nacks[0] != claim || q.delays[0] <= 0 {
			t.Fatalf("failure must back off using original receipt: ack=%d nack=%d delays=%v", len(q.acks), len(q.nacks), q.delays)
		}
	}
}

func TestSourceEventCancellationDoesNotAck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := new(fakeEventSourceQueue)
	claim := newEventSourceTestClaim(t, ctx, sourcequeue.Realtime)
	err := executeSourceEventClaim(ctx, q, claim, sourcequeue.Realtime, func(workCtx context.Context, _ int64) error {
		cancel()
		<-workCtx.Done()
		return nil // A handler returning nil after cancellation is not success.
	})
	if !errors.Is(err, context.Canceled) || len(q.acks) != 0 || len(q.nacks) != 0 {
		t.Fatalf("cancelled ownership must not be confirmed: err=%v ack=%d nack=%d", err, len(q.acks), len(q.nacks))
	}
}

func TestSourceEventProcessingLeaseErrorDoesNotAck(t *testing.T) {
	q := &fakeEventSourceQueue{nackErr: sourcequeue.ErrLeaseLost}
	claim := newEventSourceTestClaim(t, context.Background(), sourcequeue.Realtime)
	err := executeSourceEventClaim(context.Background(), q, claim, sourcequeue.Realtime, func(context.Context, int64) error {
		return sourcequeue.ErrLeaseLost
	})
	if !errors.Is(err, sourcequeue.ErrLeaseLost) || len(q.acks) != 0 {
		t.Fatalf("lease error must not ACK: err=%v ack=%d", err, len(q.acks))
	}
}

func TestSourceEventAckFailureRemainsVisible(t *testing.T) {
	failure := errors.New("Redis ACK unavailable")
	q := &fakeEventSourceQueue{ackErr: failure}
	claim := newEventSourceTestClaim(t, context.Background(), sourcequeue.Realtime)
	err := executeSourceEventClaim(context.Background(), q, claim, sourcequeue.Realtime, func(context.Context, int64) error { return nil })
	if !errors.Is(err, failure) || len(q.acks) != 1 || len(q.nacks) != 0 {
		t.Fatalf("ACK failure must propagate: err=%v ack=%d nack=%d", err, len(q.acks), len(q.nacks))
	}
}

func TestSourceEventCarrierRestoresProducerTrace(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("11111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("2222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	producerSpan := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
	producerCtx := trace.ContextWithSpanContext(context.Background(), producerSpan)
	claim := newEventSourceTestClaim(t, producerCtx, sourcequeue.Realtime)
	q := new(fakeEventSourceQueue)
	err = executeSourceEventClaim(context.Background(), q, claim, sourcequeue.Realtime, func(ctx context.Context, _ int64) error {
		got := trace.SpanContextFromContext(ctx)
		if got.TraceID() != traceID || got.SpanID() != spanID || !got.IsRemote() {
			t.Fatalf("producer trace not restored: %v", got)
		}
		if timing, ok := collectobserve.QueueTimingFrom(ctx); !ok || timing.EnqueuedAtUnixNano <= 0 {
			t.Fatal("carrier lost queue timing")
		}
		return nil
	})
	if err != nil || len(q.acks) != 1 {
		t.Fatalf("restored claim err=%v ack=%d", err, len(q.acks))
	}
}
