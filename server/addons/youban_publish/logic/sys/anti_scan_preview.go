package sys

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"strings"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	_ "golang.org/x/image/webp"

	"hotgo/addons/youban_publish/global"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
	"hotgo/internal/library/addons"
	"hotgo/internal/library/storager"
	baseservice "hotgo/internal/service"
	"hotgo/utility/file"
)

const (
	antiScanCacheTable           = "hg_youban_publish_anti_scan_cache"
	antiScanPreviewRenderVersion = 3
)

// AdminAntiScanPreview 生成防扫图实时预览，并按图片 pHash + 配置 hash 复用缓存。
func (s *sSysPublish) AdminAntiScanPreview(ctx context.Context, in *sysin.AntiScanPreviewInp, upload *ghttp.UploadFile) (res *sysin.AntiScanPreviewModel, err error) {
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	imageBytes, originalUrl, err := readAntiScanPreviewImage(ctx, upload, in.UseDefaultImage)
	if err != nil {
		return nil, err
	}
	imageHash, err := antiScanImagePHash(imageBytes)
	if err != nil {
		return nil, err
	}
	cloudConf, err := service.SysConfig().GetCloudResource(ctx)
	if err != nil {
		return nil, err
	}
	configHash := antiScanConfigHash(in, cloudConf)
	noop := isAntiScanNoop(in)
	if cached, ok := s.getAntiScanPreviewCache(ctx, imageHash, configHash); ok {
		cached.CacheHit = 1
		return cached, nil
	}
	detectRes := &antiScanDetectResult{Provider: "none"}
	warnings := []string{}
	if !noop {
		detectRes, warnings, err = s.detectAntiScanImage(ctx, imageHash, imageBytes, in, cloudConf)
		if err != nil {
			return nil, err
		}
	}
	previewBytes, renderWarnings, err := renderAntiScanPreview(ctx, imageBytes, in, detectRes)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, renderWarnings...)
	previewUrl, err := uploadAntiScanPreview(ctx, previewBytes, in)
	if err != nil {
		return nil, err
	}
	res = &sysin.AntiScanPreviewModel{
		CacheHit:      0,
		ConfigHash:    configHash,
		FaceCount:     detectRes.FaceCount,
		ImageHash:     imageHash,
		OriginalUrl:   originalUrl,
		PreviewUrl:    previewUrl,
		Provider:      detectRes.Provider,
		Warnings:      warnings,
		CloudRawSaved: detectRes.CloudRawSaved,
	}
	if err = s.saveAntiScanPreviewCache(ctx, res, detectRes); err != nil {
		return nil, err
	}
	return res, nil
}

type antiScanDetectResult struct {
	CloudRawSaved int
	FaceCount     int
	FaceRaw       string
	Provider      string
	SegmentRaw    string
}

// detectAntiScanImage 按功能需要获取云识别结果，避免同一张图重复调用收费接口。
func (s *sSysPublish) detectAntiScanImage(ctx context.Context, imageHash string, imageBytes []byte, in *sysin.AntiScanPreviewInp, conf *model.CloudResourceConfig) (*antiScanDetectResult, []string, error) {
	if conf == nil {
		return nil, nil, gerror.New("云资源配置不合法")
	}
	res := &antiScanDetectResult{Provider: "none"}
	warnings := []string{}
	if needsAntiScanFaceDetection(in) {
		faceRaw, faceCount, err := s.getOrCreateAntiScanFaceDetection(ctx, imageHash, imageBytes, conf)
		if err != nil {
			return nil, nil, err
		}
		res.FaceRaw = faceRaw
		res.FaceCount = faceCount
		if faceRaw != "" {
			res.CloudRawSaved = 1
			res.Provider = appendAntiScanProvider(res.Provider, "tencent-face")
		} else {
			warnings = append(warnings, "腾讯云人脸检测未启用，二维码/贴图只能按默认位置避让")
		}
	}
	if needsAntiScanMatting(in) {
		segmentRaw, err := s.getOrCreateAntiScanMatting(ctx, imageHash, imageBytes, conf)
		if err != nil {
			return nil, nil, err
		}
		res.SegmentRaw = segmentRaw
		if segmentRaw != "" {
			res.CloudRawSaved = 1
			res.Provider = appendAntiScanProvider(res.Provider, "fapihub-matting")
		} else {
			warnings = append(warnings, "FAPIHub 抠图未启用，背景替换将降级为本地纹理处理")
		}
	}
	return res, warnings, nil
}

func needsAntiScanFaceDetection(in *sysin.AntiScanPreviewInp) bool {
	return in.MaskEnabled == 1 && in.MaskCount > 0
}

func needsAntiScanMatting(in *sysin.AntiScanPreviewInp) bool {
	return in.BackgroundReplaceEnabled == 1 || in.PortraitBackgroundEnabled == 1
}

func appendAntiScanProvider(current string, next string) string {
	if current == "" || current == "none" {
		return next
	}
	if strings.Contains(current, next) {
		return current
	}
	return current + "+" + next
}

