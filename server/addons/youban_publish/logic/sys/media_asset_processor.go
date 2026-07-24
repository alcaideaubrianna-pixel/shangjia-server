package sys

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/addons/youban_publish/model/input/sysin"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/utility/file"
)

type mediaUploadAssets struct {
	PerceptualHash string
	Poster         *videoPosterResult
}

// prepareStoredMediaAssets processes a media file that is already in local storage.
// It is used by TG collection/import paths so they share the upload pipeline's
// pHash and video poster behavior.
func prepareStoredMediaAssets(ctx context.Context, mediaType string, storagePath string, fileName string) (*mediaUploadAssets, error) {
	storagePath = strings.TrimSpace(storagePath)
	if storagePath == "" {
		return &mediaUploadAssets{}, nil
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if fileName == "" {
		fileName = filepath.Base(storagePath)
	}
	switch mediaType {
	case "video":
		poster, err := buildVideoPosterResultFromPath(ctx, storagePath, fileName)
		if err != nil {
			return nil, err
		}
		res := &mediaUploadAssets{Poster: poster}
		if poster != nil {
			res.PerceptualHash = poster.PerceptualHash
		}
		return res, nil
	case "image":
		hash, err := imagePHashFromPath(storagePath)
		if err != nil {
			return nil, err
		}
		return &mediaUploadAssets{PerceptualHash: fmt.Sprintf("%016x", hash.GetHash())}, nil
	default:
		return &mediaUploadAssets{}, nil
	}
}

func (s *sSysPublish) ProcessStoredMediaAssets(ctx context.Context, in *sysin.StoredMediaAssetsInp) (res *sysin.StoredMediaAssetsModel, err error) {
	res = &sysin.StoredMediaAssetsModel{}
	if in == nil || strings.TrimSpace(in.LocalPath) == "" {
		return res, nil
	}
	localPath := strings.TrimSpace(in.LocalPath)
	if !filepath.IsAbs(localPath) {
		localPath, err = filepath.Abs(localPath)
		if err != nil {
			return nil, gerror.Wrap(err, "解析媒体本地路径失败")
		}
	}
	if !fileExists(localPath) {
		return res, nil
	}
	assets, err := prepareStoredMediaAssets(ctx, in.MediaType, localPath, in.FileName)
	if err != nil {
		return nil, err
	}
	res.Processed = true
	if assets == nil {
		return res, nil
	}
	res.PerceptualHash = assets.PerceptualHash
	res.PosterUrl = normalizeMediaFileURL(mediaPosterURL(assets.Poster), mediaPosterStoragePathValue(assets.Poster))
	res.PosterStoragePath = mediaPosterStoragePathValue(assets.Poster)
	return res, nil
}

func prepareMediaUploadAssets(
	ctx context.Context,
	mediaType string,
	source *ghttp.UploadFile,
	poster *ghttp.UploadFile,
) (*mediaUploadAssets, error) {
	mediaType = strings.TrimSpace(mediaType)
	if mediaType == "" {
		mediaType = "image"
	}
	res := &mediaUploadAssets{}
	switch mediaType {
	case "image":
		hash, err := uploadImagePHash(source)
		if err != nil {
			return nil, err
		}
		res.PerceptualHash = hash
	case "video":
		posterResult, err := prepareVideoPosterAsset(ctx, source, poster)
		if err != nil {
			return nil, err
		}
		res.Poster = posterResult
		if posterResult != nil {
			res.PerceptualHash = posterResult.PerceptualHash
		}
	default:
		hash, err := uploadImagePHash(source)
		if err != nil {
			return nil, err
		}
		res.PerceptualHash = hash
	}
	return res, nil
}

func prepareVideoPosterAsset(
	ctx context.Context,
	source *ghttp.UploadFile,
	poster *ghttp.UploadFile,
) (*videoPosterResult, error) {
	if poster != nil {
		return buildVideoPosterResultFromImageUpload(ctx, poster)
	}
	return buildVideoPosterResultFromUpload(ctx, source)
}

func buildVideoPosterResultFromImageUpload(
	ctx context.Context,
	file *ghttp.UploadFile,
) (*videoPosterResult, error) {
	if file == nil {
		return nil, nil
	}
	perceptualHash, err := uploadImagePHash(file)
	if err != nil {
		return nil, err
	}
	attachment, err := uploadMediaPoster(ctx, file)
	if err != nil {
		return nil, err
	}
	return &videoPosterResult{Attachment: attachment, PerceptualHash: perceptualHash}, nil
}

func buildVideoPosterResultFromUpload(
	ctx context.Context,
	file *ghttp.UploadFile,
) (*videoPosterResult, error) {
	if file == nil {
		return nil, nil
	}
	tempDir, err := os.MkdirTemp("", "ybp-upload-video-*")
	if err != nil {
		return nil, gerror.Wrap(err, "创建视频封面临时目录失败")
	}
	defer os.RemoveAll(tempDir)

	fileName, err := file.Save(tempDir, true)
	if err != nil {
		return nil, gerror.Wrap(err, "保存视频临时文件失败")
	}
	videoPath := filepath.Join(tempDir, fileName)
	return buildVideoPosterResultFromPath(ctx, videoPath, fileName)
}

func buildVideoPosterResultFromBytes(
	ctx context.Context,
	videoName string,
	content []byte,
) (*videoPosterResult, error) {
	if len(content) == 0 {
		return nil, nil
	}
	input, err := os.CreateTemp("", "ybp-video-*"+filepath.Ext(videoName))
	if err != nil {
		return nil, gerror.Wrap(err, "创建视频临时文件失败")
	}
	inputPath := input.Name()
	defer os.Remove(inputPath)
	if _, err = input.Write(content); err != nil {
		_ = input.Close()
		return nil, gerror.Wrap(err, "写入视频临时文件失败")
	}
	if err = input.Close(); err != nil {
		return nil, gerror.Wrap(err, "关闭视频临时文件失败")
	}
	return buildVideoPosterResultFromPath(ctx, inputPath, videoName)
}

func buildVideoPosterResultFromContent(
	ctx context.Context,
	videoName string,
	content []byte,
) (*videoPosterResult, error) {
	return buildVideoPosterResultFromBytes(ctx, videoName, content)
}

func buildVideoPosterResultFromPath(
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
	attachment, err := uploadMediaPoster(ctx, upload)
	if err != nil {
		return nil, err
	}
	return &videoPosterResult{Attachment: attachment, PerceptualHash: perceptualHash}, nil
}

func mediaPosterAttachment(res *videoPosterResult) *basesysin.AttachmentListModel {
	if res == nil {
		return nil
	}
	return res.Attachment
}

func mediaPosterURL(res *videoPosterResult) string {
	return posterFileUrl(mediaPosterAttachment(res))
}

func mediaPosterStoragePathValue(res *videoPosterResult) string {
	return posterStoragePath(mediaPosterAttachment(res))
}

func requireMediaUploadAssets(
	ctx context.Context,
	mediaType string,
	source *ghttp.UploadFile,
	poster *ghttp.UploadFile,
) (*mediaUploadAssets, error) {
	if source == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	return prepareMediaUploadAssets(ctx, mediaType, source, poster)
}

func cachedRemoteImagePHashValue(ctx context.Context, imageURL string) (*goimagehash.ImageHash, error) {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return nil, gerror.New("请发送要搜索的图片")
	}
	path, err := cachedRemoteMediaFile(ctx, mediaFileCacheKey(nil, imageURL), imageURL, mediaFileCacheExt(&telegramMediaItem{MediaType: "image", FileUrl: imageURL}, imageURL))
	if err != nil {
		return nil, err
	}
	hash, err := imagePHashFromPath(path)
	if err != nil {
		return nil, err
	}
	return hash, nil
}

func cachedRemoteImagePHash(ctx context.Context, imageURL string) (string, error) {
	hash, err := cachedRemoteImagePHashValue(ctx, imageURL)
	if err != nil {
		return "", err
	}
	return remotePHashString(hash), nil
}

func cachedRemoteVideoPHashValue(ctx context.Context, videoURL string) (*goimagehash.ImageHash, error) {
	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" {
		return nil, gerror.New("视频文件为空")
	}
	media := &telegramMediaItem{MediaType: "video", FileUrl: videoURL}
	path, err := cachedRemoteMediaFile(ctx, mediaFileCacheKey(media, videoURL), videoURL, mediaFileCacheExt(media, videoURL))
	if err != nil {
		return nil, err
	}
	posterPath, err := generateVideoPosterPath(ctx, path)
	if err != nil {
		return nil, err
	}
	defer os.Remove(posterPath)
	return imagePHashFromPath(posterPath)
}

func remotePHashString(hash *goimagehash.ImageHash) string {
	if hash == nil {
		return ""
	}
	return strings.ToLower(fmt.Sprintf("%016x", hash.GetHash()))
}
