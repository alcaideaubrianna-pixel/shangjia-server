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
	"hotgo/internal/service"
)

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
	in.ChannelIds, err = s.availableProfileChannelIds(ctx, in.ChannelIds, tenantId)
	if err != nil {
		return nil, err
	}
	channelJSON, err := encodeBotIds(in.ChannelIds)
	if err != nil {
		return nil, err
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
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	profile, err := s.profileView(ctx, profileId, tenantId, 0)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileSaveModel{Id: profileId, Uuid: profile.Uuid, TaskId: taskId, ProfileNo: profile.ProfileNo}, nil
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
		if err = s.disableCyclePlanForProfile(ctx, tenantId, accountId, id, 0); err != nil {
			return err
		}
	}
	columns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).WhereIn(columns.Id, ids).Data(g.Map{columns.DeletedAt: gtime.Now()}).Unscoped().Update(); err != nil {
		return gerror.Wrap(err, "删除资料失败")
	}
	_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).WhereIn("profile_id", ids).Data(g.Map{"deleted_by": contexts.GetUserId(ctx), "deleted_at": gtime.Now()}).Update()
	service.SysContent().ClearHomeProfileCardsCache(ctx)
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
	for _, id := range ids {
		if _, err = s.syncProfilePublishState(ctx, id, in.Status, consts.ContentVisibilityPrivate, nil); err != nil {
			return nil, gerror.Wrap(err, "更新资料状态失败")
		}
	}
	taskData := g.Map{
		"status":     sysin.PublishTaskStatusCanceled,
		"updated_by": contexts.GetUserId(ctx),
		"updated_at": gtime.Now(),
	}
	_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).WhereIn("profile_id", ids).Data(taskData).Update()
	profileAccountIds, err := s.profileAccountIdsByIds(ctx, ids, tenantId)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		ownerAccountId := accountId
		if ownerAccountId <= 0 {
			ownerAccountId = profileAccountIds[id]
		}
		if err = s.disableCyclePlanForProfile(ctx, tenantId, ownerAccountId, id, 0); err != nil {
			return nil, err
		}
	}
	if err = s.enqueueProfileDownRun(ctx, tenantId, ids, 0); err != nil {
		return nil, gerror.Wrap(err, "加入资料下架队列失败")
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return &sysin.ProfileStatusModel{Message: "资料已下架，已有TG消息将在后台清理"}, nil
}

func (s *sSysPublish) profileAccountIdsByIds(ctx context.Context, ids []int64, tenantId int64) (map[int64]int64, error) {
	res := make(map[int64]int64)
	if len(ids) == 0 {
		return res, nil
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("profile_id,account_id").
		WhereIn("profile_id", ids).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	var rows []struct {
		ProfileId int64 `json:"profile_id"`
		AccountId int64 `json:"account_id"`
	}
	if err := mod.Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取资料归属账号失败")
	}
	for _, row := range rows {
		if row.ProfileId > 0 && row.AccountId > 0 {
			res[row.ProfileId] = row.AccountId
		}
	}
	return res, nil
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
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
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

func (s *sSysPublish) updateProfileFromInput(ctx context.Context, tx gdb.TX, in *sysin.ProfileSaveInp, profileId int64, taskId int64, tenantId int64, accountId int64, channelJSON string, tgPushEnabled int, tgStatus string, publishAt *gtime.Time) error {
	columns := dao.ContentProfile.Columns()
	now := gtime.Now()
	current, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).
		Where(columns.Id, profileId).
		Fields(
			columns.Status,
			columns.PublishedAt,
			columns.Visibility,
			columns.Title,
			columns.PlainText,
			columns.Province,
			columns.City,
			columns.CupSize,
		).
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
		columns.Title:       in.Title,
		columns.Summary:     profileSummary(in.PlainText),
		columns.PlainText:   in.PlainText,
		columns.Province:    in.Province,
		columns.City:        in.City,
		columns.CupSize:     in.Tag,
		columns.Visibility:  nextVisibility,
		columns.AdminRemark: in.CustomerRemark,
		columns.Status:      nextStatus,
	}
	if profileContentChanged(current, in) {
		data[columns.SourceUpdateBy] = strconv.FormatInt(accountId, 10)
		data[columns.SourceUpdatedAt] = now
		data[columns.UpdatedAt] = now
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

func profileContentChanged(current gdb.Record, in *sysin.ProfileSaveInp) bool {
	if current.IsEmpty() || in == nil {
		return false
	}
	return strings.TrimSpace(current["title"].String()) != strings.TrimSpace(in.Title) ||
		strings.TrimSpace(current["plain_text"].String()) != strings.TrimSpace(in.PlainText) ||
		strings.TrimSpace(current["province"].String()) != strings.TrimSpace(in.Province) ||
		strings.TrimSpace(current["city"].String()) != strings.TrimSpace(in.City) ||
		strings.TrimSpace(current["cup_size"].String()) != strings.TrimSpace(in.Tag)
}

func isProfileNoUniqueConstraintError(err error) bool {
	if !isUniqueConstraintError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "profile_no") || strings.Contains(message, "uk_content_profile_no")
}
