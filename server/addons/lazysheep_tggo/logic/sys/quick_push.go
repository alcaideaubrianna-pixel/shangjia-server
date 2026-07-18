// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/internal/dao"
)

const (
	quickMediaGroupLimit          = 10
	quickMergeVerifyVideoLimit    = 3
	quickMediaMaxBytes            = 48 << 20
	quickPhotoMaxBytes            = 10 << 20
	quickPhotoCompressTargetBytes = 9500 << 10
	quickPhotoGroupTargetBytes    = 4200 << 10
	quickMediaGroupMaxUploadBytes = 45 << 20
	quickMediaGroupTargetBytes    = 42 << 20
	quickMediaDownloadConcurrency = 8
	quickVideoThumbnailMaxBytes   = 190 << 10
	bangchatMediaSecret           = "dc7f7fbb4f36fbb43071882d4a1ae7a514996adcb21464e6988eccaa64aa3ed3"
)

const quickMediaTypeDocument = "TG_DOCUMENT"

var quickMediaHTTPClient = &http.Client{
	Timeout: 90 * time.Second,
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

func (s *sLazySheepTGGo) pushQuickCollectedNote(ctx context.Context, botKey string, binding *model.BindingRecord, raw json.RawMessage, fallbackChatID int64) error {
	if binding == nil {
		return nil
	}
	rt := s.runtime.get(botKey)
	if rt == nil || rt.client == nil {
		return gerror.New("机器人运行实例不存在，请先启动机器人")
	}

	targetChatID := binding.PublishChatID
	if targetChatID == 0 {
		targetChatID = fallbackChatID
	}
	if targetChatID == 0 {
		return nil
	}

	var msg sourceMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return gerror.Wrap(err, "解析消息失败")
	}
	if msg.Type != "MESSAGE_TYPE_NOTES" {
		return nil
	}
	var note noteContent
	if err := json.Unmarshal([]byte(msg.Content), &note); err != nil {
		return gerror.Wrap(err, "解析笔记内容失败")
	}

	title, text := noteText(note.Items)
	started := time.Now()
	g.Log().Debugf(ctx, "%s 快速推送开始 botKey:%s binding:%s targetChat:%d title:%s", pullTraceTag(ctx), botKey, binding.Key, targetChatID, title)
	plugins := s.collectorPlugins(ctx, botKey)
	settings := map[string]any{}
	if cfg := plugins["collector"]; cfg != nil && cfg.Settings != nil {
		settings = cfg.Settings
	}
	settings = withBindingCollectorSettings(settings, plugins, binding.PluginState)
	locations := collectQuickLocations(note.Items)
	caption := buildQuickCaption(title, text, settings, locations, plugins, binding.PluginState)
	items, merged := selectQuickMediaItemsForPush(note.Items, settings)
	mediaAssets, err := buildQuickMediaAssets(ctx, items)
	if err != nil {
		return err
	}
	g.Log().Debugf(ctx, "%s 快速推送媒体准备完成 botKey:%s binding:%s assets:%d elapsed:%s", pullTraceTag(ctx), botKey, binding.Key, len(mediaAssets), time.Since(started).Round(time.Millisecond))
	if len(mediaAssets) == 0 {
		return s.sendQuickText(ctx, rt.client, targetChatID, caption)
	}

	msgs, err := s.sendQuickMediaAssetsWithMode(ctx, rt.client, rt.cfg.Token, targetChatID, mediaAssets, caption, merged)
	if err != nil {
		return err
	}
	g.Log().Debugf(ctx, "%s 快速推送完成 botKey:%s binding:%s targetChat:%d messages:%d total:%s", pullTraceTag(ctx), botKey, binding.Key, targetChatID, len(msgs), time.Since(started).Round(time.Millisecond))
	return nil
}

func (s *sLazySheepTGGo) sendQuickText(ctx context.Context, client *bot.Bot, chatID int64, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_, err := client.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
	return err
}

func buildQuickCaption(title, text string, settings map[string]any, locations []quickLocationItem, plugins map[string]*model.PluginConfig, bindingState map[string]any) string {
	footer := resolveContentFooter(plugins, settings, bindingState)
	parts := make([]string, 0, 4)
	if strings.TrimSpace(title) != "" {
		parts = append(parts, html.EscapeString(title))
	}
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		parts = append(parts, html.EscapeString(limitCaption(trimmed, 650)))
	}
	if locationBlock := buildQuickLocationBlock(locations); locationBlock != "" {
		parts = append(parts, locationBlock)
	}
	if footer != "" {
		parts = append(parts, footer)
	}
	return strings.Join(parts, "\n\n")
}

type quickMediaAsset struct {
	Type         string
	Filename     string
	Data         []byte
	SourceURL    string
	ThumbnailURL string
	Thumbnail    []byte
	Duration     int
	AspectRatio  float64
	TgFileID     string
	VerifyVideo  bool
}

func selectQuickMediaItemsForPush(items []noteItem, settings map[string]any) ([]noteItem, bool) {
	if !pushSettingBool(settings, "mergeVerifyInGroup", false) {
		return items, false
	}
	media := make([]noteItem, 0, len(items))
	hasVideo := false
	for _, item := range items {
		if item.Type != noteTypeImage && item.Type != noteTypeVideo {
			continue
		}
		media = append(media, item)
		if item.Type == noteTypeVideo {
			hasVideo = true
		}
	}
	if len(media) == 0 || !hasVideo {
		return items, false
	}
	if len(media) <= quickMediaGroupLimit {
		return media, true
	}
	selected := make([]bool, len(media))
	selectedCount := 0
	selectItem := func(index int) {
		if index < 0 || index >= len(media) || selected[index] || selectedCount >= quickMediaGroupLimit {
			return
		}
		selected[index] = true
		selectedCount++
	}
	verifyCount := 0
	for index, item := range media {
		if item.Type != noteTypeVideo || !item.VerifyVideo {
			continue
		}
		if verifyCount >= quickMergeVerifyVideoLimit {
			break
		}
		selectItem(index)
		verifyCount++
	}
	normalVideoCount := 0
	for index, item := range media {
		if item.Type != noteTypeVideo || item.VerifyVideo {
			continue
		}
		selectItem(index)
		normalVideoCount++
		if normalVideoCount >= 1 {
			break
		}
	}
	for index := range media {
		selectItem(index)
	}
	out := make([]noteItem, 0, quickMediaGroupLimit)
	for index, item := range media {
		if selected[index] {
			out = append(out, item)
			if len(out) >= quickMediaGroupLimit {
				break
			}
		}
	}
	return out, true
}

