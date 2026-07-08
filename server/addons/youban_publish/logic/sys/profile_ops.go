package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model/input/form"
	basesysin "hotgo/internal/model/input/sysin"
	iservice "hotgo/internal/service"
)

func (s *sSysPublish) MyProfileList(ctx context.Context, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	in.TenantId = account.TenantId
	in.AccountId = account.Id
	list, totalCount, err = s.profileList(ctx, in)
	markProfilesPermission(list, sysin.ProfilePermissionCreator)
	return
}

func (s *sSysPublish) MyProfileView(ctx context.Context, in *sysin.ProfileViewInp) (res *sysin.ProfileViewModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return nil, gerror.New("资料UUID不能为空")
	}
	profileId, err := s.resolveProfileId(ctx, in.Id, in.Uuid, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileView(ctx, profileId, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	markProfilePermission(profile, sysin.ProfilePermissionCreator)
	media, err := s.mediaListByProfile(ctx, profile.Id, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileViewModel{Profile: profile, Media: media}, nil
}

func (s *sSysPublish) MyProfileOptions(ctx context.Context) (res *sysin.ProfileOptionsModel, err error) {
	channels, _, err := s.MyChannelList(ctx, &sysin.ChannelListInp{
		PageReq: form.PageReq{
			Page:    1,
			PerPage: 200,
		},
	})
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileOptionsModel{Channels: channels}, nil
}

func (s *sSysPublish) MyProfileSave(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.saveProfile(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) MyProfileDelete(ctx context.Context, in *sysin.ProfileDeleteInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteProfiles(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) MyProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp) (res *sysin.ProfileStatusModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.updateProfileStatus(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) MyNoteList(ctx context.Context, in *sysin.NoteListInp) (list []*sysin.NoteModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	in.TenantId = account.TenantId
	in.AccountId = account.Id
	list, totalCount, err = s.noteList(ctx, in)
	markNotesPermission(list, sysin.ProfilePermissionCreator)
	return
}

func (s *sSysPublish) MyTagList(ctx context.Context, in *sysin.TagListInp) (list []*sysin.TagModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	return s.tagList(ctx, in, account.Id, false)
}

func (s *sSysPublish) MyTagSave(ctx context.Context, in *sysin.TagSaveInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.saveTag(ctx, in, account.Id, false)
}

func (s *sSysPublish) MyTagDelete(ctx context.Context, in *sysin.TagDeleteInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteTags(ctx, in, account.Id, false)
}

func (s *sSysPublish) MyCityForward(ctx context.Context, in *sysin.CityForwardInp) (res *sysin.CityForwardModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.cityForward(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) MyProfileStats(ctx context.Context, in *sysin.TrendInp) (res *sysin.ProfileStatsModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.profileStats(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminProfileList(ctx context.Context, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	in.TenantId = account.TenantId
	list, totalCount, err = s.profileList(ctx, in)
	markProfilesPermission(list, sysin.ProfilePermissionAdmin)
	return
}

func (s *sSysPublish) AdminProfileView(ctx context.Context, in *sysin.ProfileViewInp) (res *sysin.ProfileViewModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return nil, gerror.New("资料UUID不能为空")
	}
	profileId, err := s.resolveProfileId(ctx, in.Id, in.Uuid, account.TenantId, 0)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileView(ctx, profileId, account.TenantId, 0)
	if err != nil {
		return nil, err
	}
	markProfilePermission(profile, sysin.ProfilePermissionAdmin)
	media, err := s.mediaListByProfile(ctx, profile.Id, account.TenantId, 0)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileViewModel{Profile: profile, Media: media}, nil
}

func (s *sSysPublish) AdminProfileSave(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in != nil && in.Id <= 0 && normalizeProfileUUID(in.Uuid) != "" {
		if in.Id, err = s.resolveProfileId(ctx, 0, in.Uuid, account.TenantId, 0); err != nil {
			return nil, err
		}
	}
	return s.saveProfile(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminProfileDelete(ctx context.Context, in *sysin.ProfileDeleteInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteProfiles(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) AdminProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp) (res *sysin.ProfileStatusModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.updateProfileStatus(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) AdminNoteList(ctx context.Context, in *sysin.NoteListInp) (list []*sysin.NoteModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	in.TenantId = account.TenantId
	list, totalCount, err = s.noteList(ctx, in)
	markNotesPermission(list, sysin.ProfilePermissionAdmin)
	return
}

func (s *sSysPublish) AdminTagList(ctx context.Context, in *sysin.TagListInp) (list []*sysin.TagModel, totalCount int, err error) {
	return s.tagList(ctx, in, 0, true)
}

func (s *sSysPublish) ServerTagSave(ctx context.Context, in *sysin.TagSaveInp) (err error) {
	return s.saveTag(ctx, in, 0, true)
}

func (s *sSysPublish) ServerTagDelete(ctx context.Context, in *sysin.TagDeleteInp) (err error) {
	return s.deleteTags(ctx, in, contexts.GetUserId(ctx), true)
}

func (s *sSysPublish) AdminTagSave(ctx context.Context, in *sysin.TagSaveInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.saveTag(ctx, in, account.Id, true)
}

func (s *sSysPublish) AdminTagDelete(ctx context.Context, in *sysin.TagDeleteInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteTags(ctx, in, account.Id, true)
}

func (s *sSysPublish) AdminCityForward(ctx context.Context, in *sysin.CityForwardInp) (res *sysin.CityForwardModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.cityForward(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) AdminProfileStats(ctx context.Context, in *sysin.TrendInp) (res *sysin.ProfileStatsModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.profileStats(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) ServerProfileList(ctx context.Context, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	return s.profileList(ctx, in)
}

func (s *sSysPublish) ServerProfileView(ctx context.Context, in *sysin.ProfileViewInp) (res *sysin.ProfileViewModel, err error) {
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return nil, gerror.New("资料ID不能为空")
	}
	profileId, err := s.resolveProfileId(ctx, in.Id, in.Uuid, 0, 0)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileView(ctx, profileId, 0, 0)
	if err != nil {
		return nil, err
	}
	media, err := s.mediaListByProfile(ctx, profile.Id, 0, 0)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileViewModel{Profile: profile, Media: media}, nil
}

func (s *sSysPublish) ServerProfileSave(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("资料ID不能为空")
	}
	task, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("profile_id", in.Id).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料任务失败")
	}
	if task.IsEmpty() {
		return nil, gerror.New("资料不属于上架端")
	}
	return s.saveProfile(ctx, in, task["tenant_id"].Int64(), task["account_id"].Int64())
}

func (s *sSysPublish) ServerProfileDelete(ctx context.Context, in *sysin.ProfileDeleteInp) (err error) {
	return s.deleteProfiles(ctx, in, 0, 0)
}

func (s *sSysPublish) ServerProfileReview(ctx context.Context, in *sysin.ProfileReviewInp) (err error) {
	if err = in.Filter(ctx); err != nil {
		return err
	}
	targetIds, err := s.allowedProfileTargetIds(ctx, in.Ids, in.Uuids, 0, 0)
	if err != nil {
		return err
	}
	ids, err := s.allowedProfileIds(ctx, targetIds, 0, 0)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return gerror.New("资料不存在或无权操作")
	}
	columns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).WhereIn(columns.Id, ids).Data(g.Map{
		columns.ReviewStatus: in.ReviewStatus,
		columns.UpdatedAt:    gtime.Now(),
	}).Update(); err != nil {
		return gerror.Wrap(err, "审核资料失败")
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) profileList(ctx context.Context, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	base, err := s.profileBaseModel(ctx, in.TenantId, in.AccountId)
	if err != nil {
		return nil, 0, err
	}
	base = s.applyProfileFilters(ctx, base, in)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计资料失败")
	}
	if totalCount == 0 {
		return []*sysin.ProfileModel{}, 0, nil
	}
	if err = base.Fields(profileListFields()).Page(in.Page, in.PerPage).OrderDesc("p.updated_at").OrderDesc("p.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取资料列表失败")
	}
	if err = s.ensureProfileListUUID(ctx, list); err != nil {
		return nil, 0, err
	}
	if err = s.applyProfileOwnerNames(ctx, list); err != nil {
		return nil, 0, err
	}
	return
}

func markProfilesPermission(list []*sysin.ProfileModel, permission string) {
	for _, item := range list {
		markProfilePermission(item, permission)
	}
}

func markProfilePermission(item *sysin.ProfileModel, permission string) {
	if item == nil {
		return
	}
	if permission == "" {
		permission = sysin.ProfilePermissionVisitor
	}
	item.Permission = permission
	item.CanEdit = permission == sysin.ProfilePermissionCreator || permission == sysin.ProfilePermissionAdmin
}

func markNotesPermission(list []*sysin.NoteModel, permission string) {
	for _, item := range list {
		if item == nil {
			continue
		}
		markProfilePermission(&item.ProfileModel, permission)
	}
}

func (s *sSysPublish) profileView(ctx context.Context, profileId int64, tenantId int64, accountId int64) (res *sysin.ProfileModel, err error) {
	if profileId <= 0 {
		return nil, gerror.New("资料ID不能为空")
	}
	base, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if err = base.Where("p.id", profileId).Fields(profileListFields()).Scan(&res); err != nil {
		return nil, gerror.Wrap(err, "获取资料详情失败")
	}
	if res == nil || res.Id <= 0 {
		return nil, gerror.New("资料不存在或无权操作")
	}
	if err = s.ensureProfileModelUUID(ctx, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *sSysPublish) noteList(ctx context.Context, in *sysin.NoteListInp) (list []*sysin.NoteModel, totalCount int, err error) {
	profiles, totalCount, err := s.profileList(ctx, &in.ProfileListInp)
	if err != nil {
		return nil, 0, err
	}
	list = make([]*sysin.NoteModel, 0, len(profiles))
	for _, item := range profiles {
		note := &sysin.NoteModel{ProfileModel: *item}
		note.Media, err = s.mediaListByProfile(ctx, item.Id, item.TenantId, item.AccountId)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, note)
	}
	return
}

func (s *sSysPublish) saveProfile(ctx context.Context, in *sysin.ProfileSaveInp, tenantId int64, accountId int64) (res *sysin.ProfileSaveModel, err error) {
	if in == nil {
		return nil, gerror.New("资料信息不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if tenantId <= 0 || accountId <= 0 {
		return nil, gerror.New("上架账号信息不完整")
	}
	if in.Id <= 0 && normalizeProfileUUID(in.Uuid) != "" {
		if in.Id, err = s.resolveProfileId(ctx, 0, in.Uuid, tenantId, accountId); err != nil {
			return nil, err
		}
	}
	in.ChannelIds = uniqueIds(in.ChannelIds)
	channelJSON, err := encodeBotIds(in.ChannelIds)
	if err != nil {
		return nil, err
	}
	if len(in.ChannelIds) > 0 {
		if err = s.ensureProfileChannels(ctx, in.ChannelIds, tenantId); err != nil {
			return nil, err
		}
	}
	tgPushEnabled := 0
	tgStatus := "skipped"
	if len(in.ChannelIds) > 0 {
		tgPushEnabled = 1
		tgStatus = "pending"
	}
	var publishAt *gtime.Time
	if in.PublishAt != "" {
		publishAt = gtime.NewFromStr(in.PublishAt)
		if publishAt == nil {
			return nil, gerror.New("定时上架时间不合法")
		}
	}
	now := gtime.Now()
	var profileId int64
	var taskId int64
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if in.Id > 0 {
			task, taskErr := s.profileTask(ctx, tx, in.Id, tenantId, 0)
			if taskErr != nil {
				return taskErr
			}
			profileId = in.Id
			taskId = task["id"].Int64()
			if taskId <= 0 {
				return gerror.New("资料不属于上架端")
			}
		} else if in.TaskId > 0 {
			task, taskErr := s.profileTaskByTaskId(ctx, tx, in.TaskId, tenantId, accountId)
			if taskErr != nil {
				return taskErr
			}
			taskId = task["id"].Int64()
			profileId = task["profile_id"].Int64()
		}
		if taskId == 0 {
			newTaskId, taskErr := tx.Model(publishTaskTable).Ctx(ctx).Data(g.Map{
				"tenant_id":         tenantId,
				"merchant_id":       tenantId,
				"account_id":        accountId,
				"title":             in.Title,
				"province":          in.Province,
				"city":              in.City,
				"plain_text":        in.PlainText,
				"media_count":       0,
				"channel_id_json":   channelJSON,
				"customer_remark":   in.CustomerRemark,
				"anti_scan_enabled": in.AntiScanEnabled,
				"tg_push_enabled":   tgPushEnabled,
				"tg_status":         tgStatus,
				"status":            sysin.PublishTaskStatusDraft,
				"published_at":      publishAt,
				"created_by":        contexts.GetUserId(ctx),
				"updated_by":        contexts.GetUserId(ctx),
				"created_at":        now,
				"updated_at":        now,
			}).InsertAndGetId()
			if taskErr != nil {
				return gerror.Wrap(taskErr, "创建资料任务失败")
			}
			taskId = newTaskId
		}
		if profileId == 0 {
			profileId, err = s.createProfileFromInput(ctx, tx, in, tenantId, accountId, taskId)
			if err != nil {
				return err
			}
			if _, err = tx.Model(publishTaskTable).Ctx(ctx).Where("id", taskId).Data(g.Map{
				"profile_id":        profileId,
				"channel_id_json":   channelJSON,
				"customer_remark":   in.CustomerRemark,
				"anti_scan_enabled": in.AntiScanEnabled,
				"tg_push_enabled":   tgPushEnabled,
				"tg_status":         tgStatus,
				"published_at":      publishAt,
				"updated_at":        now,
			}).Update(); err != nil {
				return gerror.Wrap(err, "回写资料任务失败")
			}
		} else {
			if err = s.updateProfileFromInput(ctx, tx, in, profileId, taskId, tenantId, accountId, channelJSON, tgPushEnabled, tgStatus, publishAt); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	profile, err := s.profileView(ctx, profileId, tenantId, 0)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileSaveModel{Id: profileId, Uuid: profile.Uuid, TaskId: taskId}, nil
}

func (s *sSysPublish) deleteProfiles(ctx context.Context, in *sysin.ProfileDeleteInp, tenantId int64, accountId int64) (err error) {
	if in == nil || (len(in.Ids) == 0 && len(in.Uuids) == 0) {
		return gerror.New("请选择要删除的资料")
	}
	targetIds, err := s.allowedProfileTargetIds(ctx, in.Ids, in.Uuids, tenantId, accountId)
	if err != nil {
		return err
	}
	ids, err := s.allowedProfileIds(ctx, targetIds, tenantId, accountId)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return gerror.New("资料不存在或无权操作")
	}
	if err = s.enqueueProfilesTelegramCleanupBeforeDelete(ctx, ids, tenantId); err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.disableCyclePlanForProfile(ctx, tenantId, accountId, id); err != nil {
			return err
		}
	}
	columns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).WhereIn(columns.Id, ids).Data(g.Map{columns.DeletedAt: gtime.Now()}).Unscoped().Update(); err != nil {
		return gerror.Wrap(err, "删除资料失败")
	}
	_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).WhereIn("profile_id", ids).Data(g.Map{"deleted_by": contexts.GetUserId(ctx), "deleted_at": gtime.Now()}).Update()
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) enqueueProfilesTelegramCleanupBeforeDelete(ctx context.Context, ids []int64, tenantId int64) error {
	if len(ids) == 0 {
		return nil
	}
	var jobs []telegramResubmitJob
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("profile_id", ids).
		Where("status", "sent").
		Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取资料TG清理任务失败")
	}
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		if err := s.enqueueTelegramCleanupJob(ctx, job.Id, 0); err != nil {
			return gerror.Wrap(err, "加入资料TG清理队列失败")
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "delete", "queued", "资料已删除，TG历史消息已加入异步清理队列")
	}
	return nil
}

func (s *sSysPublish) updateProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp, tenantId int64, accountId int64) (res *sysin.ProfileStatusModel, err error) {
	if in == nil || (len(in.Ids) == 0 && len(in.Uuids) == 0) {
		return nil, gerror.New("请选择要处理的资料")
	}
	if in.Status != 1 && in.Status != 2 {
		return nil, gerror.New("资料状态不合法")
	}
	targetIds, err := s.allowedProfileTargetIds(ctx, in.Ids, in.Uuids, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	ids, err := s.allowedProfileIds(ctx, targetIds, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, gerror.New("资料不存在或无权操作")
	}
	if in.Status == 1 {
		if err = s.submitProfilesByIds(ctx, ids, tenantId, accountId); err != nil {
			return nil, err
		}
		return &sysin.ProfileStatusModel{Message: "资料已提交上架"}, nil
	}
	columns := dao.ContentProfile.Columns()
	data := g.Map{columns.Status: in.Status, columns.UpdatedAt: gtime.Now()}
	data[columns.Visibility] = consts.ContentVisibilityPrivate
	if _, err = dao.ContentProfile.Ctx(ctx).WhereIn(columns.Id, ids).Data(data).Update(); err != nil {
		return nil, gerror.Wrap(err, "更新资料状态失败")
	}
	taskData := g.Map{
		"status":     sysin.PublishTaskStatusCanceled,
		"updated_by": contexts.GetUserId(ctx),
		"updated_at": gtime.Now(),
	}
	_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).WhereIn("profile_id", ids).Data(taskData).Update()
	for _, id := range ids {
		if err = s.disableCyclePlanForProfile(ctx, tenantId, accountId, id); err != nil {
			return nil, err
		}
	}
	if err = s.enqueueProfileDownRun(ctx, tenantId, ids, 0); err != nil {
		return nil, gerror.Wrap(err, "加入资料下架队列失败")
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return &sysin.ProfileStatusModel{Message: "资料已下架，已有TG消息将在后台清理"}, nil
}

func (s *sSysPublish) submitProfilesByIds(ctx context.Context, ids []int64, tenantId int64, accountId int64) error {
	if len(ids) == 0 {
		return nil
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("id").
		WhereIn("profile_id", ids).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	var rows []struct {
		Id int64 `orm:"id"`
	}
	if err := mod.Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取资料上架任务失败")
	}
	if len(rows) == 0 {
		return gerror.New("资料缺少上架任务")
	}
	for _, row := range rows {
		if row.Id <= 0 {
			continue
		}
		if err := s.submitTaskByTenant(ctx, row.Id, tenantId, contexts.GetUserId(ctx)); err != nil {
			return err
		}
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) tagList(ctx context.Context, in *sysin.TagListInp, operatorId int64, isAdmin bool) (list []*sysin.TagModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.TagListInp{}
	}
	mod := g.DB().Model(publishTagTable+" t").Safe().Ctx(ctx).
		LeftJoin(publishAccountTable+" a", "a.id=t.created_by AND a.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" tenant", "tenant.id=a.tenant_id AND tenant.deleted_at IS NULL").
		WhereNull("t.deleted_at")
	if !isAdmin {
		mod = mod.Where("(t.review_status = ? OR t.created_by = ?)", sysin.PublishTagReviewApproved, operatorId)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		mod = mod.WhereLike("t.name", "%"+keyword+"%")
	}
	if in.ReviewStatus != "" {
		mod = mod.Where("t.review_status", in.ReviewStatus)
	}
	if in.Status > 0 {
		mod = mod.Where("t.status", in.Status)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计标签失败")
	}
	if totalCount == 0 {
		return []*sysin.TagModel{}, 0, nil
	}
	fields := "t.id,t.name,t.review_status,t.status,t.use_count,t.created_by,t.created_at,t.updated_at,a.username AS creator_username,a.tenant_id AS creator_tenant_id,tenant.name AS creator_tenant_name"
	if err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("t.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取标签列表失败")
	}
	return
}

func (s *sSysPublish) saveTag(ctx context.Context, in *sysin.TagSaveInp, operatorId int64, isAdmin bool) (err error) {
	if in == nil {
		return gerror.New("标签信息不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	if !isAdmin {
		in.ReviewStatus = sysin.PublishTagReviewPending
		in.Status = 1
	}
	now := gtime.Now()
	names := splitTagNames(in.Name)
	if len(names) == 0 {
		return gerror.New("标签名称不能为空")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, name := range names {
			if err := s.saveOneTag(ctx, tx, name, in.ReviewStatus, in.Status, operatorId, isAdmin, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *sSysPublish) saveOneTag(ctx context.Context, tx gdb.TX, name string, reviewStatus string, status int, operatorId int64, isAdmin bool, now *gtime.Time) error {
	existing, err := tx.Model(publishTagTable).Safe().Ctx(ctx).Where("name", name).WhereNull("deleted_at").Fields("id,created_by,review_status").One()
	if err != nil {
		return gerror.Wrap(err, "检查标签失败")
	}
	if !existing.IsEmpty() && !isAdmin && existing["created_by"].Int64() != operatorId {
		return gerror.New("标签已存在，请等待审核或选择已有标签")
	}
	data := g.Map{
		"name":          name,
		"review_status": reviewStatus,
		"status":        status,
		"updated_by":    operatorId,
		"updated_at":    now,
	}
	if !existing.IsEmpty() {
		_, err = tx.Model(publishTagTable).Safe().Ctx(ctx).Where("id", existing["id"].Int64()).Data(data).Update()
	} else {
		data["created_by"] = operatorId
		data["created_at"] = now
		_, err = tx.Model(publishTagTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存标签失败")
	}
	return nil
}

func splitTagNames(name string) []string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == ',' || r == '，'
	})
	list := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		list = append(list, item)
	}
	return list
}

func (s *sSysPublish) deleteTags(ctx context.Context, in *sysin.TagDeleteInp, operatorId int64, isAdmin bool) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的标签")
	}
	mod := g.DB().Model(publishTagTable).Safe().Ctx(ctx).WhereIn("id", in.Ids).WhereNull("deleted_at")
	if !isAdmin {
		mod = mod.Where("created_by", operatorId)
	}
	result, err := mod.Data(g.Map{
		"deleted_by": operatorId,
		"deleted_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除标签失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认标签删除结果失败")
	}
	if affected == 0 {
		return gerror.New("标签不存在或无权删除")
	}
	return nil
}

