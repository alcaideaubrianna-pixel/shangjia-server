package sys

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	basesysin "hotgo/internal/model/input/sysin"
)

func (s *sSysPublish) AdminMediaList(ctx context.Context, in *sysin.MediaListInp) (list []*sysin.MediaModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.mediaListByTenant(ctx, in.TaskId, account.TenantId)
}

func (s *sSysPublish) AdminMediaDelete(ctx context.Context, in *sysin.MediaDeleteInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteMediaByTenant(ctx, in.Id, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminMediaSort(ctx context.Context, in *sysin.MediaSortInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("媒体排序不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	task, err := s.getTaskByTenant(ctx, in.TaskId, account.TenantId)
	if err != nil {
		return err
	}
	return s.sortTaskMedia(ctx, in, task, 0)
}

func (s *sSysPublish) ServerMediaList(ctx context.Context, in *sysin.MediaListInp) (list []*sysin.MediaModel, err error) {
	if in == nil {
		return nil, gerror.New("任务ID不能为空")
	}
	return s.mediaListByTenant(ctx, in.TaskId, 0)
}

func (s *sSysPublish) ServerMediaDelete(ctx context.Context, in *sysin.MediaDeleteInp) (err error) {
	if in == nil {
		return gerror.New("媒体ID不能为空")
	}
	return s.deleteMediaByTenant(ctx, in.Id, 0, contexts.GetUserId(ctx))
}

func (s *sSysPublish) MyMediaList(ctx context.Context, in *sysin.MediaListInp) (list []*sysin.MediaModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.mediaList(ctx, in.TaskId, account.Id)
}

func (s *sSysPublish) MyMediaDelete(ctx context.Context, in *sysin.MediaDeleteInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteMedia(ctx, in.Id, account.Id)
}

func (s *sSysPublish) MyMediaSort(ctx context.Context, in *sysin.MediaSortInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("媒体排序不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	task, err := s.getTask(ctx, in.TaskId, account.Id)
	if err != nil {
		return err
	}
	return s.sortTaskMedia(ctx, in, task, account.Id)
}

func (s *sSysPublish) sortTaskMedia(ctx context.Context, in *sysin.MediaSortInp, task gdb.Record, accountId int64) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		profileId := task["profile_id"].Int64()
		if profileId > 0 {
			release, lockErr := s.lockTaskMediaSyncTx(ctx, tx, in.TaskId, profileId)
			if lockErr != nil {
				return lockErr
			}
			defer release()
		}
		for _, item := range in.Items {
			mod := tx.Model(publishMediaTable).Ctx(ctx).
				Where("id", item.Id).
				Where("task_id", in.TaskId).
				WhereNull("deleted_at")
			if accountId > 0 {
				mod = mod.Where("account_id", accountId)
			}
			result, updateErr := mod.Data(g.Map{
				"purpose":    item.Purpose,
				"sort_index": item.SortIndex,
				"updated_by": contexts.GetUserId(ctx),
				"updated_at": gtime.Now(),
			}).
				Update()
			if updateErr != nil {
				return gerror.Wrap(updateErr, "更新媒体排序失败")
			}
			affected, _ := result.RowsAffected()
			if affected == 0 {
				return gerror.New("媒体不存在或无权操作")
			}
		}
		if profileId > 0 {
			if syncErr := s.syncTaskMediaToProfile(ctx, tx, in.TaskId, profileId); syncErr != nil {
				return syncErr
			}
		}
		return nil
	})
}

func (s *sSysPublish) MyProfileImageSearch(ctx context.Context, in *sysin.ProfileImageSearchInp, file *ghttp.UploadFile) (list []*sysin.NoteModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ProfileImageSearchInp{}
	}
	in.TenantId = account.TenantId
	in.AccountId = account.Id
	return s.profileImageSearch(ctx, in, file, sysin.ProfilePermissionCreator)
}

func (s *sSysPublish) AdminProfileImageSearch(ctx context.Context, in *sysin.ProfileImageSearchInp, file *ghttp.UploadFile) (list []*sysin.NoteModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ProfileImageSearchInp{}
	}
	in.TenantId = account.TenantId
	in.AccountId = 0
	return s.profileImageSearch(ctx, in, file, sysin.ProfilePermissionAdmin)
}

