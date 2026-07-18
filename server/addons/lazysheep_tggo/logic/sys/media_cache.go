package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"hotgo/internal/dao"
	"hotgo/internal/model/entity"
	isysin "hotgo/internal/model/input/sysin"
)

const (
	quickMediaCacheDir          = "storage/lazysheep_tggo/media_cache"
	quickMediaCacheMaxBytes     = int64(10 << 30)
	quickMediaCacheTargetBytes  = int64(8 << 30)
	quickMediaCacheMaxAge       = 72 * time.Hour
	quickMediaCacheCleanSpacing = 10 * time.Minute
)

var quickMediaCacheLastCleanUnix int64

func cachedAttachmentBySourceURL(ctx context.Context, rawURL string) (*isysin.AttachmentListModel, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, nil
	}
	cols := dao.AddonLazysheepTggoNoteAsset.Columns()
	var asset *entity.AddonLazysheepTggoNoteAsset
	if err := dao.AddonLazysheepTggoNoteAsset.Ctx(ctx).
		Where(cols.SourceUrl, rawURL).
		WhereGT(cols.AttachmentId, 0).
		WhereNot(cols.PreviewUrl, "").
		OrderDesc(cols.Id).
		Scan(&asset); err != nil {
		return nil, gerror.Wrap(err, "查询媒体缓存失败")
	}
	if asset == nil || asset.AttachmentId <= 0 || strings.TrimSpace(asset.PreviewUrl) == "" {
		return nil, nil
	}
	return &isysin.AttachmentListModel{
		SysAttachment: entity.SysAttachment{
			Id:      asset.AttachmentId,
			FileUrl: asset.PreviewUrl,
			Path:    asset.LocalPath,
			Size:    asset.FileSize,
		},
	}, nil
}

func downloadCachedMedia(ctx context.Context, rawURL, itemType string, index int) (filename string, data []byte, contentType string, err error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil, "", fmt.Errorf("媒体地址为空")
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return "", nil, "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", nil, "", fmt.Errorf("仅支持 HTTP/HTTPS 媒体链接")
	}
	filename = quickMediaFilename(rawURL, itemType, index)
	cachePath := mediaCachePath(rawURL, itemType, filename)
	if gfile.Exists(cachePath) && gfile.Size(cachePath) > 0 {
		data = gfile.GetBytes(cachePath)
		if len(data) > 0 {
			_ = os.Chtimes(cachePath, time.Now(), time.Now())
			g.Log().Debugf(ctx, "命中 BangChat 媒体本地缓存 url:%s path:%s", rawURL, cachePath)
			return filename, data, mimeFromBytes(data), nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "image/*,video/*,*/*;q=0.8")
	resp, err := quickMediaHTTPClient.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", nil, "", fmt.Errorf("下载媒体失败，HTTP状态：%d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, quickMediaMaxBytes+1))
	if err != nil {
		return "", nil, "", err
	}
	if len(body) == 0 {
		return "", nil, "", fmt.Errorf("媒体内容为空")
	}
	if len(body) > quickMediaMaxBytes {
		return "", nil, "", fmt.Errorf("媒体文件超过 %dMB", quickMediaMaxBytes>>20)
	}
	body = decodeBangchatMedia(rawURL, body)
	if err = gfile.Mkdir(gfile.Dir(cachePath)); err != nil {
		g.Log().Warningf(ctx, "创建 BangChat 媒体缓存目录失败 path:%s err:%+v", cachePath, err)
	} else if err = gfile.PutBytes(cachePath, body); err != nil {
		g.Log().Warningf(ctx, "写入 BangChat 媒体缓存失败 path:%s err:%+v", cachePath, err)
	} else {
		maybeCleanQuickMediaCache(ctx)
	}
	return filename, body, resp.Header.Get("Content-Type"), nil
}

func mediaCachePath(rawURL, itemType, filename string) string {
	sum := sha256.Sum256([]byte(rawURL))
	ext := strings.ToLower(path.Ext(filename))
	if ext == "" {
		if itemType == noteTypeVideo {
			ext = ".mp4"
		} else {
			ext = ".jpg"
		}
	}
	return gfile.Join(quickMediaCacheDir, hex.EncodeToString(sum[:])+ext)
}

type quickMediaCacheFile struct {
	path    string
	size    int64
	modTime time.Time
}

func maybeCleanQuickMediaCache(ctx context.Context) {
	now := time.Now()
	last := atomic.LoadInt64(&quickMediaCacheLastCleanUnix)
	if last > 0 && now.Sub(time.Unix(last, 0)) < quickMediaCacheCleanSpacing {
		return
	}
	if !atomic.CompareAndSwapInt64(&quickMediaCacheLastCleanUnix, last, now.Unix()) {
		return
	}
	go cleanQuickMediaCache(context.WithoutCancel(ctx))
}

func cleanQuickMediaCache(ctx context.Context) {
	entries, err := os.ReadDir(quickMediaCacheDir)
	if err != nil {
		if !os.IsNotExist(err) {
			g.Log().Warningf(ctx, "读取 BangChat 媒体缓存目录失败 dir:%s err:%+v", quickMediaCacheDir, err)
		}
		return
	}
	now := time.Now()
	files := make([]quickMediaCacheFile, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := gfile.Join(quickMediaCacheDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > quickMediaCacheMaxAge {
			if err = os.Remove(path); err != nil {
				g.Log().Warningf(ctx, "清理过期 BangChat 媒体缓存失败 path:%s err:%+v", path, err)
			}
			continue
		}
		size := info.Size()
		total += size
		files = append(files, quickMediaCacheFile{path: path, size: size, modTime: info.ModTime()})
	}
	if total <= quickMediaCacheMaxBytes {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})
	for _, file := range files {
		if total <= quickMediaCacheTargetBytes {
			break
		}
		if err = os.Remove(file.path); err != nil {
			g.Log().Warningf(ctx, "清理 BangChat 媒体缓存失败 path:%s err:%+v", file.path, err)
			continue
		}
		total -= file.size
	}
	g.Log().Infof(ctx, "BangChat 媒体缓存清理完成 remainBytes:%d", total)
}
