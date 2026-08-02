package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

func (s *sSysPublish) publishRecordListFast(ctx context.Context, in *sysin.PublishRecordListInp, tenantId int64, accountId int64) (list []*sysin.PublishRecordModel, totalCount int, err error) {
	mod := g.DB().Model(publishSuccessRecordTable+" l").Safe().Ctx(ctx).Where("l.tenant_id", tenantId)
	if accountId > 0 {
		mod = mod.Where("l.account_id", accountId)
	} else if in.AccountId > 0 {
		mod = mod.Where("l.account_id", in.AccountId)
	}
	if in.ProfileId > 0 {
		mod = mod.Where("l.profile_id", in.ProfileId)
	}
	if in.TaskId > 0 {
		mod = mod.Where("l.task_id", in.TaskId)
	}
	if in.Action != "" {
		mod = mod.Where("l.action", in.Action)
	}
	mod = applyPublishRecordStatusFilter(mod, in.Status)

	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取发送记录总数失败")
	}
	if err = mod.Fields("l.id,l.job_id,l.task_id,l.tenant_id,l.account_id,l.profile_id,l.channel_id,l.bot_id,l.operation_no,l.target_chat_id,l.action,l.status,l.message,l.created_at").
		Page(in.Page, in.PerPage).
		OrderDesc("l.id").
		Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取发送记录失败")
	}
	if len(list) == 0 {
		return list, totalCount, nil
	}
	if err = s.enrichPublishRecordListFast(ctx, tenantId, list); err != nil {
		return nil, 0, err
	}
	if err = s.enrichPublishRecordFullPushProgress(ctx, list); err != nil {
		return nil, 0, err
	}
	normalizeCollectPublishRecordActions(list)
	return
}

func (s *sSysPublish) enrichPublishRecordListFast(ctx context.Context, tenantId int64, list []*sysin.PublishRecordModel) error {
	jobIds := uniquePositiveInt64sFromRecordList(list, func(item *sysin.PublishRecordModel) int64 { return item.JobId })
	accountIds := uniquePositiveInt64sFromRecordList(list, func(item *sysin.PublishRecordModel) int64 { return item.AccountId })
	tenantIds := uniquePositiveInt64sFromRecordList(list, func(item *sysin.PublishRecordModel) int64 { return item.TenantId })
	profileIds := uniquePositiveInt64sFromRecordList(list, func(item *sysin.PublishRecordModel) int64 { return item.ProfileId })
	botIds := uniquePositiveInt64sFromRecordList(list, func(item *sysin.PublishRecordModel) int64 { return item.BotId })

	jobMap, err := s.publishRecordJobSnapshotMap(ctx, jobIds)
	if err != nil {
		return err
	}
	accountMap, err := s.publishRecordAccountNameMap(ctx, accountIds)
	if err != nil {
		return err
	}
	tenantMap, err := s.publishRecordTenantNameMap(ctx, tenantIds)
	if err != nil {
		return err
	}
	botMap, err := s.publishRecordBotNameMap(ctx, botIds)
	if err != nil {
		return err
	}
	profileMap, err := s.publishRecordProfileTitleMap(ctx, profileIds)
	if err != nil {
		return err
	}
	channelIds := make([]int64, 0, len(jobMap))
	for _, job := range jobMap {
		if job.ChannelId > 0 {
			channelIds = append(channelIds, job.ChannelId)
		}
	}
	channelMap, err := s.publishRecordChannelNameMap(ctx, uniquePositiveInt64s(channelIds))
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if job, ok := jobMap[item.JobId]; ok {
			item.ChannelId = job.ChannelId
			item.TargetChatId = job.TargetChatId
			item.OperationNo = job.OperationNo
		}
		if title, ok := profileMap[item.ProfileId]; ok {
			item.Title = title
		}
		if name, ok := accountMap[item.AccountId]; ok {
			item.AccountName = name
		}
		if name, ok := tenantMap[item.TenantId]; ok {
			item.TenantName = name
		}
		if bot, ok := botMap[item.BotId]; ok {
			item.BotName = bot.Name
			item.BotUsername = bot.Username
		}
		if channel, ok := channelMap[item.ChannelId]; ok {
			item.ChannelTitle = channel.Title
			item.ChannelUsername = channel.Username
		}
	}
	return s.enrichPublishRecordChannelDisplays(ctx, tenantId, list)
}

func (s *sSysPublish) publishRecordTenantNameMap(ctx context.Context, ids []int64) (map[int64]string, error) {
	if len(ids) == 0 {
		return map[int64]string{}, nil
	}
	type row struct {
		Id   int64  `json:"id"`
		Name string `json:"name"`
	}
	var rows []row
	if err := g.DB().Model(publishTenantTable+" t").Safe().Ctx(ctx).
		LeftJoin(publishAccountTable+" owner", "owner.tenant_id=t.id AND owner.account_type='admin' AND owner.deleted_at IS NULL").
		Fields("t.id,COALESCE(NULLIF(owner.username, ''), NULLIF(t.name, '')) AS name").
		WhereIn("t.id", ids).
		WhereNull("t.deleted_at").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取发送记录账号归属失败")
	}
	res := make(map[int64]string, len(rows))
	for _, item := range rows {
		res[item.Id] = strings.TrimSpace(item.Name)
	}
	return res, nil
}

