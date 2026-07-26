package sys

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net"
	"net/url"
	"os"
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
	"hotgo/internal/library/storager"
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

func (s *sSysPublish) AdminMediaReconcile(ctx context.Context, in *sysin.MediaReconcileInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("媒体清单不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	task, err := s.getTaskByTenant(ctx, in.TaskId, account.TenantId)
	if err != nil {
		return err
	}
	for _, id := range in.RemovedMediaIds {
		mediaId, resolveErr := s.resolveTaskMediaId(ctx, task, id, 0)
		if resolveErr != nil {
			return resolveErr
		}
		if err = s.deleteMediaByTenant(ctx, mediaId, account.TenantId, account.Id); err != nil {
			return err
		}
	}
	return s.sortTaskMedia(ctx, &sysin.MediaSortInp{TaskId: in.TaskId, Items: in.Items}, task, 0)
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

func (s *sSysPublish) MyMediaReconcile(ctx context.Context, in *sysin.MediaReconcileInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("媒体清单不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	task, err := s.getTask(ctx, in.TaskId, account.Id)
	if err != nil {
		return err
	}
	for _, id := range in.RemovedMediaIds {
		mediaId, resolveErr := s.resolveTaskMediaId(ctx, task, id, account.Id)
		if resolveErr != nil {
			return resolveErr
		}
		if err = s.deleteMedia(ctx, mediaId, account.Id); err != nil {
			return err
		}
	}
	return s.sortTaskMedia(ctx, &sysin.MediaSortInp{TaskId: in.TaskId, Items: in.Items}, task, account.Id)
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
			mediaId, resolveErr := s.resolveTaskMediaIdTx(ctx, tx, task, item.Id, accountId)
			if resolveErr != nil {
				return resolveErr
			}
			mod := tx.Model(publishMediaTable).Ctx(ctx).
				Where("id", mediaId).
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

func (s *sSysPublish) resolveTaskMediaId(ctx context.Context, task gdb.Record, mediaId int64, accountId int64) (int64, error) {
	var resolved int64
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var err error
		resolved, err = s.resolveTaskMediaIdTx(ctx, tx, task, mediaId, accountId)
		return err
	})
	return resolved, err
}

func (s *sSysPublish) resolveTaskMediaIdTx(ctx context.Context, tx gdb.TX, task gdb.Record, mediaId int64, accountId int64) (int64, error) {
	if mediaId <= 0 || task.IsEmpty() {
		return 0, gerror.New("媒体不存在或无权操作")
	}
	taskId := task["id"].Int64()
	profileId := task["profile_id"].Int64()
	mod := tx.Model(publishMediaTable).Ctx(ctx).Where("id", mediaId).WhereNull("deleted_at")
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	source, err := mod.One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取媒体失败")
	}
	if source.IsEmpty() || source["profile_id"].Int64() != profileId {
		return 0, gerror.New("媒体不存在或无权操作")
	}
	if source["task_id"].Int64() == taskId {
		return mediaId, nil
	}
	targetMod := tx.Model(publishMediaTable).Ctx(ctx).
		Where("task_id", taskId).
		Where("profile_id", profileId).
		Where("attachment_id", source["attachment_id"].Int64()).
		Where("purpose", source["purpose"].String()).
		WhereNull("deleted_at")
	if accountId > 0 {
		targetMod = targetMod.Where("account_id", accountId)
	}
	target, err := targetMod.OrderAsc(fmt.Sprintf("ABS(sort_index - %d)", source["sort_index"].Int())).OrderAsc("id").One()
	if err != nil {
		return 0, gerror.Wrap(err, "定位当前任务媒体失败")
	}
	if target.IsEmpty() {
		return 0, gerror.New("媒体不存在或无权操作")
	}
	return target["id"].Int64(), nil
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
	return s.profileImageSearchByAccountIds(ctx, in, file, []int64{account.Id}, account)
}

func (s *sSysPublish) AdminProfileImageSearch(ctx context.Context, in *sysin.ProfileImageSearchInp, file *ghttp.UploadFile) (list []*sysin.NoteModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ProfileImageSearchInp{}
	}
	scope, err := s.adminProfileVisibleScope(ctx, account, &in.ProfileListInp)
	if err != nil {
		return nil, 0, err
	}
	if len(scope.AccountIds) == 0 {
		return []*sysin.NoteModel{}, 0, nil
	}
	if err = s.ensureAdminProfileScopeTenants(ctx, scope); err != nil {
		return nil, 0, err
	}
	// The account list is already permission-scoped. Keep tenant unset here so
	// followed accounts from another tenant are not discarded by the result query.
	in.TenantId = 0
	return s.profileImageSearchByAccountIds(ctx, in, file, scope.AccountIds, account)
}