func (s *sSysPublish) getOrCreateAntiScanFaceDetection(ctx context.Context, imageHash string, imageBytes []byte, conf *model.CloudResourceConfig) (string, int, error) {
	if cached, ok := s.getAntiScanFaceCache(ctx, imageHash); ok {
		return cached.FaceRaw, cached.FaceCount, nil
	}
	if conf.TencentVisionEnabled != 1 {
		return "", 0, nil
	}
	normalized, err := normalizeTencentVisionImageBytes(imageBytes)
	if err != nil {
		return "", 0, err
	}
	client := newTencentVisionClient(conf.TencentSecretId, conf.TencentSecretKey, conf.TencentCloudSite, conf.TencentRegion, conf.TencentBdaEndpoint, conf.TencentIaiEndpoint)
	faceRaw, faceCount, err := client.detectFace(ctx, base64.StdEncoding.EncodeToString(normalized))
	if err != nil {
		return "", 0, err
	}
	if err = s.saveAntiScanDetectionPart(ctx, imageHash, &antiScanDetectResult{
		CloudRawSaved: 1,
		FaceCount:     faceCount,
		FaceRaw:       faceRaw,
		Provider:      "tencent-face",
	}); err != nil {
		return "", 0, err
	}
	return faceRaw, faceCount, nil
}

func (s *sSysPublish) getOrCreateAntiScanMatting(ctx context.Context, imageHash string, imageBytes []byte, conf *model.CloudResourceConfig) (string, error) {
	if cached, ok := s.getAntiScanSegmentCache(ctx, imageHash); ok {
		return cached.SegmentRaw, nil
	}
	if conf.FapiHubEnabled != 1 {
		return "", nil
	}
	client := newFapiHubClient(conf.FapiHubApiKey, conf.FapiHubEndpoint, conf.FapiHubModel)
	pngBytes, err := client.removeBackground(ctx, imageBytes)
	if err != nil {
		return "", err
	}
	segmentRaw := encodeFapiHubSegmentPortrait(pngBytes)
	if err = s.saveAntiScanDetectionPart(ctx, imageHash, &antiScanDetectResult{
		CloudRawSaved: 1,
		Provider:      "fapihub-matting",
		SegmentRaw:    segmentRaw,
	}); err != nil {
		return "", err
	}
	return segmentRaw, nil
}

func encodeFapiHubSegmentPortrait(imageBytes []byte) string {
	data, _ := json.Marshal(g.Map{
		"Provider": "fapihub",
		"Response": g.Map{
			"ResultImage": base64.StdEncoding.EncodeToString(imageBytes),
		},
	})
	return string(data)
}

func readAntiScanPreviewImage(ctx context.Context, upload *ghttp.UploadFile, useDefault int) ([]byte, string, error) {
	if upload != nil {
		reader, err := upload.Open()
		if err != nil {
			return nil, "", gerror.Wrap(err, "读取预览图片失败")
		}
		defer reader.Close()
		content := bytes.NewBuffer(nil)
		if _, err = content.ReadFrom(reader); err != nil {
			return nil, "", gerror.Wrap(err, "读取预览图片内容失败")
		}
		return content.Bytes(), "", nil
	}
	if useDefault != 1 {
		return nil, "", gerror.New("请上传预览图片或使用默认图片")
	}
	path := gfile.Join(addons.GetResourcePath(ctx), "addons", global.GetSkeleton().Name, "public", "antiscan", "default-preview.webp")
	if !gfile.Exists(path) {
		return nil, "", gerror.New("默认防扫图预览图片不存在")
	}
	return gfile.GetBytes(path), "/addons/" + global.GetSkeleton().Name + "/antiscan/default-preview.webp", nil
}

func antiScanImagePHash(imageBytes []byte) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return "", gerror.New("图片格式不支持，请上传 JPG、PNG、GIF 或 WEBP")
	}
	hash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return "", gerror.Wrap(err, "计算图片感知哈希失败")
	}
	return fmt.Sprintf("%016x", hash.GetHash()), nil
}

func antiScanConfigHash(in *sysin.AntiScanPreviewInp, cloudConf *model.CloudResourceConfig) string {
	cloudData := g.Map{}
	if cloudConf != nil {
		cloudData = g.Map{
			"fapiHubEnabled":       cloudConf.FapiHubEnabled,
			"fapiHubModel":         cloudConf.FapiHubModel,
			"tencentCloudSite":     cloudConf.TencentCloudSite,
			"tencentIaiEndpoint":   cloudConf.TencentIaiEndpoint,
			"tencentVisionEnabled": cloudConf.TencentVisionEnabled,
		}
	}
	data, _ := json.Marshal(g.Map{
		"antiScan":      in.AntiScanConfig,
		"cloud":         cloudData,
		"renderVersion": antiScanPreviewRenderVersion,
	})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func uploadAntiScanPreview(ctx context.Context, imageBytes []byte, in *sysin.AntiScanPreviewInp) (string, error) {
	_, format, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil {
		return "", gerror.New("图片格式不支持，请上传 JPG、PNG、GIF 或 WEBP")
	}
	imageBytes, format, err = normalizeAntiScanPreviewUploadBytes(imageBytes, format)
	if err != nil {
		return "", err
	}
	fileHeader, err := file.NewMultipartFileHeader("anti-scan-preview."+antiScanImageExt(format), imageBytes)
	if err != nil {
		return "", gerror.Wrap(err, "创建预览图片失败")
	}
	attachment, err := baseservice.CommonUpload().UploadFile(ctx, storager.KindImg, &ghttp.UploadFile{FileHeader: fileHeader})
	if err != nil {
		return "", err
	}
	return attachment.FileUrl, nil
}

func normalizeAntiScanPreviewUploadBytes(imageBytes []byte, format string) ([]byte, string, error) {
	if strings.ToLower(format) != "webp" {
		return imageBytes, format, nil
	}
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, "", gerror.New("图片格式不支持，请上传 JPG、PNG、GIF 或 WEBP")
	}
	buf := bytes.NewBuffer(nil)
	if err = jpeg.Encode(buf, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, "", gerror.Wrap(err, "转换预览图片失败")
	}
	return buf.Bytes(), "jpeg", nil
}

