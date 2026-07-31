package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/utility/file"
)

type mediaUploadAssets struct {
	PerceptualHash string
	Poster         *videoPosterResult
}

type mediaPipelineSource struct {
	MediaType  string
	LocalPath  string
	FileURL    string
	PosterURL  string
	FileName   string
	UploadFile *ghttp.UploadFile
	PosterFile *ghttp.UploadFile
}

type mediaPipelineResult struct {
	Processed      bool
	PerceptualHash string
	Poster         *videoPosterResult
	PosterURL      string
	PosterPath     string
}

type mediaAssetMetadata struct {
	PerceptualHash    string
	PosterURL         string
	PosterStoragePath string
}

func processMediaAssetMetadata(
	ctx context.Context,
	mediaType string,
	storagePath string,
	fileURL string,
	posterURL string,
	fileName string,
) (*mediaAssetMetadata, error) {
	pipelineResult, err := processMediaPipeline(ctx, mediaPipelineSource{
		MediaType: mediaType,
		LocalPath: storagePath,
		FileURL:   fileURL,
		PosterURL: posterURL,
		FileName:  fileName,
	})
	if pipelineResult == nil {
		return &mediaAssetMetadata{}, err
	}
	return &mediaAssetMetadata{
		PerceptualHash:    pipelineResult.PerceptualHash,
		PosterURL:         firstNonEmpty(pipelineResult.PosterURL, mediaPosterURL(pipelineResult.Poster)),
		PosterStoragePath: firstNonEmpty(pipelineResult.PosterPath, mediaPosterStoragePathValue(pipelineResult.Poster)),
	}, err
}

func (s *sSysPublish) ProcessMediaAssets(ctx context.Context, in *sysin.MediaAssetsInp) (*sysin.MediaAssetsModel, error) {
	if in == nil {
		return &sysin.MediaAssetsModel{}, nil
	}
	result, err := processMediaPipeline(ctx, mediaPipelineSource{
		MediaType: in.MediaType,
		LocalPath: in.LocalPath,
		FileURL:   in.FileURL,
		PosterURL: in.PosterURL,
		FileName:  in.FileName,
	})
	if result == nil {
		return &sysin.MediaAssetsModel{}, err
	}
	return &sysin.MediaAssetsModel{
		Processed:         result.Processed,
		PerceptualHash:    result.PerceptualHash,
		PosterUrl:         firstNonEmpty(result.PosterURL, mediaPosterURL(result.Poster)),
		PosterStoragePath: firstNonEmpty(result.PosterPath, mediaPosterStoragePathValue(result.Poster)),
	}, err
}

func (s *sSysPublish) saveMediaRecordAndIndex(ctx context.Context, data g.Map, errorMessage string) (int64, error) {
	mediaId, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, errorMessage)
	}
	if err = s.syncMediaPHashBucketByMediaId(ctx, mediaId); err != nil {
		return 0, err
	}
	return mediaId, nil
}