func (s *sSysPublish) profileImageSearch(ctx context.Context, in *sysin.ProfileImageSearchInp, file *ghttp.UploadFile, permission string) (list []*sysin.NoteModel, totalCount int, err error) {
	normalizeProfileImageSearchInput(in)
	queryHash, err := uploadImagePHashValue(file)
	if err != nil {
		return nil, 0, err
	}
	profileIds, totalCount, err := s.findSimilarProfileIdsByPHash(ctx, queryHash, in, nil)
	if err != nil {
		return nil, 0, err
	}
	if len(profileIds) == 0 {
		return []*sysin.NoteModel{}, totalCount, nil
	}
	list = make([]*sysin.NoteModel, 0, len(profileIds))
	for _, profileId := range profileIds {
		profile, viewErr := s.profileView(ctx, profileId, in.TenantId, in.AccountId)
		if viewErr != nil {
			return nil, 0, viewErr
		}
		markProfilePermission(profile, permission)
		note := &sysin.NoteModel{ProfileModel: *profile}
		note.Media, err = s.mediaListByProfile(ctx, profile.Id, profile.TenantId, profile.AccountId)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, note)
	}
	return list, totalCount, nil
}

func (s *sSysPublish) profileImageSearchByAccountIds(ctx context.Context, in *sysin.ProfileImageSearchInp, file *ghttp.UploadFile, accountIds []int64, viewer *sysin.AccountModel) (list []*sysin.NoteModel, totalCount int, err error) {
	normalizeProfileImageSearchInput(in)
	accountIds = uniqueIds(accountIds)
	if len(accountIds) == 0 {
		return []*sysin.NoteModel{}, 0, nil
	}
	queryHash, err := uploadImagePHashValue(file)
	if err != nil {
		return nil, 0, err
	}
	profileIds, totalCount, err := s.findSimilarProfileIdsByPHash(ctx, queryHash, in, accountIds)
	if err != nil {
		return nil, 0, err
	}
	if len(profileIds) == 0 {
		return []*sysin.NoteModel{}, totalCount, nil
	}
	list = make([]*sysin.NoteModel, 0, len(profileIds))
	for _, profileId := range profileIds {
		profile, viewErr := s.profileView(ctx, profileId, 0, 0)
		if viewErr != nil {
			return nil, 0, viewErr
		}
		if !containsInt64(accountIds, profile.AccountId) {
			continue
		}
		markProfilePermission(profile, profilePermissionForViewer(viewer, profile))
		note := &sysin.NoteModel{ProfileModel: *profile}
		note.Media, err = s.mediaListByProfile(ctx, profile.Id, profile.TenantId, profile.AccountId)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, note)
	}
	return list, totalCount, nil
}

func normalizeProfileImageSearchInput(in *sysin.ProfileImageSearchInp) {
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 {
		in.PerPage = 20
	}
	if in.PerPage > 50 {
		in.PerPage = 50
	}
	if in.Threshold <= 0 {
		in.Threshold = 12
	}
	if in.Threshold > 32 {
		in.Threshold = 32
	}
	in.Keyword = strings.TrimSpace(in.Keyword)
	in.Province = strings.TrimSpace(in.Province)
	in.City = strings.TrimSpace(in.City)
	in.Tag = strings.TrimSpace(in.Tag)
}

type publishProfilePHashDistance struct {
	Distance  int
	ProfileId int64
}

func (s *sSysPublish) findSimilarProfileIdsByPHash(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, accountIds []int64) (profileIds []int64, totalCount int, err error) {
	candidateProfileIds, err := s.profileImageSearchCandidateProfileIds(ctx, &in.ProfileListInp, accountIds)
	if err != nil {
		return nil, 0, err
	}
	if candidateProfileIds != nil && len(candidateProfileIds) == 0 {
		return []int64{}, 0, nil
	}
	mediaMod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("profile_id,perceptual_hash").
		Where("media_type", "image").
		WhereNot("perceptual_hash", "").
		WhereNull("deleted_at")
	if in.TenantId > 0 {
		mediaMod = mediaMod.Where("tenant_id", in.TenantId)
	}
	if in.AccountId > 0 {
		mediaMod = mediaMod.Where("account_id", in.AccountId)
	}
	if len(accountIds) > 0 {
		mediaMod = mediaMod.WhereIn("account_id", accountIds)
	}
	if len(candidateProfileIds) > 0 {
		mediaMod = mediaMod.WhereIn("profile_id", candidateProfileIds)
	}
	rows, err := mediaMod.All()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "查询图片相似资料失败")
	}
	distanceByProfile := make(map[int64]int)
	for _, row := range rows {
		profileId := row["profile_id"].Int64()
		if profileId <= 0 {
			continue
		}
		hash, ok := parseUploadPHash(row["perceptual_hash"].String())
		if !ok {
			continue
		}
		distance, distanceErr := queryHash.Distance(hash)
		if distanceErr != nil || distance > in.Threshold {
			continue
		}
		current, exists := distanceByProfile[profileId]
		if !exists || distance < current {
			distanceByProfile[profileId] = distance
		}
	}
	items := make([]publishProfilePHashDistance, 0, len(distanceByProfile))
	for profileId, distance := range distanceByProfile {
		items = append(items, publishProfilePHashDistance{ProfileId: profileId, Distance: distance})
	}
	items, err = s.filterVisibleProfilePHashItems(ctx, items, &in.ProfileListInp, accountIds)
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Distance == items[j].Distance {
			return items[i].ProfileId > items[j].ProfileId
		}
		return items[i].Distance < items[j].Distance
	})
	totalCount = len(items)
	if totalCount == 0 {
		return []int64{}, 0, nil
	}
	start := (in.Page - 1) * in.PerPage
	if start < 0 {
		start = 0
	}
	if start >= totalCount {
		return []int64{}, totalCount, nil
	}
	end := int(math.Min(float64(start+in.PerPage), float64(totalCount)))
	profileIds = make([]int64, 0, end-start)
	for _, item := range items[start:end] {
		profileIds = append(profileIds, item.ProfileId)
	}
	return profileIds, totalCount, nil
}

