package storager

import (
	"context"
	"mime"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// R2Drive implements Cloudflare R2 through its S3-compatible API.
// The provider is deliberately separate from MinioDrive because R2 uses
// region=auto and its public URL is configured independently.
type R2Drive struct{}

// NewR2Client creates a client from the current upload configuration.
func NewR2Client() (*minio.Client, error) {
	endpoint, opts, err := r2ClientConfig()
	if err != nil {
		return nil, err
	}
	return minio.New(endpoint, opts)
}

// R2PresignedPutObject returns a short-lived upload URL.
func R2PresignedPutObject(ctx context.Context, key string, expiry time.Duration) (*url.URL, error) {
	client, err := NewR2Client()
	if err != nil {
		return nil, gerror.Wrap(err, "初始化R2客户端失败")
	}
	return client.PresignedPutObject(ctx, config.R2Bucket, key, expiry)
}

func R2HeadObject(ctx context.Context, key string) (minio.ObjectInfo, error) {
	client, err := NewR2Client()
	if err != nil {
		return minio.ObjectInfo{}, err
	}
	return client.StatObject(ctx, config.R2Bucket, key, minio.StatObjectOptions{})
}

func R2GetObject(ctx context.Context, key string) (*minio.Object, error) {
	client, err := NewR2Client()
	if err != nil {
		return nil, err
	}
	return client.GetObject(ctx, config.R2Bucket, key, minio.GetObjectOptions{})
}

func R2CopyObject(ctx context.Context, sourceKey, targetKey string) error {
	client, err := NewR2Client()
	if err != nil {
		return err
	}
	_, err = client.CopyObject(ctx, minio.CopyDestOptions{Bucket: config.R2Bucket, Object: targetKey}, minio.CopySrcOptions{Bucket: config.R2Bucket, Object: sourceKey})
	return err
}

func R2DeleteObject(ctx context.Context, key string) error {
	client, err := NewR2Client()
	if err != nil {
		return err
	}
	return client.RemoveObject(ctx, config.R2Bucket, key, minio.RemoveObjectOptions{})
}

func r2ClientConfig() (string, *minio.Options, error) {
	if config == nil {
		return "", nil, gerror.New("R2存储配置不存在")
	}
	endpoint, secure, err := normalizeR2Endpoint(config.R2Endpoint)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(config.R2Bucket) == "" {
		return "", nil, gerror.New("R2存储驱动缺少Bucket配置")
	}
	if strings.TrimSpace(config.R2AccessKey) == "" || strings.TrimSpace(config.R2SecretKey) == "" {
		return "", nil, gerror.New("R2存储驱动缺少Access Key或Secret Key配置")
	}
	region := strings.TrimSpace(config.R2Region)
	if region == "" {
		region = "auto"
	}
	return endpoint, &minio.Options{Creds: credentials.NewStaticV4(strings.TrimSpace(config.R2AccessKey), strings.TrimSpace(config.R2SecretKey), ""), Secure: secure, Region: region}, nil
}

func normalizeR2Endpoint(raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, gerror.New("R2存储驱动缺少Endpoint配置")
	}
	if !strings.Contains(raw, "://") {
		return strings.TrimSuffix(raw, "/"), true, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", false, gerror.New("R2 Endpoint格式不正确")
	}
	return u.Host, u.Scheme == "https", nil
}

func (d *R2Drive) objectPath(meta *FileMeta) (string, error) {
	return GenContentPath(config.CosPath, meta.Md5, meta.Ext)
}