func (s *sSysPublish) profileImageSearchByAccountIds(ctx context.Context, in *sysin.ProfileImageSearchInp, file *ghttp.UploadFile, accountIds []int64, viewer *sysin.AccountModel) (list []*sysin.NoteModel, totalCount int, err error) {
	normalizeProfileImageSearchInput(in)
	accountIds = uniqueIds(accountIds)
	if len(accountIds) == 0 {
		return []*sysin.NoteModel{}, 0, nil
	}
	fingerprint, err := uploadImageFingerprint(file)
	if err != nil {
		return nil, 0, err
	}
	matches, totalCount, err := s.findSimilarProfileMatchesByFingerprint(ctx, fingerprint, in, accountIds)
	if err != nil {
		return nil, 0, err
	}
	profileIds := make([]int64, 0, len(matches))
	for _, match := range matches {
		profileIds = append(profileIds, match.ProfileId)
	}
	if len(profileIds) == 0 {
		return []*sysin.NoteModel{}, totalCount, nil
	}
	list, err = s.profileImageSearchNotesByProfileIds(ctx, profileIds, in.TenantId, accountIds, viewer, "")
	if err != nil {
		return nil, 0, err
	}
	if err = s.attachProfileImageSearchMatches(ctx, list, matches); err != nil {
		return nil, 0, err
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
		// Image search must be stricter than the detail-page similarity query.
		// A 64-bit pHash threshold of 12 produces visible false positives.
		in.Threshold = 8
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
	MediaId   int64
	MediaType string
}

func (s *sSysPublish) findSimilarProfileMatchesByPHash(ctx context.Context, queryHash *goimagehash.ImageHash, in *sysin.ProfileImageSearchInp, accountIds []int64) (matches []publishProfilePHashDistance, totalCount int, err error) {
	return s.findSimilarProfileMatchesByPHashBucket(ctx, queryHash, in, accountIds)
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

func imagePHashFromPath(path string) (*goimagehash.ImageHash, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, gerror.New("图片文件为空")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, gerror.Wrap(err, "读取图片文件失败")
	}
	defer file.Close()
	img, _, err := image.Decode(file)
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
	// 编辑已发布资料时，前端仍持有历史媒体 ID。草稿复制后按原始
	// 附件和位置定位草稿副本，避免新增一条重复媒体。
	if existing.IsEmpty() && in.MediaId > 0 {
		source, sourceErr := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("id", in.MediaId).WhereNull("deleted_at").One()
		if sourceErr != nil {
			return nil, gerror.Wrap(sourceErr, "读取原媒体失败")
		}
		if !source.IsEmpty() && source["profile_id"].Int64() == task["profile_id"].Int64() {
			existing, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
				Where("task_id", task["id"].Int64()).
				Where("attachment_id", source["attachment_id"].Int64()).
				Where("purpose", source["purpose"].String()).
				Where("sort_index", source["sort_index"].Int()).
				WhereNull("deleted_at").One()
			if err != nil {
				return nil, gerror.Wrap(err, "定位草稿媒体失败")
			}
		}
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
	if err = s.syncMediaPHashBucketByMediaId(ctx, mediaId); err != nil {
		return nil, err
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

func normalizeMediaFileURL(fileURL string, storagePath string) string {
	fileURL = strings.TrimSpace(fileURL)
	storagePath = strings.TrimSpace(storagePath)
	if contentPath := normalizeTelegramContentStoragePath(fileURL); contentPath != "" {
		return normalizeTelegramContentURL(contentPath)
	}
	if storagePath == "" {
		return fileURL
	}
	if contentPath := normalizeTelegramContentStoragePath(storagePath); contentPath != "" {
		return normalizeTelegramContentURL(contentPath)
	}
	if fileURL == "" || isLocalMediaURL(fileURL) || isTelegramFileURL(fileURL) {
		if isAbsoluteMediaURL(storagePath) {
			return storagePath
		}
		return "/" + strings.TrimLeft(storagePath, "/")
	}
	return fileURL
}

func normalizeTelegramContentURL(storagePath string) string {
	storagePath = strings.TrimSpace(storagePath)
	if storagePath == "" {
		return ""
	}
	cdnBase := mediaContentCDNBaseURL()
	if cdnBase != "" {
		return cdnBase + "/" + strings.TrimLeft(storagePath, "/")
	}
	return "/" + strings.TrimLeft(storagePath, "/")
}

func normalizeTelegramContentStoragePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "telegram/content/") {
		return strings.TrimLeft(raw, "/")
	}
	if strings.HasPrefix(raw, "/telegram/content/") {
		return strings.TrimLeft(raw, "/")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Path == "" {
		return ""
	}
	path := strings.TrimLeft(parsed.Path, "/")
	if idx := strings.Index(path, "telegram/content/"); idx >= 0 {
		return path[idx:]
	}
	return ""
}

func mediaContentCDNBaseURL() string {
	cdnBase := strings.TrimRight(g.Cfg().MustGet(context.Background(), "content.cdnBaseUrl", "").String(), "/")
	if cdnBase != "" {
		return cdnBase
	}
	uploadConfig := storager.GetConfig()
	if uploadConfig == nil {
		return ""
	}
	return strings.TrimRight(uploadConfig.CosPublicURL, "/")
}

func isAbsoluteMediaURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme != "" && parsed.Hostname() != ""
}

func isTelegramFileURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), "api.telegram.org") && strings.HasPrefix(parsed.Path, "/file/bot")
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
	row, err = s.editableMediaRow(ctx, row, row["tenant_id"].Int64(), accountId)
	if err != nil {
		return err
	}
	id = row["id"].Int64()
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
	_ = s.deleteMediaPHashBucketByMediaId(ctx, id)
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
	row, err = s.editableMediaRow(ctx, row, tenantId, 0)
	if err != nil {
		return err
	}
	id = row["id"].Int64()
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
	_ = s.deleteMediaPHashBucketByMediaId(ctx, id)
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

