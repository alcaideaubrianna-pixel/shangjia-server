package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"google.golang.org/protobuf/encoding/protowire"
	"hotgo/addons/youban_publish/library/accountschedule"
)

func message(id, payload, kind string) []byte {
	var b []byte
	for _, f := range []struct {
		n protowire.Number
		s string
	}{{1, kind}, {2, payload}, {3, id}, {4, queue}} {
		b = protowire.AppendTag(b, f.n, protowire.BytesType)
		b = protowire.AppendBytes(b, []byte(f.s))
	}
	return b
}
func setup(t *testing.T) (*migrator, *accountschedule.Queue) {
	t.Helper()
	srv := miniredis.RunT(t)
	c, e := gredis.New(&gredis.Config{Address: srv.Addr(), Db: 15})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	q := accountschedule.New(c, "test:{schedule}")
	m := &migrator{client: c, token: "owner", mark: func(ctx context.Context, s accountschedule.Scope) error {
		return q.Mark(ctx, s, accountschedule.Changed)
	}}
	if n, e := m.eval(context.Background(), acquireScript, []string{lock}, m.token); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if _, e := m.eval(context.Background(), pauseScript, []string{lock, paused}, m.token); e != nil {
		t.Fatal(e)
	}
	return m, q
}
func put(t *testing.T, m *migrator, id, payload, kind string) {
	t.Helper()
	ctx := context.Background()
	if _, e := m.client.Do(ctx, "HSET", prefix+"t:"+id, "state", "pending", "msg", string(message(id, payload, kind))); e != nil {
		t.Fatal(e)
	}
	if _, e := m.client.Do(ctx, "LPUSH", pending, id); e != nil {
		t.Fatal(e)
	}
}
func count(t *testing.T, m *migrator, key string) int {
	t.Helper()
	v, e := m.client.Do(context.Background(), "LLEN", key)
	if e != nil {
		t.Fatal(e)
	}
	return v.Int()
}
func TestDecodeRejectsUnknownAndMalformed(t *testing.T) {
	for _, payload := range []string{`{"tenantId":1,"accountId":2,"turn":3}`, `{"tenantId":1,"accountId":2,"accountId":3}`, `{"tenantId":1,"accountId":2,"sourceId":8}`, `{"tenantId":0,"accountId":2}`, `{"tenantId":1,"accountId":2} {}`, `null`} {
		if _, e := decodeTask(message("one", payload, taskType), "one"); e == nil {
			t.Fatalf("accepted %s", payload)
		}
	}
	if _, e := decodeTask([]byte{255}, "one"); e == nil {
		t.Fatal("malformed accepted")
	}
	if _, e := decodeTask(message("other", `{"tenantId":1,"accountId":2}`, taskType), "one"); e == nil {
		t.Fatal("identity mismatch accepted")
	}
}
func TestBoundedMigrationCoalescesAndPreservesUnknown(t *testing.T) {
	m, q := setup(t)
	ctx := context.Background()
	put(t, m, "a", `{"tenantId":1,"accountId":2}`, taskType)
	put(t, m, "b", `{"tenantId":1,"accountId":2}`, taskType)
	put(t, m, "unknown", `{"tenantId":1,"accountId":2}`, "publish-job")
	n, k, e := m.batch(ctx, 1)
	if e != nil || n != 1 || k != 0 || count(t, m, pending) != 2 {
		t.Fatal(n, k, e)
	}
	n, k, e = m.batch(ctx, 10)
	if e != nil || n != 1 || k != 1 || count(t, m, pending) != 0 || count(t, m, quarantine) != 1 {
		t.Fatal(n, k, e)
	}
	v, _ := m.client.Do(ctx, "EXISTS", prefix+"t:unknown")
	if v.Int() != 1 {
		t.Fatal("unknown hash deleted")
	}
	claim, e := q.Claim(ctx, time.Minute)
	if e != nil || claim == nil || claim.Scope.AccountID != 2 {
		t.Fatal(claim, e)
	}
	if another, e := q.Claim(ctx, time.Minute); e != nil || another != nil {
		t.Fatal("duplicate runnable account", another, e)
	}
}
func TestFailedWakeLeavesWorkAndResumes(t *testing.T) {
	m, q := setup(t)
	ctx := context.Background()
	put(t, m, "a", `{"tenantId":1,"accountId":2}`, taskType)
	mark := m.mark
	m.mark = func(context.Context, accountschedule.Scope) error { return errors.New("injected") }
	if _, _, e := m.batch(ctx, 1); e == nil {
		t.Fatal("failure swallowed")
	}
	if count(t, m, work) != 1 {
		t.Fatal("work lost")
	}
	v, _ := m.client.Do(ctx, "EXISTS", prefix+"t:a")
	if v.Int() != 1 {
		t.Fatal("hash lost")
	}
	m.mark = mark
	if n, _, e := m.batch(ctx, 1); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	claim, e := q.Claim(ctx, time.Minute)
	if e != nil || claim == nil {
		t.Fatal(claim, e)
	}
}
func TestLostLeaseAfterWakeCannotDelete(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	put(t, m, "a", `{"tenantId":1,"accountId":2}`, taskType)
	mark := m.mark
	m.mark = func(ctx context.Context, s accountschedule.Scope) error {
		if e := mark(ctx, s); e != nil {
			return e
		}
		_, e := m.client.Do(ctx, "SET", lock, "new-owner")
		return e
	}
	if _, _, e := m.batch(ctx, 1); e == nil {
		t.Fatal("stale lease accepted")
	}
	if count(t, m, work) != 1 {
		t.Fatal("work lost")
	}
	v, _ := m.client.Do(ctx, "EXISTS", prefix+"t:a")
	if v.Int() != 1 {
		t.Fatal("stale owner deleted hash")
	}
}
func TestWakeDuringRunningSurvivesAck(t *testing.T) {
	m, q := setup(t)
	ctx := context.Background()
	scope := accountschedule.Scope{TenantID: 1, AccountID: 2}
	if e := q.Mark(ctx, scope, accountschedule.Changed); e != nil {
		t.Fatal(e)
	}
	claim, e := q.Claim(ctx, time.Minute)
	if e != nil || claim == nil {
		t.Fatal(claim, e)
	}
	put(t, m, "a", `{"tenantId":1,"accountId":2}`, taskType)
	if _, _, e = m.batch(ctx, 1); e != nil {
		t.Fatal(e)
	}
	if ok, e := q.Ack(ctx, *claim, accountschedule.Success); e != nil || !ok {
		t.Fatal(ok, e)
	}
	// Ack after the migration must retain the changed generation's wake.
	time.Sleep(5 * time.Millisecond)
	if next, e := q.Claim(ctx, time.Minute); e != nil || next == nil {
		t.Fatal("wake swallowed by running ACK", next, e)
	}
}

