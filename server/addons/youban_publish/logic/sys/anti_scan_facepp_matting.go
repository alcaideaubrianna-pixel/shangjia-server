package sys

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model"
)

const (
	// Face++ documents a decimal 2 MB request limit. Keep multipart overhead headroom.
	facePPMaximumImageBytes = 2_000_000 - 16*1024
	facePPMaximumDimension  = 4096
)

var errFacePPPermanent = errors.New("Face++ permanent request error")

type facePPAPIError struct {
	StatusCode int
	Message    string
}

func (e *facePPAPIError) Error() string {
	return fmt.Sprintf("Face++ 人体抠图失败（%d）：%s", e.StatusCode, e.Message)
}

func (e *facePPAPIError) Unwrap() error {
	if isFacePPPermanentResponse(e.StatusCode, e.Message) {
		return errFacePPPermanent
	}
	return nil
}

var facePPHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   64,
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
	n, contentType, filename, err := prepareFacePPMattingImageBytes(imageBytes)
	if err != nil {
		return "", err
	}
	g.Log().Warningf(ctx, "防扫图 Face++ 图片准备完成 imageHash:%s inputBytes:%d requestBytes:%d durationMs:%d", imageHash, len(imageBytes), len(n), time.Since(startedAt).Milliseconds())
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("api_key", conf.FacePlusApiKey)
	_ = mw.WriteField("api_secret", conf.FacePlusApiSecret)
	_ = mw.WriteField("return_grayscale", "0")
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image_file"; filename="%s"`, filename))
	partHeader.Set("Content-Type", contentType)
	part, err := mw.CreatePart(partHeader)
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
	readStartedAt := time.Now()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", err
	}
	g.Log().Warningf(ctx, "防扫图 Face++ 响应读取完成 imageHash:%s status:%d responseBytes:%d durationMs:%d", imageHash, resp.StatusCode, len(raw), time.Since(readStartedAt).Milliseconds())
	jsonStartedAt := time.Now()
	var out struct {
		BodyImage string `json:"body_image"`
		Error     string `json:"error_message"`
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return "", gerror.Wrap(err, "解析 Face++ 响应失败")
	}
	g.Log().Warningf(ctx, "防扫图 Face++ JSON 解析完成 imageHash:%s durationMs:%d", imageHash, time.Since(jsonStartedAt).Milliseconds())
	if resp.StatusCode != http.StatusOK || out.Error != "" {
		return "", &facePPAPIError{StatusCode: resp.StatusCode, Message: out.Error}
	}
	if out.BodyImage == "" {
		return "", gerror.New("Face++ 未返回人像图片")
	}
	decodeStartedAt := time.Now()
	decoded, err := decodeBase64Image(out.BodyImage)
	if err != nil {
		return "", gerror.Wrap(err, "解码 Face++ 人像图片失败")
	}
	g.Log().Warningf(ctx, "防扫图 Face++ Base64 解码完成 imageHash:%s outputBytes:%d durationMs:%d", imageHash, len(decoded), time.Since(decodeStartedAt).Milliseconds())
	uploadStartedAt := time.Now()
	url, err := uploadAntiScanSegment(ctx, decoded, imageHash)
	g.Log().Warningf(ctx, "防扫图 Face++ 结果上传完成 imageHash:%s durationMs:%d success:%t", imageHash, time.Since(uploadStartedAt).Milliseconds(), err == nil)
	return url, err
}

func decodeBase64Image(v string) ([]byte, error) { return base64.StdEncoding.DecodeString(v) }

func prepareFacePPMattingImageBytes(imageBytes []byte) ([]byte, string, string, error) {
	img, format, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, "", "", gerror.Wrap(err, "Face++ 图片格式无法解析")
	}
	format = strings.ToLower(format)
	bounds := img.Bounds()
	if len(imageBytes) <= facePPMaximumImageBytes && bounds.Dx() <= facePPMaximumDimension && bounds.Dy() <= facePPMaximumDimension {
		switch format {
		case "jpeg":
			return imageBytes, "image/jpeg", "image.jpg", nil
		case "png":
			return imageBytes, "image/png", "image.png", nil
		}
	}
	if bounds.Dx() > facePPMaximumDimension || bounds.Dy() > facePPMaximumDimension {
		img = resizeAntiScanPreviewImage(img, facePPMaximumDimension)
	}
	if format == "png" {
		var lossless bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err = encoder.Encode(&lossless, img); err == nil && lossless.Len() <= facePPMaximumImageBytes {
			return lossless.Bytes(), "image/png", "image.png", nil
		}
	}
	for _, quality := range []int{95, 92, 88, 84, 80, 76} {
		var output bytes.Buffer
		if err = jpeg.Encode(&output, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, "", "", gerror.Wrap(err, "压缩 Face++ 图片失败")
		}
		if output.Len() <= facePPMaximumImageBytes {
			return output.Bytes(), "image/jpeg", "image.jpg", nil
		}
	}
	for maxDimension := max(bounds.Dx(), bounds.Dy()) * 9 / 10; maxDimension >= 640; maxDimension = maxDimension * 9 / 10 {
		resized := resizeAntiScanPreviewImage(img, maxDimension)
		var output bytes.Buffer
		if err = jpeg.Encode(&output, resized, &jpeg.Options{Quality: 88}); err != nil {
			return nil, "", "", gerror.Wrap(err, "缩放 Face++ 图片失败")
		}
		if output.Len() <= facePPMaximumImageBytes {
			return output.Bytes(), "image/jpeg", "image.jpg", nil
		}
	}
	return nil, "", "", gerror.New("图片压缩后仍超过 Face++ 2MB 限制")
}

func isFacePPPermanentResponse(statusCode int, message string) bool {
	if statusCode == http.StatusRequestTimeout || statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError {
		return false
	}
	if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError {
		return true
	}
	message = strings.ToUpper(strings.TrimSpace(message))
	for _, code := range []string{"IMAGE_ERROR_UNSUPPORTED_FORMAT", "INVALID_IMAGE_SIZE", "INVALID_IMAGE_URL", "IMAGE_FILE_TOO_LARGE", "MISSING_ARGUMENTS", "BAD_ARGUMENTS", "COEXISTENCE_ARGUMENTS", "AUTHENTICATION_ERROR", "AUTHORIZATION_ERROR"} {
		if strings.Contains(message, code) {
			return true
		}
	}
	return false
}
