package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
)

type quickPushPlanRecord struct {
	Id            int64       `json:"id"`
	TenantId      int64       `json:"tenantId"`
	Name          string      `json:"name"`
	AccountId     int64       `json:"accountId"`
	TargetChatIds string      `json:"targetChatIds"`
	Status        int         `json:"status"`
	CreatedBy     int64       `json:"createdBy"`
	UpdatedBy     int64       `json:"updatedBy"`
	DeletedBy     int64       `json:"deletedBy"`
	CreatedAt     *gtime.Time `json:"createdAt"`
	UpdatedAt     *gtime.Time `json:"updatedAt"`
	DeletedAt     *gtime.Time `json:"deletedAt"`
}

func (s *sSysPublish) AdminQuickPushPlanList(ctx context.Context, in *sysin.QuickPushPlanListInp) (list []*sysin.QuickPushPlanModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.QuickPushPlanListInp{}
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return nil, 0, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(quickPushPlanTable).Safe().Ctx(ctx).Where("tenant_id", account.TenantId).WhereNull("deleted_at")
	if in.Keyword != "" {
		mod = mod.WhereLike("name", "%"+in.Keyword+"%")
	}
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取快速推送计划总数失败")
	}
	var records []*quickPushPlanRecord
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").Scan(&records); err != nil {
		return nil, 0, gerror.Wrap(err, "获取快速推送计划列表失败")
	}
	models := quickPushPlanModels(records)
	groups := make(map[int64][]string)
	for _, item := range models {
		if item == nil {
			continue
		}
		groups[item.AccountId] = append(groups[item.AccountId], item.TargetChatIds...)
	}
	labels, err := s.resolveTargetChatLabels(ctx, account.TenantId, groups)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "读取快速推送目标名称失败")
	}
	for _, item := range models {
		if item != nil {
			item.TargetChatLabels = labels[item.AccountId]
		}
	}
	return models, totalCount, nil
}