func (s *sSysPublish) profileImageSearchCandidateProfileIds(ctx context.Context, in *sysin.ProfileListInp, accountIds []int64) ([]int64, error) {
	if !hasProfileSearchFilters(in) {
		return nil, nil
	}
	base, err := s.profileBaseModel(ctx, in.TenantId, in.AccountId)
	if err != nil {
		return nil, err
	}
	base = s.applyProfileFilters(ctx, base, in)
	if len(accountIds) > 0 {
		base = base.WhereIn("t.account_id", accountIds)
	}
	rows, err := base.Fields("p.id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取图片搜索候选资料失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		id := row["id"].Int64()
		if id > 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func hasProfileSearchFilters(in *sysin.ProfileListInp) bool {
	if in == nil {
		return false
	}
	return strings.TrimSpace(in.Keyword) != "" ||
		strings.TrimSpace(in.Province) != "" ||
		strings.TrimSpace(in.City) != "" ||
		strings.TrimSpace(in.Tag) != "" ||
		strings.TrimSpace(in.ReviewStatus) != "" ||
		strings.TrimSpace(in.Visibility) != "" ||
		in.Status > 0
}

func (s *sSysPublish) filterVisibleProfilePHashItems(ctx context.Context, items []publishProfilePHashDistance, in *sysin.ProfileListInp, accountIds []int64) ([]publishProfilePHashDistance, error) {
	if len(items) == 0 {
		return items, nil
	}
	profileIds := make([]int64, 0, len(items))
	for _, item := range items {
		profileIds = append(profileIds, item.ProfileId)
	}
	base, err := s.profileBaseModel(ctx, in.TenantId, in.AccountId)
	if err != nil {
		return nil, err
	}
	base = s.applyProfileFilters(ctx, base, in)
	if len(accountIds) > 0 {
		base = base.WhereIn("t.account_id", accountIds)
	}
	rows, err := base.Fields("p.id").WhereIn("p.id", profileIds).All()
	if err != nil {
		return nil, gerror.Wrap(err, "过滤图片相似资料失败")
	}
	visibleIds := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		id := row["id"].Int64()
		if id > 0 {
			visibleIds[id] = struct{}{}
		}
	}
	if len(visibleIds) == len(items) {
		return items, nil
	}
	filtered := make([]publishProfilePHashDistance, 0, len(visibleIds))
	for _, item := range items {
		if _, ok := visibleIds[item.ProfileId]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func uploadImagePHash(file *ghttp.UploadFile) (string, error) {
	hash, err := uploadImagePHashValue(file)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%016x", hash.GetHash()), nil
}

func uploadImagePHashValue(file *ghttp.UploadFile) (*goimagehash.ImageHash, error) {
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, gerror.Wrap(err, "读取上传图片失败")
	}
	defer reader.Close()
	img, _, err := image.Decode(reader)
	if err != nil {
		return nil, gerror.New("图片格式不支持，请上传 JPG、PNG 或 GIF")
	}
	hash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return nil, gerror.Wrap(err, "计算图片感知哈希失败")
	}
	return hash, nil
}

func parseUploadPHash(value string) (*goimagehash.ImageHash, bool) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return nil, false
	}
	normalized = strings.TrimPrefix(normalized, "0x")
	hashValue, err := strconv.ParseUint(normalized, 16, 64)
	if err != nil {
		return nil, false
	}
	return goimagehash.NewImageHash(hashValue, goimagehash.PHash), true
}