func TestUnpausedQueueStopsWithoutMoving(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	put(t, m, "a", `{"tenantId":1,"accountId":2}`, taskType)
	if _, e := m.client.Do(ctx, "DEL", paused); e != nil {
		t.Fatal(e)
	}
	if _, _, e := m.batch(ctx, 1); e == nil {
		t.Fatal("unpaused queue accepted")
	}
	if count(t, m, pending) != 1 || count(t, m, work) != 0 {
		t.Fatal("unpaused pending moved")
	}
}
func TestActiveTaskIsIsolatedNotDeleted(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	put(t, m, "a", `{"tenantId":1,"accountId":2}`, taskType)
	_, _ = m.client.Do(ctx, "HSET", prefix+"t:a", "state", "active")
	n, k, e := m.batch(ctx, 1)
	if e != nil || n != 0 || k != 1 {
		t.Fatal(n, k, e)
	}
	v, _ := m.client.Do(ctx, "HGET", prefix+"t:a", "state")
	if v.String() != "active" {
		t.Fatal("active modified")
	}
}

func TestPipelineMixedOrderAndBatchMarkCoalescing(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	mark := m.mark
	calls := 0
	m.mark = func(ctx context.Context, s accountschedule.Scope) error { calls++; return mark(ctx, s) }
	for _, id := range []string{"first", "second", "third"} {
		put(t, m, id, `{"tenantId":1,"accountId":2}`, taskType)
	}
	put(t, m, "fourth", `{"tenantId":1,"accountId":2,"unknown":1}`, taskType)
	n, k, e := m.batch(ctx, 4)
	if e != nil || n != 3 || k != 1 || calls != 1 {
		t.Fatal(n, k, calls, e)
	}
	if count(t, m, work) != 0 || count(t, m, pending) != 0 {
		t.Fatal("work not drained")
	}
	v, _ := m.client.Do(ctx, "LINDEX", quarantine, -1)
	if v.String() != "fourth" {
		t.Fatal("ordering wrong")
	}
}
func TestPipelineOwnershipConflictPreservesRemainder(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	for _, id := range []string{"first", "conflict", "last"} {
		put(t, m, id, `{"tenantId":1,"accountId":2}`, taskType)
	}
	_, _ = m.client.Do(ctx, "ZADD", prefix+"lease", time.Now().Add(time.Minute).Unix(), "conflict")
	n, k, e := m.batch(ctx, 3)
	if e == nil || n != 1 || k != 0 {
		t.Fatal(n, k, e)
	}
	if count(t, m, work) != 2 {
		t.Fatal("pipeline lost remainder")
	}
	for _, id := range []string{"conflict", "last"} {
		v, _ := m.client.Do(ctx, "EXISTS", prefix+"t:"+id)
		if v.Int() != 1 {
			t.Fatal("conflict deleted", id)
		}
	}
	_, _ = m.client.Do(ctx, "ZREM", prefix+"lease", "conflict")
	n, k, e = m.batch(ctx, 3)
	if e != nil || n != 2 || k != 0 {
		t.Fatal("resume failed", n, k, e)
	}
}
func TestPipelineMetadataChangeIsNotDeleted(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	put(t, m, "a", `{"tenantId":1,"accountId":2}`, taskType)
	mark := m.mark
	m.mark = func(ctx context.Context, s accountschedule.Scope) error {
		if e := mark(ctx, s); e != nil {
			return e
		}
		_, e := m.client.Do(ctx, "HSET", prefix+"t:a", "msg", "changed")
		return e
	}
	n, k, e := m.batch(ctx, 1)
	if e != nil || n != 0 || k != 1 {
		t.Fatal(n, k, e)
	}
	v, _ := m.client.Do(ctx, "HGET", prefix+"t:a", "msg")
	if v.String() != "changed" {
		t.Fatal("changed hash deleted")
	}
}

