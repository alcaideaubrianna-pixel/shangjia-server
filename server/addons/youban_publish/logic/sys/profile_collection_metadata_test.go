package sys

import "testing"

func TestApplyProfileCollectionJobSource(t *testing.T) {
	metadata := map[int64]profileCollectionMetadataRow{
		301948: {ProfileId: 301948, SourceId: 80, SourceChatId: "3974614787"},
	}
	applyProfileCollectionJobSource(metadata, profileCollectionJobSourceRow{
		ProfileId: 301948, SourceId: 80, SourceChatId: "3974614787", SourceMessageId: 1749,
	})

	got := metadata[301948]
	if got.SourceChatId != "3974614787" || got.SourceMessageId != 1749 {
		t.Fatalf("source fallback = (%q,%d), want (3974614787,1749)", got.SourceChatId, got.SourceMessageId)
	}
}

func TestApplyProfileCollectionJobSourceKeepsCurrentEvent(t *testing.T) {
	metadata := map[int64]profileCollectionMetadataRow{
		1: {ProfileId: 1, SourceId: 80, SourceChatId: "-1001", SourceMessageId: 99},
	}
	applyProfileCollectionJobSource(metadata, profileCollectionJobSourceRow{
		ProfileId: 1, SourceId: 80, SourceChatId: "-1002", SourceMessageId: 100,
	})

	got := metadata[1]
	if got.SourceChatId != "-1001" || got.SourceMessageId != 99 {
		t.Fatalf("current event source was overwritten: (%q,%d)", got.SourceChatId, got.SourceMessageId)
	}
}

func TestApplyProfileCollectionJobSourceRejectsDifferentSource(t *testing.T) {
	metadata := map[int64]profileCollectionMetadataRow{
		1: {ProfileId: 1, SourceId: 80},
	}
	applyProfileCollectionJobSource(metadata, profileCollectionJobSourceRow{
		ProfileId: 1, SourceId: 81, SourceChatId: "-1002", SourceMessageId: 100,
	})

	if got := metadata[1]; got.SourceMessageId != 0 {
		t.Fatalf("different source was accepted: %#v", got)
	}
}

func TestCollectedTelegramMessageURLUsesSourceUsername(t *testing.T) {
	got := collectedTelegramMessageURL("3974614787", "@Qidi_SKS2", 1749)
	if got != "https://t.me/Qidi_SKS2/1749" {
		t.Fatalf("source URL = %q", got)
	}
}
