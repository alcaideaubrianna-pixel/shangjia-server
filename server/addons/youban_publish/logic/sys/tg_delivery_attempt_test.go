package sys

import (
	"strings"
	"testing"
)

func TestTelegramAttemptMarkerStableAndDistinct(t *testing.T) {
	first := telegramAttemptMarker("attempt-a")
	if first == "" || first != telegramAttemptMarker("attempt-a") {
		t.Fatal("attempt marker must be stable and non-empty")
	}
	if first == telegramAttemptMarker("attempt-b") {
		t.Fatal("different attempts must not share a marker")
	}
	if !strings.HasPrefix(first, "\u2063") || !strings.HasSuffix(first, "\u2064") {
		t.Fatal("attempt marker must retain its invisible boundaries")
	}
}

func TestTelegramAttemptMarkerSurvivesCaptionComposition(t *testing.T) {
	marker := telegramAttemptMarker("attempt-caption")
	caption := telegramCaptionWithJobMarker("资料标题", 42, "display") + marker
	if !strings.Contains(caption, marker) {
		t.Fatal("composed caption lost attempt marker")
	}
}