func TestProcessReceiptsAvoidRepeatedWakesAcrossBatches(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	m.receipts = make(map[accountschedule.Scope]struct{})
	mark := m.mark
	calls := 0
	m.mark = func(ctx context.Context, s accountschedule.Scope) error { calls++; return mark(ctx, s) }
	put(t, m, "a", `{"tenantId":1,"accountId":2}`, taskType)
	put(t, m, "b", `{"tenantId":1,"accountId":2}`, taskType)
	for i := 0; i < 2; i++ {
		if n, _, e := m.batch(ctx, 1); e != nil || n != 1 {
			t.Fatal(n, e)
		}
	}
	if calls != 1 || len(m.receipts) != 1 {
		t.Fatal(calls, len(m.receipts))
	}
	// Process restart loses receipts; a repeated wake is safe and required.
	m.receipts = make(map[accountschedule.Scope]struct{})
	put(t, m, "c", `{"tenantId":1,"accountId":2}`, taskType)
	if n, _, e := m.batch(ctx, 1); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestReceiptOnlyAfterSuccessfulMark(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	m.receipts = make(map[accountschedule.Scope]struct{})
	mark := m.mark
	m.mark = func(context.Context, accountschedule.Scope) error { return errors.New("injected") }
	put(t, m, "a", `{"tenantId":1,"accountId":2}`, taskType)
	if _, _, e := m.batch(ctx, 1); e == nil {
		t.Fatal("error swallowed")
	}
	if len(m.receipts) != 0 || count(t, m, work) != 1 {
		t.Fatal("receipt saved before mark")
	}
	m.mark = mark
	if n, _, e := m.batch(ctx, 1); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if len(m.receipts) != 1 {
		t.Fatal("receipt missing")
	}
}
func TestReceiptLimitStopsWithoutDeletingBatch(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	m.receipts = make(map[accountschedule.Scope]struct{})
	m.receiptLimit = 1
	put(t, m, "a", `{"tenantId":1,"accountId":2}`, taskType)
	put(t, m, "b", `{"tenantId":1,"accountId":3}`, taskType)
	if _, _, e := m.batch(ctx, 2); e == nil {
		t.Fatal("limit ignored")
	}
	if len(m.receipts) != 1 || count(t, m, work) != 2 {
		t.Fatal("limit lost work")
	}
	for _, id := range []string{"a", "b"} {
		v, _ := m.client.Do(ctx, "EXISTS", prefix+"t:"+id)
		if v.Int() != 1 {
			t.Fatal("deleted before all wakes accepted", id)
		}
	}
}
func TestInitialSnapshotExcludesLaterLPush(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	put(t, m, "old1", `{"tenantId":1,"accountId":2}`, taskType)
	// Interrupted work is included in the snapshot.
	if _, e := m.eval(ctx, stageScript, []string{lock, pending, work, paused}, m.token, 1); e != nil {
		t.Fatal(e)
	}
	put(t, m, "old2", `{"tenantId":1,"accountId":2}`, taskType)
	initial, e := m.eval(ctx, snapshotScript, []string{lock, paused, pending, work}, m.token)
	if e != nil || initial != 2 {
		t.Fatal(initial, e)
	}
	put(t, m, "new", `{"tenantId":1,"accountId":2}`, taskType)
	for total := 0; total < initial; {
		n, k, e := m.batch(ctx, min(2, initial-total))
		if e != nil {
			t.Fatal(e)
		}
		total += n + k
	}
	if count(t, m, pending) != 1 {
		t.Fatal("new arrival consumed")
	}
	v, _ := m.client.Do(ctx, "LINDEX", pending, -1)
	if v.String() != "new" {
		t.Fatal(v.String())
	}
}
func TestConservativeModeStillMarksEachBatch(t *testing.T) {
	m, _ := setup(t)
	ctx := context.Background()
	mark := m.mark
	calls := 0
	m.mark = func(ctx context.Context, s accountschedule.Scope) error { calls++; return mark(ctx, s) }
	for _, id := range []string{"a", "b"} {
		put(t, m, id, `{"tenantId":1,"accountId":2}`, taskType)
		if _, _, e := m.batch(ctx, 1); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 2 {
		t.Fatal("default behavior changed", calls)
	}
}
