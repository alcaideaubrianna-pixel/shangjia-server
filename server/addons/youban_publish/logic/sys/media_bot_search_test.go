package sys

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/corona10/goimagehash"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestBotMediaSearchResultLimitIsBounded(t *testing.T) {
	if botMediaSearchMaxResults <= 0 || botMediaSearchMaxResults > 20 {
		t.Fatalf("unexpected bot media search result limit: %d", botMediaSearchMaxResults)
	}
}

func TestBotMediaFingerprintCacheKeyUsesStableTelegramId(t *testing.T) {
	if got := botMediaFingerprintCacheKey("  unique-id  "); got != "youban_publish:bot_media_fingerprint:v1:unique-id" {
		t.Fatalf("unexpected cache key: %q", got)
	}
}

func TestBotMediaSearchDeadlinesBoundColdDownloads(t *testing.T) {
	if botMediaSearchTimeout >= mediaFileCacheDownloadTimeout {
		t.Fatalf("search timeout %s must bound the media cache timeout %s", botMediaSearchTimeout, mediaFileCacheDownloadTimeout)
	}
	if botMediaColdItemTimeout <= 0 || botMediaColdItemTimeout >= botMediaSearchTimeout {
		t.Fatalf("unexpected cold item timeout: %s", botMediaColdItemTimeout)
	}
}

func TestLoadBotMediaColdFingerprintsBoundsConcurrencyAndPreservesOrder(t *testing.T) {
	items := make([]*sysin.BotMediaSearchItem, 10)
	for i := range items {
		items[i] = &sysin.BotMediaSearchItem{FileUniqueId: string(rune('a' + i))}
	}
	var active atomic.Int32
	var maximum atomic.Int32
	loader := func(_ context.Context, fileUniqueId string, _ string) (*mediaFingerprint, bool, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		return &mediaFingerprint{MD5: fileUniqueId, PHash: goimagehash.NewImageHash(uint64(fileUniqueId[0]), goimagehash.PHash)}, false, nil
	}
	results := loadBotMediaColdFingerprints(context.Background(), "TRACE123", items, loader)
	if got := maximum.Load(); got != botMediaColdConcurrency {
		t.Fatalf("maximum concurrency: got %d, want %d", got, botMediaColdConcurrency)
	}
	for i, result := range results {
		if result.err != nil || result.fingerprint == nil || result.fingerprint.MD5 != items[i].FileUniqueId {
			t.Fatalf("result %d did not preserve input order: %+v", i, result)
		}
	}
}

func TestRankBotMediaCandidatesCountsDistinctImages(t *testing.T) {
	groups := [][]*botMediaSearchCandidate{
		{{ProfileId: 11, Distance: 10}, {ProfileId: 11, Distance: 10}, {ProfileId: 22, Distance: 12}},
		{{ProfileId: 22, Distance: 12}},
	}
	if got := rankBotMediaSearchCandidates(groups); !reflect.DeepEqual(got, []int64{22, 11}) {
		t.Fatalf("ranking=%v", got)
	}
}

func TestLoadBotMediaCandidatesDeduplicatesAndBoundsQueries(t *testing.T) {
	var active, maximum, calls atomic.Int32
	fingerprints := []*mediaFingerprint{{MD5: "a"}, {MD5: "b"}, {MD5: "a"}, nil, {MD5: "c"}}
	results, err := loadBotMediaSearchCandidates(context.Background(), fingerprints, func(ctx context.Context, f *mediaFingerprint) ([]*botMediaSearchCandidate, error) {
		calls.Add(1)
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := maximum.Load()
			if n <= old || maximum.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
		return []*botMediaSearchCandidate{{ProfileId: int64(f.MD5[0])}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || maximum.Load() != 2 {
		t.Fatalf("calls=%d concurrency=%d", calls.Load(), maximum.Load())
	}
	for i, want := range []int64{'a', 'b', 'c'} {
		if results[i][0].ProfileId != want {
			t.Fatalf("result order=%v", results)
		}
	}
}

func TestLoadBotMediaCandidatesDoesNotTurnTimeoutIntoEmptySuccess(t *testing.T) {
	results, err := loadBotMediaSearchCandidates(context.Background(), []*mediaFingerprint{{MD5: "a"}}, func(context.Context, *mediaFingerprint) ([]*botMediaSearchCandidate, error) {
		return nil, context.DeadlineExceeded
	})
	if err == nil || results != nil {
		t.Fatalf("results=%v error=%v", results, err)
	}
}

func TestRankBotMediaCandidatesRejectsDistanceAboveTwelve(t *testing.T) {
	groups := [][]*botMediaSearchCandidate{
		{{ProfileId: 676231, Distance: 14}, {ProfileId: 397235, Distance: 14}, {ProfileId: 11, Distance: 12}},
		{{ProfileId: 676231, Distance: 14}},
	}
	if got := rankBotMediaSearchCandidates(groups); !reflect.DeepEqual(got, []int64{11}) {
		t.Fatalf("unconfirmed wider match returned: %v", got)
	}
}

func TestAntiScanSearchFingerprintVersionsDoNotOverwriteEachOther(t *testing.T) {
	first := antiScanSearchFingerprintKey("same-source-media", 0x1234)
	second := antiScanSearchFingerprintKey("same-source-media", 0x5678)
	if first == second {
		t.Fatal("successive generated images would overwrite the same search fingerprint")
	}
	if first != antiScanSearchFingerprintKey("same-source-media", 0x1234) || len(first) > 64 {
		t.Fatal("fingerprint key must be idempotent and fit the persisted field")
	}
}
