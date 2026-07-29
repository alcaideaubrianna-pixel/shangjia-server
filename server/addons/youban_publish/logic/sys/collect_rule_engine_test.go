package sys

import "testing"

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

func TestCollectDedupeMaterialMatchesAnyLayer(t *testing.T) {
	current := collectDedupeMaterialFromValues("text-current", `[
		{"type":"photo","fileId":"photo-current","filePhash":"phash-current"}
	]`)

	cases := []struct {
		name     string
		previous collectDedupeMaterial
		want     string
	}{
		{
			name:     "media fingerprint",
			previous: collectDedupeMaterialFromValues("different-text", `[{"type":"photo","fileId":"photo-current","filePhash":"different-phash"}]`),
			want:     "media_fingerprint",
		},
		{
			name:     "text hash",
			previous: collectDedupeMaterialFromValues("text-current", `[{"type":"photo","fileId":"different-photo","filePhash":"different-phash"}]`),
			want:     "text_hash",
		},
		{
			name:     "image phash",
			previous: collectDedupeMaterialFromValues("different-text", `[{"type":"photo","fileId":"different-photo","filePhash":"phash-current"}]`),
			want:     "image_phash",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := current.matchLayer(testCase.previous); got != testCase.want {
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