func buildQuickMediaAssets(ctx context.Context, items []noteItem) ([]quickMediaAsset, error) {
	results := make([]*quickMediaAsset, len(items))
	expected := 0
	errs := make([]string, 0)
	var errMu sync.Mutex
	thumbURL := ""
	for _, item := range items {
		if item.Type == noteTypeImage && thumbURL == "" {
			thumbURL = strings.TrimSpace(item.Content)
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, quickMediaDownloadConcurrency)
	seenURLs := make(map[string]struct{}, len(items))
	for index, item := range items {
		if item.Type != noteTypeImage && item.Type != noteTypeVideo {
			continue
		}
		mediaURL := normalizeDedupMediaURL(item.Content)
		if mediaURL == "" {
			continue
		}
		if _, ok := seenURLs[mediaURL]; ok {
			continue
		}
		seenURLs[mediaURL] = struct{}{}
		switch item.Type {
		case noteTypeImage:
			expected++
			wg.Add(1)
			go func(idx int, item noteItem) {
				defer wg.Done()
				if asset := cachedQuickTelegramMedia(ctx, item, idx); asset != nil {
					results[idx] = asset
					return
				}
				sem <- struct{}{}
				defer func() { <-sem }()
				g.Log().Debugf(ctx, "%s 下载快速推送图片开始 index:%d url:%s", pullTraceTag(ctx), idx, item.Content)
				filename, data, contentType, err := downloadQuickMediaWithType(ctx, item.Content, item.Type, idx)
				if err != nil {
					g.Log().Warningf(ctx, "%s 下载快速推送图片失败 url:%s err:%+v", pullTraceTag(ctx), item.Content, err)
					errMu.Lock()
					errs = append(errs, fmt.Sprintf("图片:%s %v", abbreviateMediaURL(item.Content), err))
					errMu.Unlock()
					return
				}
				g.Log().Debugf(ctx, "%s 下载快速推送图片完成 index:%d bytes:%d", pullTraceTag(ctx), idx, len(data))
				mediaType := resolveQuickMediaType(item.Type, filename, data, contentType)
				if mediaType == noteTypeImage && len(data) > quickPhotoMaxBytes {
					compressedName, compressedData, compressErr := compressQuickPhotoForTelegram(data)
					if compressErr != nil {
						g.Log().Warningf(ctx, "%s 图片压缩失败 url:%s bytes:%d err:%+v", pullTraceTag(ctx), item.Content, len(data), compressErr)
						errMu.Lock()
						errs = append(errs, fmt.Sprintf("图片:%s 压缩失败: %v", abbreviateMediaURL(item.Content), compressErr))
						errMu.Unlock()
						return
					} else {
						g.Log().Debugf(ctx, "%s 图片压缩完成 index:%d before:%d after:%d", pullTraceTag(ctx), idx, len(data), len(compressedData))
						filename = quickCompressedMediaFilename(filename, compressedName)
						data = compressedData
					}
				}
				results[idx] = &quickMediaAsset{
					Type:        mediaType,
					Filename:    filename,
					Data:        data,
					SourceURL:   strings.TrimSpace(item.Content),
					VerifyVideo: item.VerifyVideo,
				}
			}(index, item)
		case noteTypeVideo:
			expected++
			wg.Add(1)
			go func(idx int, item noteItem) {
				defer wg.Done()
				if asset := cachedQuickTelegramMedia(ctx, item, idx); asset != nil {
					asset.ThumbnailURL = thumbURL
					results[idx] = asset
					return
				}
				sem <- struct{}{}
				defer func() { <-sem }()
				g.Log().Debugf(ctx, "%s 下载快速推送视频开始 index:%d url:%s", pullTraceTag(ctx), idx, item.Content)
				filename, data, contentType, err := downloadQuickMediaWithType(ctx, item.Content, item.Type, idx)
				if err != nil {
					g.Log().Warningf(ctx, "%s 下载快速推送视频失败 url:%s err:%+v", pullTraceTag(ctx), item.Content, err)
					errMu.Lock()
					errs = append(errs, fmt.Sprintf("视频:%s %v", abbreviateMediaURL(item.Content), err))
					errMu.Unlock()
					return
				}
				g.Log().Debugf(ctx, "%s 下载快速推送视频完成 index:%d bytes:%d", pullTraceTag(ctx), idx, len(data))
				mediaType := resolveQuickMediaType(item.Type, filename, data, contentType)
				results[idx] = &quickMediaAsset{
					Type:         mediaType,
					Filename:     filename,
					Data:         data,
					SourceURL:    strings.TrimSpace(item.Content),
					ThumbnailURL: thumbURL,
					Thumbnail:    buildQuickVideoThumbnail(ctx, data, filename),
					Duration:     item.Duration,
					AspectRatio:  item.AspectRatio,
					VerifyVideo:  item.VerifyVideo,
				}
			}(index, item)
		}
	}
	wg.Wait()
	out := make([]quickMediaAsset, 0, len(items))
	for _, asset := range results {
		if asset == nil {
			continue
		}
		out = append(out, *asset)
	}
	if expected > 0 && len(out) != expected {
		return nil, gerror.Newf("媒体准备失败，已取消推送：需要 %d 个媒体，成功 %d 个，失败 %d 个。%s", expected, len(out), expected-len(out), limitMediaPrepareErrors(errs))
	}
	return out, nil
}

func buildQuickVideoThumbnail(ctx context.Context, data []byte, filename string) []byte {
	frame := extractQuickVideoFrame(ctx, data, filename)
	if len(frame) == 0 {
		return nil
	}
	return encodeQuickVideoThumbnail(frame)
}

func encodeQuickVideoThumbnail(data []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil
	}
	if width > 320 || height > 320 {
		scale := 320.0 / float64(width)
		if hScale := 320.0 / float64(height); hScale < scale {
			scale = hScale
		}
		width = int(float64(width) * scale)
		height = int(float64(height) * scale)
		img = resizeQuickImage(img, width, height)
	}
	for _, quality := range []int{82, 76, 70, 64, 58, 52, 46} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			continue
		}
		if buf.Len() > 0 && buf.Len() <= quickVideoThumbnailMaxBytes {
			return buf.Bytes()
		}
	}
	return nil
}

func extractQuickVideoFrame(ctx context.Context, data []byte, filename string) []byte {
	if len(data) == 0 {
		return nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		g.Log().Warningf(ctx, "%s ffmpeg 不存在，跳过视频缩略图生成", pullTraceTag(ctx))
		return nil
	}
	dir, err := os.MkdirTemp("", "tggo-video-thumb-*")
	if err != nil {
		g.Log().Warningf(ctx, "%s 创建视频缩略图临时目录失败 err:%+v", pullTraceTag(ctx), err)
		return nil
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, sanitizeQuickMediaFilename(filename))
	if strings.TrimSpace(filepath.Ext(input)) == "" {
		input += ".mp4"
	}
	output := filepath.Join(dir, "thumb.jpg")
	if err = os.WriteFile(input, data, 0600); err != nil {
		g.Log().Warningf(ctx, "%s 写入视频缩略图临时文件失败 err:%+v", pullTraceTag(ctx), err)
		return nil
	}
	cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	args := []string{
		"-y",
		"-ss", "00:00:01",
		"-i", input,
		"-frames:v", "1",
		"-vf", "scale='min(320,iw)':-2",
		"-q:v", "3",
		output,
	}
	out, err := exec.CommandContext(cmdCtx, "ffmpeg", args...).CombinedOutput()
	if err != nil {
		g.Log().Warningf(ctx, "%s 生成视频缩略图失败 err:%+v output:%s", pullTraceTag(ctx), err, limitPushLogError(string(out)))
		return nil
	}
	thumb, err := os.ReadFile(output)
	if err != nil {
		g.Log().Warningf(ctx, "%s 读取视频缩略图失败 err:%+v", pullTraceTag(ctx), err)
		return nil
	}
	return thumb
}

func abbreviateMediaURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if len(rawURL) <= 90 {
		return rawURL
	}
	return rawURL[:90] + "..."
}

func limitMediaPrepareErrors(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) > 3 {
		items = items[:3]
	}
	return strings.Join(items, "；")
}

func compressQuickPhotoForTelegram(data []byte) (filename string, out []byte, err error) {
	return compressQuickPhotoWithinLimit(data, quickPhotoCompressTargetBytes)
}

func compressQuickPhotoWithinLimit(data []byte, limit int) (filename string, out []byte, err error) {
	if limit <= 0 {
		limit = quickPhotoCompressTargetBytes
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", nil, gerror.Wrap(err, "解码图片失败")
	}
	img = resizeQuickImageForTelegramPhoto(img)
	if encoded := encodeQuickJPEGWithinLimit(img, limit); len(encoded) > 0 {
		return "photo.jpg", encoded, nil
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	for _, scale := range []float64{0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.32, 0.25} {
		resized := resizeQuickImage(img, int(float64(width)*scale), int(float64(height)*scale))
		if encoded := encodeQuickJPEGWithinLimit(resized, limit); len(encoded) > 0 {
			return "photo.jpg", encoded, nil
		}
	}
	return "", nil, gerror.New("图片压缩后仍超过 Telegram photo 限制")
}

func resizeQuickImageForTelegramPhoto(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return img
	}
	maxSide := 2560
	minSide := width
	if height < minSide {
		minSide = height
	}
	maxActualSide := width
	if height > maxActualSide {
		maxActualSide = height
	}
	scale := 1.0
	if maxActualSide > maxSide {
		scale = float64(maxSide) / float64(maxActualSide)
	}
	if minSide > 0 && maxActualSide/minSide > 20 {
		if width > height {
			scale = float64(height*20) / float64(width)
		} else {
			scale = float64(width*20) / float64(height)
		}
	}
	if scale >= 1 {
		return img
	}
	return resizeQuickImage(img, int(float64(width)*scale), int(float64(height)*scale))
}

func compressQuickVideoForTelegram(ctx context.Context, asset quickMediaAsset, targetBytes int) (quickMediaAsset, error) {
	if len(asset.Data) == 0 {
		return asset, gerror.New("视频内容为空")
	}
	if targetBytes <= 0 || len(asset.Data) <= targetBytes {
		return asset, nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return asset, gerror.Wrap(err, "ffmpeg 不存在，无法压缩视频")
	}
	dir, err := os.MkdirTemp("", "tggo-video-compress-*")
	if err != nil {
		return asset, gerror.Wrap(err, "创建视频压缩临时目录失败")
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, sanitizeQuickMediaFilename(asset.Filename))
	if strings.TrimSpace(filepath.Ext(input)) == "" {
		input += ".mp4"
	}
	output := filepath.Join(dir, "compressed.mp4")
	if err = os.WriteFile(input, asset.Data, 0600); err != nil {
		return asset, gerror.Wrap(err, "写入视频压缩临时文件失败")
	}
	targetKB := targetBytes / 1024
	if targetKB < 512 {
		targetKB = 512
	}
	args := []string{
		"-y",
		"-i", input,
		"-vf", "scale='min(720,iw)':-2",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "30",
		"-maxrate", fmt.Sprintf("%dk", targetKB/2),
		"-bufsize", fmt.Sprintf("%dk", targetKB),
		"-c:a", "aac",
		"-b:a", "96k",
		"-movflags", "+faststart",
		output,
	}
	cmdCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(cmdCtx, "ffmpeg", args...).CombinedOutput()
	if err != nil {
		return asset, gerror.Newf("视频压缩失败：%v %s", err, limitPushLogError(string(raw)))
	}
	compressed, err := os.ReadFile(output)
	if err != nil {
		return asset, gerror.Wrap(err, "读取压缩视频失败")
	}
	if len(compressed) == 0 {
		return asset, gerror.New("压缩后视频为空")
	}
	if len(compressed) >= len(asset.Data) {
		return asset, gerror.Newf("视频压缩无收益 before:%d after:%d", len(asset.Data), len(compressed))
	}
	if len(compressed) > targetBytes {
		return asset, gerror.Newf("视频压缩后仍超出目标 before:%d after:%d target:%d", len(asset.Data), len(compressed), targetBytes)
	}
	asset.Data = compressed
	asset.Filename = quickCompressedVideoFilename(asset.Filename)
	asset.TgFileID = ""
	asset.Thumbnail = buildQuickVideoThumbnail(ctx, compressed, asset.Filename)
	return asset, nil
}

func encodeQuickJPEGWithinLimit(img image.Image, limit int) []byte {
	for _, quality := range []int{88, 82, 76, 70, 64, 58, 52} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			continue
		}
		if buf.Len() > 0 && buf.Len() <= limit {
			return buf.Bytes()
		}
	}
	return nil
}

func resizeQuickImage(src image.Image, width, height int) image.Image {
	if width <= 0 || height <= 0 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

func quickCompressedMediaFilename(original, fallback string) string {
	base := strings.TrimSuffix(path.Base(original), filepath.Ext(original))
	if base == "" || base == "." || base == "/" {
		return fallback
	}
	return sanitizeQuickMediaFilename(base + ".jpg")
}

func quickCompressedVideoFilename(original string) string {
	base := strings.TrimSuffix(path.Base(original), filepath.Ext(original))
	if base == "" || base == "." || base == "/" {
		base = "video"
	}
	return sanitizeQuickMediaFilename(base + "_compressed.mp4")
}

type quickLocationItem struct {
	Title    string
	SubTitle string
	Content  string
}

func collectQuickLocations(items []noteItem) []quickLocationItem {
	out := make([]quickLocationItem, 0, len(items))
	for _, item := range items {
		if item.Type != noteTypeLocation {
			continue
		}
		out = append(out, quickLocationItem{
			Title:    item.Title,
			SubTitle: item.SubTitle,
			Content:  item.Content,
		})
	}
	return out
}

func buildQuickLocationBlock(items []quickLocationItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		blockParts := make([]string, 0, 2)
		if strings.TrimSpace(item.Title) != "" {
			blockParts = append(blockParts, html.EscapeString(item.Title))
		}
		if strings.TrimSpace(item.SubTitle) != "" {
			blockParts = append(blockParts, html.EscapeString(item.SubTitle))
		}
		if len(blockParts) == 0 {
			continue
		}
		parts = append(parts, "<blockquote>"+strings.Join(blockParts, "\n")+"</blockquote>")
	}
	return strings.Join(parts, "\n\n")
}

