package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/utility/file"
)

func (s *sSysPublish) saveMaterialImportGroupProfile(ctx context.Context, task *sysin.MaterialImportTaskModel, group *sysin.MaterialImportGroupModel, mediaJson string) (int64, int64, error) {
	if task == nil || group == nil {
		return 0, 0, gerror.New("资料导入分组不存在")
	}
	title := strings.TrimSpace(group.Title)
	if title == "" {
		title = firstNonEmpty(group.ProfileNo, group.Nickname, fmt.Sprintf("TG资料%d", group.Id))
	}
	input := &sysin.ProfileSaveInp{
		Id:         group.ProfileId,
		Title:      title,
		PlainText:  strings.TrimSpace(firstNonEmpty(group.ProfileText, group.RawText)),
		Visibility: consts.ContentVisibilityPrivate,
		Status:     1,
	}
	if group.ProfileId <= 0 {
		profileId, _, err := s.materialImportExistingProfile(ctx, group)
		if err != nil {
			return 0, 0, err
		}
		input.Id = profileId
	}
	saved, err := s.saveProfile(ctx, input, task.TenantId, task.AccountId)
	if err != nil {
		return 0, 0, err
	}
	if err = s.updateMaterialImportProfileSource(ctx, saved.Id, group, task, title); err != nil {
		return 0, 0, err
	}
	if err = s.replaceMaterialImportMedia(ctx, task, saved, group, mediaJson); err != nil {
		return 0, 0, err
	}
	if err = s.ensureMaterialImportTelegramIndex(ctx, task, group, saved.Id, saved.TaskId); err != nil {
		return 0, 0, err
	}
	if err = s.syncProfileNoteIndex(ctx, saved.Id); err != nil {
		return 0, 0, err
	}
	_ = s.appendMaterialImportPublishLog(ctx, task, saved.Id, "imported", fmt.Sprintf("资料导入完成：%s", strings.TrimSpace(title)))
	return saved.Id, saved.TaskId, nil
}

func (s *sSysPublish) materialImportExistingProfile(ctx context.Context, group *sysin.MaterialImportGroupModel) (int64, int64, error) {
	sourceKey := materialImportProfileSourceKey(group)
	profileCols := dao.ContentProfile.Columns()
	profile, err := dao.ContentProfile.Ctx(ctx).
		Fields(profileCols.Id).
		Where(profileCols.SourceKey, sourceKey).
		WhereNull(profileCols.DeletedAt).
		One()
	if err != nil {
		return 0, 0, gerror.Wrap(err, "读取TG导入资料失败")
	}
	if profile.IsEmpty() {
		return 0, 0, nil
	}
	task, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("id").
		Where("profile_id", profile["id"].Int64()).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return 0, 0, gerror.Wrap(err, "读取TG导入资料任务失败")
	}
	return profile["id"].Int64(), task["id"].Int64(), nil
}

func (s *sSysPublish) updateMaterialImportProfileSource(ctx context.Context, profileId int64, group *sysin.MaterialImportGroupModel, task *sysin.MaterialImportTaskModel, title string) error {
	profileCols := dao.ContentProfile.Columns()
	messageAt := materialImportMessageTime(group)
	_, err := dao.ContentProfile.Ctx(ctx).Where(profileCols.Id, profileId).Data(g.Map{
		profileCols.Title:           title,
		profileCols.SourceType:      publishProfileSourceType,
		profileCols.SourceKey:       materialImportProfileSourceKey(group),
		profileCols.ImportStatus:    "imported",
		profileCols.SourceCreateBy:  task.SourceTitle,
		profileCols.SourceUpdateBy:  task.SourceTitle,
		profileCols.SourceCreatedAt: messageAt,
		profileCols.SourceUpdatedAt: messageAt,
		profileCols.CreatedAt:       messageAt,
		profileCols.UpdatedAt:       messageAt,
	}).Update()
	return gerror.Wrap(err, "更新TG导入资料来源失败")
}

