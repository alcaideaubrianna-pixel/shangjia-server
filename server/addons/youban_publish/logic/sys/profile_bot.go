package sys

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

var botProfileNoRegexp = regexp.MustCompile(`^[A-Z][0-9]{5}$`)

func normalizeBotProfileNo(no string) string {
	return strings.ToUpper(strings.TrimSpace(no))
}

func (s *sSysPublish) BotMediaCacheFile(ctx context.Context, in *sysin.BotMediaCacheFileInp) (res *sysin.BotMediaCacheFileModel, err error) {
	if in == nil || in.Media == nil {
		return nil, gerror.New("媒体信息不能为空")
	}
	media := in.Media
	item := &telegramMediaItem{
		Id:                media.Id,
		AttachmentId:      media.AttachmentId,
		MediaType:         media.MediaType,
		Purpose:           media.Purpose,
		FileUrl:           normalizeBotMediaCacheURL(firstNonEmpty(media.FileUrl, media.OriginalFileUrl, media.EditedFileUrl)),
		PosterUrl:         normalizeBotMediaCacheURL(firstNonEmpty(media.PosterUrl, media.PosterStoragePath)),
		StoragePath:       firstNonEmpty(media.StoragePath, media.OriginalStoragePath, media.EditedStoragePath),
		PosterStoragePath: media.PosterStoragePath,
		TgFileId:          media.TgFileId,
		TgThumbFileId:     media.TgThumbFileId,
		AssetHash:         firstNonEmpty(media.TgCacheAssetHash, media.Md5),
		SortIndex:         media.SortIndex,
	}
	path, _, err := cachedTelegramMediaFile(ctx, item)
	if err != nil {
		return nil, err
	}
	return &sysin.BotMediaCacheFileModel{Path: path}, nil
}

func normalizeBotMediaCacheURL(u string) string {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "/https://") || strings.HasPrefix(u, "/http://") {
		return strings.TrimPrefix(u, "/")
	}
	if strings.HasPrefix(u, "https:/") && !strings.HasPrefix(u, "https://") {
		return "https://" + strings.TrimPrefix(u, "https:/")
	}
	if strings.HasPrefix(u, "http:/") && !strings.HasPrefix(u, "http://") {
		return "http://" + strings.TrimPrefix(u, "http:/")
	}
	return u
}

func (s *sSysPublish) BotProfileStatus(ctx context.Context, in *sysin.BotProfileStatusInp) (res *sysin.ProfileStatusModel, err error) {
	if in == nil {
		return nil, gerror.New("资料信息不能为空")
	}
	ids := append([]int64{}, in.Ids...)
	for _, no := range in.Nos {
		id, err := s.botResolveProfileId(ctx, in.TenantId, in.AccountId, no, false)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return s.updateProfileStatus(ctx, &sysin.ProfileStatusInp{Ids: ids, Status: in.Status}, in.TenantId, in.AccountId)
}

func (s *sSysPublish) BotProfileCreate(ctx context.Context, in *sysin.BotProfileCreateInp) (res *sysin.ProfileSaveModel, err error) {
	if in == nil {
		return nil, gerror.New("资料信息不能为空")
	}
	title := strings.TrimSpace(in.Title)
	plainText := strings.TrimSpace(in.PlainText)
	if title == "" {
		title = firstLineForProfileTitle(plainText)
	}
	if title == "" {
		return nil, gerror.New("标题不能为空")
	}
	status := in.Status
	if status == 0 {
		status = 2
	}
	res, err = s.saveProfile(ctx, &sysin.ProfileSaveInp{Title: title, PlainText: plainText, Status: status, Visibility: "private"}, in.TenantId, in.AccountId)
	if err != nil {
		return nil, err
	}
	if err = s.saveBotProfileMedia(ctx, res.TaskId, res.Id, in.TenantId, in.AccountId, in.DisplayMedia, in.VerifyMedia); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.VerifyText) != "" {
		_ = s.saveBotProfileVerifyText(ctx, res.TaskId, strings.TrimSpace(in.VerifyText))
	}
	return res, nil
}

