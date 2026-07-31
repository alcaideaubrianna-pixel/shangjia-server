package sys

import (
	"testing"
	"time"
)

func TestCollectMediaFingerprintSetKeyIsOrderIndependent(t *testing.T) {
	left := []collectMediaItem{
		{Type: "photo", FileId: "photo-a"},
		{Type: "photo", FileId: "photo-b"},
		{Type: "photo", FileId: "photo-c"},
	}
	right := []collectMediaItem{
		{Type: "photo", FileId: "photo-c"},
		{Type: "photo", FileId: "photo-a"},
		{Type: "photo", FileId: "photo-b"},
	}
	if got, want := collectMediaFingerprintSetKey(left), collectMediaFingerprintSetKey(right); got != want {
		t.Fatalf("media fingerprint set key differs by order: %q != %q", got, want)
	}
}

func TestCollectDedupeMaterialMatchesByPhase(t *testing.T) {
	current := collectDedupeMaterialFromValues("text-current", `[
		{"type":"photo","fileId":"photo-current","filePhash":"phash-current"}
	]`)

	cases := []struct {
		name     string
		previous collectDedupeMaterial
		phase    collectDedupePhase
		want     string
	}{
		{
			name:     "media fingerprint",
			previous: collectDedupeMaterialFromValues("different-text", `[{"type":"photo","fileId":"photo-current","filePhash":"different-phash"}]`),
			phase:    collectDedupePhaseEarly,
			want:     "media_fingerprint",
		},
		{
			name:     "text hash",
			previous: collectDedupeMaterialFromValues("text-current", `[{"type":"photo","fileId":"different-photo","filePhash":"different-phash"}]`),
			phase:    collectDedupePhaseEarly,
			want:     "text_hash",
		},
		{
			name:     "image phash",
			previous: collectDedupeMaterialFromValues("different-text", `[{"type":"photo","fileId":"different-photo","filePhash":"phash-current"}]`),
			phase:    collectDedupePhasePHash,
			want:     "image_phash",
		},
		{
			name:     "early phase ignores phash",
			previous: collectDedupeMaterialFromValues("different-text", `[{"type":"photo","fileId":"different-photo","filePhash":"phash-current"}]`),
			phase:    collectDedupePhaseEarly,
			want:     "",
		},
		{
			name:     "phash phase ignores text",
			previous: collectDedupeMaterialFromValues("text-current", `[{"type":"photo","fileId":"different-photo","filePhash":"different-phash"}]`),
			phase:    collectDedupePhasePHash,
			want:     "",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := current.matchLayer(testCase.previous, testCase.phase); got != testCase.want {
				t.Fatalf("match layer = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestCollectImagePHashSetIgnoresOrderAndVideos(t *testing.T) {
	left := []collectMediaItem{
		{Type: "photo", FilePhash: "A"},
		{Type: "photo", FilePhash: "B"},
		{Type: "video", FilePhash: "video"},
	}
	right := []collectMediaItem{
		{Type: "photo", FilePhash: "b"},
		{Type: "photo", FilePhash: "a"},
	}
	leftKey, leftCount := collectImagePHashSetKey(left)
	rightKey, rightCount := collectImagePHashSetKey(right)
	if leftKey != rightKey || leftCount != 2 || rightCount != 2 {
		t.Fatalf("image phash set mismatch: left=(%q,%d) right=(%q,%d)", leftKey, leftCount, rightKey, rightCount)
	}
}

func TestCollectMediaPHashReadsMetadata(t *testing.T) {
	item := collectMediaItem{
		Type:     "photo",
		MetaJson: `{"perceptual_hash":"ABC123"}`,
	}
	if got := collectMediaPHash(item); got != "abc123" {
		t.Fatalf("media phash = %q, want %q", got, "abc123")
	}
}

func TestCollectDedupeCacheValue(t *testing.T) {
	wantTime := time.Unix(1_785_000_000, 0)
	value := collectDedupeCacheValue(123, wantTime)
	entry, ok := parseCollectDedupeCacheValue(value)
	if !ok || entry.EventID != 123 || entry.ReceivedAt != wantTime.Unix() {
		t.Fatalf("entry = %+v ok=%v", entry, ok)
	}
	if parseEntry, parseOK := parseCollectDedupeCacheValue("invalid"); parseOK || parseEntry.EventID != 0 {
		t.Fatalf("invalid cache value must be rejected: %+v %v", parseEntry, parseOK)
	}
}

func TestCollectDedupeCacheEntryValid(t *testing.T) {
	now := time.Unix(1_785_000_000, 0)
	recent := collectDedupeCacheEntry{EventID: 1, ReceivedAt: now.AddDate(0, 0, -2).Unix()}
	if !collectDedupeCacheEntryValid(recent, 3, now) {
		t.Fatal("recent entry must be valid inside the time window")
	}
	if collectDedupeCacheEntryValid(recent, 1, now) {
		t.Fatal("old entry must be invalid outside the time window")
	}
	if !collectDedupeCacheEntryValid(recent, 0, now) {
		t.Fatal("zero-day window must have no expiration")
	}
}
