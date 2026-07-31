package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"golang.org/x/sync/singleflight"
)

const (
	telegramVideoSanitizeCacheDir = "youban_publish_video_sanitize"
	telegramVideoSanitizeCacheTTL = 6 * time.Hour
)

var (
	telegramVideoSanitizeGroup singleflight.Group
	telegramVideoSanitizeSlot  = make(chan struct{}, 1)
	telegramVideoSanitizePrune struct {
		sync.Mutex
		lastAt time.Time
	}
)

func cachedTelegramVideoSanitize(ctx context.Context, ffmpegPath string, path string) (string, error) {
	cacheKey, ext, err := telegramVideoSanitizeCacheKey(path)
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(os.TempDir(), telegramVideoSanitizeCacheDir)
	if err = os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", gerror.Wrap(err, "创建视频元数据缓存目录失败")
	}
	pruneTelegramVideoSanitizeCache(cacheDir)
	cachePath := filepath.Join(cacheDir, cacheKey+ext)
	value, err, _ := telegramVideoSanitizeGroup.Do(cacheKey, func() (any, error) {
		if info, statErr := os.Stat(cachePath); statErr == nil && info.Size() > 0 {
			_ = os.Chtimes(cachePath, time.Now(), time.Now())
			return cachePath, nil
		}
		select {
		case telegramVideoSanitizeSlot <- struct{}{}:
			defer func() { <-telegramVideoSanitizeSlot }()
		case <-ctx.Done():
			return "", ctx.Err()
		}
		temp, createErr := os.CreateTemp(cacheDir, ".sanitize-*"+ext)
		if createErr != nil {
			return "", gerror.Wrap(createErr, "创建视频元数据缓存临时文件失败")
		}
		tempPath := temp.Name()
		_ = temp.Close()
		_ = os.Remove(tempPath)
		cmdCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		output, runErr := exec.CommandContext(cmdCtx, ffmpegPath, "-y", "-i", path, "-map_metadata", "-1", "-c", "copy", tempPath).CombinedOutput()
		if runErr != nil {
			_ = os.Remove(tempPath)
			return "", gerror.Wrapf(runErr, "ffmpeg 清理视频元数据失败：%s", ellipsisString(strings.TrimSpace(string(output)), 500))
		}
		if renameErr := os.Rename(tempPath, cachePath); renameErr != nil {
			_ = os.Remove(tempPath)
			return "", gerror.Wrap(renameErr, "保存视频元数据缓存失败")
		}
		return cachePath, nil
	})
	if err != nil {
		return "", err
	}
	return cloneTelegramVideoSanitizeFile(value.(string), ext)
}

func telegramVideoSanitizeCacheKey(path string) (string, string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", "", gerror.Wrap(err, "读取视频绝对路径失败")
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", "", gerror.Wrap(err, "读取视频文件信息失败")
	}
	ext := strings.ToLower(filepath.Ext(absolutePath))
	if ext == "" {
		ext = ".mp4"
	}
	identity := absolutePath + "\x00" + info.ModTime().UTC().Format(time.RFC3339Nano) + "\x00" + strconv.FormatInt(info.Size(), 10)
	hash := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(hash[:]), ext, nil
}

func cloneTelegramVideoSanitizeFile(cachePath string, ext string) (string, error) {
	out, err := os.CreateTemp("", "ybp-tg-video-*"+ext)
	if err != nil {
		return "", gerror.Wrap(err, "创建视频发送临时文件失败")
	}
	outPath := out.Name()
	_ = out.Close()
	_ = os.Remove(outPath)
	if err = os.Link(cachePath, outPath); err == nil {
		return outPath, nil
	}
	source, err := os.Open(cachePath)
	if err != nil {
		return "", gerror.Wrap(err, "打开视频元数据缓存失败")
	}
	defer source.Close()
	target, err := os.Create(outPath)
	if err != nil {
		return "", gerror.Wrap(err, "创建视频发送文件失败")
	}
	if _, err = io.Copy(target, source); err != nil {
		_ = target.Close()
		_ = os.Remove(outPath)
		return "", gerror.Wrap(err, "复制视频元数据缓存失败")
	}
	if err = target.Close(); err != nil {
		_ = os.Remove(outPath)
		return "", gerror.Wrap(err, "关闭视频发送文件失败")
	}
	return outPath, nil
}

func pruneTelegramVideoSanitizeCache(cacheDir string) {
	telegramVideoSanitizePrune.Lock()
	defer telegramVideoSanitizePrune.Unlock()
	if time.Since(telegramVideoSanitizePrune.lastAt) < time.Hour {
		return
	}
	telegramVideoSanitizePrune.lastAt = time.Now()
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	deadline := time.Now().Add(-telegramVideoSanitizeCacheTTL)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr == nil && info.ModTime().Before(deadline) {
			_ = os.Remove(filepath.Join(cacheDir, entry.Name()))
		}
	}
}