func (s *sSysPublish) AdminQuickPushPlanSave(ctx context.Context, in *sysin.QuickPushPlanSaveInp) (res *sysin.QuickPushPlanSaveModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("快速推送计划不能为空")
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return nil, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if in.Id > 0 {
		if err = s.ensureQuickPushPlansBelongTenant(ctx, []int64{in.Id}, account.TenantId); err != nil {
			return nil, err
		}
	}
	if err = s.ensureMessagePushTgAccountBelongTenant(ctx, in.AccountId, account.TenantId); err != nil {
		return nil, err
	}
	if err = s.ensureMessagePushTargetCaches(ctx, in.AccountId, in.TargetChatIds, account.TenantId); err != nil {
		return nil, err
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id":       account.TenantId,
		"name":            in.Name,
		"account_id":      in.AccountId,
		"target_chat_ids": mustJsonEncode(in.TargetChatIds),
		"status":          in.Status,
		"updated_by":      account.Id,
		"updated_at":      now,
	}
	if in.Id > 0 {
		_, err = g.DB().Model(quickPushPlanTable).Safe().Ctx(ctx).Where("id", in.Id).Where("tenant_id", account.TenantId).WhereNull("deleted_at").Data(data).Update()
		if err != nil {
			return nil, gerror.Wrap(err, "更新快速推送计划失败")
		}
	} else {
		data["created_by"] = account.Id
		data["created_at"] = now
		in.Id, err = g.DB().Model(quickPushPlanTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
		if err != nil {
			return nil, gerror.Wrap(err, "创建快速推送计划失败")
		}
	}
	return &sysin.QuickPushPlanSaveModel{Id: in.Id}, nil
}

func (s *sSysPublish) AdminQuickPushPlanDelete(ctx context.Context, in *sysin.QuickPushPlanDeleteInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	ids := uniqueQuickPushInt64Inputs(in.Ids)
	if len(ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return err
	}
	if err = s.ensureQuickPushPlansBelongTenant(ctx, ids, account.TenantId); err != nil {
		return err
	}
	_, err = g.DB().Model(quickPushPlanTable).Safe().Ctx(ctx).WhereIn("id", ids).Where("tenant_id", account.TenantId).Data(g.Map{"deleted_by": account.Id, "deleted_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除快速推送计划失败")
	}
	return nil
}

func (s *sSysPublish) AdminQuickPushPlanStatus(ctx context.Context, in *sysin.QuickPushPlanStatusInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("计划状态不能为空")
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return err
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	if err = s.ensureQuickPushPlansBelongTenant(ctx, []int64{in.Id}, account.TenantId); err != nil {
		return err
	}
	_, err = g.DB().Model(quickPushPlanTable).Safe().Ctx(ctx).Where("id", in.Id).Where("tenant_id", account.TenantId).WhereNull("deleted_at").Data(g.Map{"status": in.Status, "updated_by": account.Id, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新快速推送计划状态失败")
	}
	return nil
}

func (s *sSysPublish) QuickPushBotAccount(ctx context.Context, accountId int64) (*sysin.QuickPushBotAccountModel, error) {
	account, err := s.quickPushAccountById(ctx, accountId)
	if err != nil {
		return nil, err
	}
	return &sysin.QuickPushBotAccountModel{AccountId: account.Id, TenantId: account.TenantId, Username: account.Username, Nickname: account.Nickname, AccountType: account.AccountType}, nil
}

func (s *sSysPublish) QuickPushBotPlanList(ctx context.Context, accountId int64) ([]*sysin.QuickPushPlanModel, error) {
	account, err := s.quickPushAccountById(ctx, accountId)
	if err != nil {
		return nil, err
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return nil, err
	}
	var records []*quickPushPlanRecord
	if err = g.DB().Model(quickPushPlanTable).Safe().Ctx(ctx).Where("tenant_id", account.TenantId).Where("status", 1).WhereNull("deleted_at").OrderAsc("id").Scan(&records); err != nil {
		return nil, gerror.Wrap(err, "读取快速推送计划失败")
	}
	return quickPushPlanModels(records), nil
}

func (s *sSysPublish) QuickPushSaveTemplateByBot(ctx context.Context, in *sysin.QuickPushBotSaveTemplateInp) (*sysin.MessageTemplateSaveModel, error) {
	if in == nil {
		return nil, gerror.New("快速推送模板不能为空")
	}
	if err := ensureMessagePushTables(ctx); err != nil {
		return nil, err
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	account, err := s.quickPushAccountById(ctx, in.OperatorAccountId)
	if err != nil {
		return nil, err
	}
	if account.TenantId != in.TenantId {
		return nil, gerror.New("上架管理员账号无权保存当前快速推送模板")
	}
	template, err := s.createQuickPushMessageTemplate(ctx, in.TenantId, in.OperatorAccountId, in.Text, in.Media, in.SourceMessageRecordId)
	if err != nil {
		return nil, err
	}
	return &sysin.MessageTemplateSaveModel{Id: template.Id}, nil
}

func (s *sSysPublish) QuickPushExecuteByBot(ctx context.Context, in *sysin.QuickPushBotExecuteInp) (*sysin.QuickPushBotExecuteModel, error) {
	if in == nil {
		return nil, gerror.New("快速推送内容不能为空")
	}
	if err := ensureMessagePushTables(ctx); err != nil {
		return nil, err
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	account, err := s.quickPushAccountById(ctx, in.OperatorAccountId)
	if err != nil {
		return nil, err
	}
	if account.TenantId != in.TenantId {
		return nil, gerror.New("上架管理员账号无权执行当前快速推送")
	}
	plans, err := s.quickPushPlansByIds(ctx, in.PlanIds, in.TenantId)
	if err != nil {
		return nil, err
	}
	if len(plans) == 0 {
		return nil, gerror.New("快速推送计划不存在或已停用")
	}
	var template *sysin.MessageTemplateModel
	if in.TemplateId > 0 {
		template, err = s.quickPushMessageTemplateById(ctx, in.TemplateId, in.TenantId)
	} else {
		template, err = s.createQuickPushMessageTemplate(ctx, in.TenantId, in.OperatorAccountId, in.Text, in.Media, in.SourceMessageRecordId)
	}
	if err != nil {
		return nil, err
	}
	res := &sysin.QuickPushBotExecuteModel{Results: make([]*sysin.MessageTemplatePushResultModel, 0)}
	var accountId int64
	targets := make([]*messageTemplatePushTarget, 0)
	for _, plan := range plans {
		if accountId == 0 {
			accountId = plan.AccountId
		}
		channels, err := s.messagePushCachedTargets(ctx, plan.AccountId, decodeStringArray(plan.TargetChatIds), in.TenantId)
		if err != nil {
			res.Failed++
			res.Results = append(res.Results, &sysin.MessageTemplatePushResultModel{Status: sysin.MessagePushStatusFailed, Message: err.Error()})
			continue
		}
		for _, channel := range channels {
			targets = append(targets, &messageTemplatePushTarget{Channel: channel, AccountId: plan.AccountId, OperationNo: quickPushOperationNo(plan.Id, template.Id, channel.TargetChatId), Priority: tgJobPriorityUrgent, QueueName: tgQueueNameUrgent})
		}
	}
	batch := s.queueMessageTemplateTargets(ctx, template, targets, in.TenantId, accountId)
	res.Total += batch.Total
	res.Success += batch.Success
	res.Failed += batch.Failed
	res.Results = append(res.Results, batch.Results...)
	return res, nil
}

func (s *sSysPublish) quickPushMessageTemplateById(ctx context.Context, templateId int64, tenantId int64) (*sysin.MessageTemplateModel, error) {
	var template *sysin.MessageTemplateModel
	if err := g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).
		Where("id", templateId).
		Where("tenant_id", tenantId).
		Where("status", consts.StatusEnabled).
		WhereNull("deleted_at").
		Scan(&template); err != nil {
		return nil, gerror.Wrap(err, "读取快速推送模板失败")
	}
	if template == nil || template.Id <= 0 {
		return nil, gerror.New("已保存的快速推送模板不存在或已停用")
	}
	if err := s.fillMessageTemplateMedia(ctx, []*sysin.MessageTemplateModel{template}); err != nil {
		return nil, err
	}
	return template, nil
}

func (s *sSysPublish) quickPushAccountById(ctx context.Context, accountId int64) (*sysin.AccountModel, error) {
	if accountId <= 0 {
		return nil, gerror.New("上架账号不能为空")
	}
	var account *sysin.AccountModel
	if err := g.DB().Model(publishAccountTable).Safe().Ctx(ctx).Where("id", accountId).WhereNull("deleted_at").Scan(&account); err != nil {
		return nil, gerror.Wrap(err, "读取上架账号失败")
	}
	if account == nil || account.Id <= 0 {
		return nil, gerror.New("上架账号不存在")
	}
	if account.Status != consts.StatusEnabled {
		return nil, gerror.New("上架账号已被停用")
	}
	if account.AccountType != sysin.PublishAccountTypeAdmin {
		return nil, gerror.New("仅绑定上架端管理员账号后可使用快速推送")
	}
	return account, nil
}

func (s *sSysPublish) quickPushPlansByIds(ctx context.Context, ids []int64, tenantId int64) ([]quickPushPlanRecord, error) {
	ids = uniqueQuickPushInt64Inputs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	var plans []quickPushPlanRecord
	if err := g.DB().Model(quickPushPlanTable).Safe().Ctx(ctx).Where("tenant_id", tenantId).Where("status", 1).WhereIn("id", ids).WhereNull("deleted_at").OrderAsc("id").Scan(&plans); err != nil {
		return nil, gerror.Wrap(err, "读取快速推送计划失败")
	}
	if len(plans) != len(ids) {
		return nil, gerror.New("存在无效或已停用的快速推送计划")
	}
	return plans, nil
}

func (s *sSysPublish) createQuickPushMessageTemplate(ctx context.Context, tenantId int64, operatorAccountId int64, text string, media []*sysin.MessageTemplateMediaInp, sourceMessageRecordId int64) (*sysin.MessageTemplateModel, error) {
	sourceRecordIds := quickPushSourceRecordIds(sourceMessageRecordId, media)
	if err := botService.SysBot().RetainStoredMessages(ctx, sourceRecordIds); err != nil {
		return nil, err
	}
	now := gtime.Now()
	name := fmt.Sprintf("快速推送-%s", now.Format("YmdHis"))
	text = strings.TrimSpace(text)
	serialNo, err := s.ensureInlineTemplateSerial(ctx)
	if err != nil {
		return nil, err
	}
	template := &sysin.MessageTemplateModel{TenantId: tenantId, SerialNo: serialNo, PushMode: sysin.MessageTemplatePushModeBot, Name: name, Text: text, MediaCount: len(media), Media: []*sysin.MessageTemplateMediaModel{}, Status: 1, SourceMessageRecordId: sourceMessageRecordId, CreatedBy: operatorAccountId, UpdatedBy: operatorAccountId, CreatedAt: now, UpdatedAt: now}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		id, err := tx.Model(messageTemplateTable).Ctx(ctx).Data(g.Map{
			"tenant_id":                tenantId,
			"serial_no":                serialNo,
			"push_mode":                sysin.MessageTemplatePushModeBot,
			"name":                     name,
			"text":                     text,
			"media_count":              len(media),
			"status":                   1,
			"source_message_record_id": sourceMessageRecordId,
			"created_by":               operatorAccountId,
			"updated_by":               operatorAccountId,
			"created_at":               now,
			"updated_at":               now,
		}).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "创建快速推送临时模板失败")
		}
		template.Id = id
		for index, item := range media {
			if item == nil {
				continue
			}
			sortIndex := item.SortIndex
			if sortIndex <= 0 {
				sortIndex = index + 1
			}
			assetHash := messageMediaAssetHash(item)
			mediaId, err := tx.Model(messageMediaTable).Ctx(ctx).Data(g.Map{
				"template_id":              id,
				"tenant_id":                tenantId,
				"source_message_record_id": item.SourceMessageRecordId,
				"media_type":               strings.TrimSpace(item.MediaType),
				"name":                     strings.TrimSpace(item.Name),
				"file_url":                 strings.TrimSpace(item.FileUrl),
				"storage_path":             strings.TrimSpace(item.StoragePath),
				"poster_url":               strings.TrimSpace(item.PosterUrl),
				"poster_storage_path":      strings.TrimSpace(item.PosterStoragePath),
				"tg_file_id":               strings.TrimSpace(item.TgFileId),
				"tg_thumb_file_id":         strings.TrimSpace(item.TgThumbFileId),
				"asset_hash":               assetHash,
				"sort_index":               sortIndex,
				"created_at":               now,
				"updated_at":               now,
			}).InsertAndGetId()
			if err != nil {
				return gerror.Wrap(err, "保存快速推送媒体失败")
			}
			template.Media = append(template.Media, &sysin.MessageTemplateMediaModel{Id: mediaId, TemplateId: id, TenantId: tenantId, SourceMessageRecordId: item.SourceMessageRecordId, MediaType: strings.TrimSpace(item.MediaType), Name: strings.TrimSpace(item.Name), FileUrl: strings.TrimSpace(item.FileUrl), StoragePath: strings.TrimSpace(item.StoragePath), PosterUrl: strings.TrimSpace(item.PosterUrl), PosterStoragePath: strings.TrimSpace(item.PosterStoragePath), TgFileId: strings.TrimSpace(item.TgFileId), TgThumbFileId: strings.TrimSpace(item.TgThumbFileId), AssetHash: assetHash, SortIndex: sortIndex, CreatedAt: now, UpdatedAt: now})
		}
		return nil
	})
	if err != nil {
		_ = s.releaseUnusedStoredMessageRecords(ctx, sourceRecordIds)
		return nil, err
	}
	return template, nil
}

