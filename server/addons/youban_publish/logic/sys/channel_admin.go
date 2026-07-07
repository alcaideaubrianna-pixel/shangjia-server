package sys

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/contexts"
)

func (s *sSysPublish) AdminChannelList(ctx context.Context, in *sysin.ChannelListInp) (list []*sysin.ChannelModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ChannelListInp{}
	}
	in.TenantId = account.TenantId
	base := s.channelBaseModel(ctx)
	base = applyChannelFilters(base, in)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道总数失败")
	}
	if err = base.Fields("c.*,ta.display_name AS tg_account_name").
		Page(in.Page, in.PerPage).
		OrderDesc("c.id").
		Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道列表失败")
	}
	if list == nil {
		list = []*sysin.ChannelModel{}
	}
	applyChannelBotIds(list)
	return list, totalCount, nil
}

func (s *sSysPublish) MyChannelList(ctx context.Context, in *sysin.ChannelListInp) (list []*sysin.ChannelModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ChannelListInp{}
	}
	in.TenantId = account.TenantId
	in.PublishDirection = "up"
	in.Status = 1
	base := s.channelBaseModel(ctx)
	base = applyChannelFilters(base, in)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道总数失败")
	}
	if err = base.Fields("c.*,ta.display_name AS tg_account_name").
		Page(in.Page, in.PerPage).
		OrderDesc("c.is_default_selected").
		OrderDesc("c.id").
		Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道列表失败")
	}
	if list == nil {
		list = []*sysin.ChannelModel{}
	}
	applyChannelBotIds(list)
	return list, totalCount, nil
}

func (s *sSysPublish) ServerChannelList(ctx context.Context, in *sysin.ChannelListInp) (list []*sysin.ChannelModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.ChannelListInp{}
	}
	base := s.channelBaseModel(ctx)
	base = applyChannelFilters(base, in)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道总数失败")
	}
	if err = base.Fields("c.*,ta.display_name AS tg_account_name").
		Page(in.Page, in.PerPage).
		OrderDesc("c.id").
		Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道列表失败")
	}
	if list == nil {
		list = []*sysin.ChannelModel{}
	}
	applyChannelBotIds(list)
	return list, totalCount, nil
}

func (s *sSysPublish) AdminChannelSave(ctx context.Context, in *sysin.ChannelSaveInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("频道配置不能为空")
	}
	in.TenantId = account.TenantId
	if in.TenantId <= 0 {
		return gerror.New("当前账号未绑定账号归属")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	if err = s.ensureTgAccountsBelongTenant(ctx, []int64{in.TgAccountId}, in.TenantId); err != nil {
		return err
	}
	if err = s.ensureBotsBelongTenant(ctx, in.BotIds, in.TenantId); err != nil {
		return err
	}
	checkRes, err := s.checkAdminChannelBots(ctx, &sysin.ChannelCheckInp{
		BotIds:       in.BotIds,
		TargetChatId: in.TargetChatId,
		TgAccountId:  in.TgAccountId,
	}, in.TenantId, true)
	if err != nil {
		return err
	}
	if !channelCheckAllowed(checkRes) {
		return gerror.New(channelCheckMessage(checkRes))
	}
	in.ChannelTitle = checkRes.ChannelTitle
	in.ChannelUsername = checkRes.ChannelUsername
	in.TargetChatId = checkRes.TargetChatId
	if in.Id > 0 {
		if err = s.ensureChannelsBelongTenant(ctx, []int64{in.Id}, in.TenantId); err != nil {
			return err
		}
	}
	botJSON, err := encodeBotIds(in.BotIds)
	if err != nil {
		return err
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id":           in.TenantId,
		"merchant_id":         in.TenantId,
		"tg_account_id":       in.TgAccountId,
		"channel_title":       in.ChannelTitle,
		"channel_username":    in.ChannelUsername,
		"target_chat_id":      in.TargetChatId,
		"publish_direction":   in.PublishDirection,
		"is_default_selected": in.IsDefaultSelected,
		"bot_id_json":         botJSON,
		"remark":              in.Remark,
		"status":              in.Status,
		"updated_by":          account.Id,
		"updated_at":          now,
	}
	if in.Id > 0 {
		_, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
			Where("id", in.Id).
			Where("tenant_id", account.TenantId).
			WhereNull("deleted_at").
			Data(data).
			Update()
	} else {
		data["created_by"] = account.Id
		data["created_at"] = now
		_, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存频道配置失败")
	}
	return nil
}

