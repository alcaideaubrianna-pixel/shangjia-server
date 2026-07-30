package fix

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/library/storager"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
	fileutil "hotgo/utility/file"
)

const collectMediaCDNRepairBatchSize = 50

type collectMediaCDNRepairRow struct {
	Id                int64  `orm:"id"`
	MediaType         string `orm:"media_type"`
	FileURL           string `orm:"file_url"`
	StoragePath       string `orm:"storage_path"`
	PosterURL         string `orm:"poster_url"`
	PosterStoragePath string `orm:"poster_storage_path"`
}

// RepairYoubanPublishCollectMediaCDN uploads historical collection media that
// already exists locally but has no CDN URL, then removes the migrated local
// files. It never re-downloads from TG or deletes a profile. Hashes, media
// purpose/order and note indexes are deliberately not modified.
func RepairYoubanPublishCollectMediaCDN(ctx context.Context, mediaIds []int64) error {
	lastId := int64(0)
	repaired := 0
	cleaned := 0
	skipped := 0
	for {
		mod := g.DB().Model("hg_youban_publish_media m").Safe().Ctx(ctx).
			Fields("m.id,m.media_type,m.file_url,m.storage_path,m.poster_url,m.poster_storage_path").
			InnerJoin("hg_content_profile p", "p.id=m.profile_id").
			Where("p.source_type", "youban_collect").
			WhereNull("p.deleted_at").
			WhereNull("m.deleted_at").
			Where("(m.storage_path LIKE 'storage/cache/%' OR m.poster_storage_path LIKE 'storage/cache/%' OR (m.storage_path LIKE 'hotgo/file/%' AND m.file_url = '') OR (m.poster_storage_path LIKE 'hotgo/file/%' AND m.poster_url = ''))")
		if len(mediaIds) > 0 {
			mod = mod.WhereIn("m.id", mediaIds)
		} else {
			mod = mod.WhereGT("m.id", lastId)
		}
		rows := make([]collectMediaCDNRepairRow, 0, collectMediaCDNRepairBatchSize)
		err := mod.
			OrderAsc("m.id").
			Limit(collectMediaCDNRepairBatchSize).
			Scan(&rows)
		if err != nil {
			return gerror.Wrap(err, "读取待补全采集媒体失败")
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			lastId = row.Id
			mainPath := collectMediaCDNRepairPath(row.StoragePath)
			posterPath := collectMediaCDNRepairPath(row.PosterStoragePath)
			mainCloudURL := collectMediaCDNRepairCloudURL(ctx, row.StoragePath, row.FileURL)
			posterCloudURL := collectMediaCDNRepairCloudURL(ctx, row.PosterStoragePath, row.PosterURL)
			if mainPath == "" && posterPath == "" && mainCloudURL == "" && posterCloudURL == "" {
				skipped++
				continue
			}
			if err = migrateCollectMediaCDN(ctx, row, mainPath, posterPath, mainCloudURL, posterCloudURL); err != nil {
				return err
			}
			if mainPath != "" || posterPath != "" {
				repaired++
			}
			cleaned += boolToInt(mainPath != "") + boolToInt(posterPath != "")
		}
		g.Log().Infof(ctx, "历史采集媒体 CDN 修复进度 batch:%d repaired:%d cleaned:%d skipped:%d lastId:%d", len(rows), repaired, cleaned, skipped, lastId)
	}
	g.Log().Infof(ctx, "历史采集媒体 CDN 修复完成 repaired:%d cleaned:%d skipped:%d", repaired, cleaned, skipped)
	return nil
}

