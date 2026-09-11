package sys

import "testing"

func TestBuildProfileFingerprintsUsesFinalText(t *testing.T) {
	media := []collectMediaItem{{Type: "photo", FileMd5: "AABB", FilePhash: "0011223344556677"}}
	first := buildProfileFingerprints([]int64{9}, "最终文案", media)
	second := buildProfileFingerprints([]int64{9}, "最终文案", media)
	changed := buildProfileFingerprints([]int64{9}, "另一份最终文案", media)
	if len(first) != 3 || len(second) != 3 || len(changed) != 3 {
		t.Fatalf("unexpected signatures: first=%d second=%d changed=%d", len(first), len(second), len(changed))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("same final material produced different fingerprint at %d", i)
		}
	}
	if first[0].Signature == changed[0].Signature {
		t.Fatal("final text change must change text fingerprint")
	}
	if first[1].Signature != changed[1].Signature || first[2].Signature != changed[2].Signature {
		t.Fatal("media fingerprints must remain independent from final text")
	}
}

func TestBuildProfileFingerprintsIgnoresInvisibleSuffix(t *testing.T) {
	plain := buildProfileFingerprints([]int64{9}, "介绍费 7888", nil)
	obfuscated := buildProfileFingerprints([]int64{9}, "介绍费 7888\u2063\u200b\ufe0f", nil)
	if len(plain) != 1 || len(obfuscated) != 1 || plain[0] != obfuscated[0] {
		t.Fatalf("invisible suffix changed final text fingerprint: %#v != %#v", plain, obfuscated)
	}
}

func TestBuildProfileFingerprintsIsChannelScopedAndOrderStable(t *testing.T) {
	left := []collectMediaItem{
		{Type: "photo", FileMd5: "bb", FilePhash: "bbbbbbbbbbbbbbbb"},
		{Type: "photo", FileMd5: "aa", FilePhash: "aaaaaaaaaaaaaaaa"},
	}
	right := []collectMediaItem{left[1], left[0]}
	first := buildProfileFingerprints([]int64{2, 1, 2}, "正文", left)
	second := buildProfileFingerprints([]int64{1, 2}, "正文", right)
	if len(first) != 6 || len(second) != 6 {
		t.Fatalf("unexpected channel fingerprints: first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("media order changed fingerprint at %d: %#v != %#v", i, first[i], second[i])
		}
	}
}
