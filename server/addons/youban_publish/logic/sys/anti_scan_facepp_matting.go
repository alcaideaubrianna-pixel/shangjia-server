package sys

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model"
)

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
	startedAt := time.Now()
	defer func() {
		g.Log().Warningf(ctx, "防扫图 Face++ 阶段完成 imageHash:%s totalDurationMs:%d", imageHash, time.Since(startedAt).Milliseconds())
	}()
	if conf == nil || strings.TrimSpace(conf.FacePlusApiKey) == "" || strings.TrimSpace(conf.FacePlusApiSecret) == "" {
		return "", gerror.New("Face++ 缺少 API Key 或 API Secret")
	}
	n, _, _, err := prepareCloudMattingImageBytes(imageBytes, 1600)
	if err != nil {
		return "", err
	}
	g.Log().Warningf(ctx, "防扫图 Face++ 图片准备完成 imageHash:%s inputBytes:%d requestBytes:%d durationMs:%d", imageHash, len(imageBytes), len(n), time.Since(startedAt).Milliseconds())
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
	requestStartedAt := time.Now()
	resp, err := facePPHTTPClient.Do(req)
	g.Log().Warningf(ctx, "防扫图 Face++ HTTP 请求完成 imageHash:%s durationMs:%d success:%t", imageHash, time.Since(requestStartedAt).Milliseconds(), err == nil)
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
	g.Log().Warningf(ctx, "防扫图 Face++ 响应解码完成 imageHash:%s outputBytes:%d durationMs:%d", imageHash, len(decoded), time.Since(startedAt).Milliseconds())
	uploadStartedAt := time.Now()
	url, err := uploadAntiScanSegment(ctx, decoded, imageHash)
	g.Log().Warningf(ctx, "防扫图 Face++ 结果上传完成 imageHash:%s durationMs:%d success:%t", imageHash, time.Since(uploadStartedAt).Milliseconds(), err == nil)
	return url, err
}

func decodeBase64Image(v string) ([]byte, error) { return base64.StdEncoding.DecodeString(v) }
