package sys

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/internal/library/storager"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
	"hotgo/utility/file"
)

type videoPosterResult struct {
	Attachment     *basesysin.AttachmentListModel
	PerceptualHash string
}

func uploadMediaPoster(ctx context.Context, file *ghttp.UploadFile) (*basesysin.AttachmentListModel, error) {
	if file == nil {
		return nil, nil
	}
	return service.CommonUpload().UploadFile(ctx, storager.KindImg, file)
}

func posterFileUrl(attachment *basesysin.AttachmentListModel) string {
	if attachment == nil {
		return ""
	}
	return attachment.FileUrl
}

func posterStoragePath(attachment *basesysin.AttachmentListModel) string {
	if attachment == nil {
		return ""
	}
	return attachment.Path
}

func uploadVideoPoster(ctx context.Context, upload *ghttp.UploadFile) (*basesysin.AttachmentListModel, error) {
	res, err := uploadVideoPosterWithPHash(ctx, upload)
	if err != nil || res == nil {
		return nil, err
	}
	return res.Attachment, nil
}

func uploadVideoPosterWithPHash(ctx context.Context, upload *ghttp.UploadFile) (*videoPosterResult, error) {
	if upload == nil {
		return nil, nil
	}
	tempDir, err := os.MkdirTemp("", "ybp-upload-video-*")
	if err != nil {
		return nil, gerror.Wrap(err, "创建视频封面临时目录失败")
	}
	defer os.RemoveAll(tempDir)

	fileName, err := upload.Save(tempDir, true)
	if err != nil {
		return nil, gerror.Wrap(err, "保存视频临时文件失败")
	}
	videoPath := filepath.Join(tempDir, fileName)
	return generateVideoPosterAttachmentWithPHash(ctx, videoPath, fileName)
}

func generateVideoPosterAttachment(
	ctx context.Context,
	videoPath string,
	videoName string,
) (*basesysin.AttachmentListModel, error) {
	res, err := generateVideoPosterAttachmentWithPHash(ctx, videoPath, videoName)
	if err != nil || res == nil {
		return nil, err
	}
	return res.Attachment, nil
}

func generateVideoPosterAttachmentWithPHash(
	ctx context.Context,
	videoPath string,
	videoName string,
) (*videoPosterResult, error) {
	posterPath, err := generateVideoPosterPath(ctx, videoPath)
	if err != nil {
		return nil, err
	}
	posterBytes, err := os.ReadFile(posterPath)
	if err != nil {
		_ = os.Remove(posterPath)
		return nil, gerror.Wrap(err, "读取视频封面失败")
	}
	defer os.Remove(posterPath)
	if len(posterBytes) == 0 {
		return nil, nil
	}
	posterName := strings.TrimSuffix(filepath.Base(videoName), filepath.Ext(videoName)) + ".jpg"
	fileHeader, err := file.NewMultipartFileHeader(posterName, posterBytes)
	if err != nil {
		return nil, gerror.Wrap(err, "创建视频封面上传文件失败")
	}
	upload := &ghttp.UploadFile{FileHeader: fileHeader}
	perceptualHash, err := uploadImagePHash(upload)
	if err != nil {
		return nil, err
	}
	attachment, err := service.CommonUpload().UploadFile(ctx, storager.KindImg, upload)
	if err != nil {
		return nil, err
	}
	return &videoPosterResult{Attachment: attachment, PerceptualHash: perceptualHash}, nil
}

func generateVideoPosterPath(ctx context.Context, videoPath string) (string, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", gerror.New("ffmpeg 未安装，无法生成视频封面")
	}
	output, err := os.CreateTemp("", "ybp-video-poster-*.jpg")
	if err != nil {
		return "", gerror.Wrap(err, "创建视频封面临时文件失败")
	}
	outputPath := output.Name()
	_ = output.Close()
	scaleFilter := `scale=if(gt(iw\,ih)\,320\,-2):if(gt(iw\,ih)\,-2\,320)`
	attempts := [][]string{
		{"-y", "-ss", "00:00:01", "-i", videoPath, "-frames:v", "1", "-vf", scaleFilter, "-q:v", "5", outputPath},
		{"-y", "-i", videoPath, "-frames:v", "1", "-vf", scaleFilter, "-q:v", "5", outputPath},
	}
	var lastOutput []byte
	var lastErr error
	for _, args := range attempts {
		cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		lastOutput, lastErr = exec.CommandContext(cmdCtx, ffmpegPath, args...).CombinedOutput()
		cancel()
		if lastErr == nil && fileNonEmpty(outputPath) {
			return outputPath, nil
		}
	}
	_ = os.Remove(outputPath)
	return "", gerror.Wrapf(lastErr, "生成视频封面失败：%s", ellipsisString(strings.TrimSpace(string(lastOutput)), 500))
}
