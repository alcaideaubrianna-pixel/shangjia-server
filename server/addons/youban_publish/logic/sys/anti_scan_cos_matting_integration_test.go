package sys

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"

	"hotgo/internal/consts"
	"hotgo/internal/library/storager"
	internalmodel "hotgo/internal/model"
)

func TestTencentCOSPortraitMattingIntegration(t *testing.T) {
	secretID := os.Getenv("TENCENT_CI_SECRET_ID")
	secretKey := os.Getenv("TENCENT_CI_SECRET_KEY")
	bucketURL := os.Getenv("TENCENT_CI_BUCKET_URL")
	imagePath := os.Getenv("TENCENT_CI_TEST_IMAGE")
	if secretID == "" || secretKey == "" || bucketURL == "" || imagePath == "" {
		t.Skip("set TENCENT_CI_SECRET_ID, TENCENT_CI_SECRET_KEY, TENCENT_CI_BUCKET_URL and TENCENT_CI_TEST_IMAGE")
	}
	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	imageHash, err := antiScanImageHash(imageBytes)
	if err != nil {
		t.Fatal(err)
	}
	storager.SetConfig(&internalmodel.UploadConfig{
		Drive:        consts.UploadDriveCos,
		CosBucketURL: bucketURL,
		CosPublicURL: bucketURL,
		CosPath:      "hotgo/file/",
	})
	t.Run("basic-image-processing", func(t *testing.T) {
		tencentCIBasicImageProcessing(t, bucketURL, secretID, secretKey, imageBytes, imageHash)
	})

	startedAt := time.Now()
	resultURL, err := tencentCOSPortraitMatting(context.Background(), imageBytes, "integration-"+imageHash[:16], secretID, secretKey)
	if err != nil {
		t.Fatal(err)
	}
	if resultURL == "" {
		t.Fatal("empty portrait matting result URL")
	}
	t.Logf("AIPortraitMatting completed durationMs=%d", time.Since(startedAt).Milliseconds())
}

func tencentCIBasicImageProcessing(t *testing.T, bucketURL, secretID, secretKey string, imageBytes []byte, imageHash string) {
	t.Helper()
	normalized, contentType, ext, err := prepareCloudMattingImageBytes(imageBytes, 1600)
	if err != nil {
		t.Fatal(err)
	}
	parsedBucketURL, err := url.Parse(bucketURL)
	if err != nil {
		t.Fatal(err)
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: parsedBucketURL}, &http.Client{
		Timeout: 8 * time.Second,
		Transport: &cos.AuthorizationTransport{
			SecretID:  secretID,
			SecretKey: secretKey,
		},
	})
	headers := http.Header{}
	headers.Set("Pic-Operations", cos.EncodePicOperations(&cos.PicOperations{
		Rules: []cos.PicOperationsRules{{
			FileId: "hotgo/file/anti-scan/diagnostic/" + imageHash[:16] + ".jpg",
			Rule:   "imageMogr2/format/jpg",
		}},
	}))
	startedAt := time.Now()
	_, _, err = client.CI.Put(context.Background(), "hotgo/file/anti-scan/diagnostic/"+imageHash[:16]+"."+ext, bytes.NewReader(normalized), &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType:   contentType,
			XOptionHeader: &headers,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("basic image processing completed durationMs=%d", time.Since(startedAt).Milliseconds())
}