func (d *R2Drive) Upload(ctx context.Context, file *ghttp.UploadFile, meta *FileMeta) (string, error) {
	if strings.TrimSpace(config.CosPath) == "" {
		return "", gerror.New("R2存储驱动必须配置存储路径")
	}
	endpoint, opts, err := r2ClientConfig()
	if err != nil {
		return "", err
	}
	client, err := minio.New(endpoint, opts)
	if err != nil {
		return "", gerror.Wrap(err, "初始化R2客户端失败")
	}
	key, err := d.objectPath(meta)
	if err != nil {
		return "", err
	}
	reader, err := file.Open()
	if err != nil {
		return "", gerror.Wrap(err, "读取R2上传文件失败")
	}
	defer reader.Close()
	contentType := mime.TypeByExtension(filepath.Ext(file.Filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = client.PutObject(ctx, config.R2Bucket, key, reader, file.Size, minio.PutObjectOptions{ContentType: contentType, CacheControl: "public, max-age=31536000, immutable"})
	return key, err
}

func (d *R2Drive) CreateMultipart(ctx context.Context, in *CheckMultipartParams) (*MultipartProgress, error) {
	endpoint, opts, err := r2ClientConfig()
	if err != nil {
		return nil, err
	}
	core, err := minio.NewCore(endpoint, opts)
	if err != nil {
		return nil, gerror.Wrap(err, "初始化R2分片客户端失败")
	}
	key, err := d.objectPath(in.meta)
	if err != nil {
		return nil, err
	}
	uploadID, err := core.NewMultipartUpload(ctx, config.R2Bucket, key, minio.PutObjectOptions{})
	if err != nil {
		return nil, gerror.Wrap(err, "创建R2分片上传失败")
	}
	res := &MultipartProgress{UploadId: GenUploadId(ctx, in.Md5), ThirdUploadId: uploadID, ObjectPath: key, Meta: in.meta, ShardCount: in.ShardCount, UploadedIndex: []int{}}
	if err = CreateMultipartProgress(ctx, res); err != nil {
		_ = core.AbortMultipartUpload(ctx, config.R2Bucket, key, uploadID)
		return nil, err
	}
	return res, nil
}

func (d *R2Drive) UploadPart(ctx context.Context, in *UploadPartParams) (*UploadPartModel, error) {
	endpoint, opts, err := r2ClientConfig()
	if err != nil {
		return nil, err
	}
	core, err := minio.NewCore(endpoint, opts)
	if err != nil {
		return nil, gerror.Wrap(err, "初始化R2分片客户端失败")
	}
	part, err := in.File.Open()
	if err != nil {
		return nil, gerror.Wrap(err, "读取R2分片失败")
	}
	defer part.Close()
	_, err = core.PutObjectPart(ctx, config.R2Bucket, in.mp.ObjectPath, in.mp.ThirdUploadId, in.Index, part, in.File.Size, minio.PutObjectPartOptions{})
	if err != nil {
		return nil, gerror.Wrap(err, "上传R2分片失败")
	}
	for _, uploaded := range in.mp.UploadedIndex {
		if uploaded == in.Index {
			return &UploadPartModel{Progress: CalcUploadProgress(in.mp.UploadedIndex, in.mp.ShardCount)}, nil
		}
	}
	in.mp.UploadedIndex = append(in.mp.UploadedIndex, in.Index)
	if len(in.mp.UploadedIndex) > in.mp.ShardCount {
		return nil, gerror.New("R2分片编号超过预期数量")
	}
	if len(in.mp.UploadedIndex) < in.mp.ShardCount {
		_ = UpdateMultipartProgress(ctx, in.mp)
		return &UploadPartModel{Progress: CalcUploadProgress(in.mp.UploadedIndex, in.mp.ShardCount)}, nil
	}
	listed, err := core.ListObjectParts(ctx, config.R2Bucket, in.mp.ObjectPath, in.mp.ThirdUploadId, 0, 10000)
	if err != nil {
		return nil, err
	}
	parts := make([]minio.CompletePart, 0, len(listed.ObjectParts))
	for _, p := range listed.ObjectParts {
		parts = append(parts, minio.CompletePart{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	// S3 requires parts in ascending order and each expected part exactly once.
	for i, part := range parts {
		if part.PartNumber != i+1 {
			return nil, gerror.New("R2分片编号不连续，无法合并")
		}
	}
	if len(parts) != in.mp.ShardCount {
		return nil, gerror.Newf("R2分片数量不完整，预期:%d 实际:%d", in.mp.ShardCount, len(parts))
	}
	if _, err = core.CompleteMultipartUpload(ctx, config.R2Bucket, in.mp.ObjectPath, in.mp.ThirdUploadId, parts, minio.PutObjectOptions{}); err != nil {
		return nil, gerror.Wrap(err, "合并R2分片失败")
	}
	_ = DelMultipartProgress(ctx, in.mp)
	return &UploadPartModel{Progress: 100, Finish: true}, nil
}
