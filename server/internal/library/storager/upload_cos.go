// Package storager
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package storager

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/tencentyun/cos-go-sdk-v5"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// CosDrive 腾讯云cos驱动
type CosDrive struct {
}

func newCosClient() (*cos.Client, error) {
	if strings.TrimSpace(config.CosBucketURL) == "" {
		return nil, gerror.New("COS存储驱动必须配置Bucket访问域名!")
	}

	bucketURL, err := url.Parse(strings.TrimSpace(config.CosBucketURL))
	if err != nil {
		return nil, gerror.Wrap(err, "COS Bucket访问域名配置不正确")
	}
	return cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  config.CosSecretId,
			SecretKey: config.CosSecretKey,
		},
	}), nil
}

// Upload 上传到腾讯云cos对象存储
func (d *CosDrive) Upload(ctx context.Context, file *ghttp.UploadFile) (fullPath string, err error) {
	if config.CosPath == "" {
		err = gerror.New("COS存储驱动必须配置存储路径!")
		return
	}
	if strings.TrimSpace(config.CosBucketURL) == "" {
		err = gerror.New("COS存储驱动必须配置Bucket访问域名!")
		return
	}

	// 流式上传本地小文件
	f2, err := file.Open()
	if err != nil {
		return
	}
	defer func() { _ = f2.Close() }()

	client, err := newCosClient()
	if err != nil {
		return "", err
	}

	fullPath = GenFullPath(config.CosPath, gfile.Ext(file.Filename))
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			CacheControl: "public, max-age=31536000, immutable",
		},
	}
	if contentType := mime.TypeByExtension(gfile.Ext(file.Filename)); contentType != "" {
		opt.ContentType = contentType
	}
	_, err = client.Object.Put(ctx, fullPath, f2, opt)
	return
}

// CreateMultipart 创建分片事件
func (d *CosDrive) CreateMultipart(ctx context.Context, in *CheckMultipartParams) (res *MultipartProgress, err error) {
	if config.CosPath == "" {
		return nil, gerror.New("COS存储驱动必须配置存储路径!")
	}
	client, err := newCosClient()
	if err != nil {
		return nil, err
	}

	fullPath := GenFullPath(config.CosPath, gfile.Ext(in.meta.Filename))
	opt := &cos.InitiateMultipartUploadOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			CacheControl: "public, max-age=31536000, immutable",
		},
	}
	if contentType := mime.TypeByExtension(gfile.Ext(in.meta.Filename)); contentType != "" {
		opt.ContentType = contentType
	}
	created, _, err := client.Object.InitiateMultipartUpload(ctx, fullPath, opt)
	if err != nil {
		return nil, gerror.Wrap(err, "创建COS分片上传失败")
	}

	res = &MultipartProgress{
		UploadId:      GenUploadId(ctx, in.Md5),
		ThirdUploadId: created.UploadID,
		ObjectPath:    fullPath,
		Meta:          in.meta,
		ShardCount:    in.ShardCount,
		UploadedIndex: make([]int, 0),
		CreatedAt:     gtime.Now(),
	}
	if err = CreateMultipartProgress(ctx, res); err != nil {
		_, _ = client.Object.AbortMultipartUpload(ctx, fullPath, created.UploadID)
		return nil, err
	}
	return res, nil
}

// UploadPart 上传分片
func (d *CosDrive) UploadPart(ctx context.Context, in *UploadPartParams) (res *UploadPartModel, err error) {
	client, err := newCosClient()
	if err != nil {
		return nil, err
	}
	part, err := in.File.Open()
	if err != nil {
		return nil, gerror.Wrap(err, "读取上传分片失败")
	}
	defer func() { _ = part.Close() }()

	_, err = client.Object.UploadPart(ctx, in.mp.ObjectPath, in.mp.ThirdUploadId, in.Index, part, &cos.ObjectUploadPartOptions{
		ContentLength: in.File.Size,
	})
	if err != nil {
		return nil, gerror.Wrap(err, "上传COS分片失败")
	}

	in.mp.UploadedIndex = append(in.mp.UploadedIndex, in.Index)
	sort.Ints(in.mp.UploadedIndex)
	res = new(UploadPartModel)
	if len(in.mp.UploadedIndex) < in.mp.ShardCount {
		if err = UpdateMultipartProgress(ctx, in.mp); err != nil {
			return nil, err
		}
		res.Progress = CalcUploadProgress(in.mp.UploadedIndex, in.mp.ShardCount)
		return res, nil
	}

	listed, _, err := client.Object.ListParts(ctx, in.mp.ObjectPath, in.mp.ThirdUploadId, nil)
	if err != nil {
		return nil, gerror.Wrap(err, "查询COS上传分片失败")
	}
	if len(listed.Parts) != in.mp.ShardCount {
		return nil, gerror.Newf("COS分片数量不完整，预期:%d 实际:%d", in.mp.ShardCount, len(listed.Parts))
	}
	sort.Slice(listed.Parts, func(i, j int) bool {
		return listed.Parts[i].PartNumber < listed.Parts[j].PartNumber
	})
	_, _, err = client.Object.CompleteMultipartUpload(ctx, in.mp.ObjectPath, in.mp.ThirdUploadId, &cos.CompleteMultipartUploadOptions{
		Parts: listed.Parts,
	})
	if err != nil {
		return nil, gerror.Wrap(err, "合并COS上传分片失败")
	}

	attachment, err := write(ctx, in.mp.Meta, in.mp.ObjectPath)
	if err != nil {
		return nil, err
	}
	if err = DelMultipartProgress(ctx, in.mp); err != nil {
		return nil, err
	}
	res.Attachment = attachment
	res.Progress = 100
	res.Finish = true
	return res, nil
}
