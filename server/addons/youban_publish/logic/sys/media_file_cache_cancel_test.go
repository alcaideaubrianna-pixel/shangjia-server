package sys

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMediaCacheWaitRespectsCancellation(t *testing.T) {
	key := "cancel-wait-" + t.Name()
	started := make(chan struct{})
	release := make(chan struct{})
	leader := mediaFileCacheDownloadGroup.DoChan(key, func() (interface{}, error) {
		close(started)
		<-release
		return "", nil
	})
	<-started
	defer func() { close(release); <-leader }()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	begin := time.Now()
	_, err := cachedRemoteMediaFileWithMetaSourceAndDownloader(ctx, key, "https://example.invalid/thumbnail", "", ".jpg", func(context.Context, string, string) error {
		t.Error("duplicate downloader must not run")
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	if time.Since(begin) > time.Second {
		t.Fatal("cache wait ignored deadline")
	}
}
