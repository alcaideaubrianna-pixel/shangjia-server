// Package storager
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package storager

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"
)

// OssDrive 阿里云oss驱动
type OssDrive struct {
}

// OssObjectInfo contains the object metadata needed by direct upload validation.
type OssObjectInfo struct {
	Size int64
}

func newOssBucket(endpoint string) (*oss.Bucket, error) {
	return newOssBucketWithCname(endpoint, false)
}

func newOssBucketWithCname(endpoint string, useCname bool) (*oss.Bucket, error) {
	if config == nil {
		return nil, gerror.New("OSS存储配置不存在")
	}
	if strings.TrimSpace(endpoint) == "" {
		return nil, gerror.New("OSS存储驱动缺少Endpoint配置")
	}
	if strings.TrimSpace(config.OssBucket) == "" {
		return nil, gerror.New("OSS存储驱动缺少Bucket配置")
	}
	if strings.TrimSpace(config.OssSecretId) == "" || strings.TrimSpace(config.OssSecretKey) == "" {
		return nil, gerror.New("OSS存储驱动缺少Access Key或Secret Key配置")
	}
	client, err := oss.New(
		strings.TrimSpace(endpoint),
		strings.TrimSpace(config.OssSecretId),
		strings.TrimSpace(config.OssSecretKey),
		oss.UseCname(useCname),
	)
	if err != nil {
		return nil, gerror.Wrap(err, "初始化OSS客户端失败")
	}
	bucket, err := client.Bucket(strings.TrimSpace(config.OssBucket))
	if err != nil {
		return nil, gerror.Wrap(err, "初始化OSS Bucket失败")
	}
	return bucket, nil
}

func newInternalOssBucket() (*oss.Bucket, error) {
	if config == nil {
		return nil, gerror.New("OSS存储配置不存在")
	}
	return newOssBucket(config.OssEndpoint)
}

// OssPublicEndpoint converts an Alibaba Cloud internal endpoint to the public
// endpoint used by browsers. Server-side operations keep using the configured
// endpoint so SAE can access OSS over the internal network.
func OssPublicEndpoint(endpoint string) (string, error) {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return "", gerror.New("OSS存储驱动缺少Endpoint配置")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", gerror.New("OSS Endpoint格式不正确")
	}
	parsed.Host = strings.Replace(parsed.Host, "-internal.aliyuncs.com", ".aliyuncs.com", 1)
	parsed.Path = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

// OssPresignedPutObject returns a public short-lived upload URL for browsers.
func OssPresignedPutObject(ctx context.Context, key, contentType string, expiry time.Duration) (string, error) {
	if config == nil {
		return "", gerror.New("OSS存储配置不存在")
	}
	endpoint := strings.TrimSpace(config.OssUploadURL)
	useCname := endpoint != ""
	if endpoint == "" {
		var err error
		endpoint, err = OssPublicEndpoint(config.OssEndpoint)
		if err != nil {
			return "", err
		}
	}
	bucket, err := newOssBucketWithCname(endpoint, useCname)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	return bucket.SignURL(key, oss.HTTPPut, int64(expiry/time.Second), oss.ContentType(contentType), oss.WithContext(ctx))
}

func OssHeadObject(ctx context.Context, key string) (*OssObjectInfo, error) {
	bucket, err := newInternalOssBucket()
	if err != nil {
		return nil, err
	}
	headers, err := bucket.GetObjectDetailedMeta(key, oss.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	size, err := strconv.ParseInt(headers.Get("Content-Length"), 10, 64)
	if err != nil {
		return nil, gerror.Wrap(err, "解析OSS对象大小失败")
	}
	return &OssObjectInfo{Size: size}, nil
}

func OssGetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	bucket, err := newInternalOssBucket()
	if err != nil {
		return nil, err
	}
	return bucket.GetObject(key, oss.WithContext(ctx))
}

func OssCopyObject(ctx context.Context, sourceKey, targetKey string) error {
	bucket, err := newInternalOssBucket()
	if err != nil {
		return err
	}
	_, err = bucket.CopyObject(sourceKey, targetKey, oss.WithContext(ctx))
	return err
}

func OssDeleteObject(ctx context.Context, key string) error {
	bucket, err := newInternalOssBucket()
	if err != nil {
		return err
	}
	return bucket.DeleteObject(key, oss.WithContext(ctx))
}

// DownloadOssObjectToFile downloads an object through the configured endpoint.
// Production should configure the same-region internal endpoint for SAE.
func DownloadOssObjectToFile(ctx context.Context, objectPath, targetPath string) error {
	objectPath = strings.TrimLeft(strings.TrimSpace(objectPath), "/")
	if objectPath == "" || strings.TrimSpace(targetPath) == "" {
		return gerror.New("OSS下载参数不能为空")
	}
	reader, err := OssGetObject(ctx, objectPath)
	if err != nil {
		return gerror.Wrap(err, "从OSS内网下载文件失败")
	}
	defer reader.Close()
	if err = os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return gerror.Wrap(err, "创建OSS下载目录失败")
	}
	partPath := targetPath + ".part"
	_ = os.Remove(partPath)
	file, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return gerror.Wrap(err, "创建OSS下载文件失败")
	}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(partPath)
		return gerror.Wrap(copyErr, "保存OSS下载文件失败")
	}
	if closeErr != nil {
		_ = os.Remove(partPath)
		return gerror.Wrap(closeErr, "关闭OSS下载文件失败")
	}
	if err = os.Rename(partPath, targetPath); err != nil {
		_ = os.Remove(partPath)
		return gerror.Wrap(err, "写入OSS下载文件失败")
	}
	return nil
}

// Upload 上传到阿里云oss
func (d *OssDrive) Upload(ctx context.Context, file *ghttp.UploadFile, meta *FileMeta) (fullPath string, err error) {
	if config == nil {
		return "", gerror.New("OSS存储配置不存在")
	}
	if config.OssPath == "" {
		err = gerror.New("OSS存储驱动必须配置存储路径!")
		return
	}

	// 流式上传本地小文件
	f2, err := file.Open()
	if err != nil {
		return
	}
	defer func() { _ = f2.Close() }()

	bucket, err := newOssBucket(config.OssEndpoint)
	if err != nil {
		return
	}

	fullPath, err = GenContentPath(config.OssPath, meta.Md5, meta.Ext)
	if err != nil {
		return "", err
	}
	contentType := strings.TrimSpace(meta.MimeType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	err = bucket.PutObject(fullPath, f2, oss.ContentType(contentType), oss.CacheControl("public, max-age=31536000, immutable"), oss.WithContext(ctx))
	return
}

// CreateMultipart 创建分片事件
func (d *OssDrive) CreateMultipart(ctx context.Context, in *CheckMultipartParams) (res *MultipartProgress, err error) {
	err = gerror.New("当前驱动暂不支持分片上传！")
	return
}

// UploadPart 上传分片
func (d *OssDrive) UploadPart(ctx context.Context, in *UploadPartParams) (res *UploadPartModel, err error) {
	err = gerror.New("当前驱动暂不支持分片上传！")
	return
}
