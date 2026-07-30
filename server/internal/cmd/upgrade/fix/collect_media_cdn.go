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
	"hotgo/internal/service"
	fileutil "hotgo/utility/file"
)

const collectMediaCDNRepairBatchSize = 50

type collectMediaCDNRepairRow struct {
	Id          int64  `orm:"id"`
	MediaType   string `orm:"media_type"`
	StoragePath string `orm:"storage_path"`
}

// RepairYoubanPublishCollectMediaCDN uploads historical collection media that
// already exists locally but has no CDN URL. It never re-downloads from TG or
// deletes a profile; unrecoverable files are only logged for follow-up.
func RepairYoubanPublishCollectMediaCDN(ctx context.Context) error {
	lastId := int64(0)
	repaired := 0
	skipped := 0
	for {
		rows := make([]collectMediaCDNRepairRow, 0, collectMediaCDNRepairBatchSize)
		err := g.DB().Model("hg_youban_publish_media m").Safe().Ctx(ctx).
			Fields("m.id,m.media_type,m.storage_path").
			InnerJoin("hg_content_profile p", "p.id=m.profile_id").
			Where("p.source_type", "youban_collect").
			WhereNull("p.deleted_at").
			WhereNull("m.deleted_at").
			Where("m.file_url", "").
			WhereNot("m.storage_path", "").
			WhereGT("m.id", lastId).
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
			path := collectMediaCDNRepairPath(row.StoragePath)
			if path == "" {
				skipped++
				g.Log().Warningf(ctx, "历史采集媒体本地文件不存在 mediaId:%d storagePath:%s", row.Id, row.StoragePath)
				continue
			}
			if err = uploadCollectMediaCDN(ctx, row); err != nil {
				return err
			}
			repaired++
		}
		g.Log().Infof(ctx, "历史采集媒体 CDN 修复进度 batch:%d repaired:%d skipped:%d lastId:%d", len(rows), repaired, skipped, lastId)
	}
	g.Log().Infof(ctx, "历史采集媒体 CDN 修复完成 repaired:%d skipped:%d", repaired, skipped)
	return nil
}

func uploadCollectMediaCDN(ctx context.Context, row collectMediaCDNRepairRow) error {
	path := collectMediaCDNRepairPath(row.StoragePath)
	header, cleanup, err := fileutil.NewMultipartFileHeaderFromPath(path, filepath.Base(path))
	if err != nil {
		return gerror.Wrapf(err, "准备历史采集媒体上传文件失败 mediaId:%d", row.Id)
	}
	defer cleanup()
	uploadType := storager.KindImg
	if strings.EqualFold(strings.TrimSpace(row.MediaType), "video") {
		uploadType = storager.KindVideo
	}
	attachment, err := service.CommonUpload().UploadFile(ctx, uploadType, &ghttp.UploadFile{FileHeader: header})
	if err != nil {
		return gerror.Wrapf(err, "上传历史采集媒体到 CDN 失败 mediaId:%d", row.Id)
	}
	if attachment == nil || strings.TrimSpace(attachment.FileUrl) == "" {
		return gerror.Newf("上传历史采集媒体未返回 CDN 地址 mediaId:%d", row.Id)
	}
	_, err = g.DB().Model("hg_youban_publish_media").Safe().Ctx(ctx).
		Where("id", row.Id).
		WhereNull("deleted_at").
		Data(g.Map{
			"file_url":     strings.TrimSpace(attachment.FileUrl),
			"storage_path": strings.TrimSpace(attachment.Path),
			"size":         attachment.Size,
			"md5":          strings.TrimSpace(attachment.Md5),
			"updated_at":   gtime.Now(),
		}).Update()
	return gerror.Wrapf(err, "回填历史采集媒体 CDN 地址失败 mediaId:%d", row.Id)
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
