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
	configHash := antiScanConfigHash(in, cloudConf.TencentVisionEnabled)
	noop := isAntiScanNoop(in)
	if cached, ok := s.getAntiScanPreviewCache(ctx, imageHash, configHash); ok {
		cached.CacheHit = 1
		return cached, nil
	}
	detectRes := &antiScanDetectResult{Provider: "none"}
	warnings := []string{}
	if !noop {
		detectRes, warnings, err = s.detectAntiScanImage(ctx, imageHash, imageBytes, cloudConf)
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

// detectAntiScanImage 获取人脸检测和人像分割结果，避免同一张图重复请求腾讯云。
func (s *sSysPublish) detectAntiScanImage(ctx context.Context, imageHash string, imageBytes []byte, conf *model.CloudResourceConfig) (*antiScanDetectResult, []string, error) {
	if conf == nil {
		return nil, nil, gerror.New("云资源配置不合法")
	}
	if conf.TencentVisionEnabled != 1 {
		return &antiScanDetectResult{Provider: "none"}, []string{"腾讯云视觉未启用，已跳过人像分割和人脸检测"}, nil
	}
	if cached, ok := s.getAntiScanDetectionCache(ctx, imageHash); ok {
		return cached, []string{}, nil
	}
	client := newTencentVisionClient(conf.TencentSecretId, conf.TencentSecretKey, conf.TencentRegion, conf.TencentBdaEndpoint, conf.TencentIaiEndpoint)
	result, err := client.detect(ctx, base64.StdEncoding.EncodeToString(imageBytes))
	if err != nil {
		return nil, nil, err
	}
	return &antiScanDetectResult{
		CloudRawSaved: 1,
		FaceCount:     result.FaceCount,
		FaceRaw:       result.FaceRaw,
		Provider:      "tencent",
		SegmentRaw:    result.SegmentRaw,
	}, []string{}, nil
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

func antiScanConfigHash(in *sysin.AntiScanPreviewInp, cloudEnabled int) string {
	data, _ := json.Marshal(g.Map{
		"antiScan":      in.AntiScanConfig,
		"cloudEnabled":  cloudEnabled,
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

func (s *sSysPublish) getAntiScanDetectionCache(ctx context.Context, imageHash string) (*antiScanDetectResult, bool) {
	row, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).
		Where("image_hash", imageHash).
		Where("provider <> ''").
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
		SegmentRaw:    row["segment_json"].String(),
	}, true
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
