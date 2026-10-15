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

func TestMergeBotMediaSearchNotesDeduplicatesProfiles(t *testing.T) {
	notesById := make(map[int64]*sysin.NoteModel)
	ids := mergeBotMediaSearchNotes(notesById, nil, []*sysin.NoteModel{{ProfileModel: sysin.ProfileModel{Id: 11}}, {ProfileModel: sysin.ProfileModel{Id: 22}}, {ProfileModel: sysin.ProfileModel{Id: 11}}, nil})
	ids = mergeBotMediaSearchNotes(notesById, ids, []*sysin.NoteModel{{ProfileModel: sysin.ProfileModel{Id: 22}}, {ProfileModel: sysin.ProfileModel{Id: 33}}})
	if !reflect.DeepEqual(ids, []int64{11, 22, 33}) {
		t.Fatalf("merged profile ids: got %v", ids)
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
