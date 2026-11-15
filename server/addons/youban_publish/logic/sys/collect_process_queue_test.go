package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestCollectLocalWindowRemainingUsesWallClock(t *testing.T) {
	now := time.Now()
	databaseValue := gtime.NewFromTime(time.Date(
		now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, time.UTC,
	))
	remaining := collectLocalWindowRemaining(databaseValue, 3*time.Minute)
	if remaining < 2*time.Minute+50*time.Second || remaining > 3*time.Minute {
		t.Fatalf("expected local wall-clock delay near 3m, got %s", remaining)
	}
}

func TestCollectLocalWindowRemainingDue(t *testing.T) {
	now := time.Now().Add(-4 * time.Minute)
	databaseValue := gtime.NewFromTime(time.Date(
		now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, time.UTC,
	))
	if remaining := collectLocalWindowRemaining(databaseValue, 3*time.Minute); remaining != 0 {
		t.Fatalf("expected due window, got %s", remaining)
	}
}

func TestCollectProcessCandidateIndexKeyIsTenantAccountSourceScoped(t *testing.T) {
	base := collectProcessQueuePayload{TenantId: 11, AccountId: 22, SourceId: 33}
	if got, want := collectProcessCandidateIndexKey(base), "youban_publish:collect:candidates:11:22:33"; got != want {
		t.Fatalf("candidate index key = %q, want %q", got, want)
	}
	otherAccount := base
	otherAccount.AccountId++
	if collectProcessCandidateIndexKey(base) == collectProcessCandidateIndexKey(otherAccount) {
		t.Fatal("candidate index key must isolate account dimensions")
	}
	otherTenant := base
	otherTenant.TenantId++
	if collectProcessCandidateIndexKey(base) == collectProcessCandidateIndexKey(otherTenant) {
		t.Fatal("candidate index key must isolate tenant dimensions")
	}
	otherSource := base
	otherSource.SourceId++
	if collectProcessCandidateIndexKey(base) == collectProcessCandidateIndexKey(otherSource) {
		t.Fatal("candidate index key must isolate source dimensions")
	}
}

func TestCollectProcessCandidateIndexRejectsInvalidEvent(t *testing.T) {
	if got := collectProcessCandidateIndexKey(collectProcessQueuePayload{}); got != "youban_publish:collect:candidates:0:0:0" {
		t.Fatalf("invalid payload key = %q", got)
	}
}
