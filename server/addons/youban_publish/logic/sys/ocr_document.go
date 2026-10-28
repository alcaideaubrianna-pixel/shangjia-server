package sys

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model"
	"hotgo/internal/library/cache"
)

const telegramOCRCacheTTL = 30 * 24 * time.Hour
const telegramOCRCachePrefix = "youban_publish:ocr:"

type telegramOCRResult struct {
	HasText    bool    `json:"hasText"`
	Text       string  `json:"text"`
	TextCount  int     `json:"textCount"`
	Confidence float64 `json:"confidence"`
	KeywordHit bool    `json:"keywordHit"`
	Cached     bool    `json:"cached"`
}

func classifyTelegramDocumentByOCR(ctx context.Context, imageBytes []byte, conf *model.CloudResourceConfig) (result telegramOCRResult, err error) {
	if conf == nil || conf.OcrEnabled != 1 || strings.TrimSpace(conf.OcrEndpoint) == "" {
		return result, gerror.New("OCR 服务未配置")
	}
	keyMaterial := append(append([]byte{}, imageBytes...), []byte("|endpoint="+strings.TrimSpace(conf.OcrEndpoint))...)
	sum := sha256.Sum256(keyMaterial)
	cacheKey := telegramOCRCachePrefix + hex.EncodeToString(sum[:])
	if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
		if unmarshalErr := value.Scan(&result); unmarshalErr == nil {
			result.Cached = true
			result.KeywordHit = telegramOCRKeywordHit(result.Text, conf.OcrBlockedKeywords)
			return result, nil
		}
	}
	timeout := time.Duration(conf.OcrTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 800 * time.Millisecond
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	body := bytes.NewBuffer(nil)
	writer := multipart.NewWriter(body)
	part, createErr := writer.CreateFormFile("file", "image.jpg")
	if createErr != nil {
		return result, createErr
	}
	if _, err = part.Write(imageBytes); err != nil {
		return result, err
	}
	if err = writer.Close(); err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, strings.TrimRight(conf.OcrEndpoint, "/"), body)
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, gerror.Wrap(err, "OCR 请求失败")
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return result, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, gerror.Newf("OCR 服务返回 HTTP %d", resp.StatusCode)
	}
	if err = decodeTelegramOCRResponse(responseBody, &result); err != nil {
		return result, err
	}
	result.Text = strings.TrimSpace(result.Text)
	result.HasText = result.HasText || result.TextCount > 0 || result.Text != ""
	result.KeywordHit = telegramOCRKeywordHit(result.Text, conf.OcrBlockedKeywords)
	if cacheErr := cache.Instance().Set(ctx, cacheKey, result, telegramOCRCacheTTL); cacheErr != nil {
		g.Log().Warningf(ctx, "OCR 结果缓存失败 hash:%s err:%v", hex.EncodeToString(sum[:])[:16], cacheErr)
	}
	return result, nil
}

func telegramOCRKeywordHit(text, rawKeywords string) bool {
	text = strings.ToLower(text)
	for _, keyword := range strings.FieldsFunc(rawKeywords, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' }) {
		keyword = strings.TrimSpace(keyword)
		if keyword != "" && strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func decodeTelegramOCRResponse(raw []byte, result *telegramOCRResult) error {
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gerror.Wrap(err, "OCR 响应格式错误")
	}
	var texts []string
	var walk func(interface{})
	walk = func(value interface{}) {
		switch item := value.(type) {
		case map[string]interface{}:
			for key, child := range item {
				lower := strings.ToLower(key)
				if lower == "text" || lower == "txt" || lower == "rec_text" || lower == "rec_texts" {
					if s, ok := child.(string); ok && strings.TrimSpace(s) != "" {
						texts = append(texts, strings.TrimSpace(s))
					}
				}
				walk(child)
			}
		case []interface{}:
			for _, child := range item {
				walk(child)
			}
		}
	}
	walk(payload)
	result.Text = strings.Join(texts, "\n")
	result.TextCount = len(texts)
	result.HasText = result.TextCount > 0 || result.Text != ""
	if object, ok := payload.(map[string]interface{}); ok {
		if value, ok := object["hasText"].(bool); ok {
			result.HasText = value
		}
		if value, ok := object["confidence"].(float64); ok {
			result.Confidence = value
		}
	}
	return nil
}
