package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type listenerPlanRecord struct {
	Id            int64       `json:"id"`
	TenantId      int64       `json:"tenant_id"`
	Name          string      `json:"name"`
	TgAccountId   int64       `json:"tg_account_id"`
	BotId         int64       `json:"bot_id"`
	BindCode      string      `json:"bind_code"`
	NotifyChatId  string      `json:"notify_chat_id"`
	NotifyChatTyp string      `json:"notify_chat_type"`
	NotifyTitle   string      `json:"notify_chat_title"`
	NotifyBoundAt *gtime.Time `json:"notify_bound_at"`
	KeywordsJson  string      `json:"keywords_json"`
	Status        int         `json:"status"`
	LastTriggerAt *gtime.Time `json:"last_trigger_at"`
	LastResult    string      `json:"last_result"`
	CreatedBy     int64       `json:"created_by"`
	UpdatedBy     int64       `json:"updated_by"`
	DeletedBy     int64       `json:"deleted_by"`
	CreatedAt     *gtime.Time `json:"created_at"`
	UpdatedAt     *gtime.Time `json:"updated_at"`
	DeletedAt     *gtime.Time `json:"deleted_at"`
}

type listenerTargetRecord struct {
	Id                 int64       `json:"id"`
	PlanId             int64       `json:"plan_id"`
	TenantId           int64       `json:"tenant_id"`
	TargetChatId       string      `json:"target_chat_id"`
	TargetChatType     string      `json:"target_chat_type"`
	TargetChatTitle    string      `json:"target_chat_title"`
	TargetChatUsername string      `json:"target_chat_username"`
	LastMatchedAt      *gtime.Time `json:"last_matched_at"`
	LastMatchedText    string      `json:"last_matched_text"`
	LastMatchedUserId  string      `json:"last_matched_user_id"`
	Status             int         `json:"status"`
	CreatedAt          *gtime.Time `json:"created_at"`
	UpdatedAt          *gtime.Time `json:"updated_at"`
	DeletedAt          *gtime.Time `json:"deleted_at"`
}

func (s *sSysPublish) AdminListenerPlanList(ctx context.Context, in *sysin.ListenerPlanListInp) (list []*sysin.ListenerPlanModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ListenerPlanListInp{}
	}
	if err = ensureMessageListenTables(ctx); err != nil {
		return nil, 0, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		Where("tenant_id", account.TenantId).
		WhereNull("deleted_at")
	if in.Keyword != "" {
		like := "%" + in.Keyword + "%"
		mod = mod.Where("(name LIKE ? OR keywords_json LIKE ?)", like, like)
	}
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取监听计划总数失败")
	}
	var plans []*listenerPlanRecord
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").Scan(&plans); err != nil {
		return nil, 0, gerror.Wrap(err, "获取监听计划列表失败")
	}
	list, err = s.listenerPlanModels(ctx, plans, account.TenantId)
	return list, totalCount, err
}

func (s *sSysPublish) AdminListenerPlanSave(ctx context.Context, in *sysin.ListenerPlanSaveInp) (res *sysin.ListenerPlanSaveModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("监听计划不能为空")
	}
	if err = ensureMessageListenTables(ctx); err != nil {
		return nil, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if err = s.ensureMessagePushTgAccountBelongTenant(ctx, in.TgAccountId, account.TenantId); err != nil {
		return nil, err
	}
	if in.Id > 0 {
		if _, err = s.listenerPlanById(ctx, in.Id, account.TenantId); err != nil {
			return nil, err
		}
	}
	now := gtime.Now()
	planData := g.Map{
		"tenant_id":     account.TenantId,
		"name":          in.Name,
		"tg_account_id": in.TgAccountId,
		"bot_id":        in.BotId,
		"keywords_json": mustJsonEncode(in.Keywords),
		"status":        in.Status,
		"updated_by":    account.Id,
		"updated_at":    now,
	}
	if in.Id > 0 {
		_, err = g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
			Where("id", in.Id).
			Where("tenant_id", account.TenantId).
			WhereNull("deleted_at").
			Data(planData).
			Update()
		if err != nil {
			return nil, gerror.Wrap(err, "更新监听计划失败")
		}
	} else {
		planData["created_by"] = account.Id
		planData["created_at"] = now
		planData["bind_code"] = listenerBindCode(ctx)
		in.Id, err = g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).Data(planData).InsertAndGetId()
		if err != nil {
			return nil, gerror.Wrap(err, "新增监听计划失败")
		}
	}
	if err = s.listenerPlanSyncTargets(ctx, account.TenantId, account.Id, in.Id, in.TgAccountId, in.TargetChatIds); err != nil {
		return nil, err
	}
	return &sysin.ListenerPlanSaveModel{Id: in.Id}, nil
}