func (s *sSysPublish) AdminChannelDelete(ctx context.Context, in *sysin.ChannelDeleteInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的频道")
	}
	in.Ids = uniqueIds(in.Ids)
	if err = s.ensureChannelsBelongTenant(ctx, in.Ids, account.TenantId); err != nil {
		return err
	}
	if _, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		WhereIn("id", in.Ids).
		Where("tenant_id", account.TenantId).
		Data(g.Map{
			"deleted_by": account.Id,
			"deleted_at": gtime.Now(),
		}).
		Update(); err != nil {
		return gerror.Wrap(err, "删除频道配置失败")
	}
	return nil
}

func (s *sSysPublish) ServerChannelDelete(ctx context.Context, in *sysin.ChannelDeleteInp) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的频道")
	}
	in.Ids = uniqueIds(in.Ids)
	if _, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		WhereIn("id", in.Ids).
		Data(g.Map{
			"deleted_by": contexts.GetUserId(ctx),
			"deleted_at": gtime.Now(),
		}).
		Update(); err != nil {
		return gerror.Wrap(err, "删除频道配置失败")
	}
	return nil
}

func (s *sSysPublish) AdminChannelBatchBots(ctx context.Context, in *sysin.ChannelBatchBotsInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择频道")
	}
	in.Ids = uniqueIds(in.Ids)
	in.BotIds = uniqueIds(in.BotIds)
	if err = s.ensureChannelsBelongTenant(ctx, in.Ids, account.TenantId); err != nil {
		return err
	}
	if err = s.ensureBotsBelongTenant(ctx, in.BotIds, account.TenantId); err != nil {
		return err
	}
	botJSON, err := encodeBotIds(in.BotIds)
	if err != nil {
		return err
	}
	if _, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		WhereIn("id", in.Ids).
		Where("tenant_id", account.TenantId).
		WhereNull("deleted_at").
		Data(g.Map{
			"bot_id_json": botJSON,
			"updated_by":  account.Id,
			"updated_at":  gtime.Now(),
		}).
		Update(); err != nil {
		return gerror.Wrap(err, "批量编辑频道Bot失败")
	}
	return nil
}

func (s *sSysPublish) AdminChannelRefresh(ctx context.Context, in *sysin.ChannelRefreshInp) (list []*sysin.ChannelRefreshModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || len(in.Ids) == 0 {
		return nil, gerror.New("请选择要刷新的频道")
	}
	in.Ids = uniqueIds(in.Ids)
	if err = s.ensureChannelsBelongTenant(ctx, in.Ids, account.TenantId); err != nil {
		return nil, err
	}
	var channels []*sysin.ChannelModel
	if err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		WhereIn("id", in.Ids).
		Where("tenant_id", account.TenantId).
		WhereNull("deleted_at").
		Scan(&channels); err != nil {
		return nil, gerror.Wrap(err, "读取频道配置失败")
	}
	now := gtime.Now()
	list = make([]*sysin.ChannelRefreshModel, 0, len(channels))
	for _, item := range channels {
		status := "success"
		message := "mock刷新成功，真实TG频道校验待接入"
		if strings.TrimSpace(item.TargetChatId) == "" && strings.TrimSpace(item.ChannelUsername) == "" {
			status = "failed"
			message = "频道缺少Chat ID或用户名"
		}
		if _, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
			Where("id", item.Id).
			Where("tenant_id", account.TenantId).
			Data(g.Map{
				"last_refresh_status":  status,
				"last_refresh_message": message,
				"last_refresh_at":      now,
				"updated_by":           account.Id,
				"updated_at":           now,
			}).
			Update(); err != nil {
			return nil, gerror.Wrap(err, "刷新频道状态失败")
		}
		list = append(list, &sysin.ChannelRefreshModel{
			Id:                 item.Id,
			LastRefreshStatus:  status,
			LastRefreshMessage: message,
		})
	}
	return list, nil
}