func (s *sSysPublish) editableMediaRow(ctx context.Context, row gdb.Record, tenantId int64, accountId int64) (gdb.Record, error) {
	if row.IsEmpty() || row["profile_id"].Int64() <= 0 {
		return row, nil
	}
	draft, err := s.editableProfileTask(ctx, row["profile_id"].Int64(), tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if draft.IsEmpty() || draft["id"].Int64() == row["task_id"].Int64() {
		return row, nil
	}
	target, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", draft["id"].Int64()).
		Where("attachment_id", row["attachment_id"].Int64()).
		Where("purpose", row["purpose"].String()).
		WhereNull("deleted_at").OrderAsc("id").One()
	if err != nil {
		return nil, gerror.Wrap(err, "定位草稿媒体失败")
	}
	if target.IsEmpty() {
		return row, nil
	}
	return target, nil
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
	if profileId > 0 {
		return fmt.Sprintf("youban_publish:media_sync:profile:%d", profileId)
	}
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
			profileColumns.ImageCount:           0,
			profileColumns.VideoCount:           0,
			profileColumns.HasVerificationVideo: 0,
			profileColumns.UpdatedAt:            now,
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
	hasVerificationVideo := 0
	var coverMediaId int64
	for _, item := range list {
		mediaType := item.MediaType
		if mediaType == "" {
			mediaType = "image"
		}
		if mediaType == "video" {
			videoCount++
			if strings.TrimSpace(item.Purpose) == "verify" {
				hasVerificationVideo = 1
			}
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
		existing, err := tx.Model(dao.ContentMedia.Table()).Ctx(ctx).Unscoped().
			Where(mediaColumns.ProfileId, profileId).
			Where(mediaColumns.SourceAssetId, sourceAssetId).
			Fields(mediaColumns.Id).
			Value()
		if err != nil {
			return gerror.Wrap(err, "检查资料媒体失败")
		}
		if existing.Int64() > 0 {
			if _, err = tx.Model(dao.ContentMedia.Table()).Ctx(ctx).Unscoped().Where(mediaColumns.Id, existing.Int64()).Data(data).Update(); err != nil {
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
		profileColumns.ImageCount:           imageCount,
		profileColumns.VideoCount:           videoCount,
		profileColumns.HasVerificationVideo: hasVerificationVideo,
		profileColumns.UpdatedAt:            now,
	}
	if coverMediaId > 0 {
		data[profileColumns.CoverMediaId] = coverMediaId
	}
	if _, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(profileColumns.Id, profileId).Data(data).Update(); err != nil {
		return gerror.Wrap(err, "更新资料媒体数量失败")
	}
	return nil
}