func (s *sSysPublish) cityForward(ctx context.Context, in *sysin.CityForwardInp, tenantId int64, accountId int64) (res *sysin.CityForwardModel, err error) {
	if in == nil {
		in = &sysin.CityForwardInp{}
	}
	data, err := iservice.SysProvinces().Select(ctx, &basesysin.ProvincesSelectInp{
		DataType: "pc",
		Value:    in.ParentId,
	})
	if err != nil {
		return nil, err
	}
	if data == nil {
		return &sysin.CityForwardModel{List: []*sysin.CityOptionModel{}}, nil
	}
	res = &sysin.CityForwardModel{List: make([]*sysin.CityOptionModel, 0, len(data.List))}
	for _, item := range data.List {
		res.List = append(res.List, &sysin.CityOptionModel{
			IsLeaf: item.IsLeaf,
			Label:  item.Label,
			Level:  item.Level,
			Value:  item.Value,
		})
	}
	return res, nil
}

func (s *sSysPublish) profileStats(ctx context.Context, in *sysin.TrendInp, tenantId int64, accountId int64) (res *sysin.ProfileStatsModel, err error) {
	// 资料趋势和发布趋势共用同一套日期规则，避免前后端口径不一致。
	dateRange, err := resolveTrendDateRange(in)
	if err != nil {
		return nil, err
	}
	res = &sysin.ProfileStatsModel{Trend: make([]*sysin.TrendPointModel, 0, dateRange.Days)}
	base, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if res.Total, err = base.Clone().Count(); err != nil {
		return nil, gerror.Wrap(err, "统计资料总数失败")
	}
	if res.UpCount, err = base.Clone().Where("p.status", 1).Count(); err != nil {
		return nil, gerror.Wrap(err, "统计上架资料失败")
	}
	if res.DownCount, err = base.Clone().Where("p.status", 2).Count(); err != nil {
		return nil, gerror.Wrap(err, "统计下架资料失败")
	}
	if res.Pending, err = base.Clone().Where("p.review_status", consts.ContentReviewPending).Count(); err != nil {
		return nil, gerror.Wrap(err, "统计待审核资料失败")
	}
	if res.Approved, err = base.Clone().Where("p.review_status", consts.ContentReviewApproved).Count(); err != nil {
		return nil, gerror.Wrap(err, "统计审核通过资料失败")
	}
	if res.Rejected, err = base.Clone().Where("p.review_status", consts.ContentReviewRejected).Count(); err != nil {
		return nil, gerror.Wrap(err, "统计审核拒绝资料失败")
	}
	var rows []struct {
		Date   string `json:"date"`
		Count  int    `json:"count"`
		Status int    `json:"status"`
	}
	trendMod, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if err = trendMod.Fields("DATE(p.created_at) AS date,p.status,COUNT(*) AS count").
		WhereGTE("p.created_at", dateRange.Start+" 00:00:00").
		WhereLTE("p.created_at", dateRange.End+" 23:59:59").
		Group("DATE(p.created_at),p.status").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "获取资料趋势失败")
	}
	index := make(map[string]*sysin.TrendPointModel, dateRange.Days)
	start, _ := parseTrendDate(dateRange.Start, "开始日期")
	for i := 0; i < dateRange.Days; i++ {
		date := start.AddDate(0, 0, i).Format(trendDateLayout)
		point := &sysin.TrendPointModel{Date: date}
		index[date] = point
		res.Trend = append(res.Trend, point)
	}
	for _, row := range rows {
		point := index[row.Date]
		if point == nil {
			continue
		}
		point.ProfileCount += row.Count
		if row.Status == 1 {
			point.UpCount += row.Count
		}
		if row.Status == 2 {
			point.DownCount += row.Count
		}
	}
	return res, nil
}