func (s *sSysPublish) saveMediaAttachment(ctx context.Context, task gdb.Record, in *sysin.MediaUploadInp, attachment *basesysin.AttachmentListModel, poster *basesysin.AttachmentListModel, originalAttachment *basesysin.AttachmentListModel, perceptualHash string) (res *sysin.MediaModel, err error) {
	if attachment == nil || attachment.Id <= 0 {
		return nil, gerror.New("附件上传失败")
	}
	if err = ensureMediaEditColumns(ctx); err != nil {
		return nil, err
	}
	now := gtime.Now()
	editStatus := strings.TrimSpace(in.EditStatus)
	if editStatus == "" {
		editStatus = "raw"
	}
	if editStatus != "raw" && editStatus != "edited" {
		editStatus = "edited"
	}
	if in.EditStatus == "" && (in.MediaId > 0 || originalAttachment != nil) {
		editStatus = "edited"
	}
	data := g.Map{
		"tenant_id":            task["tenant_id"].Int64(),
		"merchant_id":          task["tenant_id"].Int64(),
		"account_id":           task["account_id"].Int64(),
		"task_id":              task["id"].Int64(),
		"profile_id":           task["profile_id"].Int64(),
		"attachment_id":        attachment.Id,
		"edited_attachment_id": 0,
		"media_type":           in.MediaType,
		"purpose":              in.Purpose,
		"name":                 attachment.Name,
		"file_url":             normalizeMediaFileURL(attachment.FileUrl, attachment.Path),
		"edited_file_url":      "",
		"poster_url":           normalizeMediaFileURL(posterFileUrl(poster), posterStoragePath(poster)),
		"poster_storage_path":  posterStoragePath(poster),
		"storage_path":         attachment.Path,
		"edited_storage_path":  "",
		"mime_type":            attachment.MimeType,
		"md5":                  attachment.Md5,
		"perceptual_hash":      perceptualHash,
		"edit_config_json":     strings.TrimSpace(in.EditConfigJson),
		"edit_status":          editStatus,
		"tg_file_id":           "",
		"tg_thumb_file_id":     "",
		"tg_cache_asset_hash":  "",
		"tg_cache_status":      tgCacheStatusInvalid,
		"size":                 attachment.Size,
		"sort_index":           in.SortIndex,
		"status":               1,
		"updated_by":           contexts.GetUserId(ctx),
		"updated_at":           now,
	}
	var existing gdb.Record
	if in.MediaId > 0 {
		existing, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("id", in.MediaId).
			Where("task_id", task["id"].Int64()).
			WhereNull("deleted_at").
			One()
	} else {
		existing, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("task_id", task["id"].Int64()).
			Where("attachment_id", attachment.Id).
			WhereNull("deleted_at").
			One()
	}
	if err != nil {
		return nil, gerror.Wrap(err, "检查任务媒体失败")
	}
	mediaId := existing["id"].Int64()
	if editStatus == "edited" {
		data["edited_attachment_id"] = attachment.Id
		data["edited_file_url"] = normalizeMediaFileURL(attachment.FileUrl, attachment.Path)
		data["edited_storage_path"] = attachment.Path
	}
	if editStatus == mediaEditStatusRaw {
		data["original_attachment_id"] = attachment.Id
		data["original_file_url"] = normalizeMediaFileURL(attachment.FileUrl, attachment.Path)
		data["original_storage_path"] = attachment.Path
	} else if originalAttachment != nil && originalAttachment.Id > 0 {
		data["original_attachment_id"] = originalAttachment.Id
		data["original_file_url"] = normalizeMediaFileURL(originalAttachment.FileUrl, originalAttachment.Path)
		data["original_storage_path"] = originalAttachment.Path
	} else if mediaId == 0 {
		data["original_attachment_id"] = attachment.Id
		data["original_file_url"] = normalizeMediaFileURL(attachment.FileUrl, attachment.Path)
		data["original_storage_path"] = attachment.Path
	} else if mediaId > 0 && existing["original_attachment_id"].Int64() <= 0 {
		data["original_attachment_id"] = existing["attachment_id"].Int64()
		data["original_file_url"] = normalizeMediaFileURL(existing["file_url"].String(), existing["storage_path"].String())
		data["original_storage_path"] = existing["storage_path"].String()
	}
	if mediaId > 0 {
		_, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", mediaId).Data(data).Update()
	} else {
		data["created_by"] = contexts.GetUserId(ctx)
		data["created_at"] = now
		mediaId, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	}
	if err != nil {
		return nil, gerror.Wrap(err, "保存任务媒体失败")
	}
	if err = s.refreshTaskMediaCount(ctx, task["id"].Int64()); err != nil {
		return nil, err
	}
	if task["profile_id"].Int64() > 0 {
		if err = s.withTaskMediaSyncLock(ctx, task["id"].Int64(), task["profile_id"].Int64(), func(ctx context.Context, tx gdb.TX) error {
			return s.syncTaskMediaToProfile(ctx, tx, task["id"].Int64(), task["profile_id"].Int64())
		}); err != nil {
			return nil, err
		}
	}
	var media *sysin.MediaModel
	if err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", mediaId).Scan(&media); err != nil {
		return nil, gerror.Wrap(err, "读取任务媒体失败")
	}
	normalizeMediaListFileURL([]*sysin.MediaModel{media})
	return media, nil
}