type publishRecordJobSnapshot struct {
	ChannelId    int64  `json:"channelId"`
	OperationNo  string `json:"operationNo"`
	TargetChatId string `json:"targetChatId"`
}

type publishRecordNameSnapshot struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Title    string `json:"title"`
}

func (s *sSysPublish) publishRecordJobSnapshotMap(ctx context.Context, ids []int64) (map[int64]publishRecordJobSnapshot, error) {
	if len(ids) == 0 {
		return map[int64]publishRecordJobSnapshot{}, nil
	}
	type row struct {
		Id           int64  `json:"id"`
		ChannelId    int64  `json:"channelId"`
		OperationNo  string `json:"operationNo"`
		TargetChatId string `json:"targetChatId"`
	}
	var rows []row
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id,channel_id,operation_no,target_chat_id").
		WhereIn("id", ids).
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取发送记录任务失败")
	}
	res := make(map[int64]publishRecordJobSnapshot, len(rows))
	for _, item := range rows {
		res[item.Id] = publishRecordJobSnapshot{
			ChannelId:    item.ChannelId,
			OperationNo:  item.OperationNo,
			TargetChatId: item.TargetChatId,
		}
	}
	return res, nil
}

func (s *sSysPublish) publishRecordAccountNameMap(ctx context.Context, ids []int64) (map[int64]string, error) {
	if len(ids) == 0 {
		return map[int64]string{}, nil
	}
	type row struct {
		Id       int64  `json:"id"`
		Nickname string `json:"nickname"`
		Username string `json:"username"`
	}
	var rows []row
	if err := g.DB().Model(publishAccountTable).Safe().Ctx(ctx).
		Fields("id,nickname,username").
		WhereIn("id", ids).
		WhereNull("deleted_at").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取发送记录账号失败")
	}
	res := make(map[int64]string, len(rows))
	for _, item := range rows {
		name := strings.TrimSpace(item.Nickname)
		if name == "" {
			name = strings.TrimSpace(item.Username)
		}
		res[item.Id] = name
	}
	return res, nil
}

func (s *sSysPublish) publishRecordBotNameMap(ctx context.Context, ids []int64) (map[int64]publishRecordNameSnapshot, error) {
	if len(ids) == 0 {
		return map[int64]publishRecordNameSnapshot{}, nil
	}
	type row struct {
		Id          int64  `json:"id"`
		BotName     string `json:"botName"`
		BotUsername string `json:"botUsername"`
	}
	var rows []row
	if err := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		Fields("id,bot_name,bot_username").
		WhereIn("id", ids).
		WhereNull("deleted_at").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取发送记录Bot失败")
	}
	res := make(map[int64]publishRecordNameSnapshot, len(rows))
	for _, item := range rows {
		res[item.Id] = publishRecordNameSnapshot{
			Name:     strings.TrimSpace(item.BotName),
			Username: strings.TrimSpace(item.BotUsername),
		}
	}
	return res, nil
}

func (s *sSysPublish) publishRecordProfileTitleMap(ctx context.Context, ids []int64) (map[int64]string, error) {
	if len(ids) == 0 {
		return map[int64]string{}, nil
	}
	type row struct {
		Id    int64  `json:"id"`
		Title string `json:"title"`
	}
	var rows []row
	if err := g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
		Fields("id,title").
		WhereIn("id", ids).
		WhereNull("deleted_at").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取发送记录资料失败")
	}
	res := make(map[int64]string, len(rows))
	for _, item := range rows {
		res[item.Id] = strings.TrimSpace(item.Title)
	}
	return res, nil
}

func (s *sSysPublish) publishRecordChannelNameMap(ctx context.Context, ids []int64) (map[int64]publishRecordNameSnapshot, error) {
	if len(ids) == 0 {
		return map[int64]publishRecordNameSnapshot{}, nil
	}
	type row struct {
		Id              int64  `json:"id"`
		ChannelTitle    string `json:"channelTitle"`
		ChannelUsername string `json:"channelUsername"`
	}
	var rows []row
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,channel_title,channel_username").
		WhereIn("id", ids).
		WhereNull("deleted_at").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取发送记录频道失败")
	}
	res := make(map[int64]publishRecordNameSnapshot, len(rows))
	for _, item := range rows {
		res[item.Id] = publishRecordNameSnapshot{
			Title:    strings.TrimSpace(item.ChannelTitle),
			Username: strings.TrimSpace(item.ChannelUsername),
		}
	}
	return res, nil
}

func uniquePositiveInt64sFromRecordList(list []*sysin.PublishRecordModel, get func(*sysin.PublishRecordModel) int64) []int64 {
	values := make([]int64, 0, len(list))
	for _, item := range list {
		if item == nil {
			continue
		}
		if id := get(item); id > 0 {
			values = append(values, id)
		}
	}
	return uniquePositiveInt64s(values)
}

func uniquePositiveInt64s(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(values))
	uniq := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		uniq = append(uniq, value)
	}
	return uniq
}
