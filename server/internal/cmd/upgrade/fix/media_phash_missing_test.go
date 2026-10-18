package fix

import "testing"

func TestDownloadableMediaSource(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "content url", raw: "https://img.example.com/telegram/content/prod/a.jpg", want: "https://img.example.com/telegram/content/prod/a.jpg"},
		{name: "resource url", raw: "https://img.example.com/telegram/resource/123?kind=origin", want: ""},
		{name: "local path", raw: "vf_admin/upload_path/a.jpg", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := downloadableMediaSource(test.raw); got != test.want {
				t.Fatalf("downloadableMediaSource(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestMissingMediaPHashVideoUsesOnlyPreview(t *testing.T) {
	row := mediaPHashMissingRow{MediaType: "video", StoragePath: "large.mp4", FileURL: "https://example.test/large.mp4", PosterStoragePath: "preview.jpg", PosterURL: "https://example.test/preview.jpg"}
	candidates := missingMediaPHashCandidates(row)
	if len(candidates) != 2 {
		t.Fatalf("expected two preview sources, got %+v", candidates)
	}
	for _, item := range candidates {
		if item.mediaType != "image" || item.localPath == row.StoragePath || item.fileURL == row.FileURL {
			t.Fatalf("video body candidate: %+v", item)
		}
	}
	row.PosterStoragePath = ""
	row.PosterURL = ""
	if candidates = missingMediaPHashCandidates(row); len(candidates) != 0 {
		t.Fatalf("must not download video without preview: %+v", candidates)
	}
}
