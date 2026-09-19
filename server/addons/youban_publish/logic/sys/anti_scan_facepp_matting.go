package sys

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_publish/model"
	lock "hotgo/internal/library/hgrds/lock"
)

var facePPConcurrency = struct {
	sync.Mutex
	sem chan struct{}
}{}

var facePPHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 9 * time.Second,
	},
}

func facePPPortraitMatting(ctx context.Context, imageBytes []byte, imageHash string, conf *model.CloudResourceConfig) (string, error) {
	if conf == nil || strings.TrimSpace(conf.FacePlusApiKey) == "" || strings.TrimSpace(conf.FacePlusApiSecret) == "" {
		return "", gerror.New("Face++ 缺少 API Key 或 API Secret")
	}
	n, _, _, err := prepareCloudMattingImageBytes(imageBytes, 1600)
	if err != nil {
		return "", err
	}
	concurrency := conf.FacePlusConcurrency
	if concurrency <= 0 {
		concurrency = 2
	}
	distributedLock := lock.NewConfig(45*time.Second, 100*time.Millisecond).Mutex(facePPDistributedSlot(imageHash, concurrency))
	if err = distributedLock.Lock(ctx); err != nil {
		return "", gerror.Wrap(err, "等待 Face++ 并发槽位失败")
	}
	defer func() { _ = distributedLock.Unlock(context.Background()) }()
	facePPConcurrency.Lock()
	if facePPConcurrency.sem == nil || cap(facePPConcurrency.sem) != conf.FacePlusConcurrency {
		c := conf.FacePlusConcurrency
		if c <= 0 {
			c = 2
		}
		facePPConcurrency.sem = make(chan struct{}, c)
	}
	sem := facePPConcurrency.sem
	facePPConcurrency.Unlock()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("api_key", conf.FacePlusApiKey)
	_ = mw.WriteField("api_secret", conf.FacePlusApiSecret)
	_ = mw.WriteField("return_grayscale", "0")
	part, err := mw.CreateFormFile("image_file", "image.jpg")
	if err != nil {
		return "", err
	}
	if _, err = part.Write(n); err != nil {
		return "", err
	}
	if err = mw.Close(); err != nil {
		return "", err
	}
	endpoint := strings.TrimSpace(conf.FacePlusEndpoint)
	if endpoint == "" {
		endpoint = "https://api-cn.faceplusplus.com/humanbodypp/v2/segment"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := facePPHTTPClient.Do(req)
	if err != nil {
		return "", gerror.Wrap(err, "调用 Face++ 人体抠图失败")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", err
	}
	var out struct {
		BodyImage string `json:"body_image"`
		Error     string `json:"error_message"`
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return "", gerror.Wrap(err, "解析 Face++ 响应失败")
	}
	if resp.StatusCode != http.StatusOK || out.Error != "" {
		return "", gerror.Newf("Face++ 人体抠图失败（%d）：%s", resp.StatusCode, out.Error)
	}
	if out.BodyImage == "" {
		return "", gerror.New("Face++ 未返回人像图片")
	}
	decoded, err := decodeBase64Image(out.BodyImage)
	if err != nil {
		return "", gerror.Wrap(err, "解码 Face++ 人像图片失败")
	}
	return uploadAntiScanSegment(ctx, decoded, imageHash)
}

func facePPDistributedSlot(imageHash string, concurrency int) string {
	if concurrency <= 0 {
		concurrency = 1
	}
	slot := crc32.ChecksumIEEE([]byte(imageHash)) % uint32(concurrency)
	return fmt.Sprintf("youban_publish:anti_scan:facepp:slot:%d", slot)
}

func decodeBase64Image(v string) ([]byte, error) { return base64.StdEncoding.DecodeString(v) }
