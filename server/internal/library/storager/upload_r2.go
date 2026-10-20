package storager

import (
	"context"
	"mime"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// R2Drive implements Cloudflare R2 through its S3-compatible API.
// The provider is deliberately separate from MinioDrive because R2 uses
// region=auto and its public URL is configured independently.
type R2Drive struct{}

func r2Options() (*minio.Options, error) {
	if config == nil || strings.TrimSpace(config.R2Endpoint) == "" || config.R2Bucket == "" {
		return nil, gerror.New("R2存储驱动缺少Endpoint或Bucket配置")
	}
	region := config.R2Region
	if region == "" {
		region = "auto"
	}
	return &minio.Options{Creds: credentials.NewStaticV4(config.R2AccessKey, config.R2SecretKey, ""), Secure: true, Region: region}, nil
}

func (d *R2Drive) objectPath(meta *FileMeta) (string, error) {
	return GenContentPath(config.R2Path, meta.Md5, meta.Ext)
}

func (d *R2Drive) Upload(ctx context.Context, file *ghttp.UploadFile, meta *FileMeta) (string, error) {
	if strings.TrimSpace(config.R2Path) == "" {
		return "", gerror.New("R2存储驱动必须配置存储路径")
	}
	opts, err := r2Options()
	if err != nil {
		return "", err
	}
	client, err := minio.New(config.R2Endpoint, opts)
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
	opts, err := r2Options()
	if err != nil {
		return nil, err
	}
	core, err := minio.NewCore(config.R2Endpoint, opts)
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
	opts, err := r2Options()
	if err != nil {
		return nil, err
	}
	core, err := minio.NewCore(config.R2Endpoint, opts)
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