func (s *sSysPublish) profileBaseModel(ctx context.Context, tenantId int64, accountId int64) (*gdb.Model, error) {
	mod := dao.ContentProfile.Ctx(ctx).As("p").
		LeftJoin(publishTaskTable+" t", "t.profile_id=p.id AND t.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" tenant", "tenant.id=t.tenant_id").
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id AND a.deleted_at IS NULL").
		Where("p.source_type", publishProfileSourceType).
		WhereNull("p.deleted_at")
	if tenantId > 0 {
		mod = mod.Where("t.tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("t.account_id", accountId)
	}
	return mod, nil
}

func profileListFields() string {
	return "p.id,p.source_note_uuid AS uuid,p.profile_no,p.title,p.summary,p.plain_text,p.province,p.city,p.cup_size AS tag,p.visibility,p.review_status,p.status,p.image_count,p.video_count,p.admin_remark AS customer_remark,p.published_at,p.created_at,p.updated_at,t.id AS task_id,t.tenant_id,t.account_id,tenant.name AS tenant_name,a.nickname,t.channel_id_json,t.anti_scan_enabled,t.status AS task_status,t.tg_status,t.tg_push_enabled"
}

func (s *sSysPublish) applyProfileFilters(ctx context.Context, mod *gdb.Model, in *sysin.ProfileListInp) *gdb.Model {
	if in == nil {
		return mod
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		tagIds := s.tagIdsByKeyword(ctx, keyword)
		if len(tagIds) > 0 {
			args := []interface{}{like, like, like, like, like}
			conditions := []string{"p.profile_no LIKE ?", "p.title LIKE ?", "p.summary LIKE ?", "p.plain_text LIKE ?", "p.cup_size LIKE ?"}
			for _, tagId := range tagIds {
				conditions = append(conditions, "p.cup_size = ?", "p.cup_size LIKE ?", "p.cup_size LIKE ?", "p.cup_size LIKE ?")
				args = append(args, tagId, tagId+",%", "%,"+tagId, "%,"+tagId+",%")
			}
			mod = mod.Where("("+strings.Join(conditions, " OR ")+")", args...)
		} else {
			mod = mod.Where("(p.profile_no LIKE ? OR p.title LIKE ? OR p.summary LIKE ? OR p.plain_text LIKE ? OR p.cup_size LIKE ?)", like, like, like, like, like)
		}
	}
	if in.Province != "" {
		mod = mod.Where("p.province", strings.TrimSpace(in.Province))
	}
	if in.City != "" {
		mod = mod.Where("p.city", strings.TrimSpace(in.City))
	}
	if in.Tag != "" {
		tag := strings.TrimSpace(in.Tag)
		mod = mod.Where("(p.cup_size = ? OR p.cup_size LIKE ? OR p.cup_size LIKE ? OR p.cup_size LIKE ?)", tag, tag+",%", "%,"+tag, "%,"+tag+",%")
	}
	if in.ReviewStatus != "" {
		mod = mod.Where("p.review_status", strings.TrimSpace(in.ReviewStatus))
	}
	if in.Visibility != "" {
		mod = mod.Where("p.visibility", strings.TrimSpace(in.Visibility))
	}
	if in.Status > 0 {
		mod = mod.Where("p.status", in.Status)
	}
	return mod
}

func (s *sSysPublish) applyProfileOwnerNames(ctx context.Context, list []*sysin.ProfileModel) error {
	tenantIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil && item.TenantId > 0 {
			tenantIds = append(tenantIds, item.TenantId)
		}
	}
	names, err := s.tenantOwnerNames(ctx, tenantIds)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.TenantName) == "" {
			item.TenantName = names[item.TenantId]
		}
		if item.TenantName == "" && item.TenantId > 0 {
			item.TenantName = fmt.Sprintf("账号归属#%d", item.TenantId)
		}
	}
	return nil
}

