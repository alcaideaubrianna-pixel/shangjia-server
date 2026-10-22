package sys

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/internal/bootstrap/envconfig"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestBotMediaExactPhasePreservesLaterHitsAfterApproximateTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	items := []*mediaFingerprint{{MD5: "slow"}, {MD5: "later-exact"}, {MD5: "slow"}}
	exactCalled := false
	results, err := loadBotMediaSearchCandidatesExactFirst(ctx, items,
		func(_ context.Context, unique []*mediaFingerprint) ([][]*botMediaSearchCandidate, error) {
			exactCalled = true
			if len(unique) != 2 {
				t.Fatalf("deduplication lost: %d", len(unique))
			}
			return [][]*botMediaSearchCandidate{nil, {{ProfileId: 123}}}, nil
		}, func(ctx context.Context, item *mediaFingerprint) ([]*botMediaSearchCandidate, error) {
			if !exactCalled || item.MD5 != "slow" {
				t.Error("exact phase skipped or exact hit queried again")
			}
			<-ctx.Done()
			return nil, ctx.Err()
		})
	if !errors.Is(err, context.DeadlineExceeded) || !reflect.DeepEqual(rankBotMediaSearchCandidates(results), []int64{123}) {
		t.Fatalf("confirmed hit lost: results=%v err=%v", results, err)
	}
}

func TestBotMediaExactPhaseRejectsIncompleteEmpty(t *testing.T) {
	results, err := loadBotMediaSearchCandidatesExactFirst(context.Background(), []*mediaFingerprint{{MD5: "a"}},
		func(context.Context, []*mediaFingerprint) ([][]*botMediaSearchCandidate, error) { return nil, nil },
		func(context.Context, *mediaFingerprint) ([]*botMediaSearchCandidate, error) {
			t.Fatal("invalid exact response accepted")
			return nil, nil
		})
	if err == nil || len(results) != 0 {
		t.Fatalf("results=%v err=%v", results, err)
	}
}

func TestBotMediaBatchPhasePreservesSlotsAndPartialResults(t *testing.T) {
	items := []*mediaFingerprint{{MD5: "a"}, {MD5: "exact"}, {MD5: "b"}, {MD5: "a"}}
	results, err := loadBotMediaSearchCandidatesExactFirstBatch(context.Background(), items,
		func(_ context.Context, items []*mediaFingerprint) ([][]*botMediaSearchCandidate, error) {
			return [][]*botMediaSearchCandidate{nil, {{ProfileId: 22}}, nil}, nil
		}, func(_ context.Context, items []*mediaFingerprint) ([][]*botMediaSearchCandidate, error) {
			if len(items) != 2 || items[0].MD5 != "a" || items[1].MD5 != "b" {
				t.Fatalf("wrong remaining inputs: %+v", items)
			}
			return [][]*botMediaSearchCandidate{{{ProfileId: 11}}, nil}, context.DeadlineExceeded
		})
	if !errors.Is(err, context.DeadlineExceeded) || len(results) != 3 || results[0][0].ProfileId != 11 || results[1][0].ProfileId != 22 || len(results[2]) != 0 {
		t.Fatalf("partial results lost: %+v err=%v", results, err)
	}
}

func TestBotMediaBatchPhaseRejectsMalformedResponse(t *testing.T) {
	for _, count := range []int{0, 2} {
		_, err := loadBotMediaSearchCandidatesExactFirstBatch(context.Background(), []*mediaFingerprint{{MD5: "a"}},
			func(context.Context, []*mediaFingerprint) ([][]*botMediaSearchCandidate, error) {
				return make([][]*botMediaSearchCandidate, 1), nil
			},
			func(context.Context, []*mediaFingerprint) ([][]*botMediaSearchCandidate, error) {
				return make([][]*botMediaSearchCandidate, count), nil
			})
		if err == nil {
			t.Fatalf("accepted response slots=%d", count)
		}
	}
}

