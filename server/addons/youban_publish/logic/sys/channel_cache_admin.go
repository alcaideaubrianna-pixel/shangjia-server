package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) AdminChannelCacheList(ctx context.Context, in *sysin.ChannelCacheListInp) (list []*sysin.ChannelCacheModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ChannelCacheListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	if in.TgAccountId <= 0 {
		return []*sysin.ChannelCacheModel{}, 0, nil
	}
	if err = s.ensureTgAccountBelongsAccount(ctx, in.TgAccountId, account.TenantId, account.Id); err != nil {
		return nil, 0, err
	}
	return s.channelCacheList(ctx, in, account.TenantId)
}

func (s *sSysPublish) ServerChannelCacheList(ctx context.Context, in *sysin.ChannelCacheListInp) (list []*sysin.ChannelCacheModel, totalCount int, err error) {
	if err = s.requireSystemSuperAdmin(ctx); err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ChannelCacheListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	if in.TgAccountId <= 0 {
		return []*sysin.ChannelCacheModel{}, 0, nil
	}
	tenantId, err := s.tenantIdForTgAccount(ctx, in.TgAccountId)
	if err != nil {
		return nil, 0, err
	}
	return s.channelCacheList(ctx, in, tenantId)
}

func (s *sSysPublish) channelCacheList(ctx context.Context, in *sysin.ChannelCacheListInp, tenantId int64) (list []*sysin.ChannelCacheModel, totalCount int, err error) {
	mod := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("tg_account_id", in.TgAccountId)
	if keyword := strings.Trim(strings.TrimSpace(in.Keyword), "@"); keyword != "" {
		// Telegram usernames are commonly entered with a leading '@', while
		// channel_username is stored without it.
		keyword = strings.TrimPrefix(keyword, "@")
		like := "%" + keyword + "%"
		mod = mod.Where("(channel_title LIKE ? OR channel_username LIKE ? OR channel_id LIKE ?)", like, like, like)
	}
	if len(in.ManagementRoles) > 0 {
		mod = mod.WhereIn("management_role", in.ManagementRoles)
	}
	if in.CanPostMessages == 1 {
		mod = mod.Where("can_post_messages", 1)
	}
	if in.CanInviteUsers == 1 {
		mod = mod.Where("can_invite_users", 1)
	}
	if in.CanAddAdmins == 1 {
		mod = mod.Where("can_add_admins", 1)
	}
	switch in.DisplayType {
	case "channel":
		mod = mod.Where("channel_id NOT LIKE '-%'").Where("is_broadcast", 1)
	case "group":
		mod = mod.Where("(channel_id LIKE '-%' OR is_megagroup = 1)")
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道缓存总数失败")
	}
	if err = mod.Fields(channelCacheListFields()).
		Page(in.Page, in.PerPage).
		OrderDesc("last_sync_at").
		OrderDesc("id").
		Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道缓存失败")
	}
	if list == nil {
		list = []*sysin.ChannelCacheModel{}
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		item.DisplayType = resolveChannelCacheDisplayType(item)
		item.ManagementRole = normalizeChannelManagementRole(item.ManagementRole)
		item.ManagementRoleText = channelManagementRoleText(item.ManagementRole)
	}
	return list, totalCount, nil
}

func channelCacheListFields() string {
	return strings.Join([]string{
		"id",
		"tenant_id",
		"tg_account_id",
		"channel_id",
		"channel_title",
		"channel_username",
		"management_role",
		"is_broadcast",
		"is_megagroup",
		"can_post_messages",
		"can_invite_users",
		"can_add_admins",
		"last_sync_at",
	}, ",")
}