func (s *sSysPublish) mediaList(ctx context.Context, taskId int64, accountId int64) (list []*sysin.MediaModel, err error) {
	if taskId <= 0 {
		return nil, gerror.New("任务ID不能为空")
	}
	if err = ensureMediaEditColumns(ctx); err != nil {
		return nil, err
	}
	if _, err = s.getTask(ctx, taskId, accountId); err != nil {
		return nil, err
	}
	err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		OrderAsc("sort_index").
		OrderAsc("id").
		Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "获取任务媒体失败")
	}
	if list == nil {
		list = []*sysin.MediaModel{}
	}
	normalizeMediaListFileURL(list)
	return list, nil
}

func (s *sSysPublish) mediaListByTenant(ctx context.Context, taskId int64, tenantId int64) (list []*sysin.MediaModel, err error) {
	if taskId <= 0 {
		return nil, gerror.New("任务ID不能为空")
	}
	if err = ensureMediaEditColumns(ctx); err != nil {
		return nil, err
	}
	if _, err = s.getTaskByTenant(ctx, taskId, tenantId); err != nil {
		return nil, err
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	err = mod.
		OrderAsc("sort_index").
		OrderAsc("id").
		Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "获取任务媒体失败")
	}
	if list == nil {
		list = []*sysin.MediaModel{}
	}
	normalizeMediaListFileURL(list)
	return list, nil
}

func normalizeMediaListFileURL(list []*sysin.MediaModel) {
	for _, item := range list {
		if item == nil {
			continue
		}
		if (item.EditStatus == "" || item.EditStatus == "raw") && isLikelyEditedMedia(item) {
			item.EditStatus = "edited"
		}
		item.FileUrl = normalizeMediaFileURL(item.FileUrl, item.StoragePath)
		item.OriginalFileUrl = normalizeMediaFileURL(item.OriginalFileUrl, item.OriginalStoragePath)
		item.EditedFileUrl = normalizeMediaFileURL(item.EditedFileUrl, item.EditedStoragePath)
		item.PosterUrl = normalizeMediaFileURL(item.PosterUrl, item.PosterStoragePath)
		asset := newProfileMediaFromModel(item).EffectiveAsset()
		if strings.TrimSpace(asset.FileUrl) != "" {
			item.FileUrl = normalizeMediaFileURL(asset.FileUrl, asset.StoragePath)
		}
		if strings.TrimSpace(asset.StoragePath) != "" {
			item.StoragePath = asset.StoragePath
		}
	}
}

func isLikelyEditedMedia(item *sysin.MediaModel) bool {
	if item == nil {
		return false
	}
	if item.EditedAttachmentId > 0 || strings.TrimSpace(item.EditedStoragePath) != "" || strings.TrimSpace(item.EditedFileUrl) != "" {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(item.Name))
	return strings.Contains(name, "-edited.") || strings.Contains(name, "_edited.")
}

