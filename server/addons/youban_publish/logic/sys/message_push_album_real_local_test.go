package sys

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	gotdmessage "github.com/gotd/td/telegram/message"
)

func TestLocalAccountFallbackMediaPreparation(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_ACCOUNT_FALLBACK") != "1" {
		t.Skip("set YOUBAN_LOCAL_ACCOUNT_FALLBACK=1 to replay production media preparation")
	}
	jobID, err := strconv.ParseInt(os.Getenv("YOUBAN_LOCAL_ACCOUNT_FALLBACK_JOB_ID"), 10, 64)
	if err != nil || jobID <= 0 {
		t.Fatal("YOUBAN_LOCAL_ACCOUNT_FALLBACK_JOB_ID must contain one positive job ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var media []*telegramMediaItem
	if fixturePath := os.Getenv("YOUBAN_LOCAL_ACCOUNT_FALLBACK_FIXTURE"); fixturePath != "" {
		fixture, readErr := os.ReadFile(fixturePath)
		if readErr != nil {
			t.Fatalf("read fixture: %v", readErr)
		}
		if decodeErr := json.Unmarshal(fixture, &media); decodeErr != nil {
			t.Fatalf("decode fixture: %v", decodeErr)
		}
	} else {
		service := NewSysPublish()
		job, loadErr := service.telegramJobById(ctx, jobID)
		if loadErr != nil {
			t.Fatalf("load job %d: %v", jobID, loadErr)
		}
		media, loadErr = service.telegramJobMedia(ctx, job, "display")
		if loadErr != nil {
			t.Fatalf("load job media %d: %v", jobID, loadErr)
		}
	}
	if len(media) != 10 {
		t.Fatalf("job %d display media count = %d, want 10", jobID, len(media))
	}
	started := time.Now()
	_, err = parallelOrderedMap(ctx, media, gotdAlbumUploadConcurrency, func(prepareCtx context.Context, _ int, item *telegramMediaItem) (gotdmessage.UploadOption, error) {
		option, cleanup, prepareErr := gotdMessageUploadOption(prepareCtx, item)
		if cleanup != nil {
			defer cleanup()
		}
		return option, prepareErr
	})
	if err != nil {
		t.Fatalf("prepare job %d media: %v", jobID, err)
	}
	t.Logf("prepared job %d media=%d duration=%s", jobID, len(media), time.Since(started).Round(time.Millisecond))
}
