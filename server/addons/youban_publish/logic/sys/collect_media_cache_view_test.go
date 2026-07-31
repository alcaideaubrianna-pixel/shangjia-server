package sys

import "testing"

func TestCollectEventMediaCacheViewSeparatesPendingAndDownloading(t *testing.T) {
	tests := []struct {
		name        string
		summary     collectEventMediaCacheSummary
		wantStatus  string
		wantMessage string
	}{
		{name: "pending", summary: collectEventMediaCacheSummary{Total: 2, Pending: 2}, wantStatus: "caching", wantMessage: "2 个媒体待缓存"},
		{name: "downloading", summary: collectEventMediaCacheSummary{Total: 2, Downloading: 2}, wantStatus: "caching", wantMessage: "2 个媒体下载中"},
		{name: "ready", summary: collectEventMediaCacheSummary{Total: 2, Ready: 2}, wantStatus: "cached", wantMessage: "2 个媒体已缓存"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, message := collectEventMediaCacheView(test.summary, "", "")
			if status != test.wantStatus || message != test.wantMessage {
				t.Fatalf("status=%q message=%q, want status=%q message=%q", status, message, test.wantStatus, test.wantMessage)
			}
		})
	}
}