func ensureMediaEditColumns(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		_, err := g.DB().Exec(ctx, `
ALTER TABLE "hg_youban_publish_media"
  ADD COLUMN IF NOT EXISTS "original_attachment_id" bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS "original_file_url" varchar(1024) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "original_storage_path" varchar(1024) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "edited_attachment_id" bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS "edited_file_url" varchar(1024) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "edited_storage_path" varchar(1024) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "edit_config_json" text,
  ADD COLUMN IF NOT EXISTS "edit_status" varchar(16) NOT NULL DEFAULT 'raw',
  ADD COLUMN IF NOT EXISTS "tg_cache_asset_hash" varchar(1024) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "tg_cache_status" varchar(16) NOT NULL DEFAULT 'invalid'`)
		if err != nil {
			return gerror.Wrap(err, "检查任务媒体编辑字段失败")
		}
		_, err = g.DB().Exec(ctx, `ALTER TABLE "hg_youban_publish_media" ALTER COLUMN "tg_cache_asset_hash" TYPE varchar(1024)`)
		if err != nil {
			return gerror.Wrap(err, "检查TG媒体缓存字段长度失败")
		}
		_, err = g.DB().Exec(ctx, `
UPDATE "hg_youban_publish_media"
SET "original_attachment_id" = "attachment_id",
    "original_file_url" = "file_url",
    "original_storage_path" = "storage_path"
WHERE "original_attachment_id" = 0 AND "attachment_id" > 0`)
		if err != nil {
			return gerror.Wrap(err, "补齐任务媒体原始素材字段失败")
		}
		_, err = g.DB().Exec(ctx, `
UPDATE "hg_youban_publish_media"
SET "edit_status" = 'edited'
WHERE ("edit_status" = '' OR "edit_status" = 'raw' OR "edit_status" IS NULL)
  AND ("edited_attachment_id" > 0 OR "edited_storage_path" <> '' OR "edited_file_url" <> '' OR lower("name") LIKE '%-edited.%' OR lower("name") LIKE '%_edited.%')`)
		if err != nil {
			return gerror.Wrap(err, "补齐任务媒体编辑状态失败")
		}
		_, err = g.DB().Exec(ctx, `
UPDATE "hg_youban_publish_media"
SET "tg_cache_status" = 'valid',
    "tg_cache_asset_hash" = COALESCE(NULLIF("md5", ''), NULLIF("storage_path", ''), NULLIF("file_url", ''))
WHERE "tg_file_id" <> ''
  AND ("tg_cache_status" = '' OR "tg_cache_status" = 'invalid' OR "tg_cache_status" IS NULL)
  AND "edit_status" = 'raw'`)
		if err != nil {
			return gerror.Wrap(err, "补齐TG媒体缓存状态失败")
		}
		return nil
	}
	statements := []string{
		"ALTER TABLE `hg_youban_publish_media` ADD COLUMN `original_attachment_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '原始HotGo附件ID' AFTER `attachment_id`",
		"ALTER TABLE `hg_youban_publish_media` ADD COLUMN `original_file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '原始访问地址' AFTER `file_url`",
		"ALTER TABLE `hg_youban_publish_media` ADD COLUMN `original_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '原始存储路径' AFTER `storage_path`",
		"ALTER TABLE `hg_youban_publish_media` ADD COLUMN `edited_attachment_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '编辑后HotGo附件ID' AFTER `original_attachment_id`",
		"ALTER TABLE `hg_youban_publish_media` ADD COLUMN `edited_file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '编辑后访问地址' AFTER `original_file_url`",
		"ALTER TABLE `hg_youban_publish_media` ADD COLUMN `edited_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '编辑后存储路径' AFTER `original_storage_path`",
		"ALTER TABLE `hg_youban_publish_media` ADD COLUMN `edit_config_json` text COMMENT '图片编辑配置' AFTER `perceptual_hash`",
		"ALTER TABLE `hg_youban_publish_media` ADD COLUMN `edit_status` varchar(16) NOT NULL DEFAULT 'raw' COMMENT '编辑状态：raw/edited' AFTER `edit_config_json`",
		"ALTER TABLE `hg_youban_publish_media` ADD COLUMN `tg_cache_asset_hash` varchar(1024) NOT NULL DEFAULT '' COMMENT 'TG缓存素材Hash' AFTER `tg_thumb_file_id`",
		"ALTER TABLE `hg_youban_publish_media` ADD COLUMN `tg_cache_status` varchar(16) NOT NULL DEFAULT 'invalid' COMMENT 'TG缓存状态' AFTER `tg_cache_asset_hash`",
		"ALTER TABLE `hg_youban_publish_media` MODIFY COLUMN `tg_cache_asset_hash` varchar(1024) NOT NULL DEFAULT '' COMMENT 'TG缓存素材Hash'",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil && !isIgnorableImportTaskServerIPColumnError(err) {
			return gerror.Wrap(err, "检查任务媒体编辑字段失败")
		}
	}
	if _, err := g.DB().Exec(ctx, "UPDATE `hg_youban_publish_media` SET `original_attachment_id`=`attachment_id`, `original_file_url`=`file_url`, `original_storage_path`=`storage_path` WHERE `original_attachment_id`=0 AND `attachment_id`>0"); err != nil {
		return gerror.Wrap(err, "补齐任务媒体原始素材字段失败")
	}
	if _, err := g.DB().Exec(ctx, "UPDATE `hg_youban_publish_media` SET `edit_status`='edited' WHERE (`edit_status`='' OR `edit_status`='raw' OR `edit_status` IS NULL) AND (`edited_attachment_id`>0 OR `edited_storage_path`<>'' OR `edited_file_url`<>'' OR lower(`name`) LIKE '%-edited.%' OR lower(`name`) LIKE '%_edited.%')"); err != nil {
		return gerror.Wrap(err, "补齐任务媒体编辑状态失败")
	}
	if _, err := g.DB().Exec(ctx, "UPDATE `hg_youban_publish_media` SET `tg_cache_status`='valid', `tg_cache_asset_hash`=COALESCE(NULLIF(`md5`, ''), NULLIF(`storage_path`, ''), NULLIF(`file_url`, '')) WHERE `tg_file_id`<>'' AND (`tg_cache_status`='' OR `tg_cache_status`='invalid' OR `tg_cache_status` IS NULL) AND `edit_status`='raw'"); err != nil {
		return gerror.Wrap(err, "补齐TG媒体缓存状态失败")
	}
	return nil
}

func normalizeMediaFileURL(fileURL string, storagePath string) string {
	fileURL = strings.TrimSpace(fileURL)
	storagePath = strings.TrimSpace(storagePath)
	if storagePath == "" {
		return fileURL
	}
	if fileURL == "" || isLocalMediaURL(fileURL) {
		return "/" + strings.TrimLeft(storagePath, "/")
	}
	return fileURL
}

func isLocalMediaURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	ip := net.ParseIP(host)
	return host == "localhost" || host == "0.0.0.0" || host == "::1" || ip != nil && ip.IsLoopback()
}

