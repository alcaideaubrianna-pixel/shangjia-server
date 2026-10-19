package sys

import (
	"context"
	"fmt"
	"testing"
)

func TestMediaProcessFailureStatus(t *testing.T) {
	for attempt := 0; attempt < 5; attempt++ {
		want := mediaProcessingRetry
		if attempt >= 2 {
			want = mediaProcessingFailed
		}
		err := fmt.Errorf("download failed: %w", context.DeadlineExceeded)
		if got := mediaProcessFailureStatus(context.Background(), err, attempt); got != want {
			t.Fatalf("retry=%d status=%s want=%s", attempt, got, want)
		}
	}
	if got := mediaProcessFailureStatus(context.Background(), &mediaFileCacheHTTPStatusError{statusCode: 404}, 0); got != mediaProcessingFailed {
		t.Fatal("404 must not retry")
	}
}

func TestTerminalMediaProcessingStatuses(t *testing.T) {
	statuses := terminalMediaProcessingStatuses()
	want := map[string]bool{
		mediaProcessingReady:  true,
		mediaProcessingFailed: true,
	}
	if len(statuses) != len(want) {
		t.Fatalf("terminal status count = %d, want %d", len(statuses), len(want))
	}
	for _, status := range statuses {
		if !want[status] {
			t.Fatalf("unexpected terminal media status %q", status)
		}
	}
}
