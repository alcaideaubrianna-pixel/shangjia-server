package sys

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"
)

type mediaFingerprint struct {
	MD5   string
	PHash *goimagehash.ImageHash
}

func uploadImageFingerprint(file *ghttp.UploadFile) (*mediaFingerprint, error) {
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, gerror.Wrap(err, "读取上传图片失败")
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, gerror.Wrap(err, "读取上传图片内容失败")
	}
	hash, err := imagePHashFromBytes(content)
	if err != nil {
		return nil, err
	}
	return &mediaFingerprint{MD5: md5Hex(content), PHash: hash}, nil
}

func cachedRemoteImageFingerprint(ctx context.Context, imageURL string) (*mediaFingerprint, error) {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return nil, gerror.New("请发送要搜索的图片")
	}
	path, err := cachedRemoteMediaFile(ctx, mediaFileCacheKey(nil, imageURL), imageURL, mediaFileCacheExt(&telegramMediaItem{MediaType: "image", FileUrl: imageURL}, imageURL))
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, gerror.Wrap(err, "读取缓存图片失败")
	}
	hash, err := imagePHashFromBytes(content)
	if err != nil {
		return nil, err
	}
	return &mediaFingerprint{MD5: md5Hex(content), PHash: hash}, nil
}

func imagePHashFromBytes(content []byte) (*goimagehash.ImageHash, error) {
	if len(content) == 0 {
		return nil, gerror.New("图片文件为空")
	}
	img, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, gerror.New("图片格式不支持，请上传 JPG、PNG 或 GIF")
	}
	hash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return nil, gerror.Wrap(err, "计算图片感知哈希失败")
	}
	return hash, nil
}

func md5Hex(content []byte) string {
	sum := md5.Sum(content)
	return hex.EncodeToString(sum[:])
}