func (s *sSysPublish) deleteMedia(ctx context.Context, id int64, accountId int64) (err error) {
	if id <= 0 {
		return gerror.New("媒体ID不能为空")
	}
	if err = ensureMediaEditColumns(ctx); err != nil {
		return err
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", id).WhereNull("deleted_at")
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return gerror.Wrap(err, "读取任务媒体失败")
	}
	if row.IsEmpty() {
		return gerror.New("任务媒体不存在")
	}
	if _, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{
		"tg_file_id":          "",
		"tg_thumb_file_id":    "",
		"tg_cache_asset_hash": "",
		"tg_cache_status":     tgCacheStatusInvalid,
		"deleted_by":          contexts.GetUserId(ctx),
		"deleted_at":          gtime.Now(),
	}).Update(); err != nil {
		return gerror.Wrap(err, "删除任务媒体失败")
	}
	if err = s.refreshTaskMediaCount(ctx, row["task_id"].Int64()); err != nil {
		return err
	}
	if row["profile_id"].Int64() > 0 {
		mediaColumns := dao.ContentMedia.Columns()
		sourceAssetId := row["original_attachment_id"].Int64()
		if sourceAssetId <= 0 {
			sourceAssetId = row["attachment_id"].Int64()
		}
		_, _ = dao.ContentMedia.Ctx(ctx).
			Where(mediaColumns.ProfileId, row["profile_id"].Int64()).
			Where(mediaColumns.SourceAssetId, sourceAssetId).
			Data(g.Map{mediaColumns.DeletedAt: gtime.Now()}).
			Update()
		if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			return s.syncTaskMediaToProfile(ctx, tx, row["task_id"].Int64(), row["profile_id"].Int64())
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) deleteMediaByTenant(ctx context.Context, id int64, tenantId int64, operatorId int64) (err error) {
	if id <= 0 {
		return gerror.New("媒体ID不能为空")
	}
	if err = ensureMediaEditColumns(ctx); err != nil {
		return err
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	row, err := mod.One()
	if err != nil {
		return gerror.Wrap(err, "读取任务媒体失败")
	}
	if row.IsEmpty() {
		return gerror.New("任务媒体不存在")
	}
	updateMod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", id)
	if tenantId > 0 {
		updateMod = updateMod.Where("tenant_id", tenantId)
	}
	if _, err = updateMod.Data(g.Map{
		"tg_file_id":          "",
		"tg_thumb_file_id":    "",
		"tg_cache_asset_hash": "",
		"tg_cache_status":     tgCacheStatusInvalid,
		"deleted_by":          operatorId,
		"deleted_at":          gtime.Now(),
	}).Update(); err != nil {
		return gerror.Wrap(err, "删除任务媒体失败")
	}
	if err = s.refreshTaskMediaCount(ctx, row["task_id"].Int64()); err != nil {
		return err
	}
	if row["profile_id"].Int64() > 0 {
		mediaColumns := dao.ContentMedia.Columns()
		sourceAssetId := row["original_attachment_id"].Int64()
		if sourceAssetId <= 0 {
			sourceAssetId = row["attachment_id"].Int64()
		}
		_, _ = dao.ContentMedia.Ctx(ctx).
			Where(mediaColumns.ProfileId, row["profile_id"].Int64()).
			Where(mediaColumns.SourceAssetId, sourceAssetId).
			Data(g.Map{mediaColumns.DeletedAt: gtime.Now()}).
			Update()
		if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			return s.syncTaskMediaToProfile(ctx, tx, row["task_id"].Int64(), row["profile_id"].Int64())
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) refreshTaskMediaCount(ctx context.Context, taskId int64) error {
	count, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "统计任务媒体失败")
	}
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).Data(g.Map{
		"media_count": count,
		"updated_at":  gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新任务媒体数量失败")
	}
	return nil
}

func taskMediaSyncLockKey(taskId int64, profileId int64) string {
	return fmt.Sprintf("youban_publish:media_sync:%d:%d", taskId, profileId)
}

func (s *sSysPublish) withTaskMediaSyncLock(ctx context.Context, taskId int64, profileId int64, fn func(ctx context.Context, tx gdb.TX) error) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		release, err := s.lockTaskMediaSyncTx(ctx, tx, taskId, profileId)
		if err != nil {
			return err
		}
		defer release()
		return fn(ctx, tx)
	})
}

func (s *sSysPublish) lockTaskMediaSyncTx(ctx context.Context, tx gdb.TX, taskId int64, profileId int64) (func(), error) {
	key := taskMediaSyncLockKey(taskId, profileId)
	dbType := strings.ToLower(g.DB().GetConfig().Type)
	switch dbType {
	case consts.DBPgsql:
		if _, err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", key); err != nil {
			return func() {}, gerror.Wrap(err, "获取资料媒体同步锁失败")
		}
		return func() {}, nil
	default:
		value, err := tx.GetValue("SELECT GET_LOCK(?, 60)", key)
		if err != nil {
			return func() {}, gerror.Wrap(err, "获取资料媒体同步锁失败")
		}
		if value.Int() != 1 {
			return func() {}, gerror.New("获取资料媒体同步锁超时")
		}
		return func() {
			_, _ = tx.Exec("SELECT RELEASE_LOCK(?)", key)
		}, nil
	}
}

