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
	"hotgo/internal/library/profileextractor"
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
	var manualChannelIds []int64
	if in.ChannelIds != nil {
		in.ChannelIds, err = s.availableProfileChannelIds(ctx, in.ChannelIds, tenantId)
		if err != nil {
			return nil, err
		}
		manualChannelIds = uniqueIds(in.ChannelIds)
	}
	var publishAt *gtime.Time
	if in.PublishAt != "" {
		publishAt = gtime.NewFromStr(in.PublishAt)
		if publishAt == nil {
			return nil, gerror.New("定时上架时间不合法")
		}
	}
	profileId := in.Id
	isNewProfile := profileId == 0
	var removedMediaIds []int64
	transaction := func() error {
		return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			if in.Id > 0 {
				profileId = in.Id
				stateMod := tx.Model(publishProfileStateTable).Ctx(ctx).
					Where("profile_id", profileId).
					Where("tenant_id", tenantId).
					WhereNull("deleted_at")
				if accountId > 0 {
					stateMod = stateMod.Where("account_id", accountId)
				}
				if count, checkErr := stateMod.Count(); checkErr != nil {
					return gerror.Wrap(checkErr, "检查资料归属失败")
				} else if count == 0 {
					return gerror.New("资料不存在或无权操作")
				}
			}
			if profileId == 0 {
				profileId, err = s.createProfileFromInput(ctx, tx, in, tenantId, accountId)
				if err != nil {
					return err
				}
			} else {
				if err = s.updateProfileFromInput(ctx, tx, in, profileId, accountId, publishAt); err != nil {
					return err
				}
			}
			if err = s.upsertProfileStateTx(ctx, tx, profileId, tenantId, accountId, in.CustomerRemark, in.AntiScanEnabled, publishAt); err != nil {
				return err
			}
			if isNewProfile || in.ChannelIds != nil {
				if err = replaceProfileChannelMappings(ctx, tx, tenantId, accountId, profileId, manualChannelIds); err != nil {
					return err
				}
			}
			if in.Media != nil {
				removedMediaIds, err = s.syncProfileMediaFromInput(ctx, tx, profileId, tenantId, accountId, in.Media)
				return err
			}
			return nil
		})
	}
	if profileId > 0 {
		err = s.withProfileLifecycleLock(ctx, tenantId, profileId, transaction)
	} else {
		err = transaction()
	}
	if err != nil {
		return nil, err
	}
	effectiveChannelIds, err := s.profileChannelIdsOrDefaults(ctx, tenantId, accountId, profileId)
	if err != nil {
		return nil, err
	}
	if err = s.supersedeProfilePendingTelegramJobsOutsideChannels(ctx, profileId, tenantId, accountId, effectiveChannelIds); err != nil {
		return nil, err
	}
	for _, mediaId := range removedMediaIds {
		if deleteErr := s.deleteMediaPHashBucketByMediaId(ctx, mediaId); deleteErr != nil {
			g.Log().Warningf(ctx, "清理已删除资料索引失败 mediaId:%d err:%v", mediaId, deleteErr)
		}
	}
	if in.Media != nil {
		if err = s.syncMediaPHashBucketsByProfileId(ctx, profileId); err != nil {
			return nil, err
		}
	}
	if err = s.syncProfileNoteIndex(ctx, profileId); err != nil {
		return nil, err
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	profile, err := s.profileView(ctx, profileId, tenantId, 0)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileSaveModel{Id: profileId, Uuid: profile.Uuid, ProfileNo: profile.ProfileNo}, nil
}

func (s *sSysPublish) supersedeProfilePendingTelegramJobsOutsideChannels(ctx context.Context, profileId, tenantId, accountId int64, channelIds []int64) error {
	if profileId <= 0 || tenantId <= 0 {
		return nil
	}
	mod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("profile_id", profileId).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown"})
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	channelIds = uniqueIds(channelIds)
	if len(channelIds) > 0 {
		mod = mod.WhereNotIn("channel_id", channelIds)
	}
	var jobs []telegramResubmitJob
	if err := mod.OrderAsc("id").Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取资料已移除频道的TG任务失败")
	}
	for _, job := range jobs {
		if err := s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return err
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "publish", "superseded", "资料频道配置已变更，停止已移除频道的待发送任务")
	}
	return nil
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
	if err = s.deactivateChannelProfiles(ctx, tenantId, ids); err != nil {
		return err
	}
	if err = s.supersedeProfilePendingTelegramJobs(ctx, ids, tenantId); err != nil {
		return err
	}
	columns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).WhereIn(columns.Id, ids).Unscoped().Delete(); err != nil {
		return gerror.Wrap(err, "删除资料失败")
	}
	if err = s.deleteProfileNoteIndex(ctx, ids); err != nil {
		return err
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) supersedeProfilePendingTelegramJobs(ctx context.Context, profileIds []int64, tenantId int64) error {
	profileIds = uniqueIds(profileIds)
	if tenantId <= 0 || len(profileIds) == 0 {
		return nil
	}
	var jobs []telegramResubmitJob
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("profile_id", profileIds).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown"}).
		OrderAsc("id").Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取资料待发送TG任务失败")
	}
	for _, job := range jobs {
		if err := s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return gerror.Wrap(err, "废弃资料待发送TG任务失败")
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "down", "superseded", "资料下架，停止待发送TG任务")
	}
	return nil
}

