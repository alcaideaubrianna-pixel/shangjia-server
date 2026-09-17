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
	"time"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/internal/library/cache"
)

const botMediaFingerprintCacheTTL = 90 * 24 * time.Hour

type mediaFingerprint struct {
	MD5   string
	PHash *goimagehash.ImageHash
}

type cachedBotMediaFingerprint struct {
	MD5   string `json:"md5"`
	PHash uint64 `json:"pHash"`
}

func cachedTelegramImageFingerprint(ctx context.Context, fileUniqueId string, imageURL string) (*mediaFingerprint, bool, error) {
	fileUniqueId = strings.TrimSpace(fileUniqueId)
	if fileUniqueId != "" {
		value, err := cache.Instance().Get(ctx, botMediaFingerprintCacheKey(fileUniqueId))
		if err == nil && !value.IsNil() {
			var stored cachedBotMediaFingerprint
			if scanErr := value.Scan(&stored); scanErr == nil && stored.PHash != 0 {
				return &mediaFingerprint{
					MD5: stored.MD5, PHash: goimagehash.NewImageHash(stored.PHash, goimagehash.PHash),
				}, true, nil
			}
		}
	}
	fingerprint, err := cachedRemoteImageFingerprint(ctx, imageURL)
	if err != nil {
		return nil, false, err
	}
	if fileUniqueId != "" && fingerprint != nil && fingerprint.PHash != nil {
		_ = cache.Instance().Set(ctx, botMediaFingerprintCacheKey(fileUniqueId), cachedBotMediaFingerprint{
			MD5: fingerprint.MD5, PHash: fingerprint.PHash.GetHash(),
		}, botMediaFingerprintCacheTTL)
	}
	return fingerprint, false, nil
}

func botMediaFingerprintCacheKey(fileUniqueId string) string {
	return "youban_publish:bot_media_fingerprint:v1:" + strings.TrimSpace(fileUniqueId)
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