func antiScanImageExt(format string) string {
	switch strings.ToLower(format) {
	case "jpeg":
		return "jpg"
	case "png", "gif", "webp":
		return strings.ToLower(format)
	default:
		return "jpg"
	}
}

func (s *sSysPublish) getAntiScanPreviewCache(ctx context.Context, imageHash string, configHash string) (*sysin.AntiScanPreviewModel, bool) {
	row, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).
		Where("image_hash", imageHash).
		Where("config_hash", configHash).
		Where("preview_url <> ''").
		WhereNull("deleted_at").
		One()
	if err != nil || row.IsEmpty() {
		return nil, false
	}
	warnings := []string{}
	_ = json.Unmarshal([]byte(row["warnings_json"].String()), &warnings)
	return &sysin.AntiScanPreviewModel{
		ConfigHash:  row["config_hash"].String(),
		FaceCount:   row["face_count"].Int(),
		ImageHash:   row["image_hash"].String(),
		OriginalUrl: row["original_url"].String(),
		PreviewUrl:  row["preview_url"].String(),
		Provider:    row["provider"].String(),
		Warnings:    warnings,
	}, true
}

func (s *sSysPublish) getAntiScanFaceCache(ctx context.Context, imageHash string) (*antiScanDetectResult, bool) {
	row, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).
		Where("image_hash", imageHash).
		Where("face_json <> ''").
		Where("cloud_raw_saved", 1).
		WhereNull("deleted_at").
		OrderDesc("id").
		One()
	if err != nil || row.IsEmpty() {
		return nil, false
	}
	return &antiScanDetectResult{
		CloudRawSaved: row["cloud_raw_saved"].Int(),
		FaceCount:     row["face_count"].Int(),
		FaceRaw:       row["face_json"].String(),
		Provider:      row["provider"].String(),
	}, true
}

func (s *sSysPublish) getAntiScanSegmentCache(ctx context.Context, imageHash string) (*antiScanDetectResult, bool) {
	row, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).
		Where("image_hash", imageHash).
		Where("segment_json <> ''").
		Where("cloud_raw_saved", 1).
		WhereNull("deleted_at").
		OrderDesc("id").
		One()
	if err != nil || row.IsEmpty() {
		return nil, false
	}
	return &antiScanDetectResult{
		CloudRawSaved: row["cloud_raw_saved"].Int(),
		Provider:      row["provider"].String(),
		SegmentRaw:    row["segment_json"].String(),
	}, true
}

func (s *sSysPublish) saveAntiScanDetectionPart(ctx context.Context, imageHash string, detect *antiScanDetectResult) error {
	_, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).Data(g.Map{
		"image_hash":      imageHash,
		"config_hash":     "",
		"provider":        strings.TrimSpace(detect.Provider),
		"face_count":      detect.FaceCount,
		"face_json":       detect.FaceRaw,
		"segment_json":    detect.SegmentRaw,
		"original_url":    "",
		"preview_url":     "",
		"warnings_json":   "[]",
		"cloud_raw_saved": detect.CloudRawSaved,
		"created_at":      gtime.Now(),
		"updated_at":      gtime.Now(),
	}).Insert()
	return err
}

func (s *sSysPublish) saveAntiScanPreviewCache(ctx context.Context, res *sysin.AntiScanPreviewModel, detect *antiScanDetectResult) error {
	warningsJSON, _ := json.Marshal(res.Warnings)
	_, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).Data(g.Map{
		"image_hash":      res.ImageHash,
		"config_hash":     res.ConfigHash,
		"provider":        strings.TrimSpace(res.Provider),
		"face_count":      res.FaceCount,
		"face_json":       detect.FaceRaw,
		"segment_json":    detect.SegmentRaw,
		"original_url":    res.OriginalUrl,
		"preview_url":     res.PreviewUrl,
		"warnings_json":   string(warningsJSON),
		"cloud_raw_saved": res.CloudRawSaved,
		"created_at":      gtime.Now(),
		"updated_at":      gtime.Now(),
	}).Insert()
	return err
}