func (s *sSysPublish) ServerChannelRefresh(ctx context.Context, in *sysin.ChannelRefreshInp) (list []*sysin.ChannelRefreshModel, err error) {
	if in == nil || len(in.Ids) == 0 {
		return nil, gerror.New("请选择要刷新的频道")
	}
	in.Ids = uniqueIds(in.Ids)
	var channels []*sysin.ChannelModel
	if err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		WhereIn("id", in.Ids).
		WhereNull("deleted_at").
		Scan(&channels); err != nil {
		return nil, gerror.Wrap(err, "读取频道配置失败")
	}
	now := gtime.Now()
	list = make([]*sysin.ChannelRefreshModel, 0, len(channels))
	for _, item := range channels {
		status := "success"
		message := "mock刷新成功，真实TG频道校验待接入"
		if strings.TrimSpace(item.TargetChatId) == "" && strings.TrimSpace(item.ChannelUsername) == "" {
			status = "failed"
			message = "频道缺少Chat ID或用户名"
		}
		if _, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
			Where("id", item.Id).
			Data(g.Map{
				"last_refresh_status":  status,
				"last_refresh_message": message,
				"last_refresh_at":      now,
				"updated_by":           contexts.GetUserId(ctx),
				"updated_at":           now,
			}).
			Update(); err != nil {
			return nil, gerror.Wrap(err, "刷新频道状态失败")
		}
		list = append(list, &sysin.ChannelRefreshModel{
			Id:                 item.Id,
			LastRefreshStatus:  status,
			LastRefreshMessage: message,
		})
	}
	return list, nil
}

func (s *sSysPublish) channelBaseModel(ctx context.Context) *gdb.Model {
	return g.DB().Model(publishChannelTable+" c").Safe().Ctx(ctx).
		LeftJoin(publishTgAccountTable+" ta", "ta.id=c.tg_account_id AND ta.deleted_at IS NULL").
		WhereNull("c.deleted_at")
}

func applyChannelFilters(mod *gdb.Model, in *sysin.ChannelListInp) *gdb.Model {
	if in.TenantId > 0 {
		mod = mod.Where("c.tenant_id", in.TenantId)
	}
	if in.TgAccountId > 0 {
		mod = mod.Where("c.tg_account_id", in.TgAccountId)
	}
	if direction := strings.TrimSpace(in.PublishDirection); direction != "" {
		mod = mod.Where("c.publish_direction", direction)
	}
	if in.Status > 0 {
		mod = mod.Where("c.status", in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(c.channel_title LIKE ? OR c.channel_username LIKE ? OR c.target_chat_id LIKE ? OR c.remark LIKE ?)", like, like, like, like)
	}
	return mod
}

func applyChannelBotIds(list []*sysin.ChannelModel) {
	for _, item := range list {
		if item == nil {
			continue
		}
		item.BotIds = decodeBotIds(item.BotIdJson)
	}
}

func encodeBotIds(ids []int64) (string, error) {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return "", gerror.Wrap(err, "编码频道Bot配置失败")
	}
	return string(data), nil
}

func decodeBotIds(data string) []int64 {
	data = strings.TrimSpace(data)
	if data == "" {
		return []int64{}
	}
	var ids []int64
	if err := json.Unmarshal([]byte(data), &ids); err != nil {
		return []int64{}
	}
	return uniqueIds(ids)
}

func (s *sSysPublish) ensureChannelsBelongTenant(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return gerror.New("请选择频道")
	}
	count, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查频道权限失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作的频道")
	}
	return nil
}

func (s *sSysPublish) ensureBotsBelongTenant(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return nil
	}
	count, err := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查Bot权限失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作的Bot")
	}
	return nil
}
