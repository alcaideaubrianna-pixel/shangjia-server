// Package storager
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package storager

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// LocalDrive 本地驱动
type LocalDrive struct {
}

// Upload 上传到本地
func (d *LocalDrive) Upload(ctx context.Context, file *ghttp.UploadFile, meta *FileMeta) (fullPath string, err error) {
	sp := g.Cfg().MustGet(ctx, "server.serverRoot")

	if sp.IsEmpty() {
		err = gerror.New("本地上传驱动必须配置静态路径!")
		return
	}

	if config.LocalPath == "" {
		err = gerror.New("本地上传驱动必须配置本地存储路径!")
		return
	}

	fullPath, err = GenContentPath(config.LocalPath, meta.Md5, meta.Ext)
	if err != nil {
		return "", err
	}
	targetPath := filepath.Join(strings.TrimRight(sp.String(), "/"), filepath.FromSlash(fullPath))
	if err = os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", gerror.Wrap(err, "创建本地上传目录失败")
	}
	if gfile.Exists(targetPath) {
		return fullPath, nil
	}
	source, err := file.Open()
	if err != nil {
		return "", gerror.Wrap(err, "读取上传文件失败")
	}
	defer source.Close()
	target, err := os.CreateTemp(filepath.Dir(targetPath), ".upload-*.part")
	if err != nil {
		return "", gerror.Wrap(err, "创建本地上传文件失败")
	}
	partPath := target.Name()
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(partPath)
		if copyErr != nil {
			return "", gerror.Wrap(copyErr, "保存本地上传文件失败")
		}
		return "", gerror.Wrap(closeErr, "关闭本地上传文件失败")
	}
	if err = os.Rename(partPath, targetPath); err != nil {
		_ = os.Remove(partPath)
		return "", gerror.Wrap(err, "写入本地上传文件失败")
	}
	return fullPath, nil
}

// CreateMultipart 创建分片事件
func (d *LocalDrive) CreateMultipart(ctx context.Context, in *CheckMultipartParams) (mp *MultipartProgress, err error) {
	mp = new(MultipartProgress)
	mp.UploadId = GenUploadId(ctx, in.Md5)
	mp.Meta = in.meta
	mp.ShardCount = in.ShardCount
	mp.UploadedIndex = make([]int, 0)
	mp.CreatedAt = gtime.Now()
	if err = CreateMultipartProgress(ctx, mp); err != nil {
		return nil, err
	}
	return
}

// UploadPart 上传分片
func (d *LocalDrive) UploadPart(ctx context.Context, in *UploadPartParams) (res *UploadPartModel, err error) {
	sp := g.Cfg().MustGet(ctx, "server.serverRoot")
	if sp.IsEmpty() {
		err = gerror.New("本地上传驱动必须配置静态路径!")
		return
	}

	spStr := strings.TrimRight(sp.String(), "/") + "/"

	if config.LocalPath == "" {
		err = gerror.New("本地上传驱动必须配置本地存储路径!")
		return
	}

	// 分片文件存放路径
	partFilePath := spStr + config.LocalPath + "tmp/" + in.Md5

	// 写入文件
	in.File.Filename = gconv.String(in.Index)
	if _, err = in.File.Save(partFilePath, false); err != nil {
		return
	}

	// 更新上传进度
	in.mp.UploadedIndex = append(in.mp.UploadedIndex, in.Index)
	if err = UpdateMultipartProgress(ctx, in.mp); err != nil {
		return nil, err
	}

	res = new(UploadPartModel)

	// 已全部上传完毕
	if len(in.mp.UploadedIndex) == in.mp.ShardCount {
		// 删除进度统计
		if err = DelMultipartProgress(ctx, in.mp); err != nil {
			return nil, err
		}

		// 合并文件
		finalDirPath, pathErr := GenContentPath(config.LocalPath, in.mp.Meta.Md5, in.mp.Meta.Ext)
		if pathErr != nil {
			return nil, pathErr
		}
		if err = os.MkdirAll(filepath.Dir(spStr+finalDirPath), 0o755); err != nil {
			return nil, gerror.Wrap(err, "创建本地分片上传目录失败")
		}
		finalPath := filepath.Clean(spStr + finalDirPath)
		if gfile.Exists(finalPath) {
			if err = os.RemoveAll(partFilePath); err != nil {
				return nil, gerror.Wrap(err, "删除临时分片失败")
			}
		} else {
			merged, createErr := os.CreateTemp(filepath.Dir(finalPath), ".multipart-*.part")
			if createErr != nil {
				return nil, gerror.Wrap(createErr, "创建分片合并临时文件失败")
			}
			mergedPath := merged.Name()
			if closeErr := merged.Close(); closeErr != nil {
				_ = os.Remove(mergedPath)
				return nil, gerror.Wrap(closeErr, "关闭分片合并临时文件失败")
			}
			if err = MergePartFile(partFilePath, mergedPath); err != nil {
				_ = os.Remove(mergedPath)
				err = gerror.Newf("合并分片文件出错:%v", err.Error())
				return nil, err
			}
			if err = os.Rename(mergedPath, finalPath); err != nil {
				_ = os.Remove(mergedPath)
				return nil, gerror.Wrap(err, "写入分片上传文件失败")
			}
		}

		// 删除临时分片
		if gfile.Exists(partFilePath) {
			if err = os.RemoveAll(partFilePath); err != nil {
				err = gerror.Newf("删除临时分片文件出错:%v", err.Error())
				return nil, err
			}
		}

		// 写入附件记录
		attachment, err := write(ctx, in.mp.Meta, finalDirPath)
		if err != nil {
			return nil, err
		}

		res.Finish = true
		res.Progress = 100
		res.Attachment = attachment
		return res, nil
	}

	// 计算上传进度
	res.Progress = CalcUploadProgress(in.mp.UploadedIndex, in.mp.ShardCount)
	return
}

// MergePartFile 合并分片文件
func MergePartFile(srcPath, dstPath string) (err error) {
	dir, err := os.ReadDir(srcPath)
	if err != nil {
		return err
	}
	sort.Slice(dir, func(i, j int) bool {
		fiIndex, _ := strconv.Atoi(dir[i].Name())
		fjIndex, _ := strconv.Atoi(dir[j].Name())
		return fiIndex < fjIndex
	})
	for _, file := range dir {
		filePath := filepath.Join(srcPath, file.Name())
		if err = gfile.PutBytesAppend(dstPath, gfile.GetBytes(filePath)); err != nil {
			return err
		}
	}
	return
}