func (s *sSysPublish) AdminListenerPlanDelete(ctx context.Context, in *sysin.ListenerPlanDeleteInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的监听计划")
	}
	if err = ensureMessageListenTables(ctx); err != nil {
		return err
	}
	ids := uniqueIds(in.Ids)
	if err = s.ensureListenerPlansBelongTenant(ctx, ids, account.TenantId); err != nil {
		return err
	}
	now := gtime.Now()
	if _, err = g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", account.TenantId).
		WhereNull("deleted_at").
		Data(g.Map{"deleted_by": account.Id, "deleted_at": now, "updated_by": account.Id, "updated_at": now}).
		Update(); err != nil {
		return gerror.Wrap(err, "删除监听计划失败")
	}
	if _, err = g.DB().Model(messageListenTargetTable).Safe().Ctx(ctx).
		WhereIn("plan_id", ids).
		Where("tenant_id", account.TenantId).
		WhereNull("deleted_at").
		Data(g.Map{"deleted_at": now, "updated_at": now}).Update(); err != nil {
		return gerror.Wrap(err, "删除监听目标失败")
	}
	return nil
}

func (s *sSysPublish) AdminListenerPlanStatus(ctx context.Context, in *sysin.ListenerPlanStatusInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("计划状态不能为空")
	}
	if err = ensureMessageListenTables(ctx); err != nil {
		return err
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	if _, err = s.listenerPlanById(ctx, in.Id, account.TenantId); err != nil {
		return err
	}
	_, err = g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		Where("id", in.Id).
		Where("tenant_id", account.TenantId).
		WhereNull("deleted_at").
		Data(g.Map{"status": in.Status, "updated_by": account.Id, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新监听计划状态失败")
	}
	return nil
}

func (s *sSysPublish) AdminListenerPlanUnbind(ctx context.Context, in *sysin.ListenerPlanUnbindInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("解绑参数不能为空")
	}
	if err = ensureMessageListenTables(ctx); err != nil {
		return err
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	_, err = s.listenerPlanById(ctx, in.Id, account.TenantId)
	if err != nil {
		return err
	}
	_, err = g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		Where("id", in.Id).
		Where("tenant_id", account.TenantId).
		WhereNull("deleted_at").
		Data(g.Map{
			"bind_code":         listenerBindCode(ctx),
			"notify_chat_id":    "",
			"notify_chat_type":  "",
			"notify_chat_title": "",
			"notify_bound_at":   nil,
			"updated_by":        account.Id,
			"updated_at":        gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "解绑通知目标失败")
	}
	return nil
}

func (s *sSysPublish) listenerPlanModels(ctx context.Context, plans []*listenerPlanRecord, tenantId int64) (list []*sysin.ListenerPlanModel, err error) {
	if len(plans) == 0 {
		return []*sysin.ListenerPlanModel{}, nil
	}
	planIds := make([]int64, 0, len(plans))
	for _, plan := range plans {
		if plan != nil && plan.Id > 0 {
			planIds = append(planIds, plan.Id)
		}
	}
	if err = s.normalizeListenerPlanBindCodes(ctx, planIds, tenantId); err != nil {
		return nil, err
	}
	if err = s.reloadListenerPlanBindFields(ctx, plans, tenantId); err != nil {
		return nil, err
	}
	targetMap, err := s.listenerPlanTargetsByPlanIds(ctx, planIds, tenantId)
	if err != nil {
		return nil, err
	}
	list = make([]*sysin.ListenerPlanModel, 0, len(plans))
	for _, plan := range plans {
		if plan == nil {
			continue
		}
		targets := targetMap[plan.Id]
		model := &sysin.ListenerPlanModel{
			Id:            plan.Id,
			TenantId:      plan.TenantId,
			Name:          plan.Name,
			TgAccountId:   plan.TgAccountId,
			BotId:         plan.BotId,
			KeywordText:   strings.Join(listenerDecodeStringArray(plan.KeywordsJson), ","),
			Keywords:      listenerDecodeStringArray(plan.KeywordsJson),
			TargetCount:   len(targets),
			BoundCount:    boolToInt(strings.TrimSpace(plan.NotifyChatId) != ""),
			BindCode:      plan.BindCode,
			NotifyChatId:  plan.NotifyChatId,
			NotifyChatTyp: plan.NotifyChatTyp,
			NotifyTitle:   plan.NotifyTitle,
			NotifyBoundAt: plan.NotifyBoundAt,
			Status:        plan.Status,
			LastTriggerAt: plan.LastTriggerAt,
			LastResult:    plan.LastResult,
			CreatedBy:     plan.CreatedBy,
			UpdatedBy:     plan.UpdatedBy,
			DeletedBy:     plan.DeletedBy,
			CreatedAt:     plan.CreatedAt,
			UpdatedAt:     plan.UpdatedAt,
			DeletedAt:     plan.DeletedAt,
			Targets:       listenerTargetModels(targets),
		}
		for _, target := range targets {
			if target == nil {
				continue
			}
			model.TargetChatIds = append(model.TargetChatIds, target.TargetChatId)
		}
		list = append(list, model)
	}
	return list, nil
}

func (s *sSysPublish) listenerPlanById(ctx context.Context, id int64, tenantId int64) (listenerPlanRecord, error) {
	var plan listenerPlanRecord
	err := g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Scan(&plan)
	if err != nil {
		return plan, gerror.Wrap(err, "读取监听计划失败")
	}
	if plan.Id <= 0 {
		return plan, gerror.New("监听计划不存在")
	}
	return plan, nil
}

func (s *sSysPublish) listenerTargetById(ctx context.Context, id int64, tenantId int64) (listenerTargetRecord, error) {
	var target listenerTargetRecord
	err := g.DB().Model(messageListenTargetTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Scan(&target)
	if err != nil {
		return target, gerror.Wrap(err, "读取监听目标失败")
	}
	if target.Id <= 0 {
		return target, gerror.New("监听目标不存在")
	}
	return target, nil
}

func (s *sSysPublish) listenerPlanTargetsByPlanIds(ctx context.Context, planIds []int64, tenantId int64) (map[int64][]*listenerTargetRecord, error) {
	targetMap := make(map[int64][]*listenerTargetRecord)
	if len(planIds) == 0 {
		return targetMap, nil
	}
	var rows []*listenerTargetRecord
	mod := g.DB().Model(messageListenTargetTable).Safe().Ctx(ctx).
		WhereIn("plan_id", uniqueIds(planIds)).
		WhereNull("deleted_at").
		OrderAsc("plan_id").
		OrderAsc("id")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if err := mod.Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取监听目标失败")
	}
	for _, row := range rows {
		targetMap[row.PlanId] = append(targetMap[row.PlanId], row)
	}
	return targetMap, nil
}

func (s *sSysPublish) listenerPlanSyncTargets(ctx context.Context, tenantId int64, userId int64, planId int64, tgAccountId int64, targetChatIds []string) error {
	channels, err := s.messagePushCachedTargets(ctx, tgAccountId, targetChatIds, tenantId)
	if err != nil {
		return gerror.Wrap(err, "检查监听目标失败")
	}
	existingRows, err := s.listenerPlanTargetRows(ctx, planId, tenantId)
	if err != nil {
		return err
	}
	existingMap := make(map[string]*listenerTargetRecord, len(existingRows))
	for _, row := range existingRows {
		if row == nil {
			continue
		}
		existingMap[strings.TrimSpace(row.TargetChatId)] = row
	}
	now := gtime.Now()
	seen := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		targetChatId := normalizeTelegramChannelChatID(channel.TargetChatId)
		seen[targetChatId] = struct{}{}
		row := existingMap[targetChatId]
		data := g.Map{
			"plan_id":              planId,
			"tenant_id":            tenantId,
			"target_chat_id":       targetChatId,
			"target_chat_type":     listenerTargetType(channel),
			"target_chat_title":    strings.TrimSpace(channel.ChannelTitle),
			"target_chat_username": "",
			"status":               sysin.MessageListenerStatusEnabled,
			"updated_at":           now,
		}
		if row == nil {
			data["created_at"] = now
			data["created_by"] = userId
			data["updated_by"] = userId
			if _, err = g.DB().Model(messageListenTargetTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
				return gerror.Wrap(err, "新增监听目标失败")
			}
			continue
		}
		if row.LastMatchedAt != nil {
			data["last_matched_at"] = row.LastMatchedAt
		}
		if strings.TrimSpace(row.LastMatchedText) != "" {
			data["last_matched_text"] = row.LastMatchedText
		}
		if strings.TrimSpace(row.LastMatchedUserId) != "" {
			data["last_matched_user_id"] = row.LastMatchedUserId
		}
		data["updated_by"] = userId
		if _, err = g.DB().Model(messageListenTargetTable).Safe().Ctx(ctx).
			Where("id", row.Id).
			Where("tenant_id", tenantId).
			WhereNull("deleted_at").
			Data(data).Update(); err != nil {
			return gerror.Wrap(err, "更新监听目标失败")
		}
	}
	removedIds := make([]int64, 0)
	for _, row := range existingRows {
		if row == nil {
			continue
		}
		if _, ok := seen[strings.TrimSpace(row.TargetChatId)]; ok {
			continue
		}
		removedIds = append(removedIds, row.Id)
	}
	if len(removedIds) > 0 {
		_, err = g.DB().Model(messageListenTargetTable).Safe().Ctx(ctx).
			WhereIn("id", removedIds).
			Where("tenant_id", tenantId).
			WhereNull("deleted_at").
			Data(g.Map{"deleted_at": now, "updated_at": now, "updated_by": userId}).Update()
		if err != nil {
			return gerror.Wrap(err, "移除监听目标失败")
		}
	}
	return nil
}

