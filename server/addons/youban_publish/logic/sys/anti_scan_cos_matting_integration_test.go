package sys

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"

	addonmodel "hotgo/addons/youban_publish/model"
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
		CosSecretId:  secretID,
		CosSecretKey: secretKey,
		CosBucketURL: bucketURL,
		CosPublicURL: bucketURL,
		CosPath:      "hotgo/file/",
	})
	client := newTencentCITestClient(t, bucketURL, secretID, secretKey)
	sourceKey := "hotgo/file/anti-scan/source/integration-" + imageHash[:16] + ".png"
	resultKey := "hotgo/file/anti-scan/segment/integration-" + imageHash[:16] + ".png"
	defer deleteTencentCITestObjects(t, client, sourceKey, resultKey)
	t.Run("basic-image-processing", func(t *testing.T) {
		tencentCIBasicImageProcessing(t, bucketURL, secretID, secretKey, imageBytes, imageHash)
	})

	startedAt := time.Now()
	parsedBucketURL, err := url.Parse(bucketURL)
	if err != nil {
		t.Fatal(err)
	}
	hostParts := strings.Split(parsedBucketURL.Hostname(), ".")
	if len(hostParts) < 4 {
		t.Fatalf("invalid TENCENT_CI_BUCKET_URL: %s", bucketURL)
	}
	result, err := tencentCOSPortraitMatting(context.Background(), imageBytes, "integration-"+imageHash[:16], &addonmodel.CloudResourceConfig{
		TencentSecretId:      secretID,
		TencentSecretKey:     secretKey,
		TencentMattingBucket: hostParts[0],
		TencentRegion:        hostParts[2],
		TencentMattingPath:   "youban-matting/integration",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.URL == "" {
		t.Fatal("empty portrait matting result URL")
	}
	t.Logf("AIPortraitMatting completed durationMs=%d outputBytes=%d", time.Since(startedAt).Milliseconds(), result.OutputBytes)
}

func tencentCIBasicImageProcessing(t *testing.T, bucketURL, secretID, secretKey string, imageBytes []byte, imageHash string) {
	t.Helper()
	normalized, contentType, ext, err := prepareCloudMattingImageBytes(imageBytes, 1600)
	if err != nil {
		t.Fatal(err)
	}
	client := newTencentCITestClient(t, bucketURL, secretID, secretKey)
	sourceKey := "hotgo/file/anti-scan/diagnostic/" + imageHash[:16] + "." + ext
	resultKey := "hotgo/file/anti-scan/diagnostic/" + imageHash[:16] + ".jpg"
	defer deleteTencentCITestObjects(t, client, sourceKey, resultKey)
	headers := http.Header{}
	headers.Set("Pic-Operations", cos.EncodePicOperations(&cos.PicOperations{
		Rules: []cos.PicOperationsRules{{
			FileId: resultKey,
			Rule:   "imageMogr2/format/jpg",
		}},
	}))
	startedAt := time.Now()
	_, _, err = client.CI.Put(context.Background(), sourceKey, bytes.NewReader(normalized), &cos.ObjectPutOptions{
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

func newTencentCITestClient(t *testing.T, bucketURL, secretID, secretKey string) *cos.Client {
	t.Helper()
	parsedBucketURL, err := url.Parse(bucketURL)
	if err != nil {
		t.Fatal(err)
	}
	return cos.NewClient(&cos.BaseURL{BucketURL: parsedBucketURL}, &http.Client{
		Timeout:   8 * time.Second,
		Transport: &cos.AuthorizationTransport{SecretID: secretID, SecretKey: secretKey},
	})
}

func deleteTencentCITestObjects(t *testing.T, client *cos.Client, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, err := client.Object.Delete(context.Background(), key); err != nil {
			t.Logf("cleanup object failed key=%s err=%v", key, err)
		}
	}
}