func migrateCollectMediaCDN(ctx context.Context, row collectMediaCDNRepairRow, mainPath, posterPath, mainCloudURL, posterCloudURL string) error {
	data := g.Map{}
	if mainPath != "" {
		if strings.TrimSpace(row.FileURL) == "" {
			attachment, err := uploadCollectMediaCDNFile(ctx, row.Id, row.MediaType, mainPath)
			if err != nil {
				return err
			}
			data["file_url"] = strings.TrimSpace(attachment.FileUrl)
			data["storage_path"] = strings.TrimSpace(attachment.Path)
			data["size"] = attachment.Size
			data["md5"] = strings.TrimSpace(attachment.Md5)
		} else {
			data["storage_path"] = ""
		}
	} else if mainCloudURL != "" {
		data["file_url"] = mainCloudURL
	}
	if posterPath != "" {
		if strings.TrimSpace(row.PosterURL) == "" {
			attachment, err := uploadCollectMediaCDNFile(ctx, row.Id, "image", posterPath)
			if err != nil {
				return err
			}
			data["poster_url"] = strings.TrimSpace(attachment.FileUrl)
			data["poster_storage_path"] = strings.TrimSpace(attachment.Path)
		} else {
			data["poster_storage_path"] = ""
		}
	} else if posterCloudURL != "" {
		data["poster_url"] = posterCloudURL
	}
	if len(data) == 0 {
		return nil
	}
	data["updated_at"] = gtime.Now()
	if _, err := g.DB().Model("hg_youban_publish_media").Safe().Ctx(ctx).
		Where("id", row.Id).
		WhereNull("deleted_at").
		Data(data).Update(); err != nil {
		return gerror.Wrapf(err, "回填历史采集媒体 CDN 地址失败 mediaId:%d", row.Id)
	}
	if mainPath != "" {
		removeCollectMediaCDNLocalFile(ctx, row.Id, "媒体", mainPath)
	}
	if posterPath != "" {
		removeCollectMediaCDNLocalFile(ctx, row.Id, "预览图", posterPath)
	}
	return nil
}

func collectMediaCDNRepairCloudURL(ctx context.Context, rawPath, existingURL string) string {
	if strings.TrimSpace(existingURL) != "" || !strings.HasPrefix(strings.TrimSpace(rawPath), "hotgo/file/") {
		return ""
	}
	config := storager.GetConfig()
	if config == nil || strings.TrimSpace(config.Drive) == "" {
		return ""
	}
	return strings.TrimSpace(storager.LastUrl(ctx, strings.TrimSpace(rawPath), config.Drive))
}

func uploadCollectMediaCDNFile(ctx context.Context, mediaId int64, mediaType, path string) (*basesysin.AttachmentListModel, error) {
	header, cleanup, err := fileutil.NewMultipartFileHeaderFromPath(path, filepath.Base(path))
	if err != nil {
		return nil, gerror.Wrapf(err, "准备历史采集媒体上传文件失败 mediaId:%d", mediaId)
	}
	defer cleanup()
	uploadType := storager.KindImg
	if strings.EqualFold(strings.TrimSpace(mediaType), "video") {
		uploadType = storager.KindVideo
	}
	attachment, err := service.CommonUpload().UploadFile(ctx, uploadType, &ghttp.UploadFile{FileHeader: header})
	if err != nil {
		return nil, gerror.Wrapf(err, "上传历史采集媒体到 CDN 失败 mediaId:%d", mediaId)
	}
	if attachment == nil || strings.TrimSpace(attachment.FileUrl) == "" {
		return nil, gerror.Newf("上传历史采集媒体未返回 CDN 地址 mediaId:%d", mediaId)
	}
	return attachment, nil
}

func removeCollectMediaCDNLocalFile(ctx context.Context, mediaId int64, kind, path string) {
	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			g.Log().Warningf(ctx, "删除历史采集媒体本地%s失败 mediaId:%d path:%s err:%+v", kind, mediaId, path, err)
		}
		return
	}
	g.Log().Debugf(ctx, "删除历史采集媒体本地%s成功 mediaId:%d path:%s", kind, mediaId, path)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func collectMediaCDNRepairPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "://") {
		return ""
	}
	raw = filepath.Clean(filepath.FromSlash(raw))
	if filepath.IsAbs(raw) {
		return existingCollectMediaCDNRepairPath(raw)
	}
	for _, root := range []string{
		"/app",
		g.Cfg().MustGet(context.Background(), "server.root", ".").String(),
		g.Cfg().MustGet(context.Background(), "server.serverRoot", "resource/public").String(),
		"/app/resource/public",
		"/app/storage",
	} {
		candidate := filepath.Join(root, raw)
		if path := existingCollectMediaCDNRepairPath(candidate); path != "" {
			return path
		}
	}
	return ""
}

func existingCollectMediaCDNRepairPath(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() <= 0 {
		return ""
	}
	return path
}