func (s *sSysPublish) saveBotProfileMedia(ctx context.Context, taskId int64, profileId int64, tenantId int64, accountId int64, displayMedia []*sysin.MessageTemplateMediaInp, verifyMedia []*sysin.MessageTemplateMediaInp) error {
	if taskId <= 0 || profileId <= 0 {
		return nil
	}
	if len(displayMedia) == 0 && len(verifyMedia) == 0 {
		return nil
	}
	now := gtime.Now()
	insertItems := func(purpose string, media []*sysin.MessageTemplateMediaInp) error {
		for index, item := range media {
			if item == nil {
				continue
			}
			mediaType := strings.TrimSpace(item.MediaType)
			if mediaType == "" {
				mediaType = "image"
			}
			sortIndex := item.SortIndex
			if sortIndex <= 0 {
				sortIndex = index + 1
			}
			fileURL := strings.TrimSpace(item.FileUrl)
			storagePath := strings.TrimSpace(item.StoragePath)
			assetHash := mediaAssetHash(strings.TrimSpace(item.AssetHash), storagePath, fileURL)
			perceptualHash := ""
			posterURL := strings.TrimSpace(item.PosterUrl)
			posterStoragePath := strings.TrimSpace(item.PosterStoragePath)
			if storedAssets, assetErr := s.ProcessStoredMediaAssets(ctx, &sysin.StoredMediaAssetsInp{MediaType: mediaType, LocalPath: storagePath, FileName: item.Name}); assetErr != nil {
				g.Log().Warning(ctx, "处理机器人资料媒体失败", g.Map{"profileId": profileId, "mediaType": mediaType, "path": storagePath, "err": assetErr})
			} else if storedAssets != nil && storedAssets.Processed {
				perceptualHash = storedAssets.PerceptualHash
				posterURL = firstNonEmpty(storedAssets.PosterUrl, posterURL)
				posterStoragePath = firstNonEmpty(storedAssets.PosterStoragePath, posterStoragePath)
			}
			if perceptualHash == "" && strings.EqualFold(mediaType, "image") {
				imageURL := normalizeBotMediaCacheURL(fileURL)
				if imageURL != "" {
					if hash, hashErr := cachedRemoteImagePHash(ctx, imageURL); hashErr != nil {
						g.Log().Warning(ctx, "计算机器人资料图片哈希失败", g.Map{"profileId": profileId, "taskId": taskId, "url": imageURL, "err": hashErr})
					} else {
						perceptualHash = hash
					}
				}
			}
			data := g.Map{
				"tenant_id":              tenantId,
				"merchant_id":            tenantId,
				"account_id":             accountId,
				"task_id":                taskId,
				"profile_id":             profileId,
				"attachment_id":          0,
				"original_attachment_id": 0,
				"edited_attachment_id":   0,
				"media_type":             mediaType,
				"purpose":                purpose,
				"name":                   strings.TrimSpace(item.Name),
				"file_url":               fileURL,
				"original_file_url":      fileURL,
				"edited_file_url":        "",
				"poster_url":             posterURL,
				"poster_storage_path":    posterStoragePath,
				"tg_file_id":             strings.TrimSpace(item.TgFileId),
				"tg_thumb_file_id":       strings.TrimSpace(item.TgThumbFileId),
				"tg_cache_asset_hash":    assetHash,
				"tg_cache_status":        tgCacheStatusValid,
				"storage_path":           storagePath,
				"original_storage_path":  storagePath,
				"edited_storage_path":    "",
				"mime_type":              "",
				"md5":                    strings.TrimSpace(item.AssetHash),
				"perceptual_hash":        perceptualHash,
				"edit_status":            mediaEditStatusRaw,
				"size":                   0,
				"sort_index":             sortIndex,
				"status":                 1,
				"created_by":             accountId,
				"updated_by":             accountId,
				"created_at":             now,
				"updated_at":             now,
			}
			mediaId, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
			if err != nil {
				return gerror.Wrap(err, "保存机器人资料媒体失败")
			}
			if err = s.syncMediaPHashBucketByMediaId(ctx, mediaId); err != nil {
				return err
			}
		}
		return nil
	}
	if err := insertItems("display", displayMedia); err != nil {
		return err
	}
	if err := insertItems("verify", verifyMedia); err != nil {
		return err
	}
	_ = s.refreshTaskMediaCount(ctx, taskId)
	return nil
}

