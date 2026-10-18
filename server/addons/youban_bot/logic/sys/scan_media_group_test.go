package sys

import (
	"context"
	"fmt"
	"strings"
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

func TestScanTraceIdRoundTrip(t *testing.T) {
	traceId := newScanTraceId()
	if len(traceId) != 8 || traceId != strings.ToUpper(traceId) {
		t.Fatalf("unexpected scan trace id: %q", traceId)
	}
	if got := scanTraceId(withScanTrace(context.Background(), traceId)); got != traceId {
		t.Fatalf("scan trace id round trip: got %q, want %q", got, traceId)
	}
	if acknowledgement := scanAcknowledgement(traceId); !strings.Contains(acknowledgement, traceId) {
		t.Fatalf("acknowledgement does not contain trace id: %q", acknowledgement)
	}
}

func TestNormalizeScanTraceIdRejectsLegacyAcknowledgementValue(t *testing.T) {
	if got := normalizeScanTraceId("1"); got != "" {
		t.Fatalf("legacy acknowledgement value must not become a trace id: %q", got)
	}
}

func TestEnsureScanTraceIdGeneratesForLegacyEmptyValue(t *testing.T) {
	traceId := ensureScanTraceId("")
	if len(traceId) != 8 {
		t.Fatalf("generated trace id length: got %d, want 8", len(traceId))
	}
	if got := scanTraceId(withScanTrace(context.Background(), "")); len(got) != 8 {
		t.Fatalf("context trace id length: got %d, want 8", len(got))
	}
}

func TestScanSearchUserErrorHidesDatabaseTimeout(t *testing.T) {
	err := scanSearchUserError("TRACE123", fmt.Errorf("BEGIN: %w", context.DeadlineExceeded))
	if got := err.Error(); !strings.Contains(got, "TRACE123") || strings.Contains(got, "BEGIN") {
		t.Fatalf("unexpected user error: %q", got)
	}
}

func TestScanVideoResolvesThumbnailWithoutRequestingVideoFile(t *testing.T) {
	msg := &models.Message{Video: &models.Video{FileID: "large-video", FileUniqueID: "stable-video", Thumbnail: &models.PhotoSize{FileID: "preview", FileUniqueID: "preview-id", Width: 320, Height: 240}}}
	preview, err := scanMediaPreviewMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Video != nil || len(preview.Photo) != 1 || preview.Photo[0].FileID != "preview" {
		t.Fatalf("video body could reach getFile: %+v", preview)
	}
	if msg.Video == nil || msg.Video.FileID != "large-video" {
		t.Fatal("original message mutated")
	}
	msg.Video.Thumbnail = nil
	if _, err = scanMediaPreviewMessage(msg); err == nil {
		t.Fatal("video without thumbnail must not download body")
	}
}