func (s *sSysPublish) tagIdsByKeyword(ctx context.Context, keyword string) []string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return []string{}
	}
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model(publishTagTable).Safe().Ctx(ctx).
		Fields("id").
		WhereLike("name", "%"+keyword+"%").
		Where("status", 1).
		Where("review_status", sysin.PublishTagReviewApproved).
		WhereNull("deleted_at").
		Limit(50).
		Scan(&rows)
	if err != nil || len(rows) == 0 {
		return []string{}
	}
	res := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			res = append(res, strconv.FormatInt(row.Id, 10))
		}
	}
	return res
}

func (s *sSysPublish) createProfileFromInput(ctx context.Context, tx gdb.TX, in *sysin.ProfileSaveInp, tenantId int64, accountId int64, taskId int64) (int64, error) {
	columns := dao.ContentProfile.Columns()
	now := gtime.Now()
	data := g.Map{
		columns.SourceType:      publishProfileSourceType,
		columns.SourceNoteUuid:  newPublishProfileUUID(),
		columns.SourceKey:       fmt.Sprintf("youban_publish:profile:%d", taskId),
		columns.Title:           in.Title,
		columns.Summary:         profileSummary(in.PlainText),
		columns.PlainText:       in.PlainText,
		columns.Province:        in.Province,
		columns.City:            in.City,
		columns.CupSize:         in.Tag,
		columns.Visibility:      in.Visibility,
		columns.ReviewStatus:    consts.ContentReviewPending,
		columns.ImportStatus:    "manual",
		columns.SourceCreateBy:  strconv.FormatInt(accountId, 10),
		columns.SourceUpdateBy:  strconv.FormatInt(accountId, 10),
		columns.SourceCreatedAt: now,
		columns.SourceUpdatedAt: now,
		columns.Status:          in.Status,
		columns.CreatedAt:       now,
		columns.UpdatedAt:       now,
	}
	publishedAt := gtime.NewFromStr(in.PublishAt)
	if in.Status == 1 && publishedAt == nil {
		publishedAt = now
	}
	data[columns.PublishedAt] = publishedAt
	data[columns.AdminRemark] = in.CustomerRemark
	var lastErr error
	for i := 0; i < 1000; i++ {
		profileNo, err := s.nextAccountProfileNo(ctx, tx, tenantId, accountId)
		if err != nil {
			return 0, err
		}
		data[columns.ProfileNo] = profileNo
		id, insertErr := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Data(data).InsertAndGetId()
		if insertErr == nil {
			return id, nil
		}
		lastErr = insertErr
		if !isProfileNoUniqueConstraintError(insertErr) {
			return 0, gerror.Wrap(insertErr, "创建资料失败")
		}
	}
	return 0, gerror.Wrap(lastErr, "创建资料失败，资料编号重复")
}