func TestBotMediaBatchWaiterCancellationDoesNotCancelSharedQuery(t *testing.T) {
	key := t.Name()
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := botMediaSharedBatchRows(context.Background(), key, func() ([]mediaPHashBucketCandidateRow, error) {
			close(started)
			<-release
			return []mediaPHashBucketCandidateRow{{MediaId: 123}}, nil
		})
		done <- err
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := botMediaSharedBatchRows(ctx, key, func() ([]mediaPHashBucketCandidateRow, error) { t.Error("duplicate DB query started"); return nil, nil })
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter error=%v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("waiter canceled leader: %v", err)
	}
}

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

func TestLoadBotMediaCandidatesContinuesAfterOneTimeout(t *testing.T) {
	var calls atomic.Int32
	results, err := loadBotMediaSearchCandidates(context.Background(), []*mediaFingerprint{{MD5: "timeout"}, {MD5: "matching"}, {MD5: "later"}}, func(ctx context.Context, f *mediaFingerprint) ([]*botMediaSearchCandidate, error) {
		calls.Add(1)
		if f.MD5 == "timeout" {
			return nil, context.DeadlineExceeded
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return []*botMediaSearchCandidate{{ProfileId: 101, Distance: 12}}, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 3 || len(results) != 3 || len(results[1]) != 1 || len(results[2]) != 1 {
		t.Fatalf("album queries were cancelled: calls=%d results=%v err=%v", calls.Load(), results, err)
	}
}

func TestLoadBotMediaCandidatesIncompleteEmptyIsNotNoMatch(t *testing.T) {
	results, err := loadBotMediaSearchCandidates(context.Background(), []*mediaFingerprint{{MD5: "timeout"}, {MD5: "empty"}}, func(_ context.Context, f *mediaFingerprint) ([]*botMediaSearchCandidate, error) {
		if f.MD5 == "timeout" {
			return nil, context.DeadlineExceeded
		}
		return nil, nil
	})
	if err == nil || results != nil {
		t.Fatalf("incomplete search returned empty success: results=%v err=%v", results, err)
	}
}

func TestLoadBotMediaCandidatesSharesAlbumDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := loadBotMediaSearchCandidates(ctx, []*mediaFingerprint{{MD5: "a"}, {MD5: "b"}, {MD5: "c"}, {MD5: "d"}}, func(ctx context.Context, _ *mediaFingerprint) ([]*botMediaSearchCandidate, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("album deadline was not respected: elapsed=%s err=%v", time.Since(started), err)
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

func TestBotMediaCandidateQueryTimeoutEnvironment(t *testing.T) {
	ctx := context.Background()
	adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
	if !ok {
		t.Fatal("file configuration adapter required")
	}
	const key = "youbanPublish.mediaSearch.queryTimeoutSeconds"
	original := g.Cfg().MustGet(ctx, key).Val()
	t.Cleanup(func() { _ = adapter.Set(key, original) })
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"", 5 * time.Second}, {"7", 7 * time.Second}, {"0", 5 * time.Second}, {"-1", 5 * time.Second}, {"invalid", 5 * time.Second},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("YOUBAN_PUBLISH_MEDIA_SEARCH_QUERY_TIMEOUT_SECONDS", tc.value)
			envconfig.Apply(ctx)
			if got := botMediaCandidateQueryTimeout(ctx); got != tc.want {
				t.Fatalf("timeout=%s want=%s", got, tc.want)
			}
			_, err := loadBotMediaSearchCandidates(ctx, []*mediaFingerprint{{MD5: "deadline-probe"}}, func(queryCtx context.Context, _ *mediaFingerprint) ([]*botMediaSearchCandidate, error) {
				deadline, ok := queryCtx.Deadline()
				if !ok || time.Until(deadline) > tc.want || time.Until(deadline) < tc.want-time.Second {
					t.Errorf("incorrect query deadline: %s", time.Until(deadline))
				}
				return nil, nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