func sendQuickMediaChunk(ctx context.Context, client *bot.Bot, chatID int64, assets []quickMediaAsset, caption string, reply *models.ReplyParameters) ([]*models.Message, error) {
	if len(assets) == 1 {
		var msg *models.Message
		var err error
		switch assets[0].Type {
		case noteTypeVideo:
			msg, err = sendQuickVideoAsset(ctx, client, chatID, assets[0], caption, reply)
		case quickMediaTypeDocument:
			msg, err = sendQuickDocumentAsset(ctx, client, chatID, assets[0], caption, reply)
		default:
			msg, err = sendQuickPhotoAsset(ctx, client, chatID, assets[0], caption, reply)
		}
		if err != nil {
			return nil, err
		}
		return []*models.Message{msg}, nil
	}
	media := quickMediaAssetsToInput(assets)
	applyCaptionToFirstMedia(media, caption)
	params := &bot.SendMediaGroupParams{
		ChatID:          chatID,
		Media:           media,
		ReplyParameters: reply,
	}
	started := time.Now()
	msgs, err := client.SendMediaGroup(ctx, params)
	if err != nil {
		g.Log().Warningf(ctx, "%s TG 媒体组发送失败 items:%d elapsed:%s err:%+v", pullTraceTag(ctx), len(assets), time.Since(started).Round(time.Millisecond), err)
		return nil, err
	}
	g.Log().Debugf(ctx, "%s TG 媒体组发送成功 items:%d elapsed:%s", pullTraceTag(ctx), len(assets), time.Since(started).Round(time.Millisecond))
	rememberQuickTelegramMedia(ctx, assets, msgs)
	return msgs, nil
}

func sendQuickMediaAssets(ctx context.Context, client *bot.Bot, chatID int64, assets []quickMediaAsset, caption string) ([]*models.Message, error) {
	return (&sLazySheepTGGo{}).sendQuickMediaAssetsWithMode(ctx, client, "", chatID, assets, caption, false)
}

func (s *sLazySheepTGGo) sendQuickMediaAssetsWithMode(ctx context.Context, client *bot.Bot, token string, chatID int64, assets []quickMediaAsset, caption string, mergeGroup bool) ([]*models.Message, error) {
	messages := make([]*models.Message, 0, len(assets))
	captionUsed := false
	for _, part := range splitQuickMediaAssetsWithMode(assets, mergeGroup) {
		g.Log().Debugf(ctx, "%s TG 媒体发送分组 merge:%t items:%d uploadBytes:%d", pullTraceTag(ctx), mergeGroup, len(part), quickMediaChunkUploadBytes(part))
		partCaption := ""
		if !captionUsed {
			partCaption = caption
			captionUsed = strings.TrimSpace(caption) != ""
		}
		var reply *models.ReplyParameters
		if len(messages) > 0 {
			reply = &models.ReplyParameters{
				MessageID:                messages[0].ID,
				AllowSendingWithoutReply: true,
			}
		}
		if len(part) == 1 && (part[0].Type == noteTypeVideo || part[0].Type == quickMediaTypeDocument) {
			var msg *models.Message
			var err error
			if part[0].Type == noteTypeVideo {
				msg, err = sendQuickVideoAsset(ctx, client, chatID, part[0], partCaption, reply)
			} else {
				msg, err = sendQuickDocumentAsset(ctx, client, chatID, part[0], partCaption, reply)
			}
			if err != nil {
				if len(messages) > 0 {
					g.Log().Warningf(ctx, "%s TG 媒体部分发送成功，跳过剩余媒体避免整条重复 chat:%d sent:%d err:%+v", pullTraceTag(ctx), chatID, len(messages), err)
					return messages, nil
				}
				return messages, err
			}
			messages = append(messages, msg)
			continue
		}
		var msgs []*models.Message
		var err error
		if mergeGroup {
			part, err = prepareQuickMediaGroupForTelegram(ctx, part)
			if err != nil {
				return messages, err
			}
		}
		if mergeGroup && strings.TrimSpace(token) != "" {
			msgs, err = s.sendQuickMediaGroupMultipart(ctx, token, chatID, part, partCaption, reply)
		} else {
			msgs, err = sendQuickMediaChunk(ctx, client, chatID, part, partCaption, reply)
		}
		if err != nil && isTelegramWrongFileIdentifier(err) && quickMediaPartUsesFileID(part) {
			g.Log().Warningf(ctx, "%s TG file_id 失效，清理缓存后重传媒体组 chat:%d items:%d err:%+v", pullTraceTag(ctx), chatID, len(part), err)
			clearQuickTelegramFileIDs(ctx, part)
			reload, reloadErr := reloadQuickMediaAssetsWithoutFileID(ctx, part)
			if reloadErr != nil {
				return messages, reloadErr
			}
			if mergeGroup {
				reload, reloadErr = prepareQuickMediaGroupForTelegram(ctx, reload)
				if reloadErr != nil {
					return messages, reloadErr
				}
			}
			part = reload
			if mergeGroup && strings.TrimSpace(token) != "" {
				msgs, err = s.sendQuickMediaGroupMultipart(ctx, token, chatID, part, partCaption, reply)
			} else {
				msgs, err = sendQuickMediaChunk(ctx, client, chatID, part, partCaption, reply)
			}
		}
		if err != nil {
			if len(messages) > 0 {
				g.Log().Warningf(ctx, "%s TG 媒体部分发送成功，跳过剩余媒体避免整条重复 chat:%d sent:%d err:%+v", pullTraceTag(ctx), chatID, len(messages), err)
				return messages, nil
			}
			return messages, err
		}
		messages = append(messages, msgs...)
	}
	return messages, nil
}

func isTelegramWrongFileIdentifier(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "wrong file identifier") || strings.Contains(text, "file_id")
}

func quickMediaPartUsesFileID(assets []quickMediaAsset) bool {
	for _, asset := range assets {
		if strings.TrimSpace(asset.TgFileID) != "" {
			return true
		}
	}
	return false
}

func clearQuickTelegramFileIDs(ctx context.Context, assets []quickMediaAsset) {
	for _, asset := range assets {
		if strings.TrimSpace(asset.SourceURL) == "" || strings.TrimSpace(asset.TgFileID) == "" {
			continue
		}
		clearTelegramFileID(ctx, asset.SourceURL)
	}
}