func quickPushSourceRecordIds(sourceMessageRecordId int64, media []*sysin.MessageTemplateMediaInp) []int64 {
	ids := make([]int64, 0, len(media)+1)
	if sourceMessageRecordId > 0 {
		ids = append(ids, sourceMessageRecordId)
	}
	for _, item := range media {
		if item != nil && item.SourceMessageRecordId > 0 {
			ids = append(ids, item.SourceMessageRecordId)
		}
	}
	return uniqueQuickPushInt64Inputs(ids)
}

func (s *sSysPublish) ensureQuickPushPlansBelongTenant(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueQuickPushInt64Inputs(ids)
	if len(ids) == 0 {
		return gerror.New("请选择快速推送计划")
	}
	count, err := g.DB().Model(quickPushPlanTable).Safe().Ctx(ctx).Where("tenant_id", tenantId).WhereIn("id", ids).WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "检查快速推送计划失败")
	}
	if count != len(ids) {
		return gerror.New("存在不属于当前账号的快速推送计划")
	}
	return nil
}

func quickPushPlanModels(records []*quickPushPlanRecord) []*sysin.QuickPushPlanModel {
	list := make([]*sysin.QuickPushPlanModel, 0, len(records))
	for _, item := range records {
		if item == nil {
			continue
		}
		list = append(list, quickPushPlanModel(*item))
	}
	return list
}

