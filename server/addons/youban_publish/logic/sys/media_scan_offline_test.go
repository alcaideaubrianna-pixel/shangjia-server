package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"
	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

// Replays the candidate pipeline against a private, isolated local snapshot.
// It neither starts workers nor calls Telegram, COS or a production database.
// Downloading/hashing and final note hydration are outside this measurement.
func TestScanOfflineCorpusReplay(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_OFFLINE_CORPUS") != "1" {
		t.Skip("requires private local search snapshot and incident fixture")
	}
	var fixture struct {
		Accounts []struct {
			ID       int64 `json:"id"`
			TenantID int64 `json:"tenantId"`
		} `json:"accounts"`
		Inputs []struct {
			TraceID string `json:"traceId"`
			Item    struct {
				FileUniqueID string `json:"fileUniqueId"`
			} `json:"item"`
		} `json:"inputs"`
		Fingerprints map[string]string `json:"fingerprints"`
	}
	data, err := os.ReadFile(os.Getenv("YOUBAN_SCAN_INCIDENT_FIXTURE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	partitions := make([]publishmodel.MediaSearchScopePartition, 0, len(fixture.Accounts))
	for _, account := range fixture.Accounts {
		partitions = append(partitions, publishmodel.MediaSearchScopePartition{TenantId: account.TenantID, AccountIds: []int64{account.ID}})
	}
	if len(partitions) == 0 {
		t.Fatal("missing real permission scope")
	}
	scope := mediaSearchScopeFromPartitions(partitions)
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	// Host and DB are deliberately fixed: an environment variable cannot
	// redirect this replay into production.
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "youban_scan_offline_20261023"}})
	resetLocalScanTableFields(t, g.DB())
	ctx := context.Background()
	t.Run("md5_real_corpus", func(t *testing.T) {
		values := []string{"deliberately-missing-fingerprint"}
		for _, input := range fixture.Inputs {
			switch input.TraceID {
			case "ICFBUMHJ", "FPKMXDZB", "YOEKMFUD", "7LYKUCBD":
			default:
				continue
			}
			var fp cachedBotMediaFingerprint
			if err := json.Unmarshal([]byte(fixture.Fingerprints[botMediaFingerprintCacheKey(input.Item.FileUniqueID)]), &fp); err != nil {
				t.Fatal(err)
			}
			values = append(values, fp.MD5)
		}
		query, args := botMediaMD5CandidatesSQL(values, scope)
		// Compare the same exact-match and ownership semantics without the
		// fingerprint-first materialization barrier.
		scopeSQL, scopeArgs := mediaPHashBucketScopeSQL("m", scope.Partitions)
		oldArgs := make([]any, 0, len(values)+len(scopeArgs))
		for _, value := range values {
			oldArgs = append(oldArgs, value)
		}
		oldArgs = append(oldArgs, scopeArgs...)
		oldSQL := "SELECT m.md5 AS fingerprint_key,m.profile_id FROM hg_youban_publish_media m WHERE m.md5 IN (" + strings.TrimSuffix(strings.Repeat("?,", len(values)), ",") + ") AND (" + scopeSQL + ") AND m.deleted_at IS NULL AND m.media_type IN ('image','video') AND EXISTS (SELECT 1 FROM hg_content_profile p WHERE p.id=m.profile_id AND p.deleted_at IS NULL) AND EXISTS (SELECT 1 FROM hg_youban_publish_profile_state ps WHERE ps.profile_id=m.profile_id AND ps.tenant_id=m.tenant_id AND ps.account_id=m.account_id AND ps.deleted_at IS NULL) GROUP BY m.md5,m.profile_id"
		identity := func(rows gdb.Result) []string {
			keys := make([]string, 0, len(rows))
			for _, row := range rows {
				keys = append(keys, row["fingerprint_key"].String()+":"+row["profile_id"].String())
			}
			sort.Strings(keys)
			return keys
		}
		want, err := g.DB().GetAll(ctx, oldSQL, oldArgs...)
		if err != nil {
			t.Fatal(err)
		}
		for round := 0; round < 3; round++ {
			started := time.Now()
			got, err := g.DB().GetAll(ctx, query, args...)
			elapsed := time.Since(started)
			t.Logf("md5 round=%d inputs=%d elapsed=%s matches=%d", round+1, len(values), elapsed, len(got))
			if err != nil || !reflect.DeepEqual(identity(got), identity(want)) {
				t.Fatalf("MD5 result mismatch: %v", err)
			}
			if elapsed > time.Second {
				t.Errorf("MD5 exceeded local 1s budget: %s", elapsed)
			}
		}
	})
	if os.Getenv("YOUBAN_SCAN_OFFLINE_MD5_ONLY") == "1" {
		return
	}
	if !cache.Initialized() {
		adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
		if !ok {
			t.Fatal("requires file config adapter")
		}
		original := g.Cfg().MustGet(ctx, "cache.adapter").Val()
		if err := adapter.Set("cache.adapter", "memory"); err != nil {
			t.Fatal(err)
		}
		cache.SetAdapter(ctx)
		if err := adapter.Set("cache.adapter", original); err != nil {
			t.Fatal(err)
		}
	}
	previous := cache.Instance().GetAdapter()
	t.Cleanup(func() { cache.Instance().SetAdapter(previous) })
	service := NewSysPublish()
	cases := make(map[string][]*mediaFingerprint)
	names := []string{"ICFBUMHJ", "FPKMXDZB", "YOEKMFUD", "7LYKUCBD"}
	all := make([]*mediaFingerprint, 0)
	for _, trace := range names {
		items := make([]*mediaFingerprint, 0)
		for _, input := range fixture.Inputs {
			if input.TraceID != trace {
				continue
			}
			var fp cachedBotMediaFingerprint
			if err := json.Unmarshal([]byte(fixture.Fingerprints[botMediaFingerprintCacheKey(input.Item.FileUniqueID)]), &fp); err != nil {
				t.Fatal("missing original fingerprint", err)
			}
			items = append(items, &mediaFingerprint{MD5: fp.MD5, PHash: goimagehash.NewImageHash(fp.PHash, goimagehash.PHash)})
		}
		if len(items) == 0 {
			t.Fatal("missing original inputs")
		}
		cases[trace] = items
		all = append(all, items...)
	}
	for _, size := range []int{1, 10, 30} {
		name := fmt.Sprintf("composed_%d", size)
		for i := 0; i < size; i++ {
			cases[name] = append(cases[name], all[i%len(all)])
		}
		names = append(names, name)
	}
	for _, trace := range names {
		t.Run(trace, func(t *testing.T) {
			items := cases[trace]
			var previousIDs []int64
			for round := 0; round < 3; round++ {
				// Reset application caches each time. This does not claim cold
				// PostgreSQL/OS pages, which are shared by the local server.
				cache.Instance().SetAdapter(gcache.NewAdapterMemory())
				if err := markMediaPHashLshReady(ctx); err != nil {
					t.Fatal(err)
				}
				queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				started := time.Now()
				groups, err := loadBotMediaSearchCandidatesExactFirstBatch(queryCtx, items,
					func(ctx context.Context, items []*mediaFingerprint) ([][]*botMediaSearchCandidate, error) {
						return botMediaExactCandidates(ctx, items, scope)
					}, func(ctx context.Context, items []*mediaFingerprint) ([][]*botMediaSearchCandidate, error) {
						return service.botMediaApproximateBatch(ctx, items, &sysin.ProfileImageSearchInp{Threshold: 12}, scope)
					})
				elapsed := time.Since(started)
				cancel()
				ids := rankBotMediaSearchCandidates(groups)
				t.Logf("case=%s inputs=%d cold_round=%d candidate_pipeline=%s results=%d", trace, len(items), round+1, elapsed, len(ids))
				if err != nil || len(ids) == 0 {
					t.Fatalf("original incident not resolved: %v", err)
				}
				if elapsed > 3*time.Second {
					t.Errorf("candidate pipeline exceeded 3s: %s", elapsed)
				}
				if round > 0 && !reflect.DeepEqual(previousIDs, ids) {
					t.Error("cold replay ranking changed")
				}
				previousIDs = ids
			}
			// Compare every original input with the existing single-query
			// implementation; latency assertions above exclude reference work.
			unique := uniqueBotMediaFingerprints(items)
			referenceGroups, err := botMediaExactCandidates(ctx, unique, scope)
			if err != nil {
				t.Fatal(err)
			}
			identity := func(rows []mediaPHashBucketCandidateRow) []string {
				keys := make([]string, 0, len(rows))
				for _, row := range rows {
					keys = append(keys, fmt.Sprintf("%d:%d:%d:%d:%s:%s:%d", row.MediaId, row.ProfileId, row.TenantId, row.AccountId, row.MediaType, row.HashValue, row.BucketHits))
				}
				sort.Strings(keys)
				return keys
			}
			for i, item := range unique {
				hash := fmt.Sprintf("%016x", item.PHash.GetHash())
				batchSQL, batchArgs, err := mediaPHashBatchSQL([]string{hash}, 12, scope.Partitions, false)
				if err != nil {
					t.Fatal(err)
				}
				singleSQL, singleArgs, err := mediaPHashLshCandidateSQLWithStrategy(hash, 12, scope.Partitions, nil, "image", 0, false)
				if err != nil {
					t.Fatal(err)
				}
				var batch, single []mediaPHashBucketCandidateRow
				if err := g.DB().Raw(batchSQL, batchArgs...).Scan(&batch); err != nil {
					t.Fatal(err)
				}
				if err := g.DB().Raw(singleSQL, singleArgs...).Scan(&single); err != nil {
					t.Fatal(err)
				}
				// The old SQL only orders by bucket_hits, so ties have no
				// defined order. Compare identities and counts, then separately
				// verify the user-facing merged ranking below.
				if !reflect.DeepEqual(identity(batch), identity(single)) {
					t.Fatalf("input=%d candidate identities/bucket counts differ", i)
				}
				if len(referenceGroups[i]) > 0 {
					continue
				}
				matches := make([]publishProfilePHashDistance, 0, len(single))
				for _, row := range single {
					hash, ok := parseUploadPHash(row.HashValue)
					if !ok {
						t.Fatal("invalid reference hash")
					}
					distance, err := item.PHash.Distance(hash)
					if err != nil || distance > 12 {
						t.Fatal("reference distance outside threshold")
					}
					matches = append(matches, publishProfilePHashDistance{ProfileId: row.ProfileId, MediaId: row.MediaId, MediaType: row.MediaType, Distance: distance})
				}
				referenceGroups[i] = botMediaCandidatesFromMatches(mediaPHashDeduplicateProfiles(matches))
			}
			if want := rankBotMediaSearchCandidates(referenceGroups); !reflect.DeepEqual(previousIDs, want) {
				t.Fatal("merged profile identities/ranking differ from single-query reference")
			}
		})
	}
}
