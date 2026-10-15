package storager

import "testing"

func TestGenContentPath(t *testing.T) {
	const digest = "A9B71234567890123456789012345678"
	got, err := GenContentPath("hotgo/file/", digest, ".JPG")
	if err != nil {
		t.Fatalf("GenContentPath returned error: %v", err)
	}
	want := "hotgo/file/a9/a9b71234567890123456789012345678.jpg"
	if got != want {
		t.Fatalf("GenContentPath = %q, want %q", got, want)
	}
}

func TestGenContentPathRejectsInvalidDigest(t *testing.T) {
	for _, digest := range []string{"", "abc", "zzb71234567890123456789012345678", "a9b712345678901234567890123456789"} {
		if _, err := GenContentPath("hotgo/file", digest, "jpg"); err == nil {
			t.Fatalf("GenContentPath accepted invalid digest %q", digest)
		}
	}
}

func TestGenStagingPath(t *testing.T) {
	got, err := GenStagingPath("hotgo/file/", "SessionABC", ".MP4")
	if err != nil {
		t.Fatalf("GenStagingPath returned error: %v", err)
	}
	if want := "hotgo/file/staging/sessionabc.mp4"; got != want {
		t.Fatalf("GenStagingPath = %q, want %q", got, want)
	}
}