func processMediaPipeline(ctx context.Context, source mediaPipelineSource) (*mediaPipelineResult, error) {
	cacheable := source.UploadFile == nil
	cacheKey := ""
	if cacheable {
		cacheKey = mediaPipelineCacheKey(source)
		if value, err := cache.Instance().Get(ctx, cacheKey); err == nil && !value.IsNil() {
			cached := &mediaPipelineCacheResult{}
			if scanErr := value.Scan(cached); scanErr == nil {
				return &mediaPipelineResult{
					Processed:      cached.Processed,
					PerceptualHash: cached.PerceptualHash,
					PosterURL:      cached.PosterURL,
					PosterPath:     cached.PosterPath,
				}, nil
			}
		}
	}
	if source.UploadFile != nil {
		assets, err := prepareMediaUploadAssets(ctx, source.MediaType, source.UploadFile, source.PosterFile)
		if err != nil {
			return nil, err
		}
		if assets == nil {
			return &mediaPipelineResult{}, nil
		}
		result := &mediaPipelineResult{
			Processed:      true,
			PerceptualHash: assets.PerceptualHash,
			Poster:         assets.Poster,
		}
		result.PosterURL = mediaPosterURL(result.Poster)
		result.PosterPath = mediaPosterStoragePathValue(result.Poster)
		return result, nil
	}

	result := &mediaPipelineResult{}
	var lastErr error
	localPath := strings.TrimSpace(source.LocalPath)
	if localPath != "" {
		localPath, lastErr = resolveMediaLocalPath(localPath)
		if localPath != "" && fileExists(localPath) {
			assets, err := prepareStoredMediaAssets(ctx, source.MediaType, localPath, source.FileName)
			if err != nil {
				lastErr = err
			} else if assets != nil {
				result.Processed = true
				result.PerceptualHash = assets.PerceptualHash
				result.Poster = assets.Poster
				result.PosterURL = mediaPosterURL(assets.Poster)
				result.PosterPath = mediaPosterStoragePathValue(assets.Poster)
			}
		}
	}
	if result.PerceptualHash == "" && (strings.TrimSpace(source.FileURL) != "" || strings.TrimSpace(source.PosterURL) != "") {
		remote, err := processRemoteMediaAssets(ctx, &sysin.MediaAssetsInp{
			MediaType: source.MediaType,
			FileURL:   source.FileURL,
			PosterURL: firstNonEmpty(mediaPosterURL(result.Poster), source.PosterURL),
		})
		if err != nil {
			lastErr = err
		} else if remote != nil && remote.Processed {
			result.Processed = true
			result.PerceptualHash = remote.PerceptualHash
			result.PosterURL = firstNonEmpty(result.PosterURL, remote.PosterUrl)
			result.PosterPath = firstNonEmpty(result.PosterPath, remote.PosterStoragePath)
		}
	}
	if result.PerceptualHash == "" && lastErr != nil {
		return result, lastErr
	}
	if cacheable && cacheKey != "" {
		_ = cache.Instance().Set(ctx, cacheKey, mediaPipelineCacheResult{
			Processed:      result.Processed,
			PerceptualHash: result.PerceptualHash,
			PosterURL:      result.PosterURL,
			PosterPath:     result.PosterPath,
		}, 24*time.Hour)
	}
	return result, nil
}

func resolveMediaLocalPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if filepath.IsAbs(raw) {
		return raw, nil
	}
	path, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	if fileExists(path) {
		return path, nil
	}
	serverRoot := strings.TrimSpace(g.Cfg().MustGet(context.Background(), "server.serverRoot", "").String())
	if serverRoot == "" {
		return path, nil
	}
	root, err := filepath.Abs(filepath.FromSlash(serverRoot))
	if err != nil {
		return path, nil
	}
	return filepath.Join(root, filepath.FromSlash(raw)), nil
}

type mediaPipelineCacheResult struct {
	Processed      bool
	PerceptualHash string
	PosterURL      string
	PosterPath     string
}

func mediaPipelineCacheKey(source mediaPipelineSource) string {
	localPath := strings.TrimSpace(source.LocalPath)
	if localPath != "" && !filepath.IsAbs(localPath) {
		if absolutePath, err := filepath.Abs(localPath); err == nil {
			localPath = absolutePath
		}
	}
	parts := []string{
		strings.ToLower(strings.TrimSpace(source.MediaType)),
		localPath,
		strings.TrimSpace(source.FileURL),
		strings.TrimSpace(source.PosterURL),
		strings.TrimSpace(source.FileName),
	}
	if localPath != "" {
		if info, err := os.Stat(localPath); err == nil {
			parts = append(parts, fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano()))
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "youban_publish:media_pipeline:v2:" + hex.EncodeToString(sum[:])
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
	result, err := processMediaPipeline(ctx, mediaPipelineSource{
		MediaType:  mediaType,
		UploadFile: source,
		PosterFile: poster,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &mediaUploadAssets{}, nil
	}
	return &mediaUploadAssets{PerceptualHash: result.PerceptualHash, Poster: result.Poster}, nil
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

func remotePHashString(hash *goimagehash.ImageHash) string {
	if hash == nil {
		return ""
	}
	return strings.ToLower(fmt.Sprintf("%016x", hash.GetHash()))
}
