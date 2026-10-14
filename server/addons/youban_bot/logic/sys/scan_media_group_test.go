package sys

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

func TestClaimScanMediaGroupResolvedOnlyOnce(t *testing.T) {
	const attempts = 16
	var (
		claimed atomic.Int32
		stored  atomic.Bool
		wait    sync.WaitGroup
	)
	setIfNotExist := func(context.Context, interface{}, interface{}, time.Duration) (bool, error) {
		return stored.CompareAndSwap(false, true), nil
	}
	for range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ok, err := claimScanMediaGroupOnce(context.Background(), "group-a", setIfNotExist)
			if err != nil {
				t.Errorf("claim media group: %v", err)
				return
			}
			if ok {
				claimed.Add(1)
			}
		}()
	}
	wait.Wait()
	if got := claimed.Load(); got != 1 {
		t.Fatalf("expected exactly one claim, got %d", got)
	}
}

func TestClaimScanMediaGroupResolvedScopesGroups(t *testing.T) {
	var stored sync.Map
	setIfNotExist := func(_ context.Context, key interface{}, _ interface{}, _ time.Duration) (bool, error) {
		_, loaded := stored.LoadOrStore(key, struct{}{})
		return !loaded, nil
	}
	for _, groupId := range []string{"group-a", "group-b"} {
		ok, err := claimScanMediaGroupOnce(context.Background(), groupId, setIfNotExist)
		if err != nil {
			t.Fatalf("claim %s: %v", groupId, err)
		}
		if !ok {
			t.Fatalf("expected independent claim for %s", groupId)
		}
	}
}

func TestHasScanMediaKeepsSingleImageEligible(t *testing.T) {
	msg := &models.Message{Photo: []models.PhotoSize{{FileID: "photo"}}}
	if !hasScanMedia(msg) {
		t.Fatal("single image must remain eligible for scan")
	}
}
