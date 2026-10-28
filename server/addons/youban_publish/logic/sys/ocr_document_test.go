package sys

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gcache"

	"hotgo/addons/youban_publish/model"
	"hotgo/internal/library/cache"
)

func TestDecodeTelegramOCRResponse(t *testing.T) {
	var result telegramOCRResult
	if err := decodeTelegramOCRResponse([]byte(`{"result":[{"text":"资料编号 W28631"},{"rec_text":"姓名"}]}`), &result); err != nil {
		t.Fatal(err)
	}
	if !result.HasText || result.TextCount != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !telegramOCRKeywordHit(result.Text, "W28631\n身份证") {
		t.Fatalf("keyword not detected: %q", result.Text)
	}
}

func TestDecodeTelegramOCRResponseCompact(t *testing.T) {
	var result telegramOCRResult
	if err := decodeTelegramOCRResponse([]byte(`{"hasText":true,"text":"hello","textCount":1,"confidence":0.91}`), &result); err != nil {
		t.Fatal(err)
	}
	if !result.HasText || result.TextCount != 1 || result.Confidence < 0.9 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestTelegramOCRAvailabilityAndCache(t *testing.T) {
	cache.SetAdapter(context.Background())
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	defer cache.Instance().SetAdapter(previous)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"text":"普通文本"},{"text":"身份证号码"}]}`))
	}))
	defer server.Close()
	conf := &model.CloudResourceConfig{OcrEnabled: 1, OcrEndpoint: server.URL, OcrTimeoutMs: 800, OcrBlockedKeywords: "联系方式\n身份证"}
	ctx := context.Background()
	started := time.Now()
	first, err := classifyTelegramDocumentByOCR(ctx, []byte("same-image"), conf)
	firstDuration := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	second, err := classifyTelegramDocumentByOCR(ctx, []byte("same-image"), conf)
	secondDuration := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || !second.Cached {
		t.Fatalf("OCR cache not used requests=%d second=%+v", requests, second)
	}
	if !first.KeywordHit || !second.KeywordHit {
		t.Fatalf("any keyword should block replacement: first=%+v second=%+v", first, second)
	}
	t.Logf("mock OCR request=%s, Redis-cache lookup=%s", firstDuration, secondDuration)
	if secondDuration >= firstDuration {
		t.Fatalf("cache should be faster first=%s second=%s", firstDuration, secondDuration)
	}
}