func (s *sSysPublish) enqueueProfilesTelegramCleanupBeforeDelete(ctx context.Context, ids []int64, tenantId int64) error {
	if len(ids) == 0 {
		return nil
	}
	jobs, err := s.telegramJobsWithActiveMessages(ctx, tenantId, ids, "")
	if err != nil {
		return err
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
	if err = s.setProfilesOffline(ctx, ids, tenantId); err != nil {
		return nil, err
	}
	if err = s.enqueueProfileDownRun(ctx, tenantId, ids, 0); err != nil {
		return nil, gerror.Wrap(err, "加入资料下架队列失败")
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return &sysin.ProfileStatusModel{Message: "资料已下架，已有TG消息将在后台清理"}, nil
}

// setProfilesOffline applies the shared local state transition used by both
// regular batch operations and collect-source shutdowns. Telegram cleanup is
// intentionally handled by each caller's queue workflow.
func (s *sSysPublish) setProfilesOffline(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	for _, id := range ids {
		if err := s.withProfileLifecycleLock(ctx, tenantId, id, func() error {
			_, lockErr := s.syncProfilePublishState(ctx, id, 2, consts.ContentVisibilityPrivate, nil)
			return lockErr
		}); err != nil {
			return gerror.Wrap(err, "更新资料状态失败")
		}
	}
	for _, id := range ids {
		if err := s.syncProfileNoteIndex(ctx, id); err != nil {
			return err
		}
	}
	if err := s.deactivateChannelProfiles(ctx, tenantId, ids); err != nil {
		return err
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) submitProfilesByIds(ctx context.Context, ids []int64, tenantId int64, accountId int64) error {
	if len(ids) == 0 {
		return nil
	}
	for _, profileId := range uniqueIds(ids) {
		if err := s.submitProfilePublish(ctx, profileId, tenantId, accountId, contexts.GetUserId(ctx), "", nil, false); err != nil {
			return err
		}
		if err := s.syncProfileNoteIndex(ctx, profileId); err != nil {
			return err
		}
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) createProfileFromInput(ctx context.Context, tx gdb.TX, in *sysin.ProfileSaveInp, tenantId int64, accountId int64) (int64, error) {
	columns := dao.ContentProfile.Columns()
	now := gtime.Now()
	extracted := profileextractor.Merge(in.PlainText, 0, 0, in.Tag)
	data := g.Map{
		columns.SourceType:      publishProfileSourceType,
		columns.SourceNoteUuid:  newPublishProfileUUID(),
		columns.SourceKey:       fmt.Sprintf("youban_publish:profile:%s", newPublishProfileUUID()),
		columns.Title:           in.Title,
		columns.Summary:         profileSummary(in.PlainText),
		columns.PlainText:       in.PlainText,
		columns.Province:        in.Province,
		columns.City:            in.City,
		columns.Height:          extracted.Height,
		columns.Weight:          extracted.Weight,
		columns.CupSize:         extracted.Cup,
		columns.Visibility:      consts.ContentVisibilityPublic,
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

func (s *sSysPublish) updateProfileFromInput(ctx context.Context, tx gdb.TX, in *sysin.ProfileSaveInp, profileId int64, accountId int64, publishAt *gtime.Time) error {
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
			columns.Height,
			columns.Weight,
			columns.CupSize,
		).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取资料当前状态失败")
	}
	nextStatus := in.Status
	nextVisibility := in.Visibility
	profilePublishedAt := publishAt
	if in.KeepPublishState {
		nextStatus = current[columns.Status].Int()
		nextVisibility = current[columns.Visibility].String()
		profilePublishedAt = current[columns.PublishedAt].GTime()
	} else if current[columns.Status].Int() == 1 {
		nextStatus = 1
		nextVisibility = consts.ContentVisibilityPublic
	}
	extracted := profileextractor.Refresh(in.PlainText, profileextractor.Fields{
		Height: current[columns.Height].Int(),
		Weight: current[columns.Weight].Int(),
		Cup:    current[columns.CupSize].String(),
	})
	if cup := profileextractor.NormalizeCup(in.Tag); cup != "" {
		extracted.Cup = cup
	}
	data := g.Map{
		columns.Title:       in.Title,
		columns.Summary:     profileSummary(in.PlainText),
		columns.PlainText:   in.PlainText,
		columns.Province:    in.Province,
		columns.City:        in.City,
		columns.Height:      extracted.Height,
		columns.Weight:      extracted.Weight,
		columns.CupSize:     extracted.Cup,
		columns.Visibility:  nextVisibility,
		columns.AdminRemark: in.CustomerRemark,
		columns.Status:      nextStatus,
	}
	if profileContentChanged(current, in) {
		data[columns.SourceUpdateBy] = strconv.FormatInt(accountId, 10)
		data[columns.SourceUpdatedAt] = now
		data[columns.UpdatedAt] = now
	}
	if nextStatus == 1 && profilePublishedAt == nil {
		profilePublishedAt = now
	}
	data[columns.PublishedAt] = profilePublishedAt
	if _, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileId).Data(data).Update(); err != nil {
		return gerror.Wrap(err, "更新资料失败")
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
		(profileextractor.NormalizeCup(in.Tag) != "" &&
			strings.TrimSpace(current["cup_size"].String()) != profileextractor.NormalizeCup(in.Tag))
}

func isProfileNoUniqueConstraintError(err error) bool {
	if !isUniqueConstraintError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "profile_no") || strings.Contains(message, "uk_content_profile_no")
}
