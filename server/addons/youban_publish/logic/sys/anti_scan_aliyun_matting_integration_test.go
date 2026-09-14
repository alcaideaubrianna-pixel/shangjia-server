package sys

import (
	"context"
	"os"
	"testing"

	"hotgo/addons/youban_publish/model"
)

func TestAliyunSegmentBodyIntegration(t *testing.T) {
	accessKeyID := os.Getenv("ALIYUN_IMAGESEG_ACCESS_KEY_ID")
	accessKeySecret := os.Getenv("ALIYUN_IMAGESEG_ACCESS_KEY_SECRET")
	if accessKeyID == "" || accessKeySecret == "" {
		t.Skip("set ALIYUN_IMAGESEG_ACCESS_KEY_ID and ALIYUN_IMAGESEG_ACCESS_KEY_SECRET")
	}
	result, err := aliyunSegmentBodyFromURL(context.Background(), aliyunSegmentBodyTestImageURL, &model.CloudResourceConfig{
		AliyunAccessKeyId:     accessKeyID,
		AliyunAccessKeySecret: accessKeySecret,
		AliyunEndpoint:        "imageseg.cn-shanghai.aliyuncs.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ImageBytes) == 0 || result.RequestID == "" {
		t.Fatalf("invalid result: bytes=%d requestId=%q", len(result.ImageBytes), result.RequestID)
	}
	t.Logf("SegmentBody apiDurationMs=%d downloadDurationMs=%d totalDurationMs=%d outputBytes=%d requestId=%s", result.ApiDuration.Milliseconds(), result.DownloadDuration.Milliseconds(), result.TotalDuration.Milliseconds(), len(result.ImageBytes), result.RequestID)
}