func (s *sSysPublish) AdminChannelCacheResolve(ctx context.Context, in *sysin.ChannelCacheResolveInp) ([]*sysin.ChannelCacheResolveModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("解析目标不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if err = s.ensureTgAccountBelongsAccount(ctx, in.TgAccountId, account.TenantId, account.Id); err != nil {
		return nil, err
	}
	displays, err := s.resolveTelegramChannelDisplays(ctx, account.TenantId, in.TgAccountId, in.TargetChatIds)
	if err != nil {
		return nil, err
	}
	list := make([]*sysin.ChannelCacheResolveModel, 0, len(in.TargetChatIds))
	for _, raw := range in.TargetChatIds {
		channelId := normalizeTelegramChannelChatID(raw)
		if channelId == "" {
			continue
		}
		display := displays[channelId]
		list = append(list, &sysin.ChannelCacheResolveModel{
			TgAccountId:     in.TgAccountId,
			ChannelId:       channelId,
			ChannelTitle:    display.Title,
			ChannelUsername: display.Username,
		})
	}
	return list, nil
}

func (s *sSysPublish) AdminChannelCacheRefresh(ctx context.Context, in *sysin.ChannelCacheRefreshInp) (res *sysin.ChannelCacheRefreshModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.TgAccountId <= 0 {
		return nil, gerror.New("请选择TG账号")
	}
	if err = s.ensureTgAccountBelongsAccount(ctx, in.TgAccountId, account.TenantId, account.Id); err != nil {
		return nil, err
	}
	item, err := s.adminTgAccountById(ctx, in.TgAccountId, account.TenantId)
	if err != nil {
		return nil, err
	}
	if item.Status != sysin.PublishTgAccountStatusAuthorized {
		return nil, gerror.New("TG账号未授权，请先刷新账号状态或重新扫码登录")
	}
	taskId, err := collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{
		TenantID:    account.TenantId,
		AccountID:   in.TgAccountId,
		TaskType:    collectorin.AccountTaskTypeDialogCacheRefresh,
		TaskKey:     fmt.Sprintf("dialog-cache-refresh:%d:%d", in.TgAccountId, time.Now().UnixNano()),
		Priority:    100,
		MaxAttempts: 3,
	})
	if err != nil {
		return nil, gerror.Wrap(err, "创建频道缓存刷新任务失败")
	}
	collectorservice.AccountRuntime().Refresh()
	return &sysin.ChannelCacheRefreshModel{
		Message:     "频道缓存刷新任务已提交",
		TgAccountId: in.TgAccountId,
		TaskId:      taskId,
		Status:      collectorin.AccountTaskStatusPending,
	}, nil
}

func (s *sSysPublish) AdminChannelCacheRefreshStatus(ctx context.Context, in *sysin.ChannelCacheRefreshStatusInp) (*sysin.ChannelCacheRefreshModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.TaskId <= 0 {
		return nil, gerror.New("刷新任务无效")
	}
	task, err := collectorservice.AccountTasks().Get(ctx, in.TaskId)
	if err != nil {
		return nil, err
	}
	if task.TenantID != account.TenantId || task.TaskType != collectorin.AccountTaskTypeDialogCacheRefresh {
		return nil, gerror.New("刷新任务不存在")
	}
	if err = s.ensureTgAccountBelongsAccount(ctx, task.AccountID, account.TenantId, account.Id); err != nil {
		return nil, err
	}
	count := 0
	if task.CreatedAt != nil {
		count, err = g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
			Where("tenant_id", account.TenantId).
			Where("tg_account_id", task.AccountID).
			WhereGTE("last_sync_at", gtime.New(*task.CreatedAt)).
			Count()
		if err != nil {
			return nil, gerror.Wrap(err, "读取频道缓存刷新进度失败")
		}
	}
	message := "频道缓存刷新中"
	if task.Status == collectorin.AccountTaskStatusCompleted {
		message = "群聊 / 频道缓存已更新"
	} else if task.Status == collectorin.AccountTaskStatusDead {
		message = "频道缓存刷新失败"
	}
	syncedAt := ""
	if task.CompletedAt != nil {
		syncedAt = gtime.New(*task.CompletedAt).String()
	}
	return &sysin.ChannelCacheRefreshModel{
		Count: count, Message: message, SyncedAt: syncedAt, TgAccountId: task.AccountID,
		TaskId: task.ID, Status: task.Status, ErrorMessage: task.ErrorMessage,
	}, nil
}