func (s *sSysPublish) normalizeListenerPlanBindCodes(ctx context.Context, planIds []int64, tenantId int64) error {
	planIds = uniqueIds(planIds)
	if len(planIds) == 0 {
		return nil
	}
	var rows []*listenerPlanRecord
	mod := g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		Fields("id,bind_code").
		WhereIn("id", planIds).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if err := mod.Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取监听计划绑定ID失败")
	}
	for _, row := range rows {
		if row == nil || row.Id <= 0 {
			continue
		}
		if listenerBindCodeValid(row.BindCode) {
			continue
		}
		if _, err := g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
			Where("id", row.Id).
			WhereNull("deleted_at").
			Data(g.Map{"bind_code": listenerBindCode(ctx), "updated_at": gtime.Now()}).
			Update(); err != nil {
			return gerror.Wrap(err, "更新监听计划绑定ID失败")
		}
	}
	return nil
}

func (s *sSysPublish) reloadListenerPlanBindFields(ctx context.Context, plans []*listenerPlanRecord, tenantId int64) error {
	planIds := make([]int64, 0, len(plans))
	planMap := make(map[int64]*listenerPlanRecord, len(plans))
	for _, plan := range plans {
		if plan == nil || plan.Id <= 0 {
			continue
		}
		planIds = append(planIds, plan.Id)
		planMap[plan.Id] = plan
	}
	if len(planIds) == 0 {
		return nil
	}
	var rows []*listenerPlanRecord
	mod := g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		Fields("id,bind_code,notify_chat_id,notify_chat_type,notify_chat_title,notify_bound_at").
		WhereIn("id", uniqueIds(planIds)).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if err := mod.Scan(&rows); err != nil {
		return gerror.Wrap(err, "刷新监听计划绑定信息失败")
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		plan := planMap[row.Id]
		if plan == nil {
			continue
		}
		plan.BindCode = row.BindCode
		plan.NotifyChatId = row.NotifyChatId
		plan.NotifyChatTyp = row.NotifyChatTyp
		plan.NotifyTitle = row.NotifyTitle
		plan.NotifyBoundAt = row.NotifyBoundAt
	}
	return nil
}

func (s *sSysPublish) listenerPlanTargetRows(ctx context.Context, planId int64, tenantId int64) ([]*listenerTargetRecord, error) {
	var rows []*listenerTargetRecord
	if err := g.DB().Model(messageListenTargetTable).Safe().Ctx(ctx).
		Where("plan_id", planId).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取监听目标失败")
	}
	return rows, nil
}
