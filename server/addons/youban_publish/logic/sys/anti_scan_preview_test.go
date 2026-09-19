package sys

import (
	"context"
	"strings"
	"testing"
	"time"

	"hotgo/addons/youban_publish/model"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
)

func TestAntiScanMattingQuotaReferenceIsStableAcrossJobs(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	first := antiScanMattingQuotaReference(7, "tencent", "image-hash", now)
	second := antiScanMattingQuotaReference(7, "tencent", "image-hash", now.Add(time.Hour))
	if first != second {
		t.Fatalf("same monthly matting result must share quota reference: %q != %q", first, second)
	}
	if first == antiScanMattingQuotaReference(7, "aliyun", "image-hash", now) {
		t.Fatal("different providers must not share quota reference")
	}
}

func TestAntiScanSegmentUploadContextProvidesWorkerModule(t *testing.T) {
	ctx := antiScanSegmentUploadContext(context.Background())
	if got := contexts.GetModule(ctx); got != consts.AppAdmin {
		t.Fatalf("unexpected module: got %q want %q", got, consts.AppAdmin)
	}
	if got := contexts.GetAddonName(ctx); got != "youban_publish" {
		t.Fatalf("unexpected addon: got %q", got)
	}
}

func TestFacePPDistributedSlotIsStableAndBounded(t *testing.T) {
	const concurrency = 2
	first := facePPDistributedSlot("image-hash", concurrency)
	if second := facePPDistributedSlot("image-hash", concurrency); second != first {
		t.Fatalf("slot must be stable: %q != %q", first, second)
	}
	if !strings.HasSuffix(first, ":0") && !strings.HasSuffix(first, ":1") {
		t.Fatalf("slot must stay within concurrency: %q", first)
	}
}

func TestAntiScanMattingPublicErrorDoesNotExposeProvider(t *testing.T) {
	errMessage := antiScanMattingPublicError().Error()
	if errMessage != antiScanMattingErrorMessage {
		t.Fatalf("unexpected public error: %s", errMessage)
	}
	for _, sensitiveText := range []string{"FAPIHub", "fapihub", "HTTP", "quota_exceeded"} {
		if strings.Contains(errMessage, sensitiveText) {
			t.Fatalf("public error exposes sensitive text %q: %s", sensitiveText, errMessage)
		}
	}
}

func TestAntiScanMattingCacheMatchesSelectedProvider(t *testing.T) {
	tencentCache := &antiScanDetectResult{Provider: "tencent-ci-matting"}
	fapiHubCache := &antiScanDetectResult{Provider: "fapihub-matting"}
	aliyunCache := &antiScanDetectResult{Provider: "aliyun-matting"}
	if !antiScanMattingCacheMatches(aliyunCache, antiScanMattingProvider(&model.CloudResourceConfig{MattingProvider: "aliyun"})) {
		t.Fatal("Aliyun cache should match Aliyun provider")
	}
	if !antiScanMattingCacheMatches(tencentCache, antiScanMattingProvider(&model.CloudResourceConfig{MattingProvider: "tencent"})) {
		t.Fatal("Tencent cache should match Tencent provider")
	}
	if antiScanMattingCacheMatches(fapiHubCache, "tencent") {
		t.Fatal("FAPIHub cache must not match Tencent provider")
	}
	if antiScanMattingCacheMatches(tencentCache, "fapihub") {
		t.Fatal("Tencent cache must not match FAPIHub provider")
	}
	if antiScanMattingCacheMatches(fapiHubCache, "aliyun") {
		t.Fatal("FAPIHub cache must not match Aliyun provider")
	}
}

func TestAntiScanTencentMattingFallbackRequiresCompleteConfig(t *testing.T) {
	complete := &model.CloudResourceConfig{TencentSecretId: "id", TencentSecretKey: "key", TencentMattingBucket: "bucket"}
	if !antiScanTencentMattingFallbackReady(complete) {
		t.Fatal("complete Tencent matting configuration should enable fallback")
	}
	for _, conf := range []*model.CloudResourceConfig{
		nil,
		{TencentSecretKey: "key", TencentMattingBucket: "bucket"},
		{TencentSecretId: "id", TencentMattingBucket: "bucket"},
		{TencentSecretId: "id", TencentSecretKey: "key"},
	} {
		if antiScanTencentMattingFallbackReady(conf) {
			t.Fatalf("incomplete Tencent matting configuration must not enable fallback: %#v", conf)
		}
	}
}