func isProfileNoUniqueConstraintError(err error) bool {
	if !isUniqueConstraintError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "profile_no") || strings.Contains(message, "uk_content_profile_no")
}

func (s *sSysPublish) updateProfileFromInput(ctx context.Context, tx gdb.TX, in *sysin.ProfileSaveInp, profileId int64, taskId int64, tenantId int64, accountId int64, channelJSON string, tgPushEnabled int, tgStatus string, publishAt *gtime.Time) error {
	columns := dao.ContentProfile.Columns()
	now := gtime.Now()
	current, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).
		Where(columns.Id, profileId).
		Fields(columns.Status, columns.PublishedAt, columns.Visibility).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取资料当前状态失败")
	}
	nextStatus := in.Status
	nextVisibility := in.Visibility
	if current[columns.Status].Int() == 1 && tgPushEnabled == 1 {
		nextStatus = 1
		nextVisibility = consts.ContentVisibilityPublic
	}
	data := g.Map{
		columns.Title:           in.Title,
		columns.Summary:         profileSummary(in.PlainText),
		columns.PlainText:       in.PlainText,
		columns.Province:        in.Province,
		columns.City:            in.City,
		columns.CupSize:         in.Tag,
		columns.Visibility:      nextVisibility,
		columns.AdminRemark:     in.CustomerRemark,
		columns.SourceUpdateBy:  strconv.FormatInt(accountId, 10),
		columns.SourceUpdatedAt: now,
		columns.Status:          nextStatus,
		columns.UpdatedAt:       now,
	}
	if nextStatus == 1 && publishAt == nil {
		publishAt = now
	}
	data[columns.PublishedAt] = publishAt
	if _, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileId).Data(data).Update(); err != nil {
		return gerror.Wrap(err, "更新资料失败")
	}
	if _, err := tx.Model(publishTaskTable).Ctx(ctx).Where("id", taskId).Data(g.Map{
		"title":             in.Title,
		"province":          in.Province,
		"city":              in.City,
		"plain_text":        in.PlainText,
		"channel_id_json":   channelJSON,
		"customer_remark":   in.CustomerRemark,
		"anti_scan_enabled": in.AntiScanEnabled,
		"tg_push_enabled":   tgPushEnabled,
		"tg_status":         tgStatus,
		"status":            sysin.PublishTaskStatusDraft,
		"published_at":      publishAt,
		"updated_by":        contexts.GetUserId(ctx),
		"updated_at":        now,
	}).Update(); err != nil {
		return gerror.Wrap(err, "更新资料任务失败")
	}
	return nil
}

