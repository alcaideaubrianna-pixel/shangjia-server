package fix

import (
	"context"
	"net/url"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

const (
	mediaPHashMissingBatchSize = 50
)

type mediaPHashMissingRow struct {
	Id                int64  `orm:"id"`
	MediaType         string `orm:"media_type"`
	FileURL           string `orm:"file_url"`
	StoragePath       string `orm:"storage_path"`
	PosterURL         string `orm:"poster_url"`
	PosterStoragePath string `orm:"poster_storage_path"`
}

type mediaPHashCandidate struct {
	mediaType string
	localPath string
	fileURL   string
	posterURL string
}

// BackfillYoubanPublishMediaMissingPHash calculates missing media pHash values
// and rebuilds the corresponding search indexes. Existing valid hashes are skipped.
func BackfillYoubanPublishMediaMissingPHash(ctx context.Context) error {
	lastId := int64(0)
	processed := 0
	succeeded := 0
	failed := 0
	for {
		rows, err := mediaPHashMissingRows(ctx, lastId, mediaPHashMissingBatchSize)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		lastId = rows[len(rows)-1].Id
		for _, row := range rows {
			processed++
			if err := backfillMediaPHash(ctx, row); err != nil {
				failed++
				g.Log().Warningf(ctx, "补全媒体感知哈希失败 mediaId=%d mediaType=%s err=%+v", row.Id, row.MediaType, err)
				continue
			}
			succeeded++
		}
		g.Log().Infof(ctx, "上架媒体缺失感知哈希补全进度：lastId=%d processed=%d succeeded=%d failed=%d", lastId, processed, succeeded, failed)
	}
	g.Log().Infof(ctx, "上架媒体缺失感知哈希补全完成：processed=%d succeeded=%d failed=%d", processed, succeeded, failed)
	return nil
}

func mediaPHashMissingRows(ctx context.Context, lastId int64, limit int) ([]mediaPHashMissingRow, error) {
	rows := make([]mediaPHashMissingRow, 0)
	err := g.DB().Model(youbanPublishMediaTable).Safe().Ctx(ctx).
		Fields("id,media_type,file_url,storage_path,poster_url,poster_storage_path").
		WhereGT("id", lastId).
		Where("(perceptual_hash IS NULL OR LENGTH(TRIM(perceptual_hash)) <> 16)").
		WhereNull("deleted_at").
		OrderAsc("id").
		Limit(limit).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取缺失媒体感知哈希失败")
	}
	return rows, nil
}

func backfillMediaPHash(ctx context.Context, row mediaPHashMissingRow) error {
	if row.Id <= 0 {
		return nil
	}
	assets, err := processMissingMediaPHash(ctx, row)
	if err != nil {
		return err
	}
	hash := strings.ToLower(strings.TrimSpace(assets.PerceptualHash))
	if len(hash) != 16 {
		return gerror.New("媒体处理未返回合法感知哈希")
	}
	_, err = g.DB().Model(youbanPublishMediaTable).Safe().Ctx(ctx).
		Data(g.Map{"perceptual_hash": hash, "updated_at": gtime.Now()}).
		Where("id", row.Id).
		Where("(perceptual_hash IS NULL OR LENGTH(TRIM(perceptual_hash)) <> 16)").
		Update()
	if err != nil {
		return gerror.Wrap(err, "写入媒体感知哈希失败")
	}
	if err = service.SysPublish().SyncMediaPHashBucketByMediaId(ctx, row.Id); err != nil {
		return gerror.Wrap(err, "同步媒体感知哈希索引失败")
	}
	return nil
}

func processMissingMediaPHash(ctx context.Context, row mediaPHashMissingRow) (*sysin.MediaAssetsModel, error) {
	mediaType := strings.ToLower(strings.TrimSpace(row.MediaType))
	candidates := make([]mediaPHashCandidate, 0, 4)
	if mediaType == "video" {
		if path := strings.TrimSpace(row.PosterStoragePath); path != "" && !isRemoteMediaSource(path) {
			candidates = append(candidates, mediaPHashCandidate{mediaType: "image", localPath: path})
		}
		if url := downloadableMediaSource(row.PosterURL); url != "" {
			candidates = append(candidates, mediaPHashCandidate{mediaType: "image", fileURL: url})
		}
	}
	if path := strings.TrimSpace(row.StoragePath); path != "" && !isRemoteMediaSource(path) {
		candidates = append(candidates, mediaPHashCandidate{mediaType: mediaType, localPath: path})
	}
	if fileURL := downloadableMediaSource(row.FileURL); fileURL != "" {
		candidates = append(candidates, mediaPHashCandidate{mediaType: mediaType, fileURL: fileURL, posterURL: downloadableMediaSource(row.PosterURL)})
	}
	var lastErr error
	for _, item := range candidates {
		assets, err := processMissingMediaCandidate(ctx, item)
		if err != nil {
			lastErr = err
			continue
		}
		if assets != nil && len(strings.TrimSpace(assets.PerceptualHash)) == 16 {
			return assets, nil
		}
		lastErr = gerror.New("候选媒体未返回合法感知哈希")
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, gerror.New("媒体没有可用的远程地址或本地路径")
}

func processMissingMediaCandidate(ctx context.Context, item mediaPHashCandidate) (*sysin.MediaAssetsModel, error) {
	if item.localPath == "" && item.fileURL == "" {
		return nil, gerror.New("媒体候选地址为空")
	}
	return service.SysPublish().ProcessMediaAssets(ctx, &sysin.MediaAssetsInp{
		MediaType: item.mediaType, LocalPath: item.localPath, FileURL: item.fileURL, PosterURL: item.posterURL,
	})
}

func downloadableMediaSource(value string) string {
	value = strings.TrimSpace(value)
	if !isRemoteMediaSource(value) {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || strings.Contains(strings.ToLower(parsed.Path), "/telegram/resource/") {
		return ""
	}
	return value
}

func processStoredMissingPoster(ctx context.Context, posterPath string) (*sysin.MediaAssetsModel, error) {
	return service.SysPublish().ProcessMediaAssets(ctx, &sysin.MediaAssetsInp{
		MediaType: "image", LocalPath: posterPath,
	})
}

func isRemoteMediaSource(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
