package sys

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
)

func compressTelegramVideoForUpload(ctx context.Context, path string) (string, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", gerror.New("ffmpeg 未安装，无法压缩超限验证视频")
	}
	for _, crf := range []int{28, 32, 36, 40} {
		out, createErr := os.CreateTemp("", "ybp-tg-video-compressed-*.mp4")
		if createErr != nil {
			return "", gerror.Wrap(createErr, "创建验证视频压缩文件失败")
		}
		outPath := out.Name()
		_ = out.Close()
		_ = os.Remove(outPath)

		cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		output, runErr := exec.CommandContext(cmdCtx, ffmpegPath,
			"-hide_banner", "-loglevel", "error", "-y", "-i", path,
			"-map", "0:v:0", "-map", "0:a:0?", "-map_metadata", "-1",
			"-vf", "scale='min(1280,iw)':-2:force_original_aspect_ratio=decrease,format=yuv420p",
			"-c:v", "libx264", "-preset", "veryfast", "-crf", strconv.Itoa(crf),
			"-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart", outPath,
		).CombinedOutput()
		cancel()
		if runErr != nil {
			_ = os.Remove(outPath)
			return "", gerror.Wrapf(runErr, "压缩验证视频失败：%s", ellipsisString(strings.TrimSpace(string(output)), 500))
		}
		info, statErr := os.Stat(outPath)
		if statErr == nil && info.Size() > 0 && info.Size() <= telegramVideoMaxUploadBytes {
			return outPath, nil
		}
		_ = os.Remove(outPath)
	}
	return "", gerror.Newf("验证视频压缩后仍超过官方Bot API上传限制 limitBytes:%d", telegramVideoMaxUploadBytes)
}
