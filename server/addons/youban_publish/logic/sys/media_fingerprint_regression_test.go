package sys

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gcache"
	"hotgo/internal/library/cache"
)

func TestTelegramFingerprintRebuildsMD5OnlyCache(t *testing.T) {
	ctx := context.Background()
	if !cache.Initialized() {
		cache.SetAdapter(ctx)
	}
	previousAdapter := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	defer cache.Instance().SetAdapter(previousAdapter)
	id := "regression-md5-only-" + t.Name()
	key := botMediaFingerprintCacheKey(id)
	defer cache.Instance().Remove(ctx, key)
	if err := cache.Instance().Set(ctx, key, cachedBotMediaFingerprint{MD5: "old-md5"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, ok := telegramImageFingerprintFromCache(ctx, id); ok {
		t.Fatal("MD5-only cache must trigger fingerprint rebuild")
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), uint8(x * y), 255})
		}
	}
	var content bytes.Buffer
	if err := png.Encode(&content, img); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(content.Bytes()) }))
	defer server.Close()
	fingerprint, hit, err := cachedTelegramImageFingerprint(ctx, id, server.URL+"/image.png")
	if err != nil {
		t.Fatal(err)
	}
	if hit || fingerprint == nil || fingerprint.PHash == nil || fingerprint.MD5 != md5Hex(content.Bytes()) {
		t.Fatalf("incomplete rebuilt fingerprint: %+v hit=%v", fingerprint, hit)
	}
	again, hit, err := cachedTelegramImageFingerprint(ctx, id, "http://127.0.0.1:1/must-not-download")
	if err != nil || !hit || again.PHash == nil || again.PHash.GetHash() != fingerprint.PHash.GetHash() {
		t.Fatalf("complete cache not reused: %+v hit=%v err=%v", again, hit, err)
	}
}