func (s *sSysPublish) replaceBotProfileMedia(ctx context.Context, taskId int64, profileId int64, tenantId int64, accountId int64, displayMedia []*sysin.MessageTemplateMediaInp, verifyMedia []*sysin.MessageTemplateMediaInp) error {
	if taskId <= 0 || profileId <= 0 {
		return nil
	}
	if _, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("profile_id", profileId).
		WhereIn("purpose", []string{"display", "verify"}).
		Data(g.Map{"deleted_at": gtime.Now(), "deleted_by": accountId, "updated_at": gtime.Now()}).
		Update(); err != nil {
		return gerror.Wrap(err, "清空原资料媒体失败")
	}
	_ = s.deleteMediaPHashBucketByProfileId(ctx, profileId)
	return s.saveBotProfileMedia(ctx, taskId, profileId, tenantId, accountId, displayMedia, verifyMedia)
}

func (s *sSysPublish) saveBotProfileVerifyText(ctx context.Context, taskId int64, verifyText string) error {
	// 当前资料表只有展示正文；验证资料文本先落到任务备注，避免丢失，后续后台可按媒体 purpose=verify 展示。
	_, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).Data(g.Map{"customer_remark": verifyText, "updated_at": gtime.Now()}).Update()
	return err
}

func (s *sSysPublish) botResolveProfileId(ctx context.Context, tenantId int64, accountId int64, profileNo string, publicOnly bool) (int64, error) {
	no := normalizeBotProfileNo(profileNo)
	if no == "" {
		return 0, gerror.New("资料编号不能为空")
	}
	columns := dao.ContentProfile.Columns()
	mod, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return 0, err
	}
	if publicOnly {
		mod = mod.Where("p."+columns.Status, 1)
	}
	var row struct {
		Id int64 `json:"id"`
	}
	if err = mod.Fields("p."+columns.Id).Where("p."+columns.ProfileNo, no).Scan(&row); err != nil {
		return 0, gerror.Wrap(err, "读取资料失败")
	}
	if row.Id <= 0 {
		return 0, gerror.New("资料不存在或无权操作")
	}
	return row.Id, nil
}

func firstLineForProfileTitle(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	line := strings.TrimSpace(strings.Split(text, "\n")[0])
	runes := []rune(line)
	if len(runes) > 30 {
		return string(runes[:30])
	}
	return line
}

