package sys

import "testing"

func TestJoinMediaStorageURL(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		storagePath string
		want        string
	}{
		{name: "cos relative path", baseURL: "https://cdn.example.com/", storagePath: "hotgo/file/video.mov", want: "https://cdn.example.com/hotgo/file/video.mov"},
		{name: "site relative path", storagePath: "hotgo/file/video.mov", want: "/hotgo/file/video.mov"},
		{name: "leading slash", baseURL: "https://cdn.example.com", storagePath: "/hotgo/file/video.mov", want: "https://cdn.example.com/hotgo/file/video.mov"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := joinMediaStorageURL(test.baseURL, test.storagePath); got != test.want {
				t.Fatalf("joinMediaStorageURL() = %q, want %q", got, test.want)
			}
		})
	}
}