func reloadQuickMediaAssetsWithoutFileID(ctx context.Context, assets []quickMediaAsset) ([]quickMediaAsset, error) {
	out := make([]quickMediaAsset, 0, len(assets))
	for index, asset := range assets {
		asset.TgFileID = ""
		if len(asset.Data) > 0 {
			out = append(out, asset)
			continue
		}
		filename, data, contentType, err := downloadQuickMediaWithType(ctx, asset.SourceURL, asset.Type, index)
		if err != nil {
			return nil, gerror.Wrapf(err, "重新下载媒体失败 url:%s", abbreviateMediaURL(asset.SourceURL))
		}
		asset.Filename = filename
		asset.Data = data
		asset.Type = resolveQuickMediaType(asset.Type, filename, data, contentType)
		if asset.Type == noteTypeVideo {
			asset.Thumbnail = buildQuickVideoThumbnail(ctx, data, filename)
		}
		out = append(out, asset)
	}
	return out, nil
}

func prepareQuickMediaGroupForTelegram(ctx context.Context, assets []quickMediaAsset) ([]quickMediaAsset, error) {
	if len(assets) == 0 {
		return assets, nil
	}
	out := append([]quickMediaAsset(nil), assets...)
	for i, asset := range out {
		if asset.Type == noteTypeImage && strings.TrimSpace(asset.TgFileID) == "" && len(asset.Data) > quickPhotoGroupTargetBytes {
			name, data, err := compressQuickPhotoWithinLimit(asset.Data, quickPhotoGroupTargetBytes)
			if err != nil {
				return nil, gerror.Wrap(err, "图片超过 Telegram 限制且压缩失败")
			}
			out[i].Filename = quickCompressedMediaFilename(asset.Filename, name)
			out[i].Data = data
		}
	}
	total := quickMediaChunkUploadBytes(out)
	if total <= quickMediaGroupMaxUploadBytes {
		return out, nil
	}
	videoIndexes := make([]int, 0)
	fixedBytes := 0
	for i, asset := range out {
		if strings.TrimSpace(asset.TgFileID) != "" {
			continue
		}
		if asset.Type == noteTypeVideo {
			videoIndexes = append(videoIndexes, i)
			continue
		}
		fixedBytes += len(asset.Data)
	}
	if len(videoIndexes) == 0 {
		return nil, gerror.Newf("媒体组上传体积过大且没有可压缩视频 total:%d limit:%d", total, quickMediaGroupMaxUploadBytes)
	}
	remaining := quickMediaGroupTargetBytes - fixedBytes
	if remaining <= len(videoIndexes)*512*1024 {
		for i, asset := range out {
			if asset.Type != noteTypeImage || strings.TrimSpace(asset.TgFileID) != "" || len(asset.Data) <= 0 {
				continue
			}
			name, data, err := compressQuickPhotoWithinLimit(asset.Data, 2600<<10)
			if err != nil {
				return nil, gerror.Wrapf(err, "媒体组图片二次压缩失败 index:%d", i)
			}
			out[i].Filename = quickCompressedMediaFilename(asset.Filename, name)
			out[i].Data = data
		}
		fixedBytes = 0
		for _, asset := range out {
			if strings.TrimSpace(asset.TgFileID) == "" && asset.Type != noteTypeVideo {
				fixedBytes += len(asset.Data)
			}
		}
		remaining = quickMediaGroupTargetBytes - fixedBytes
		if remaining <= len(videoIndexes)*512*1024 {
			return nil, gerror.Newf("媒体组图片体积过大，无法为视频保留压缩空间 fixed:%d target:%d", fixedBytes, quickMediaGroupTargetBytes)
		}
	}
	perVideoTarget := remaining / len(videoIndexes)
	for _, index := range videoIndexes {
		compressed, err := compressQuickVideoForTelegram(ctx, out[index], perVideoTarget)
		if err != nil {
			return nil, gerror.Wrapf(err, "视频压缩失败 index:%d", index)
		}
		out[index] = compressed
	}
	if total = quickMediaChunkUploadBytes(out); total > quickMediaGroupMaxUploadBytes {
		return nil, gerror.Newf("媒体组压缩后仍过大 total:%d limit:%d", total, quickMediaGroupMaxUploadBytes)
	}
	g.Log().Debugf(ctx, "%s 媒体组压缩完成 before:%d after:%d", pullTraceTag(ctx), quickMediaChunkUploadBytes(assets), total)
	return out, nil
}

func splitQuickMediaAssetsWithMode(assets []quickMediaAsset, mergeGroup bool) [][]quickMediaAsset {
	if mergeGroup {
		if len(assets) == 0 {
			return nil
		}
		if len(assets) > quickMediaGroupLimit {
			assets = assets[:quickMediaGroupLimit]
		}
		return [][]quickMediaAsset{assets}
	}
	return splitQuickMediaAssets(assets)
}

func splitQuickMediaAssets(assets []quickMediaAsset) [][]quickMediaAsset {
	parts := make([][]quickMediaAsset, 0, len(assets))
	photos := make([]quickMediaAsset, 0, quickMediaGroupLimit)
	flushPhotos := func() {
		if len(photos) == 0 {
			return
		}
		parts = append(parts, chunkQuickMediaAssets(photos, quickMediaGroupLimit)...)
		photos = make([]quickMediaAsset, 0, quickMediaGroupLimit)
	}
	for _, asset := range assets {
		if asset.Type == quickMediaTypeDocument {
			flushPhotos()
			parts = append(parts, []quickMediaAsset{asset})
			continue
		}
		if len(photos) > 0 && asset.Type != photos[0].Type {
			flushPhotos()
		}
		photos = append(photos, asset)
		if len(photos) >= quickMediaGroupLimit {
			flushPhotos()
		}
	}
	flushPhotos()
	return parts
}

func sendQuickPhotoAsset(ctx context.Context, client *bot.Bot, chatID int64, asset quickMediaAsset, caption string, reply *models.ReplyParameters) (*models.Message, error) {
	params := &bot.SendPhotoParams{
		ChatID:          chatID,
		Photo:           quickInputFile(asset),
		Caption:         caption,
		ParseMode:       models.ParseModeHTML,
		ReplyParameters: reply,
	}
	started := time.Now()
	msg, err := client.SendPhoto(ctx, params)
	if err != nil {
		g.Log().Warningf(ctx, "%s TG 图片发送失败 elapsed:%s err:%+v", pullTraceTag(ctx), time.Since(started).Round(time.Millisecond), err)
		return nil, err
	}
	g.Log().Debugf(ctx, "%s TG 图片发送成功 elapsed:%s", pullTraceTag(ctx), time.Since(started).Round(time.Millisecond))
	if fileID := telegramFileIDFromMessage(msg, noteTypeImage); fileID != "" {
		updateTelegramFileID(ctx, asset.SourceURL, fileID)
	}
	return msg, nil
}

