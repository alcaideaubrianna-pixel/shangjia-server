package sys

import (
	"strings"
	"testing"

	"hotgo/addons/youban_publish/model"
)

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
