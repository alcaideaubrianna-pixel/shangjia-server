package sys

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type fapiHubClient struct {
	apiKey   string
	endpoint string
	model    string
}

var fapiHubHTTPClient = &http.Client{
	Timeout: 12 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
	},
}

func newFapiHubClient(apiKey string, endpoint string, model string) *fapiHubClient {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = "https://fapihub.com/v2/rembg/"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "falcon"
	}
	return &fapiHubClient{
		apiKey:   strings.TrimSpace(apiKey),
		endpoint: endpoint,
		model:    model,
	}
}

// removeBackground 调用 FAPIHub 抠图接口，返回透明背景 PNG。
func (c *fapiHubClient) removeBackground(ctx context.Context, imageBytes []byte) ([]byte, error) {
	totalStartedAt := time.Now()
	if c.apiKey == "" {
		return nil, gerror.New("FAPIHub API Key 未配置")
	}
	imageBytes, contentType, ext, err := prepareFapiHubImageBytes(imageBytes, 1600)
	if err != nil {
		return nil, err
	}
	prepareDuration := time.Since(totalStartedAt)
	formStartedAt := time.Now()
	body := bytes.NewBuffer(nil)
	writer := multipart.NewWriter(body)
	part, err := createFapiHubImagePart(writer, contentType, ext)
	if err != nil {
		return nil, gerror.Wrap(err, "创建 FAPIHub 图片表单失败")
	}
	if _, err = part.Write(imageBytes); err != nil {
		return nil, gerror.Wrap(err, "写入 FAPIHub 图片表单失败")
	}
	if c.model != "" {
		if err = writer.WriteField("model", c.model); err != nil {
			return nil, gerror.Wrap(err, "写入 FAPIHub 模型参数失败")
		}
	}
	if err = writer.Close(); err != nil {
		return nil, gerror.Wrap(err, "关闭 FAPIHub 表单失败")
	}
	formDuration := time.Since(formStartedAt)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, body)
	if err != nil {
		return nil, gerror.Wrap(err, "创建 FAPIHub 请求失败")
	}
	req.Header.Set("ApiKey", c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	requestStartedAt := time.Now()
	resp, err := fapiHubHTTPClient.Do(req)
	if err != nil {
		return nil, gerror.Wrap(err, "请求 FAPIHub 抠图接口失败")
	}
	responseDuration := time.Since(requestStartedAt)
	defer resp.Body.Close()
	readStartedAt := time.Now()
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 30<<20))
	if err != nil {
		return nil, gerror.Wrap(err, "读取 FAPIHub 响应失败")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, gerror.Newf("FAPIHub 抠图接口 HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	if len(respBytes) == 0 {
		return nil, gerror.New("FAPIHub 抠图接口返回空图片")
	}
	g.Log().Infof(ctx, "防扫图阶段完成 stage:fapihub prepareMs:%d formMs:%d responseMs:%d readMs:%d inputBytes:%d outputBytes:%d totalMs:%d", prepareDuration.Milliseconds(), formDuration.Milliseconds(), responseDuration.Milliseconds(), time.Since(readStartedAt).Milliseconds(), len(imageBytes), len(respBytes), time.Since(totalStartedAt).Milliseconds())
	return respBytes, nil
}

func prepareFapiHubImageBytes(imageBytes []byte, maximumDimension int) ([]byte, string, string, error) {
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return normalizeFapiHubImageBytes(imageBytes)
	}
	bounds := img.Bounds()
	if maximumDimension <= 0 || (bounds.Dx() <= maximumDimension && bounds.Dy() <= maximumDimension) {
		return normalizeFapiHubImageBytes(imageBytes)
	}
	resized := resizeAntiScanPreviewImage(img, maximumDimension)
	buf := bytes.NewBuffer(make([]byte, 0, maximumDimension*maximumDimension/2))
	if err = jpeg.Encode(buf, resized, &jpeg.Options{Quality: 88}); err != nil {
		return nil, "", "", gerror.Wrap(err, "压缩 FAPIHub 预览图片失败")
	}
	return buf.Bytes(), "image/jpeg", "jpg", nil
}

func createFapiHubImagePart(writer *multipart.Writer, contentType string, ext string) (io.Writer, error) {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="anti-scan-source.%s"`, ext))
	header.Set("Content-Type", contentType)
	return writer.CreatePart(header)
}

func normalizeFapiHubImageBytes(imageBytes []byte) ([]byte, string, string, error) {
	contentType := http.DetectContentType(imageBytes)
	switch contentType {
	case "image/jpeg":
		return imageBytes, contentType, "jpg", nil
	case "image/png":
		return imageBytes, contentType, "png", nil
	case "image/gif":
		return imageBytes, contentType, "gif", nil
	case "image/webp":
		return imageBytes, contentType, "webp", nil
	case "image/bmp":
		return imageBytes, contentType, "bmp", nil
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err == nil {
		switch strings.ToLower(format) {
		case "jpeg":
			return imageBytes, "image/jpeg", "jpg", nil
		case "png", "gif", "webp", "bmp":
			format = strings.ToLower(format)
			return imageBytes, "image/" + format, format, nil
		}
	}
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, "", "", gerror.New("图片格式不支持，请上传 JPG、PNG、GIF、WEBP 或 BMP")
	}
	buf := bytes.NewBuffer(nil)
	if err = jpeg.Encode(buf, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, "", "", gerror.Wrap(err, "转换 FAPIHub 图片失败")
	}
	return buf.Bytes(), "image/jpeg", "jpg", nil
}