func (s *sSysPublish) profileTask(ctx context.Context, tx gdb.TX, profileId int64, tenantId int64, accountId int64) (gdb.Record, error) {
	mod := tx.Model(publishTaskTable).Ctx(ctx).Where("profile_id", profileId).WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("资料不存在或无权操作")
	}
	return row, nil
}

func (s *sSysPublish) profileTaskByTaskId(ctx context.Context, tx gdb.TX, taskId int64, tenantId int64, accountId int64) (gdb.Record, error) {
	mod := tx.Model(publishTaskTable).Ctx(ctx).Where("id", taskId).WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("资料任务不存在或无权操作")
	}
	return row, nil
}

func (s *sSysPublish) allowedProfileIds(ctx context.Context, ids []int64, tenantId int64, accountId int64) ([]int64, error) {
	if len(ids) == 0 {
		return []int64{}, nil
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		WhereIn("profile_id", ids).
		WhereGT("profile_id", 0).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	var rows []struct {
		ProfileId int64 `json:"profileId"`
	}
	if err := mod.Fields("profile_id").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "检查资料权限失败")
	}
	res := make([]int64, 0, len(rows))
	for _, row := range rows {
		res = append(res, row.ProfileId)
	}
	return res, nil
}

func (s *sSysPublish) ensureProfileChannels(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return nil
	}
	count, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		Where("publish_direction", "up").
		Where("status", 1).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查推送频道失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作或不可用的推送频道")
	}
	return nil
}

func (s *sSysPublish) mediaListByProfile(ctx context.Context, profileId int64, tenantId int64, accountId int64) (list []*sysin.MediaModel, err error) {
	if err = ensureMediaEditColumns(ctx); err != nil {
		return nil, err
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("profile_id", profileId).WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	if err = mod.OrderAsc("sort_index").OrderAsc("id").Scan(&list); err != nil {
		return nil, gerror.Wrap(err, "获取笔记媒体失败")
	}
	if list == nil {
		list = []*sysin.MediaModel{}
	}
	normalizeMediaListFileURL(list)
	return list, nil
}
