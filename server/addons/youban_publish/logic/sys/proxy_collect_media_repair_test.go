package sys

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseProxyMediaRepairProfileID(t *testing.T) {
	tests := []struct {
		message string
		want    int64
	}{
		{"资料库已存在相同资料 profileId:649624 channelId:106 layer:text_hash", 649624},
		{"资料已存在 profileId:1", 1},
		{"资料库已存在相同资料", 0},
		{"profileId:not-a-number", 0},
	}
	for _, test := range tests {
		if got := parseProxyMediaRepairProfileID(test.message); got != test.want {
			t.Fatalf("parseProxyMediaRepairProfileID(%q)=%d want=%d", test.message, got, test.want)
		}
	}
}

func TestProxyMediaURLMissingOnlyAcceptsDefinitiveMissingStatus(t *testing.T) {
	for _, test := range []struct {
		status int
		want   bool
	}{{http.StatusOK, false}, {http.StatusForbidden, false}, {http.StatusInternalServerError, false}, {http.StatusNotFound, true}, {http.StatusGone, true}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(test.status) }))
		got, err := proxyMediaURLMissing(context.Background(), server.URL)
		server.Close()
		if err != nil {
			t.Fatalf("status %d: %v", test.status, err)
		}
		if got != test.want {
			t.Fatalf("status %d missing=%t want=%t", test.status, got, test.want)
		}
	}
}

func TestCollectMediaRequiresStorageRefresh(t *testing.T) {
	if !collectMediaRequiresStorageRefresh("持久化媒体返回404，等待从Telegram源消息重新下载") {
		t.Fatal("repair marker must force current storage upload")
	}
	if collectMediaRequiresStorageRefresh("下载远程媒体失败：HTTP 404") {
		t.Fatal("ordinary send failure must not bypass collection cache")
	}
}
