package fix

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/dao"
)

const collectProfileMediaRepairBatchSize = 100

type collectProfileMediaRepairProfile struct {
	Id        int64  `orm:"id"`
	TenantId  int64  `orm:"tenant_id"`
	SourceKey string `orm:"source_key"`
}

type collectProfileMediaRepairMedia struct {
	Id                int64  `orm:"id"`
	MediaType         string `orm:"media_type"`
	FileURL           string `orm:"file_url"`
	StoragePath       string `orm:"storage_path"`
	OriginalFileURL   string `orm:"original_file_url"`
	OriginalPath      string `orm:"original_storage_path"`
	EditedFileURL     string `orm:"edited_file_url"`
	EditedPath        string `orm:"edited_storage_path"`
	PosterURL         string `orm:"poster_url"`
	PosterStoragePath string `orm:"poster_storage_path"`
}

// RepairYoubanPublishCollectProfileMedia repairs historical collection notes
// whose stored media is missing. Recoverable notes are sent back through the
// normal collection media pipeline; notes without any source reference are
// soft-deleted together with their derived media and indexes.
func RepairYoubanPublishCollectProfileMedia(ctx context.Context, profileIds []int64) error {
	lastId := int64(0)
	repaired := 0
	deleted := 0
	scanned := 0
	for {
		profiles, err := collectProfileMediaRepairProfiles(ctx, lastId, collectProfileMediaRepairBatchSize, profileIds)
		if err != nil {
			return err
		}
		if len(profiles) == 0 {
			break
		}
		for _, profile := range profiles {
			if profile.Id <= 0 {
				continue
			}
			scanned++
			lastId = profile.Id
			media, err := collectProfileMediaRepairMediaRows(ctx, profile.Id)
			if err != nil {
				return err
			}
			missing := false
			for _, row := range media {
				if !collectProfileMediaRepairMediaPresent(row) {
					missing = true
					break
				}
			}
			if !missing || len(media) == 0 {
				continue
			}
			eventId, unrecoverable, err := repairCollectProfileMediaSource(ctx, profile, media)
			if err != nil {
				return err
			}
			if eventId > 0 && !unrecoverable {
				repaired++
				g.Log().Infof(ctx, "历史采集资料媒体已重新排队 profileId:%d eventId:%d", profile.Id, eventId)
				continue
			}
			if err = deleteUnrecoverableCollectProfile(ctx, profile.Id); err != nil {
				return err
			}
			deleted++
			g.Log().Warningf(ctx, "历史采集资料无可用TG源，已删除 profileId:%d sourceKey:%s", profile.Id, profile.SourceKey)
		}
		g.Log().Infof(ctx, "历史采集媒体修复进度 scanned:%d repaired:%d deleted:%d lastId:%d", scanned, repaired, deleted, lastId)
	}
	g.Log().Infof(ctx, "历史采集媒体修复完成 scanned:%d repaired:%d deleted:%d", scanned, repaired, deleted)
	return nil
}

func collectProfileMediaRepairProfiles(ctx context.Context, lastId int64, limit int, profileIds []int64) ([]collectProfileMediaRepairProfile, error) {
	rows := make([]collectProfileMediaRepairProfile, 0)
	mod := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		Fields("p.id,ps.tenant_id,p.source_key").
		InnerJoin("hg_youban_publish_profile_state ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Where("p.source_type", "youban_collect").
		WhereNull("p.deleted_at").
		WhereGT("p.id", lastId)
	if len(profileIds) > 0 {
		mod = mod.WhereIn("p.id", profileIds)
	}
	err := mod.
		OrderAsc("p.id").
		Limit(limit).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取历史采集资料失败")
	}
	return rows, nil
}

func collectProfileMediaRepairMediaRows(ctx context.Context, profileId int64) ([]collectProfileMediaRepairMedia, error) {
	rows := make([]collectProfileMediaRepairMedia, 0)
	err := g.DB().Model("hg_youban_publish_media").Safe().Ctx(ctx).
		Fields("id,media_type,file_url,storage_path,original_file_url,original_storage_path,edited_file_url,edited_storage_path,poster_url,poster_storage_path").
		Where("profile_id", profileId).
		WhereNull("deleted_at").
		OrderAsc("sort_index").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrapf(err, "读取资料媒体失败 profileId:%d", profileId)
	}
	return rows, nil
}

func collectProfileMediaRepairMediaPresent(row collectProfileMediaRepairMedia) bool {
	for _, path := range []string{row.StoragePath, row.OriginalPath, row.EditedPath} {
		if path != "" && collectProfileMediaRepairPathExists(path) {
			return true
		}
	}
	for _, value := range []string{row.FileURL, row.OriginalFileURL, row.EditedFileURL} {
		if collectProfileMediaRepairURLUsable(value) {
			return true
		}
	}
	return false
}

func collectProfileMediaRepairPathExists(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return false
	}
	value = strings.TrimPrefix(value, "/")
	value = strings.TrimPrefix(value, "resource/public/")
	value = strings.TrimPrefix(value, "storage/")
	candidates := []string{
		value,
		filepath.Join("/app/resource/public/storage", value),
		filepath.Join("/app/resource/public", value),
		filepath.Join("/app", value),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Size() > 0 {
			return true
		}
	}
	return false
}

