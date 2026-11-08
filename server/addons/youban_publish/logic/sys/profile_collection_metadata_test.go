package sys

import (
	"testing"

	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestPreferProfileCollectionMetadataUsesLatestSourceEvent(t *testing.T) {
	old := gtime.New("2026-11-03 13:29:38")
	latest := gtime.New("2026-11-03 17:09:59")
	if !preferProfileCollectionMetadata(profileCollectionMetadataRow{SourceId: 352, SourceCreatedAt: latest}, profileCollectionMetadataRow{SourceId: 351, SourceCreatedAt: old}) {
		t.Fatal("latest collection source event should be preferred")
	}
}

func TestPreferProfileCollectionMetadataAcrossBotsAndAccounts(t *testing.T) {
	when := gtime.New("2026-11-03 17:09:59")
	bot := profileCollectionMetadataRow{DispatchId: 41862, SourceId: 352, BotId: 65, SourceCreatedAt: when}
	account := profileCollectionMetadataRow{DispatchId: 41861, SourceId: 351, TgAccountId: 54, SourceCreatedAt: when}
	if !preferProfileCollectionMetadata(bot, account) {
		t.Fatal("same message from bot/account collectors must use deterministic latest dispatch")
	}
}

func TestPreferProfileCollectionMetadataStableWhenSameDispatch(t *testing.T) {
	when := gtime.New("2026-11-03 17:09:59")
	if !preferProfileCollectionMetadata(
		profileCollectionMetadataRow{DispatchId: 10, SourceId: 352, SourceCreatedAt: when},
		profileCollectionMetadataRow{DispatchId: 10, SourceId: 351, SourceCreatedAt: when},
	) {
		t.Fatal("same dispatch must use the higher source id deterministically")
	}
}

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

func TestMergeProfileCollectionDisplayDoesNotEraseSnapshotUsername(t *testing.T) {
	item := &sysin.ProfileModel{CollectSourceName: "snapshot", CollectSourceUsername: "source_user"}
	mergeProfileCollectionDisplay(item, telegramChannelDisplay{Title: "Live title"})
	if item.CollectSourceName != "Live title" || item.CollectSourceUsername != "source_user" {
		t.Fatalf("display merge erased source snapshot: %#v", item)
	}
}
