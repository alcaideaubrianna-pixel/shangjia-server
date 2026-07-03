package sys

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
)

type fapiHubClient struct {
	apiKey   string
	endpoint string
	model    string
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
	if c.apiKey == "" {
		return nil, gerror.New("FAPIHub API Key 未配置")
	}
	body := bytes.NewBuffer(nil)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", "anti-scan-source.jpg")
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, body)
	if err != nil {
		return nil, gerror.Wrap(err, "创建 FAPIHub 请求失败")
	}
	req.Header.Set("ApiKey", c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, gerror.Wrap(err, "请求 FAPIHub 抠图接口失败")
	}
	defer resp.Body.Close()
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
	return respBytes, nil
}