func (s *sSysPublish) syncTaskMediaToProfile(ctx context.Context, tx gdb.TX, taskId int64, profileId int64) error {
	var list []*sysin.MediaModel
	if err := tx.Model(publishMediaTable).Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		OrderAsc("sort_index").
		OrderAsc("id").
		Scan(&list); err != nil {
		return gerror.Wrap(err, "读取任务媒体失败")
	}
	if len(list) == 0 {
		now := gtime.Now()
		profileColumns := dao.ContentProfile.Columns()
		data := g.Map{
			profileColumns.ImageCount: 0,
			profileColumns.VideoCount: 0,
			profileColumns.UpdatedAt:  now,
		}
		if _, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(profileColumns.Id, profileId).Data(data).Update(); err != nil {
			return gerror.Wrap(err, "更新资料媒体数量失败")
		}
		return nil
	}
	mediaColumns := dao.ContentMedia.Columns()
	now := gtime.Now()
	imageCount := 0
	videoCount := 0
	var coverMediaId int64
	for _, item := range list {
		mediaType := item.MediaType
		if mediaType == "" {
			mediaType = "image"
		}
		if mediaType == "video" {
			videoCount++
		} else {
			imageCount++
		}
		asset := newProfileMediaFromModel(item).EffectiveAsset()
		previewStoragePath := asset.StoragePath
		if mediaType == "video" && strings.TrimSpace(item.PosterStoragePath) != "" {
			previewStoragePath = item.PosterStoragePath
		}
		sourceAssetId := item.OriginalAttachmentId
		if sourceAssetId <= 0 {
			sourceAssetId = item.AttachmentId
		}
		originalStoragePath := strings.TrimSpace(item.OriginalStoragePath)
		if originalStoragePath == "" {
			originalStoragePath = item.StoragePath
		}
		data := g.Map{
			mediaColumns.ProfileId:           profileId,
			mediaColumns.SourceAssetId:       sourceAssetId,
			mediaColumns.MediaType:           mediaType,
			mediaColumns.SortIndex:           item.SortIndex,
			mediaColumns.OriginalStoragePath: originalStoragePath,
			mediaColumns.DisplayStoragePath:  asset.StoragePath,
			mediaColumns.PreviewStoragePath:  previewStoragePath,
			mediaColumns.BinaryMd5:           item.Md5,
			mediaColumns.PerceptualHash:      item.PerceptualHash,
			mediaColumns.ProcessStatus:       "raw",
			mediaColumns.EncryptStatus:       "none",
			mediaColumns.Status:              1,
			mediaColumns.DeletedAt:           nil,
			mediaColumns.UpdatedAt:           now,
		}
		existing, err := tx.Model(dao.ContentMedia.Table()).Ctx(ctx).
			Where(mediaColumns.ProfileId, profileId).
			Where(mediaColumns.SourceAssetId, sourceAssetId).
			Fields(mediaColumns.Id).
			Value()
		if err != nil {
			return gerror.Wrap(err, "检查资料媒体失败")
		}
		if existing.Int64() > 0 {
			if _, err = tx.Model(dao.ContentMedia.Table()).Ctx(ctx).Where(mediaColumns.Id, existing.Int64()).Data(data).Update(); err != nil {
				return gerror.Wrap(err, "更新资料媒体失败")
			}
			if coverMediaId == 0 && mediaType == "image" {
				coverMediaId = existing.Int64()
			}
			continue
		}
		data[mediaColumns.CreatedAt] = now
		id, err := tx.Model(dao.ContentMedia.Table()).Ctx(ctx).Data(data).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "创建资料媒体失败")
		}
		if coverMediaId == 0 && mediaType == "image" {
			coverMediaId = id
		}
	}
	if _, err := tx.Model(publishMediaTable).Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		Data(g.Map{"profile_id": profileId, "updated_at": now}).
		Update(); err != nil {
		return gerror.Wrap(err, "回写任务媒体资料ID失败")
	}
	profileColumns := dao.ContentProfile.Columns()
	data := g.Map{
		profileColumns.ImageCount: imageCount,
		profileColumns.VideoCount: videoCount,
		profileColumns.UpdatedAt:  now,
	}
	if coverMediaId > 0 {
		data[profileColumns.CoverMediaId] = coverMediaId
	}
	if _, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(profileColumns.Id, profileId).Data(data).Update(); err != nil {
		return gerror.Wrap(err, "更新资料媒体数量失败")
	}
	return nil
}