func sendQuickVideoAsset(ctx context.Context, client *bot.Bot, chatID int64, asset quickMediaAsset, caption string, reply *models.ReplyParameters) (*models.Message, error) {
	width, height := videoDimensions(asset.AspectRatio)
	params := &bot.SendVideoParams{
		ChatID:            chatID,
		Video:             quickInputFile(asset),
		Duration:          asset.Duration,
		Width:             width,
		Height:            height,
		Caption:           caption,
		ParseMode:         models.ParseModeHTML,
		SupportsStreaming: true,
		ReplyParameters:   reply,
	}
	if len(asset.Thumbnail) > 0 && strings.TrimSpace(asset.TgFileID) == "" {
		params.Thumbnail = &models.InputFileUpload{
			Filename: quickThumbnailFilename(asset.Filename),
			Data:     bytes.NewReader(asset.Thumbnail),
		}
	}
	started := time.Now()
	msg, err := client.SendVideo(ctx, params)
	if err != nil {
		g.Log().Warningf(ctx, "%s TG 视频发送失败 elapsed:%s err:%+v", pullTraceTag(ctx), time.Since(started).Round(time.Millisecond), err)
		return nil, err
	}
	g.Log().Debugf(ctx, "%s TG 视频发送成功 elapsed:%s", pullTraceTag(ctx), time.Since(started).Round(time.Millisecond))
	if fileID := telegramFileIDFromMessage(msg, noteTypeVideo); fileID != "" {
		updateTelegramFileID(ctx, asset.SourceURL, fileID)
	}
	return msg, nil
}

func sendQuickDocumentAsset(ctx context.Context, client *bot.Bot, chatID int64, asset quickMediaAsset, caption string, reply *models.ReplyParameters) (*models.Message, error) {
	params := &bot.SendDocumentParams{
		ChatID:          chatID,
		Document:        quickInputFile(asset),
		Caption:         caption,
		ParseMode:       models.ParseModeHTML,
		ReplyParameters: reply,
	}
	started := time.Now()
	msg, err := client.SendDocument(ctx, params)
	if err != nil {
		g.Log().Warningf(ctx, "%s TG 文件发送失败 elapsed:%s err:%+v", pullTraceTag(ctx), time.Since(started).Round(time.Millisecond), err)
		return nil, err
	}
	g.Log().Debugf(ctx, "%s TG 文件发送成功 elapsed:%s", pullTraceTag(ctx), time.Since(started).Round(time.Millisecond))
	if fileID := telegramFileIDFromMessage(msg, quickMediaTypeDocument); fileID != "" {
		updateTelegramFileID(ctx, asset.SourceURL, fileID)
	}
	return msg, nil
}

