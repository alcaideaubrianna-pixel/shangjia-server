package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/sync/singleflight"

	"hotgo/internal/library/storager"
)

const (
	mediaFileCacheDefaultDir      = "storage/cache/youban_publish/media"
	mediaFileCacheDefaultMaxBytes = int64(1 << 30)
	mediaFileCacheTargetRatio     = 0.9
)

var mediaFileCacheDownloadGroup singleflight.Group
var mediaFileCacheGenerateGroup singleflight.Group

type mediaFileCacheMeta struct {
	Key          string `json:"key"`
	Source       string `json:"source"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	CreatedAt    int64  `json:"createdAt"`
	LastAccessAt int64  `json:"lastAccessAt"`
}

type mediaFileCacheEntry struct {
	metaPath string
	filePath string
	size     int64
	lastUse  int64
}

func cachedTelegramMediaFile(ctx context.Context, media *telegramMediaItem) (string, func(), error) {
	if media == nil {
		return "", nil, gerror.New("媒体文件为空")
	}
	if path := strings.TrimSpace(media.StoragePath); path != "" {
		localPath := resolveTelegramLocalPath(path)
		if fileExists(localPath) {
			return localPath, nil, nil
		}
	}
	if path := localTelegramFileURLPath(media.FileUrl); path != "" {
		localPath := resolveTelegramLocalPath(path)
		if fileExists(localPath) {
			return localPath, nil, nil
		}
	}
	source, err := mediaFileCacheRemoteSource(ctx, media)
	if err != nil {
		return "", nil, err
	}
	if source == "" {
		return "", nil, nil
	}
	path, err := cachedRemoteMediaFile(ctx, mediaFileCacheKey(media, source), source, mediaFileCacheExt(media, source))
	return path, nil, err
}

func mediaFileCacheRemoteSource(ctx context.Context, media *telegramMediaItem) (string, error) {
	if media == nil {
		return "", nil
	}
	source := strings.TrimSpace(media.FileUrl)
	if strings.HasPrefix(strings.ToLower(source), "http") {
		return source, nil
	}
	if media.AttachmentId <= 0 {
		return "", nil
	}
	row, err := g.DB().Model(sysAttachmentTable).Safe().Ctx(ctx).Fields("file_url,path,drive").Where("id", media.AttachmentId).One()
	if err != nil {
		return "", gerror.Wrap(err, "读取资源文件地址失败")
	}
	if row == nil || row.IsEmpty() {
		return "", nil
	}
	fullPath := firstNonEmpty(row["file_url"].String(), row["path"].String())
	if strings.TrimSpace(fullPath) == "" {
		return "", nil
	}
	return storager.LastUrl(ctx, fullPath, row["drive"].String()), nil
}

func cachedRemoteMediaFile(ctx context.Context, key string, source string, ext string) (string, error) {
	return cachedRemoteMediaFileWithMetaSourceAndDownloader(ctx, key, source, source, ext, nil)
}

func cachedRemoteMediaFileWithMetaSource(ctx context.Context, key string, source string, metaSource string, ext string) (string, error) {
	return cachedRemoteMediaFileWithMetaSourceAndDownloader(ctx, key, source, metaSource, ext, nil)
}

func cachedRemoteMediaFileWithMetaSourceAndDownloader(ctx context.Context, key string, source string, metaSource string, ext string, downloader func(context.Context, string, string) error) (string, error) {
	key = strings.TrimSpace(key)
	source = strings.TrimSpace(source)
	metaSource = strings.TrimSpace(metaSource)
	if key == "" || source == "" {
		return "", nil
	}
	if metaSource == "" {
		metaSource = source
	}
	dir := mediaFileCacheDir(ctx)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", gerror.Wrap(err, "创建媒体缓存目录失败")
	}
	filePath := mediaFileCachePath(dir, key, ext)
	metaPath := filePath + ".json"
	if fileExists(filePath) {
		if err := touchMediaFileCacheMeta(metaPath, key, metaSource, filePath); err != nil {
			g.Log().Warningf(ctx, "更新媒体缓存访问时间失败 path:%s err:%+v", filePath, err)
		}
		return filePath, nil
	}
	value, err, _ := mediaFileCacheDownloadGroup.Do(key, func() (interface{}, error) {
		if fileExists(filePath) {
			if err := touchMediaFileCacheMeta(metaPath, key, metaSource, filePath); err != nil {
				g.Log().Warningf(ctx, "更新媒体缓存访问时间失败 path:%s err:%+v", filePath, err)
			}
			return filePath, nil
		}
		if downloader == nil {
			downloader = downloadMediaFileCache
		}
		if err := downloader(ctx, source, filePath); err != nil {
			return "", err
		}
		if err := touchMediaFileCacheMeta(metaPath, key, metaSource, filePath); err != nil {
			return "", err
		}
		if err := pruneMediaFileCache(ctx); err != nil {
			g.Log().Warningf(ctx, "清理媒体文件缓存失败: %+v", err)
		}
		return filePath, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func cachedGeneratedMediaFile(ctx context.Context, key string, source string, ext string, generate func(string) error) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || generate == nil {
		return "", gerror.New("生成媒体缓存参数不完整")
	}
	dir := mediaFileCacheDir(ctx)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", gerror.Wrap(err, "创建媒体缓存目录失败")
	}
	filePath := mediaFileCachePath(dir, key, ext)
	metaPath := filePath + ".json"
	if fileExists(filePath) {
		_ = touchMediaFileCacheMeta(metaPath, key, source, filePath)
		return filePath, nil
	}
	value, err, _ := mediaFileCacheGenerateGroup.Do(key, func() (interface{}, error) {
		if fileExists(filePath) {
			_ = touchMediaFileCacheMeta(metaPath, key, source, filePath)
			return filePath, nil
		}
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return "", err
		}
		partPath := filePath + ".part"
		_ = os.Remove(partPath)
		if err := generate(partPath); err != nil {
			_ = os.Remove(partPath)
			return "", err
		}
		if err := os.Rename(partPath, filePath); err != nil {
			_ = os.Remove(partPath)
			return "", err
		}
		if err := touchMediaFileCacheMeta(metaPath, key, source, filePath); err != nil {
			return "", err
		}
		if err := pruneMediaFileCache(ctx); err != nil {
			g.Log().Warningf(ctx, "清理媒体文件缓存失败: %+v", err)
		}
		return filePath, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func downloadMediaFileCache(ctx context.Context, source string, filePath string) error {
	return downloadMediaFileCacheWithClient(ctx, &http.Client{Timeout: 2 * time.Minute}, source, filePath)
}

func downloadMediaFileCacheWithClient(ctx context.Context, client *http.Client, source string, filePath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return gerror.Wrap(err, "创建远程媒体下载请求失败")
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return gerror.Wrap(err, "下载远程媒体失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return gerror.Newf("下载远程媒体失败：HTTP %d", resp.StatusCode)
	}
	if err = os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return gerror.Wrap(err, "创建媒体缓存子目录失败")
	}
	partPath := filePath + ".part"
	_ = os.Remove(partPath)
	file, err := os.Create(partPath)
	if err != nil {
		return gerror.Wrap(err, "创建媒体缓存临时文件失败")
	}
	if _, err = io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(partPath)
		return gerror.Wrap(err, "保存媒体缓存临时文件失败")
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(partPath)
		return gerror.Wrap(err, "关闭媒体缓存临时文件失败")
	}
	if err = os.Rename(partPath, filePath); err != nil {
		_ = os.Remove(partPath)
		return gerror.Wrap(err, "写入媒体缓存文件失败")
	}
	return nil
}

func touchMediaFileCacheMeta(metaPath string, key string, source string, filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	meta := mediaFileCacheMeta{
		Key:          key,
		Source:       source,
		Path:         filePath,
		Size:         info.Size(),
		CreatedAt:    now,
		LastAccessAt: now,
	}
	if existing, err := readMediaFileCacheMeta(metaPath); err == nil && existing != nil && existing.CreatedAt > 0 {
		meta.CreatedAt = existing.CreatedAt
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0o644)
}

func readMediaFileCacheMeta(path string) (*mediaFileCacheMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta mediaFileCacheMeta
	if err = json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func pruneMediaFileCache(ctx context.Context) error {
	dir := mediaFileCacheDir(ctx)
	maxBytes := mediaFileCacheMaxBytes(ctx)
	if maxBytes <= 0 {
		return nil
	}
	entries, total, err := scanMediaFileCache(dir)
	if err != nil {
		return err
	}
	if total <= maxBytes {
		return nil
	}
	target := int64(float64(maxBytes) * mediaFileCacheTargetRatio)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].lastUse == entries[j].lastUse {
			return entries[i].filePath < entries[j].filePath
		}
		return entries[i].lastUse < entries[j].lastUse
	})
	for _, entry := range entries {
		if total <= target {
			break
		}
		if err = os.Remove(entry.filePath); err != nil && !os.IsNotExist(err) {
			g.Log().Warningf(ctx, "删除媒体缓存文件失败 path:%s err:%+v", entry.filePath, err)
			continue
		}
		_ = os.Remove(entry.metaPath)
		total -= entry.size
	}
	return nil
}

func scanMediaFileCache(dir string) ([]mediaFileCacheEntry, int64, error) {
	entries := make([]mediaFileCacheEntry, 0)
	total := int64(0)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return entries, total, nil
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".part") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		metaPath := path + ".json"
		lastUse := info.ModTime().Unix()
		if meta, err := readMediaFileCacheMeta(metaPath); err == nil && meta != nil && meta.LastAccessAt > 0 {
			lastUse = meta.LastAccessAt
		}
		total += info.Size()
		entries = append(entries, mediaFileCacheEntry{metaPath: metaPath, filePath: path, size: info.Size(), lastUse: lastUse})
		return nil
	})
	return entries, total, err
}

func mediaFileCacheKey(media *telegramMediaItem, source string) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(source))
	if media != nil {
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(media.StoragePath))
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(media.AssetHash))
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(media.TgFileId))
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func mediaFileCachePath(dir string, key string, ext string) string {
	if ext == "" {
		ext = ".media"
	}
	return filepath.Join(dir, key[:2], key[2:4], key+ext)
}

func mediaFileCacheExt(media *telegramMediaItem, source string) string {
	for _, value := range []string{source, mediaValue(media, func(item *telegramMediaItem) string { return item.StoragePath }), mediaValue(media, func(item *telegramMediaItem) string { return item.FileUrl })} {
		if parsed, err := url.Parse(strings.TrimSpace(value)); err == nil && parsed.Path != "" {
			value = parsed.Path
		}
		ext := strings.ToLower(filepath.Ext(strings.Split(value, "?")[0]))
		if ext != "" {
			return ext
		}
	}
	if media != nil && strings.EqualFold(media.MediaType, "video") {
		return ".mp4"
	}
	if media != nil && strings.EqualFold(media.MediaType, "image") {
		return ".jpg"
	}
	return ".media"
}

func mediaValue(media *telegramMediaItem, pick func(*telegramMediaItem) string) string {
	if media == nil {
		return ""
	}
	return pick(media)
}

func mediaFileCacheDir(ctx context.Context) string {
	dir := strings.TrimSpace(g.Cfg().MustGet(ctx, "youbanPublish.mediaFileCache.dir", mediaFileCacheDefaultDir).String())
	if dir == "" {
		dir = mediaFileCacheDefaultDir
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	if root := strings.TrimSpace(g.Cfg().MustGet(ctx, "server.serverRoot").String()); root != "" {
		return filepath.Join(root, dir)
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

func mediaFileCacheMaxBytes(ctx context.Context) int64 {
	maxBytes := g.Cfg().MustGet(ctx, "youbanPublish.mediaFileCache.maxBytes", mediaFileCacheDefaultMaxBytes).Int64()
	if maxBytes <= 0 {
		return mediaFileCacheDefaultMaxBytes
	}
	return maxBytes
}

func attachmentMediaSource(ctx context.Context, attachmentId int64) (string, error) {
	if attachmentId <= 0 {
		return "", nil
	}
	row, err := g.DB().Model(sysAttachmentTable).Safe().Ctx(ctx).Fields("file_url,path,drive").Where("id", attachmentId).One()
	if err != nil {
		return "", gerror.Wrap(err, "读取资源文件地址失败")
	}
	return attachmentMediaSourceFromRecord(ctx, row), nil
}

func attachmentMediaSourceFromRecord(ctx context.Context, row gdb.Record) string {
	if row == nil || row.IsEmpty() {
		return ""
	}
	fullPath := firstNonEmpty(row["file_url"].String(), row["path"].String())
	if strings.TrimSpace(fullPath) == "" {
		return ""
	}
	return storager.LastUrl(ctx, fullPath, row["drive"].String())
}