func (s *sSysPublish) BotProfileEdit(ctx context.Context, in *sysin.BotProfileEditInp) (res *sysin.NoteModel, err error) {
	if in == nil {
		return nil, gerror.New("资料信息不能为空")
	}
	profileId, err := s.botResolveProfileId(ctx, in.TenantId, in.AccountId, in.ProfileNo, false)
	if err != nil {
		return nil, err
	}
	current, err := s.profileView(ctx, profileId, in.TenantId, in.AccountId)
	if err != nil {
		return nil, err
	}
	ownerAccountId := in.AccountId
	if ownerAccountId <= 0 {
		ownerAccountId = current.AccountId
	}
	if ownerAccountId <= 0 {
		return nil, gerror.New("资料归属账号信息不完整")
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = current.Title
	}
	plainText := strings.TrimSpace(in.PlainText)
	if plainText == "" {
		plainText = current.PlainText
	}
	if _, err = s.saveProfile(ctx, &sysin.ProfileSaveInp{Id: profileId, Title: title, PlainText: plainText, Status: current.Status, Visibility: current.Visibility, Province: current.Province, City: current.City, Tag: current.Tag, CustomerRemark: current.CustomerRemark}, in.TenantId, ownerAccountId); err != nil {
		return nil, err
	}
	if len(in.DisplayMedia) > 0 || len(in.VerifyMedia) > 0 || strings.TrimSpace(in.VerifyText) != "" {
		if err = s.replaceBotProfileMedia(ctx, current.TaskId, profileId, in.TenantId, ownerAccountId, in.DisplayMedia, in.VerifyMedia); err != nil {
			return nil, err
		}
		if strings.TrimSpace(in.VerifyText) != "" {
			_ = s.saveBotProfileVerifyText(ctx, current.TaskId, strings.TrimSpace(in.VerifyText))
		}
	}
	newNo := normalizeBotProfileNo(in.NewNo)
	if newNo != "" && newNo != normalizeBotProfileNo(current.ProfileNo) {
		if !botProfileNoRegexp.MatchString(newNo) {
			return nil, gerror.New("资料编号格式应为 A00001")
		}
		columns := dao.ContentProfile.Columns()
		count, err := dao.ContentProfile.Ctx(ctx).Unscoped().Where(columns.ProfileNo, newNo).Count()
		if err != nil {
			return nil, gerror.Wrap(err, "检查资料编号失败")
		}
		if count > 0 {
			return nil, gerror.New("资料编号已存在")
		}
		if _, err = dao.ContentProfile.Ctx(ctx).Where(columns.Id, profileId).Data(g.Map{columns.ProfileNo: newNo}).Update(); err != nil {
			return nil, gerror.Wrap(err, "更新资料编号失败")
		}
	}
	return s.BotProfileView(ctx, &sysin.BotProfileViewInp{TenantId: in.TenantId, AccountId: in.AccountId, ProfileId: profileId})
}

func (s *sSysPublish) BotProfileCancelQueue(ctx context.Context, in *sysin.BotProfileQueueCancelInp) (res *sysin.BotProfileQueueCancelModel, err error) {
	if in == nil || in.TenantId <= 0 {
		return nil, gerror.New("上架账号信息不完整")
	}
	profileIds := make([]int64, 0, len(in.Nos))
	for _, no := range in.Nos {
		id, err := s.botResolveProfileId(ctx, in.TenantId, in.AccountId, no, false)
		if err != nil {
			return nil, err
		}
		profileIds = append(profileIds, id)
	}
	mod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", in.TenantId).
		WhereIn("status", channelQueueClearStatuses())
	if in.AccountId > 0 {
		mod = mod.Where("account_id", in.AccountId)
	}
	if len(profileIds) > 0 {
		mod = mod.WhereIn("profile_id", profileIds)
	}
	result, err := mod.Data(g.Map{
		"status":              "superseded",
		"dispatch_status":     tgDispatchStatusDone,
		"next_retry_at":       nil,
		"next_cycle_at":       nil,
		"error_message":       "Bot取消资料推送队列",
		"last_dispatch_error": "Bot取消资料推送队列",
		"updated_at":          gtime.Now(),
	}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "取消资料推送队列失败")
	}
	affected, _ := result.RowsAffected()
	return &sysin.BotProfileQueueCancelModel{Cleared: int(affected)}, nil
}

func (s *sSysPublish) BotChannelList(ctx context.Context, in *sysin.ChannelListInp) (list []*sysin.ChannelModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.ChannelListInp{}
	}
	if in.TenantId <= 0 {
		return nil, 0, gerror.New("租户信息不能为空")
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 || in.PerPage > 10 {
		in.PerPage = 10
	}
	if strings.TrimSpace(in.PublishDirection) == "" {
		in.PublishDirection = "up"
	}
	if in.Status <= 0 {
		in.Status = 1
	}
	mod := s.channelBaseModel(ctx)
	mod = applyChannelFilters(mod, in)
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道总数失败")
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("c.id").Fields("c.*,ta.display_name AS tg_account_name").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道列表失败")
	}
	applyChannelBotIds(list)
	return list, totalCount, nil
}

