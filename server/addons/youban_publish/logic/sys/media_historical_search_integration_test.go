package sys

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/bootstrap/envconfig"
	"hotgo/internal/library/cache"
)

// Opt-in replay: reads production profiles without sending Telegram messages
// or starting account runtimes/queue consumers. Uses a local image HTTP source.
func TestHistoricalBotSearchLiveReplay(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_LIVE_REPLAY") != "1" {
		t.Skip("explicit live replay required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	envconfig.Apply(ctx)
	if !cache.Initialized() {
		cache.SetAdapter(ctx)
	}
	g.DB().GetCache().SetAdapter(gcache.NewAdapterMemory())
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	defer cache.Instance().SetAdapter(previous)
	// Verify production schema first; replay must not run runtime DDL.
	if _, err := g.DB().Model(publishMediaPHashAliasBucketTable).Ctx(ctx).Limit(1).All(); err != nil {
		t.Fatal(err)
	}
	mediaPHashAliasTableState.Lock()
	mediaPHashAliasTableState.ready = true
	mediaPHashAliasTableState.Unlock()
	if err := markMediaPHashLshReady(ctx); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(os.Getenv("YOUBAN_SCAN_FIXTURE_ROOT"))))
	defer server.Close()
	cases := []struct {
		name, pattern string
		expected      int64
	}{
		{"EEKFQFFA", "other-all/ee*.jpg", 676231},
		{"JNAAUKWG", "other-all/jn*.jpg", 302321},
		{"GVJFGVCU", "gvj-all/*.jpg", 676231},
	}
	sentPaths, _ := filepath.Glob(filepath.Join(os.Getenv("YOUBAN_SCAN_FIXTURE_ROOT"), "sent-tg", "*.jpg"))
	for _, path := range sentPaths {
		cases = append(cases, struct {
			name, pattern string
			expected      int64
		}{"sent-single-" + filepath.Base(path), "sent-tg/" + filepath.Base(path), 676231})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Isolate request/ORM caches between cases; EEK and GVJ share images.
			cache.Instance().SetAdapter(gcache.NewAdapterMemory())
			g.DB().GetCache().SetAdapter(gcache.NewAdapterMemory())
			if err := markMediaPHashLshReady(ctx); err != nil {
				t.Fatal(err)
			}
			paths, err := filepath.Glob(filepath.Join(os.Getenv("YOUBAN_SCAN_FIXTURE_ROOT"), c.pattern))
			if err != nil || len(paths) == 0 {
				t.Fatal("missing fixtures")
			}
			in := &sysin.BotMediaSearchInp{TenantId: 5, AccountId: 552, TraceId: "LOCAL-REPLAY-" + c.name}
			for _, path := range paths {
				rel, _ := filepath.Rel(os.Getenv("YOUBAN_SCAN_FIXTURE_ROOT"), path)
				in.Items = append(in.Items, &sysin.BotMediaSearchItem{FileUrl: server.URL + "/" + filepath.ToSlash(rel), FileUniqueId: "live-replay-" + c.name + filepath.Base(path), MediaType: "image"})
			}
			var previousIDs []int64
			for round, budget := range []time.Duration{3 * time.Second, 500 * time.Millisecond} {
				started := time.Now()
				notes, total, err := NewSysPublish().BotProfileMediaSearch(ctx, in)
				elapsed := time.Since(started)
				if err != nil {
					t.Fatal(err)
				}
				ids := make([]int64, 0, len(notes))
				found := false
				for _, note := range notes {
					ids = append(ids, note.Id)
					if note.Id == c.expected {
						found = true
					}
				}
				t.Logf("case=%s round=%d elapsed=%s total=%d ids=%v expectedCandidate=%d found=%t", c.name, round+1, elapsed, total, ids, c.expected, found)
				if !found || len(ids) != 1 || ids[0] != c.expected {
					t.Errorf("expected only the visually verified profile %d, got %v", c.expected, ids)
				}
				if elapsed > budget {
					t.Errorf("service replay exceeded budget %s", budget)
				}
				if round > 0 && !reflect.DeepEqual(previousIDs, ids) {
					t.Errorf("warm results differ from cold results")
				}
				previousIDs = ids
			}

		})
	}
	t.Run("sent-file-identity-without-download", func(t *testing.T) {
		row, err := g.DB().Model(publishTgMessageTable).Ctx(ctx).Fields("tg_file_unique_id").Where("profile_id", 676231).Where("tenant_id", 5).Where("status", "sent").Where("tg_file_unique_id <> ''").WhereNull("deleted_at").Limit(1).One()
		if err != nil || row.IsEmpty() {
			t.Fatal("sent fixture missing", err)
		}
		started := time.Now()
		notes, _, err := NewSysPublish().BotProfileMediaSearch(ctx, &sysin.BotMediaSearchInp{TenantId: 5, AccountId: 552, TraceId: "LOCAL-REPLAY-SENT-ID", Items: []*sysin.BotMediaSearchItem{{FileUniqueId: row["tg_file_unique_id"].String(), FileUrl: server.URL + "/must-not-download.jpg", MediaType: "image"}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(notes) != 1 || notes[0].Id != 676231 {
			t.Fatalf("incorrect exact identity result: %v", notes)
		}
		t.Logf("exact sent-file lookup elapsed=%s profile=676231 download=not-required", time.Since(started))
	})

}
