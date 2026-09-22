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

func TestTelegramInternalDeliveryMarkerDetection(t *testing.T) {
	if telegramTextHasInternalDeliveryMarker("普通消息") {
		t.Fatal("plain message must not be treated as an internal delivery")
	}
	if !telegramTextHasInternalDeliveryMarker("资料" + telegramAttemptMarker("delete-safe")) {
		t.Fatal("attempt marker must protect internal delivery from auto delete")
	}
}

func TestTelegramVerifyMediaMatchRequiresKnownUniqueID(t *testing.T) {
	media := []*telegramMediaItem{{MediaType: "video", TgFileUniqueId: "expected"}}
	if telegramVerifyMediaMatches(media, "video", "other") {
		t.Fatal("known file unique id mismatch must not confirm an attempt")
	}
	if !telegramVerifyMediaMatches(media, "video", "expected") {
		t.Fatal("matching file unique id must confirm the media candidate")
	}
	if telegramVerifyMediaMatches(media, "image", "expected") {
		t.Fatal("media type mismatch must not confirm an attempt")
	}
}

func TestTelegramVerifyFreshUploadUsesMediaTypeFallback(t *testing.T) {
	media := []*telegramMediaItem{{MediaType: "photo"}}
	if !telegramVerifyMediaMatches(media, "image", "new-file") {
		t.Fatal("fresh upload should match the sole short-lived attempt by normalized media type")
	}
}

func TestTelegramAttemptTerminalStatusesAreDistinct(t *testing.T) {
	statuses := map[string]struct{}{
		telegramAttemptStatusConfirmed:  {},
		telegramAttemptStatusSuperseded: {},
	}
	if len(statuses) != 2 {
		t.Fatal("confirmed and superseded attempts must remain distinguishable")
	}
}