func (s *sSysPublish) BotChannelCycleSave(ctx context.Context, in *sysin.BotChannelCycleSaveInp) (err error) {
	if in == nil || in.TenantId <= 0 || in.ChannelId <= 0 {
		return gerror.New("频道信息不能为空")
	}
	channel, err := s.channelById(ctx, in.TenantId, in.ChannelId)
	if err != nil {
		return err
	}
	if channel == nil || channel.Id <= 0 {
		return gerror.New("频道不存在")
	}
	enabled := 0
	if in.Enabled == 1 {
		enabled = 1
	}
	days := in.Days
	if days <= 0 {
		days = defaultCycleDays(channel.CyclePublishDays)
	}
	publishTime := strings.TrimSpace(in.Time)
	if publishTime == "" {
		publishTime = strings.TrimSpace(channel.CyclePublishTime)
	}
	if publishTime == "" {
		publishTime = "09:00"
	}
	_, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", in.TenantId).
		Where("id", in.ChannelId).
		WhereNull("deleted_at").
		Data(g.Map{"cycle_publish_enabled": enabled, "cycle_publish_days": days, "cycle_publish_time": publishTime, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "保存频道循环设置失败")
	}
	return s.syncChannelCycleAfterSave(ctx, in.TenantId, in.ChannelId, enabled, days, publishTime)
}

func (s *sSysPublish) BotChannelFullPush(ctx context.Context, in *sysin.BotChannelActionInp) (res *sysin.ChannelFullPushModel, err error) {
	if in == nil || in.TenantId <= 0 || in.ChannelId <= 0 {
		return nil, gerror.New("请选择频道")
	}
	channel, err := s.fullPushChannel(ctx, in.TenantId, in.ChannelId)
	if err != nil {
		return nil, err
	}
	batchNo := "bot_full_push:" + fmt.Sprintf("%d:%d", channel.Id, gtime.Now().TimestampNano())
	queued, err := s.fullPushPublishedTaskCount(ctx, in.TenantId)
	if err != nil {
		return nil, err
	}
	existingJobs, err := s.channelClearQueueJobs(ctx, in.TenantId, channel.Id)
	if err != nil {
		return nil, err
	}
	go s.runChannelFullPush(context.WithoutCancel(ctx), in.TenantId, channel.Id, batchNo)
	return &sysin.ChannelFullPushModel{ChannelId: channel.Id, Queued: queued, ExistingQueue: len(existingJobs)}, nil
}

func (s *sSysPublish) BotChannelClearQueue(ctx context.Context, in *sysin.BotChannelActionInp) (res *sysin.ChannelClearQueueModel, err error) {
	if in == nil || in.TenantId <= 0 {
		return nil, gerror.New("请选择频道")
	}
	if in.ChannelId > 0 {
		channel, err := s.channelById(ctx, in.TenantId, in.ChannelId)
		if err != nil {
			return nil, err
		}
		if channel == nil || channel.Id <= 0 {
			return nil, gerror.New("频道不存在")
		}
	}
	jobs, err := s.channelClearQueueJobs(ctx, in.TenantId, in.ChannelId)
	if err != nil {
		return nil, err
	}
	res = &sysin.ChannelClearQueueModel{ChannelId: in.ChannelId, Cleared: len(jobs)}
	if len(jobs) == 0 {
		return res, nil
	}
	jobIds := make([]int64, 0, len(jobs))
	taskIds := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		jobIds = append(jobIds, job.Id)
		if job.Status == "sending" {
			res.Sending++
		}
		if job.TaskId > 0 {
			taskIds = append(taskIds, job.TaskId)
		}
	}
	mod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("id", jobIds).
		Where("tenant_id", in.TenantId).
		WhereIn("status", channelQueueClearStatuses())
	if in.ChannelId > 0 {
		mod = mod.Where("channel_id", in.ChannelId)
	}
	result, err := mod.Data(g.Map{"status": "superseded", "dispatch_status": tgDispatchStatusDone, "next_retry_at": nil, "next_cycle_at": nil, "error_message": channelQueueClearMessage, "last_dispatch_error": channelQueueClearMessage, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "清空频道发送队列失败")
	}
	affected, _ := result.RowsAffected()
	res.Cleared = int(affected)
	return res, s.markChannelQueueTasksSuperseded(ctx, in.TenantId, uniqueIds(taskIds))
}