func quickPushPlanModel(item quickPushPlanRecord) *sysin.QuickPushPlanModel {
	return &sysin.QuickPushPlanModel{
		Id:            item.Id,
		SerialNo:      quickPushSerialNo(item.Id),
		TenantId:      item.TenantId,
		Name:          item.Name,
		AccountId:     item.AccountId,
		TargetChatIds: decodeStringArray(item.TargetChatIds),
		Status:        item.Status,
		CreatedBy:     item.CreatedBy,
		UpdatedBy:     item.UpdatedBy,
		DeletedBy:     item.DeletedBy,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
		DeletedAt:     item.DeletedAt,
	}
}

func quickPushSerialNo(id int64) string {
	return fmt.Sprintf("QP%06d", id)
}

func quickPushOperationNo(planId int64, templateId int64, targetChatId string) string {
	targetKey := strings.NewReplacer(":", "_", "|", "_", " ", "_").Replace(strings.TrimSpace(targetChatId))
	return "message_push:" + strconv.FormatInt(templateId, 10) + ":" + strconv.FormatInt(gtime.Now().TimestampNano(), 10) + ":" + strconv.FormatInt(planId, 10) + ":" + targetKey + ":quick:" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func uniqueQuickPushInt64Inputs(values []int64) []int64 {
	out := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
