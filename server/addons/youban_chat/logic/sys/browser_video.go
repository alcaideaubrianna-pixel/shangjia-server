package sys

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
)

type browserVideoProbe struct {
	Format struct {
		FormatName string `json:"format_name"`
	} `json:"format"`
	Streams []struct {
		CodecName string `json:"codec_name"`
		CodecType string `json:"codec_type"`
	} `json:"streams"`
}

func normalizeBrowserVideo(ctx context.Context, name, mimeType string, data []byte) (string, string, []byte, error) {
	if len(data) == 0 {
		return "", "", nil, gerror.New("视频内容为空")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return "", "", nil, gerror.Wrap(err, "ffprobe未安装，无法检查视频兼容性")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", "", nil, gerror.Wrap(err, "ffmpeg未安装，无法转换视频")
	}

	dir, err := os.MkdirTemp("", "youban-chat-video-*")
	if err != nil {
		return "", "", nil, gerror.Wrap(err, "创建视频转换目录失败")
	}
	defer os.RemoveAll(dir)

	inputName := filepath.Base(strings.TrimSpace(name))
	if inputName == "." || inputName == "" {
		inputName = "video"
	}
	if filepath.Ext(inputName) == "" {
		inputName += attachmentExtByMime(mimeType)
	}
	inputPath := filepath.Join(dir, inputName)
	if err = os.WriteFile(inputPath, data, 0600); err != nil {
		return "", "", nil, gerror.Wrap(err, "写入待转换视频失败")
	}

	probe, err := probeBrowserVideo(ctx, ffprobePath, inputPath)
	if err != nil {
		return "", "", nil, err
	}
	if browserVideoCompatible(probe, filepath.Ext(inputName)) {
		return name, mimeType, data, nil
	}

	outputPath := filepath.Join(dir, "browser-video.mp4")
	convertCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	output, err := exec.CommandContext(convertCtx, ffmpegPath,
		"-y", "-i", inputPath,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "28", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart",
		outputPath,
	).CombinedOutput()
	if err != nil {
		return "", "", nil, gerror.Wrapf(err, "转换浏览器兼容视频失败: %s", tailVideoError(output))
	}
	converted, err := os.ReadFile(outputPath)
	if err != nil {
		return "", "", nil, gerror.Wrap(err, "读取转换后视频失败")
	}
	if len(converted) == 0 {
		return "", "", nil, gerror.New("转换后视频为空")
	}
	return replaceFileExt(name, ".mp4"), "video/mp4", converted, nil
}

func probeBrowserVideo(ctx context.Context, ffprobePath, path string) (*browserVideoProbe, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, ffprobePath,
		"-v", "error", "-show_entries", "format=format_name:stream=codec_name,codec_type", "-of", "json", path,
	).CombinedOutput()
	if err != nil {
		return nil, gerror.Wrapf(err, "读取视频编码失败: %s", tailVideoError(output))
	}
	var probe browserVideoProbe
	if err = json.Unmarshal(output, &probe); err != nil {
		return nil, gerror.Wrap(err, "解析视频编码失败")
	}
	return &probe, nil
}

func browserVideoCompatible(probe *browserVideoProbe, ext string) bool {
	if probe == nil {
		return false
	}
	codec := ""
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			codec = strings.ToLower(stream.CodecName)
			break
		}
	}
	ext = strings.ToLower(ext)
	return (ext == ".mp4" && codec == "h264") || (ext == ".webm" && (codec == "vp8" || codec == "vp9"))
}

func tailVideoError(output []byte) string {
	text := strings.TrimSpace(string(output))
	if len(text) > 600 {
		return text[len(text)-600:]
	}
	return text
}