func (s *sSysPublish) replaceMaterialImportMedia(ctx context.Context, task *sysin.MaterialImportTaskModel, saved *sysin.ProfileSaveModel, group *sysin.MaterialImportGroupModel, mediaJson string) error {
	if saved == nil || saved.TaskId <= 0 {
		return nil
	}
	ctx = importRuntimeContext(ctx, firstPositiveInt64(task.UpdatedBy, task.AccountId))
	now := gtime.Now()
	_, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", saved.TaskId).
		WhereNull("deleted_at").
		Data(g.Map{"deleted_at": now, "updated_at": now}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "清理TG导入旧媒体失败")
	}
	_ = s.deleteMediaPHashBucketByProfileId(ctx, saved.Id)
	var items []collectMediaItem
	_ = json.Unmarshal([]byte(mediaJson), &items)
	for index, item := range items {
		item = normalizeCollectMediaItem(item)
		mediaType := collectPublishMediaType(item.Type)
		path := strings.TrimSpace(item.StoragePath)
		if mediaType == "" || path == "" {
			continue
		}
		purpose := materialImportMediaPurpose(item)
		upload, err := materialImportUploadFileFromPath(path, fmt.Sprintf("material-import-%d-%d%s", group.Id, index+1, filepath.Ext(path)))
		if err != nil {
			return err
		}
		if _, err = s.saveUploadedTaskMedia(ctx, gdb.Record{
			"id":         gvar.New(saved.TaskId),
			"tenant_id":  gvar.New(task.TenantId),
			"account_id": gvar.New(task.AccountId),
			"profile_id": gvar.New(saved.Id),
			"status":     gvar.New(sysin.PublishTaskStatusDraft),
		}, &sysin.MediaUploadInp{
			ProfileId: saved.Id,
			MediaType: mediaType,
			Purpose:   purpose,
			SortIndex: index + 1,
		}, upload, nil, nil); err != nil {
			return err
		}
	}
	if err := s.refreshTaskMediaCount(ctx, saved.TaskId); err != nil {
		return err
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return s.syncTaskMediaToProfile(ctx, tx, saved.TaskId, saved.Id)
	})
}

func materialImportMediaPurpose(item collectMediaItem) string {
	purpose := strings.TrimSpace(item.Purpose)
	if purpose == "verify" {
		return purpose
	}
	return "display"
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 1
}

func materialImportUploadFileFromPath(path string, name string) (*ghttp.UploadFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG导入媒体失败")
	}
	if len(content) == 0 {
		return nil, gerror.New("TG导入媒体为空")
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(path)
	}
	header, err := file.NewMultipartFileHeader(name, content)
	if err != nil {
		return nil, gerror.Wrap(err, "创建TG导入媒体上传文件失败")
	}
	return &ghttp.UploadFile{FileHeader: header}, nil
}

func materialImportProfileSourceKey(group *sysin.MaterialImportGroupModel) string {
	return "youban_publish:material_import:" + materialImportStableGroupKey(group)
}

func materialImportMessageTime(group *sysin.MaterialImportGroupModel) *gtime.Time {
	if group != nil && group.MessageAt != nil {
		return group.MessageAt
	}
	return gtime.Now()
}

func materialImportStableGroupKey(group *sysin.MaterialImportGroupModel) string {
	key := strings.TrimSpace(group.SourceUniqueKey)
	if key == "" {
		return ""
	}
	taskPrefix := fmt.Sprintf("task:%d:", group.TaskId)
	return strings.TrimPrefix(key, taskPrefix)
}

func (s *sSysPublish) appendMaterialImportPublishLog(ctx context.Context, task *sysin.MaterialImportTaskModel, profileId int64, status string, message string) error {
	if task == nil || profileId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":     0,
		"task_id":    task.Id,
		"tenant_id":  task.TenantId,
		"account_id": task.AccountId,
		"profile_id": profileId,
		"bot_id":     0,
		"action":     "material_import",
		"status":     status,
		"message":    strings.TrimSpace(message),
		"created_at": gtime.Now(),
	}).Insert()
	return err
}