func collectProfileMediaRepairURLUsable(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	if parsed.IsAbs() {
		return parsed.Host != "" && parsed.Scheme != ""
	}
	return collectProfileMediaRepairPathExists(value)
}

func repairCollectProfileMediaSource(ctx context.Context, profile collectProfileMediaRepairProfile, media []collectProfileMediaRepairMedia) (int64, bool, error) {
	prefix := "collect:"
	if !strings.HasPrefix(profile.SourceKey, prefix) {
		return 0, false, nil
	}
	value := strings.TrimPrefix(profile.SourceKey, prefix)
	separator := strings.LastIndex(value, ":")
	if separator <= 0 {
		return 0, false, nil
	}
	uniqueKey := value[:separator]
	if uniqueKey == "" {
		return 0, false, nil
	}
	var event struct {
		Id        int64 `orm:"id"`
		TenantId  int64 `orm:"tenant_id"`
		AccountId int64 `orm:"account_id"`
	}
	if err := g.DB().Model("hg_youban_publish_collect_event").Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id").
		Where("source_unique_key", uniqueKey).
		Where("tenant_id", profile.TenantId).
		OrderDesc("id").Limit(1).Scan(&event); err != nil {
		return 0, false, gerror.Wrapf(err, "读取采集源事件失败 profileId:%d", profile.Id)
	}
	if event.Id <= 0 {
		return 0, false, nil
	}
	rows, err := g.DB().Model("hg_youban_publish_collect_event_media").Safe().Ctx(ctx).
		Fields("id,source_file_id,source_message_ref,backup_message_id,meta_json,error_message").
		Where("event_id", event.Id).
		All()
	if err != nil {
		return 0, false, gerror.Wrapf(err, "读取采集源媒体失败 eventId:%d", event.Id)
	}
	if len(rows) == 0 {
		return 0, false, nil
	}
	recoverable := false
	for _, row := range rows {
		if strings.Contains(strings.ToUpper(row["error_message"].String()), "FILE_REFERENCE_EXPIRED") {
			return event.Id, true, nil
		}
		if strings.TrimSpace(row["source_file_id"].String()) != "" ||
			strings.TrimSpace(row["source_message_ref"].String()) != "" ||
			row["backup_message_id"].Int64() > 0 ||
			strings.TrimSpace(row["meta_json"].String()) != "" {
			recoverable = true
			break
		}
	}
	if !recoverable {
		return 0, false, nil
	}
	mediaIds := make([]int64, 0, len(media))
	for _, row := range media {
		if !collectProfileMediaRepairMediaPresent(row) && row.Id > 0 {
			mediaIds = append(mediaIds, row.Id)
		}
	}
	if len(mediaIds) == 0 {
		return 0, false, nil
	}
	now := gtime.Now()
	recoveryUpdatedAt := now.Add(-3*time.Minute - time.Second)
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, txErr := tx.Model("hg_youban_publish_collect_event_media").Safe().Ctx(ctx).
			Where("event_id", event.Id).
			Data(g.Map{
				"cache_status":         "pending",
				"cache_hit":            0,
				"download_duration_ms": 0,
				"download_bytes":       0,
				"download_error_type":  "",
				"file_url":             "",
				"storage_path":         "",
				"poster_url":           "",
				"next_retry_at":        nil,
				"error_message":        "历史资料媒体缺失，已重新排队补下载",
				"updated_at":           recoveryUpdatedAt,
			}).Update(); txErr != nil {
			return gerror.Wrap(txErr, "重置历史采集媒体失败")
		}
		_, txErr := tx.Model("hg_youban_publish_collect_event").Safe().Ctx(ctx).
			Where("id", event.Id).
			Data(g.Map{
				"status":        "media_pending",
				"processed_at":  nil,
				"error_message": "历史资料媒体缺失，已重新排队补下载",
				"updated_at":    recoveryUpdatedAt,
			}).Update()
		return txErr
	})
	if err != nil {
		return 0, false, err
	}
	return event.Id, false, nil
}

func deleteUnrecoverableCollectProfile(ctx context.Context, profileId int64) error {
	now := gtime.Now()
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, table := range []string{
			"hg_content_media",
			"hg_youban_publish_media_phash_bucket",
			"hg_youban_publish_media_phash_lsh",
			"hg_youban_publish_media",
			"hg_youban_publish_note_index",
			"hg_youban_publish_profile_state",
			"hg_youban_publish_channel_profile",
			"hg_content_source_map",
		} {
			if _, err := tx.Model(table).Safe().Ctx(ctx).Where("profile_id", profileId).Delete(); err != nil {
				return gerror.Wrapf(err, "清理不可恢复资料关联失败 table:%s", table)
			}
		}
		_, err := tx.Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
			Where("id", profileId).
			Data(g.Map{"status": 0, "deleted_at": now, "updated_at": now}).Update()
		return gerror.Wrap(err, "删除不可恢复资料失败")
	})
}