type telegramMediaGroupResponse struct {
	OK          bool              `json:"ok"`
	Result      []*models.Message `json:"result"`
	Description string            `json:"description"`
	ErrorCode   int               `json:"error_code"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (s *sLazySheepTGGo) sendQuickMediaGroupMultipart(ctx context.Context, token string, chatID int64, assets []quickMediaAsset, caption string, reply *models.ReplyParameters) ([]*models.Message, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, gerror.New("Telegram Bot Token 不能为空")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", fmt.Sprintf("%d", chatID)); err != nil {
		return nil, err
	}
	if reply != nil {
		replyRaw, _ := json.Marshal(reply)
		if err := writer.WriteField("reply_parameters", string(replyRaw)); err != nil {
			return nil, err
		}
	}
	media := make([]map[string]any, 0, len(assets))
	for index, asset := range assets {
		mediaName := quickMediaAttachName("media", index, asset.Filename)
		mediaValue := "attach://" + mediaName
		if strings.TrimSpace(asset.TgFileID) != "" {
			mediaValue = strings.TrimSpace(asset.TgFileID)
		}
		item := map[string]any{
			"media": mediaValue,
		}
		switch asset.Type {
		case noteTypeVideo:
			item["type"] = "video"
			item["supports_streaming"] = true
			if asset.Duration > 0 {
				item["duration"] = asset.Duration
			}
			width, height := videoDimensions(asset.AspectRatio)
			if width > 0 && height > 0 {
				item["width"] = width
				item["height"] = height
			}
			if len(asset.Thumbnail) > 0 {
				thumbName := quickMediaAttachName("thumb", index, quickThumbnailFilename(asset.Filename))
				item["thumbnail"] = "attach://" + thumbName
				if err := writeMultipartFile(writer, thumbName, quickThumbnailFilename(asset.Filename), asset.Thumbnail); err != nil {
					return nil, err
				}
			}
		case quickMediaTypeDocument:
			item["type"] = "document"
		default:
			item["type"] = "photo"
		}
		if index == 0 && strings.TrimSpace(caption) != "" {
			item["caption"] = caption
			item["parse_mode"] = string(models.ParseModeHTML)
		}
		if strings.TrimSpace(asset.TgFileID) == "" {
			if err := writeMultipartFile(writer, mediaName, asset.Filename, asset.Data); err != nil {
				return nil, err
			}
		}
		media = append(media, item)
	}
	mediaRaw, _ := json.Marshal(media)
	if err := writer.WriteField("media", string(mediaRaw)); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	httpClient, err := s.telegramHTTPClient(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMediaGroup", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	started := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		g.Log().Warningf(ctx, "%s TG 媒体组 multipart 发送失败 items:%d elapsed:%s err:%+v", pullTraceTag(ctx), len(assets), time.Since(started).Round(time.Millisecond), err)
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out telegramMediaGroupResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, gerror.Wrap(err, "解析 Telegram 媒体组响应失败")
	}
	if resp.StatusCode >= 300 || !out.OK {
		errText := strings.TrimSpace(out.Description)
		if errText == "" {
			errText = string(raw)
		}
		if out.Parameters != nil && out.Parameters.RetryAfter > 0 {
			errText = fmt.Sprintf("%s: retry_after %d", errText, out.Parameters.RetryAfter)
		}
		g.Log().Warningf(ctx, "%s TG 媒体组 multipart 发送失败 items:%d elapsed:%s err:%s", pullTraceTag(ctx), len(assets), time.Since(started).Round(time.Millisecond), errText)
		return nil, gerror.New(errText)
	}
	g.Log().Debugf(ctx, "%s TG 媒体组 multipart 发送成功 items:%d elapsed:%s", pullTraceTag(ctx), len(assets), time.Since(started).Round(time.Millisecond))
	rememberQuickTelegramMedia(ctx, assets, out.Result)
	return out.Result, nil
}

func writeMultipartFile(writer *multipart.Writer, fieldName, filename string, data []byte) error {
	if len(data) == 0 {
		return gerror.Newf("上传文件为空：%s", filename)
	}
	part, err := writer.CreateFormFile(fieldName, sanitizeQuickMediaFilename(filename))
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
}

func quickMediaAttachName(prefix string, index int, filename string) string {
	seed := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", prefix, index, filename)))
	return fmt.Sprintf("%s_%d_%s", prefix, index, hex.EncodeToString(seed[:4]))
}

func quickInputFile(asset quickMediaAsset) models.InputFile {
	if strings.TrimSpace(asset.TgFileID) != "" {
		return &models.InputFileString{Data: asset.TgFileID}
	}
	return &models.InputFileUpload{Filename: asset.Filename, Data: bytes.NewReader(asset.Data)}
}

func quickMediaAssetsToInput(assets []quickMediaAsset) []models.InputMedia {
	out := make([]models.InputMedia, 0, len(assets))
	for _, asset := range assets {
		switch asset.Type {
		case noteTypeImage:
			if strings.TrimSpace(asset.TgFileID) != "" {
				out = append(out, &models.InputMediaPhoto{Media: asset.TgFileID})
				continue
			}
			out = append(out, &models.InputMediaPhoto{
				Media:           "attach://" + asset.Filename,
				MediaAttachment: bytes.NewReader(asset.Data),
			})
		case noteTypeVideo:
			width, height := videoDimensions(asset.AspectRatio)
			video := &models.InputMediaVideo{
				SupportsStreaming: true,
				Duration:          asset.Duration,
				Width:             width,
				Height:            height,
			}
			if strings.TrimSpace(asset.TgFileID) != "" {
				video.Media = asset.TgFileID
			} else {
				video.Media = "attach://" + asset.Filename
				video.MediaAttachment = bytes.NewReader(asset.Data)
			}
			if strings.TrimSpace(asset.ThumbnailURL) != "" && len(asset.Thumbnail) == 0 {
				video.Thumbnail = &models.InputFileString{Data: asset.ThumbnailURL}
			}
			if len(asset.Thumbnail) > 0 && strings.TrimSpace(asset.TgFileID) == "" {
				name := quickThumbnailFilename(asset.Filename)
				video.Thumbnail = &models.InputFileString{Data: "attach://" + name}
			}
			out = append(out, video)
		case quickMediaTypeDocument:
			if strings.TrimSpace(asset.TgFileID) != "" {
				out = append(out, &models.InputMediaDocument{Media: asset.TgFileID})
				continue
			}
			out = append(out, &models.InputMediaDocument{
				Media:           "attach://" + asset.Filename,
				MediaAttachment: bytes.NewReader(asset.Data),
			})
		}
	}
	return out
}

func quickThumbnailFilename(filename string) string {
	base := strings.TrimSuffix(path.Base(filename), filepath.Ext(filename))
	if base == "" || base == "." || base == "/" {
		base = "thumb"
	}
	return sanitizeQuickMediaFilename(base + "_thumb.jpg")
}

func cachedQuickTelegramMedia(ctx context.Context, item noteItem, index int) *quickMediaAsset {
	if item.Type == noteTypeVideo {
		return nil
	}
	fileID := strings.TrimSpace(item.TgFileID)
	if fileID == "" {
		if cachedID, cachedType := cachedTelegramFileIDBySourceURL(ctx, item.Content); cachedID != "" {
			fileID = cachedID
			if cachedType != "" {
				item.Type = cachedType
			}
		}
	}
	if fileID == "" {
		return nil
	}
	mediaType := item.Type
	if mediaType != noteTypeVideo {
		mediaType = noteTypeImage
	}
	return &quickMediaAsset{
		Type:        mediaType,
		Filename:    quickMediaFilename(item.Content, mediaType, index),
		SourceURL:   strings.TrimSpace(item.Content),
		Duration:    item.Duration,
		AspectRatio: item.AspectRatio,
		TgFileID:    fileID,
	}
}

func cachedTelegramFileIDBySourceURL(ctx context.Context, rawURL string) (string, string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", ""
	}
	assetCols := dao.AddonLazysheepTggoNoteAsset.Columns()
	var row struct {
		TgFileId  string `json:"tgFileId"`
		AssetType string `json:"assetType"`
	}
	if err := dao.AddonLazysheepTggoNoteAsset.Ctx(ctx).
		Fields(assetCols.TgFileId, assetCols.AssetType).
		Where(assetCols.SourceUrl, rawURL).
		WhereNot(assetCols.TgFileId, "").
		OrderDesc(assetCols.Id).
		Scan(&row); err != nil {
		g.Log().Debugf(ctx, "未命中 Telegram file_id 缓存 url:%s err:%+v", rawURL, err)
		return "", ""
	}
	mediaType := noteTypeImage
	if row.AssetType == "video" || row.AssetType == "verify_video" {
		mediaType = noteTypeVideo
	} else if row.AssetType == "document" {
		mediaType = quickMediaTypeDocument
	}
	return strings.TrimSpace(row.TgFileId), mediaType
}

func rememberQuickTelegramMedia(ctx context.Context, assets []quickMediaAsset, msgs []*models.Message) {
	for i, asset := range assets {
		if i >= len(msgs) || strings.TrimSpace(asset.SourceURL) == "" {
			continue
		}
		fileID := telegramFileIDFromMessage(msgs[i], asset.Type)
		if fileID == "" {
			continue
		}
		updateTelegramFileID(ctx, asset.SourceURL, fileID)
	}
}

func telegramFileIDFromMessage(msg *models.Message, mediaType string) string {
	if msg == nil {
		return ""
	}
	if mediaType == noteTypeVideo && msg.Video != nil {
		return strings.TrimSpace(msg.Video.FileID)
	}
	if mediaType == quickMediaTypeDocument && msg.Document != nil {
		return strings.TrimSpace(msg.Document.FileID)
	}
	if len(msg.Photo) > 0 {
		return strings.TrimSpace(msg.Photo[len(msg.Photo)-1].FileID)
	}
	if msg.Video != nil {
		return strings.TrimSpace(msg.Video.FileID)
	}
	if msg.Document != nil {
		return strings.TrimSpace(msg.Document.FileID)
	}
	return ""
}

func updateTelegramFileID(ctx context.Context, sourceURL, fileID string) {
	sourceURL = strings.TrimSpace(sourceURL)
	fileID = strings.TrimSpace(fileID)
	if sourceURL == "" || fileID == "" {
		return
	}
	now := gtime.Now()
	assetCols := dao.AddonLazysheepTggoNoteAsset.Columns()
	if _, err := dao.AddonLazysheepTggoNoteAsset.Ctx(ctx).Where(assetCols.SourceUrl, sourceURL).Data(g.Map{
		assetCols.TgFileId:  fileID,
		assetCols.UpdatedAt: now,
	}).Update(); err != nil {
		g.Log().Warningf(ctx, "更新笔记资源 Telegram file_id 失败 url:%s err:%+v", sourceURL, err)
	}
	itemCols := dao.AddonLazysheepTggoNoteItem.Columns()
	if _, err := dao.AddonLazysheepTggoNoteItem.Ctx(ctx).Where(itemCols.Content, sourceURL).Data(g.Map{
		itemCols.TgFileId:  fileID,
		itemCols.UpdatedAt: now,
	}).Update(); err != nil {
		g.Log().Warningf(ctx, "更新笔记项 Telegram file_id 失败 url:%s err:%+v", sourceURL, err)
	}
}

func clearTelegramFileID(ctx context.Context, sourceURL string) {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return
	}
	now := gtime.Now()
	assetCols := dao.AddonLazysheepTggoNoteAsset.Columns()
	if _, err := dao.AddonLazysheepTggoNoteAsset.Ctx(ctx).Where(assetCols.SourceUrl, sourceURL).Data(g.Map{
		assetCols.TgFileId:  "",
		assetCols.UpdatedAt: now,
	}).Update(); err != nil {
		g.Log().Warningf(ctx, "清理笔记资源 Telegram file_id 失败 url:%s err:%+v", sourceURL, err)
	}
	itemCols := dao.AddonLazysheepTggoNoteItem.Columns()
	if _, err := dao.AddonLazysheepTggoNoteItem.Ctx(ctx).Where(itemCols.Content, sourceURL).Data(g.Map{
		itemCols.TgFileId:  "",
		itemCols.UpdatedAt: now,
	}).Update(); err != nil {
		g.Log().Warningf(ctx, "清理笔记项 Telegram file_id 失败 url:%s err:%+v", sourceURL, err)
	}
}

func videoDimensions(aspectRatio float64) (int, int) {
	if aspectRatio <= 0 {
		return 0, 0
	}
	width := 720
	height := int(float64(width) / aspectRatio)
	if height <= 0 {
		return 0, 0
	}
	return width, height
}

func downloadQuickMedia(ctx context.Context, rawURL, itemType string, index int) (filename string, data []byte, err error) {
	filename, data, _, err = downloadQuickMediaWithType(ctx, rawURL, itemType, index)
	return filename, data, err
}

func downloadQuickMediaWithType(ctx context.Context, rawURL, itemType string, index int) (filename string, data []byte, contentType string, err error) {
	filename, data, _, err = downloadCachedMedia(ctx, rawURL, itemType, index)
	contentType = mimeFromBytes(data)
	return filename, data, contentType, err
}

func resolveQuickMediaType(itemType, filename string, data []byte, contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	ext := strings.ToLower(filepath.Ext(filename))
	switch {
	case strings.HasPrefix(contentType, "video/"), ext == ".mp4" || ext == ".mov" || ext == ".m4v":
		return noteTypeVideo
	case strings.HasPrefix(contentType, "image/"):
		return noteTypeImage
	case itemType == noteTypeVideo:
		return noteTypeVideo
	case strings.HasPrefix(mimeFromBytes(data), "video/"):
		return noteTypeVideo
	default:
		return noteTypeImage
	}
}

func decodeBangchatMedia(rawURL string, body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	if mimeFromBytes(body) != "" {
		return body
	}
	decoded, err := decodeBangchatMediaBytes(rawURL, body)
	if err != nil || len(decoded) == 0 {
		return body
	}
	if mimeFromBytes(decoded) != "" {
		return decoded
	}
	return decoded
}

func decodeBangchatMediaBytes(rawURL string, body []byte) ([]byte, error) {
	parts := strings.Split(rawURL, "/")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid media url: %s", rawURL)
	}
	key := hmac.New(sha256.New, []byte(bangchatMediaSecret))
	_, _ = key.Write([]byte(strings.Join(parts[3:], "/")))
	xorKey := key.Sum(nil)
	if len(xorKey) == 0 {
		return nil, fmt.Errorf("media xor key is empty")
	}
	start := int(body[0]) + 1
	if start > len(body) {
		return nil, fmt.Errorf("invalid media offset")
	}
	payload := body[start:]
	out := make([]byte, len(payload))
	for i := range payload {
		out[i] = payload[i] ^ xorKey[i%len(xorKey)]
	}
	return out, nil
}

func quickMediaFilename(rawURL, itemType string, index int) string {
	ext := ".jpg"
	if itemType == noteTypeVideo {
		ext = ".mp4"
	}
	if u, err := url.Parse(rawURL); err == nil {
		base := path.Base(u.Path)
		if base != "." && base != "/" && strings.Contains(base, ".") {
			return fmt.Sprintf("lazy_%d_%s", index, sanitizeQuickMediaFilename(base))
		}
	}
	return fmt.Sprintf("lazy_%d%s", index, ext)
}

func sanitizeQuickMediaFilename(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	if b.Len() == 0 {
		return "media"
	}
	return b.String()
}

func chunkQuickMediaAssets(items []quickMediaAsset, size int) [][]quickMediaAsset {
	if size <= 0 {
		size = quickMediaGroupLimit
	}
	if len(items) == 0 {
		return nil
	}
	out := make([][]quickMediaAsset, 0, (len(items)+size-1)/size)
	current := make([]quickMediaAsset, 0, size)
	currentBytes := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		out = append(out, current)
		current = make([]quickMediaAsset, 0, size)
		currentBytes = 0
	}
	for _, item := range items {
		if item.Type == quickMediaTypeDocument {
			flush()
			out = append(out, []quickMediaAsset{item})
			continue
		}
		itemBytes := quickMediaUploadBytes(item)
		if len(current) >= size || (len(current) > 0 && currentBytes+itemBytes > quickMediaGroupMaxUploadBytes) {
			flush()
		}
		current = append(current, item)
		currentBytes += itemBytes
	}
	flush()
	return out
}

func quickMediaUploadBytes(item quickMediaAsset) int {
	if strings.TrimSpace(item.TgFileID) != "" {
		return 0
	}
	return len(item.Data)
}

func quickMediaChunkUploadBytes(items []quickMediaAsset) int {
	total := 0
	for _, item := range items {
		total += quickMediaUploadBytes(item)
	}
	return total
}

func applyCaptionToFirstMedia(chunk []models.InputMedia, caption string) {
	if len(chunk) == 0 || strings.TrimSpace(caption) == "" {
		return
	}
	switch media := chunk[0].(type) {
	case *models.InputMediaPhoto:
		media.Caption = caption
		media.ParseMode = models.ParseModeHTML
	case *models.InputMediaVideo:
		media.Caption = caption
		media.ParseMode = models.ParseModeHTML
	}
}

func limitCaption(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	if max < 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func mimeFromBytes(body []byte) string {
	if len(body) >= 3 && body[0] == 0xff && body[1] == 0xd8 && body[2] == 0xff {
		return "image/jpeg"
	}
	if len(body) >= 8 && bytes.Equal(body[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
		return "image/png"
	}
	if len(body) >= 6 && (bytes.Equal(body[:6], []byte("GIF87a")) || bytes.Equal(body[:6], []byte("GIF89a"))) {
		return "image/gif"
	}
	if len(body) >= 12 && bytes.Equal(body[:4], []byte("RIFF")) && bytes.Equal(body[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	if len(body) >= 12 && bytes.Equal(body[4:8], []byte("ftyp")) {
		return "video/mp4"
	}
	if len(body) >= 12 && bytes.Equal(body[:4], []byte("\x00\x00\x00\x14")) && bytes.Equal(body[4:8], []byte("ftyp")) {
		return "video/mp4"
	}
	if len(body) > 0 && body[0] == '<' {
		return "image/svg+xml"
	}
	return ""
}
