package sys

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"hotgo/utility/ownedlease"
)

func TestTelegramCompletedLeaseRecoveryAfterProcessCrash(t *testing.T) {
	server := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: server.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	ctx := context.Background()
	old := telegramJobRecord{Id: 2065649, TenantId: 78, TargetChatId: "3438550143", Status: telegramJobStatusSending, SendPhase: telegramSendPhaseDisplaySending}
	next := telegramJobRecord{Id: 2065654, TenantId: 78, TargetChatId: "-1003438550143"}
	lookup := func(_ context.Context, id int64) (telegramJobRecord, error) {
		if id != old.Id {
			t.Fatalf("unexpected lookup %d", id)
		}
		return old, nil
	}
	lease, ok, err := tryTelegramPublishChannelLease(ctx, old, lookup, client)
	if err != nil || !ok {
		t.Fatalf("old acquire: %v %v", ok, err)
	}
	_, ok, err = tryTelegramPublishChannelLease(ctx, next, lookup, client)
	if err != nil || ok {
		t.Fatalf("in-flight send must block: %v %v", ok, err)
	}
	old.Status, old.SendPhase = telegramJobStatusSent, telegramSendPhaseVerifyConfirmed
	// Simulate SIGKILL after the durable success response: no old Unlock.
	started := time.Now()
	successor, ok, err := tryTelegramPublishChannelLease(ctx, next, lookup, client)
	if err != nil || !ok {
		t.Fatalf("completed crash recovery: %v %v", ok, err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("recovery waited for the 10m30s expiry")
	}
	key := telegramChannelLockKey(next.TargetChatId)
	snapshot, err := ownedlease.Inspect(ctx, key, client)
	if err != nil {
		t.Fatal(err)
	}
	var owner telegramPublishLeaseOwner
	if err := json.Unmarshal([]byte(snapshot.Owner), &owner); err != nil || owner.JobId != next.Id {
		t.Fatalf("successor owner: %+v %v", owner, err)
	}
	// A late defer from the crashed/paused caller cannot delete the successor.
	if err := lease.Unlock(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := ownedlease.Inspect(ctx, key, client)
	if after.Token != snapshot.Token {
		t.Fatal("late old unlock deleted successor lease")
	}
	if err := successor.Unlock(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTelegramCompletedLeaseRecoveryRefusesUncertainOwners(t *testing.T) {
	owner := telegramPublishLeaseOwner{Kind: "telegram_publish_v1", JobId: 10, TenantId: 78}
	base := telegramJobRecord{Id: 10, TenantId: 78, TargetChatId: "3438550143", Status: telegramJobStatusSent, SendPhase: telegramSendPhaseVerifyConfirmed}
	for _, status := range []string{"pending", "sending", "confirming", "unknown", "failed", "superseded"} {
		job := base
		job.Status = status
		if telegramCompletedLeaseRecoverable(owner, job, base.TargetChatId) {
			t.Fatalf("unsafe recovery for %s", status)
		}
	}
	for _, phase := range []string{"", "display_confirmed", "verify_sending", "timeout_cancelled"} {
		job := base
		job.SendPhase = phase
		if telegramCompletedLeaseRecoverable(owner, job, base.TargetChatId) {
			t.Fatalf("unsafe phase recovery %s", phase)
		}
	}
	job := base
	job.TenantId++
	if telegramCompletedLeaseRecoverable(owner, job, base.TargetChatId) {
		t.Fatal("cross-tenant recovery")
	}
	if telegramCompletedLeaseRecoverable(owner, base, "3438550144") {
		t.Fatal("cross-channel recovery")
	}
	owner.Kind = "cleanup"
	if telegramCompletedLeaseRecoverable(owner, base, base.TargetChatId) {
		t.Fatal("cleanup lease reclaimed")
	}
}

func TestTelegramCompletedLeaseRecoveryDatabaseFailureKeepsLock(t *testing.T) {
	server := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: server.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	ctx := context.Background()
	job := telegramJobRecord{Id: 1, TenantId: 78, TargetChatId: "3438550143"}
	lookup := func(context.Context, int64) (telegramJobRecord, error) {
		return telegramJobRecord{}, errors.New("database unavailable")
	}
	_, ok, err := tryTelegramPublishChannelLease(ctx, job, lookup, client)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	before, _ := ownedlease.Inspect(ctx, telegramChannelLockKey(job.TargetChatId), client)
	_, ok, err = tryTelegramPublishChannelLease(ctx, job, lookup, client)
	if err == nil || ok {
		t.Fatal("database failure must fail closed")
	}
	after, _ := ownedlease.Inspect(ctx, telegramChannelLockKey(job.TargetChatId), client)
	if before.Token != after.Token {
		t.Fatal("database failure removed lock")
	}
}

func TestTelegramCompletedLeaseRecoveryConcurrentWorkersHaveOneWinner(t *testing.T) {
	server := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: server.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	ctx := context.Background()
	old := telegramJobRecord{Id: 1, TenantId: 78, TargetChatId: "3438550143", Status: telegramJobStatusSent, SendPhase: telegramSendPhaseVerifyConfirmed}
	lookup := func(_ context.Context, id int64) (telegramJobRecord, error) {
		if id == old.Id {
			return old, nil
		}
		return telegramJobRecord{Id: id, TenantId: 78, TargetChatId: old.TargetChatId, Status: "pending"}, nil
	}
	if _, ok, err := tryTelegramPublishChannelLease(ctx, old, lookup, client); err != nil || !ok {
		t.Fatal(ok, err)
	}
	var winners atomic.Int64
	var failures atomic.Int64
	var wg sync.WaitGroup
	for id := int64(2); id <= 33; id++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			next := telegramJobRecord{Id: id, TenantId: 78, TargetChatId: old.TargetChatId}
			_, ok, err := tryTelegramPublishChannelLease(ctx, next, lookup, client)
			if err != nil {
				failures.Add(1)
			}
			if ok {
				winners.Add(1)
			}
		}(id)
	}
	wg.Wait()
	if winners.Load() != 1 || failures.Load() != 0 {
		t.Fatalf("winners=%d failures=%d", winners.Load(), failures.Load())
	}
}
