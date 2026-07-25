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
