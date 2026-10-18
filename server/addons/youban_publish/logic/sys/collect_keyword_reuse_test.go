package sys

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestCollectKeywordsRequiredInAllModes(t *testing.T) {
	for _, full := range []int{0, 1} {
		for _, tc := range []struct {
			name  string
			text  string
			terms []string
			want  bool
		}{
			{"matches Chinese", "今日上海资料", []string{"上海"}, true},
			{"matches any case insensitive", "HELLO world", []string{"上海", "hello"}, true},
			{"miss", "北京资料", []string{"上海"}, false},
			{"captionless cannot bypass", "", []string{"上海"}, false},
			{"empty unrestricted", "北京资料", nil, true},
			{"captionless unrestricted", "", nil, true},
			{"regex alternative", "今日北京资料", []string{"regex:上海|北京"}, true},
			{"regex miss", "广州资料", []string{"regex:上海|北京"}, false},
			{"regex escape case preserved", "abc", []string{`regex:^\d+$`, `regex:^\D+$`}, true},
			{"regex empty media rejected", "", []string{"regex:^$"}, false},
		} {
			t.Run(tc.name+string(rune('0'+full)), func(t *testing.T) {
				rule := gdb.Record{"keywords": gvar.New(tc.terms), "full_match_enabled": gvar.New(full)}
				event := gdb.Record{"raw_text": gvar.New(tc.text), "media_count": gvar.New(1)}
				if got := precheckCollectRule(event, rule); got.Matched != tc.want {
					t.Fatalf("precheck=%+v want matched=%t", got, tc.want)
				}
				// Dispatch must independently enforce the condition on retries.
				if got := buildCollectRuleDecision(event, nil, rule); got.Matched != tc.want {
					t.Fatalf("decision=%+v want matched=%t", got, tc.want)
				}
			})
		}
	}
}

func TestPublishedCollectEnrichmentDoesNotDownload(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	item := collectMediaItem{Type: "photo", FileUrl: server.URL + "/original.jpg", FileMd5: "098f6bcd4621d373cade4e832627b4f6", DebugMetaJson: `{"reused_published_media_id":123}`}
	content := &collectContentResult{Media: []collectMediaItem{item}}
	new(sSysPublish).enrichCollectContentMediaMetadata(context.Background(), content)
	if requests.Load() != 0 {
		t.Fatalf("published media triggered %d downloads", requests.Load())
	}
	if content.Media[0].FileMd5 != item.FileMd5 || content.Media[0].FileUrl != item.FileUrl {
		t.Fatal("published asset was changed")
	}
}
