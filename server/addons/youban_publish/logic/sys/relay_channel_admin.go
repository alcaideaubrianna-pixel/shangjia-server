package sys

import (
	"context"
	relayconsts "hotgo/addons/youban_publish/consts"
	coreconsts "hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/dict"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/gotd/td/tg"
	"hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) AdminRelayChannelList(ctx context.Context, in *sysin.RelayChannelListInp) ([]*sysin.RelayChannelModel, error) {
	tenantID := int64(0)
	if in != nil {
		tenantID = in.TenantId
	}
	tenantID, accountID, err := s.relayAdminScope(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	list := []*sysin.RelayChannelModel{}
	c := dao.YoubanPublishRelayChannel.Columns()
	q := dao.YoubanPublishRelayChannel.Ctx(ctx)
	if tenantID > 0 {
		q = q.Where(c.TenantId, tenantID)
	}
	if accountID > 0 {
		q = q.WhereIn(c.TgAccountId, g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).Fields("id").Where("tenant_id", tenantID).Where("account_id", accountID).WhereNull("deleted_at"))
	}
	err = q.OrderDesc(c.Id).Limit(500).Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "读取中转频道失败")
	}
	ids := make([]int64, 0, len(list))
	for _, row := range list {
		row.HealthStatusLabel = dict.GetOptionLabel(relayconsts.RelayChannelHealthOptions, row.HealthStatus)
		row.BotPermissionStatusLabel = dict.GetOptionLabel(relayconsts.RelayChannelHealthOptions, row.BotPermissionStatus)
		row.OperationStatusLabel = dict.GetOptionLabel(relayconsts.RelayOperationStatusOptions, row.OperationStatus)
		ids = append(ids, row.Id)
		row.InFlightCount = 0
	}
	if len(ids) > 0 {
		pc := dao.YoubanPublishRelayPackage.Columns()
		var loads []struct {
			RelayChannelId int64
			Total          int
		}
		if err = dao.YoubanPublishRelayPackage.Ctx(ctx).Fields(pc.RelayChannelId+", COUNT(*) AS total").WhereIn(pc.RelayChannelId, ids).Where(pc.Status, relayconsts.RelayStatusPreparing).WhereGT(pc.LeaseUntil, gtime.Now()).Group(pc.RelayChannelId).Scan(&loads); err != nil {
			return nil, err
		}
		counts := map[int64]int{}
		for _, load := range loads {
			counts[load.RelayChannelId] = load.Total
		}
		for _, row := range list {
			row.InFlightCount = counts[row.Id]
		}
	}
	return list, nil
}

// relayAdminScope separates system identities from tenant account identities.
func (s *sSysPublish) relayAdminScope(ctx context.Context, tenantID int64) (int64, int64, error) {
	user := contexts.GetUser(ctx)
	if user == nil {
		return 0, 0, gerror.New("请先登录")
	}
	switch user.App {
	case coreconsts.AppAdmin:
		return tenantID, 0, s.requireSystemSuperAdmin(ctx)
	case coreconsts.AppApi:
		account, err := s.currentAdminAccount(ctx)
		if err != nil {
			return 0, 0, err
		}
		if tenantID != 0 && tenantID != account.TenantId {
			return 0, 0, gerror.New("无权操作其他租户的中转频道")
		}
		return account.TenantId, account.Id, nil
	default:
		return 0, 0, gerror.New("没有访问权限")
	}
}

func (s *sSysPublish) validateRelayOwner(ctx context.Context, tenantID, accountID, botID int64) error {
	_, ownerID, err := s.relayAdminScope(ctx, tenantID)
	if err != nil {
		return err
	}
	account, err := s.adminTgAccountById(ctx, accountID, tenantID)
	if err != nil {
		return err
	}
	if ownerID > 0 && account.AccountId != ownerID {
		return gerror.New("无权操作该协议账号的中转频道")
	}
	if botID > 0 {
		_, err = s.getBotById(ctx, botID, tenantID)
	}
	return err
}

func (s *sSysPublish) AdminRelayChannelCreate(ctx context.Context, in *sysin.RelayChannelCreateInp) (*sysin.RelayChannelModel, error) {
	if in != nil && strings.TrimSpace(in.Title) == "" {
		in.Title = "资料中转 " + time.Now().Format("01021504")
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	if err := s.validateRelayOwner(ctx, in.TenantId, in.TgAccountId, in.BotId); err != nil {
		return nil, err
	}
	id, err := dao.YoubanPublishRelayChannel.Ctx(ctx).Data(do.YoubanPublishRelayChannel{
		TenantId: in.TenantId, TgAccountId: in.TgAccountId, BotId: 0, Title: strings.TrimSpace(in.Title),
		TargetChatId: "pending:" + guid.S(), Capacity: in.Capacity, Status: relayChannelStatusDisabled,
		HealthStatus: relayChannelHealthUnknown, BotPermissionStatus: relayBotPermissionUnknown,
		OperationKind: "create", OperationStatus: relayconsts.RelayStatusPending,
	}).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "保存中转频道创建任务失败")
	}
	if err = s.submitRelayControl(ctx, id); err != nil {
		return nil, err
	}
	var row sysin.RelayChannelModel
	err = dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(id).Scan(&row)
	return &row, err
}

func (s *sSysPublish) AdminRelayChannelSave(ctx context.Context, in *sysin.RelayChannelSaveInp) error {
	if err := in.Filter(ctx); err != nil {
		return err
	}
	if err := s.validateRelayOwner(ctx, in.TenantId, in.TgAccountId, in.BotId); err != nil {
		return err
	}
	// Access hashes come from the server-side cache; never trust JS numbers.
	cached, err := s.tgChannelCacheByChannelId(ctx, in.TenantId, in.TgAccountId, strings.TrimPrefix(strings.TrimSpace(in.TargetChatId), "-100"))
	if err != nil {
		return err
	}
	channelID, err := strconv.ParseInt(cached.ChannelId, 10, 64)
	if err != nil || channelID <= 0 || cached.AccessHash == "" {
		return gerror.New("频道缓存无效，请先刷新协议账号频道")
	}
	data := do.YoubanPublishRelayChannel{TenantId: in.TenantId, TgAccountId: in.TgAccountId, BotId: in.BotId,
		ChannelId: cached.ChannelId, TargetChatId: "-100" + cached.ChannelId, AccessHash: cached.AccessHash,
		Title: firstNonEmpty(strings.TrimSpace(in.Title), cached.ChannelTitle), Capacity: in.Capacity, Status: relayChannelStatusDisabled,
		HealthStatus: relayChannelHealthUnknown, BotPermissionStatus: relayBotPermissionUnknown, OperationKind: "check", OperationStatus: relayconsts.RelayStatusPending}
	if in.Id > 0 {
		var existing relayChannelRecord
		if err = dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(in.Id).Scan(&existing); err != nil {
			return err
		}
		if existing.Id == 0 || existing.TenantId != in.TenantId || existing.TgAccountId != in.TgAccountId || existing.TargetChatId != "-100"+cached.ChannelId {
			return gerror.New("不能更改中转频道归属，请新增频道配置")
		}
		c := dao.YoubanPublishRelayChannel.Columns()
		result, updateErr := dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(in.Id).WhereNotIn(c.OperationStatus, []string{relayconsts.RelayStatusPending, relayconsts.RelayStatusCreating, relayconsts.RelayStatusChecking}).Data(data).Update()
		if updateErr != nil {
			return updateErr
		}
		affected, updateErr := result.RowsAffected()
		if updateErr != nil {
			return updateErr
		}
		if affected == 0 {
			return gerror.New("中转频道任务处理中，请稍后操作")
		}
	} else {
		in.Id, err = dao.YoubanPublishRelayChannel.Ctx(ctx).Data(data).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "保存中转频道失败")
		}
	}
	return s.submitRelayControl(ctx, in.Id)
}

func (s *sSysPublish) AdminRelayChannelStatus(ctx context.Context, in *sysin.RelayChannelStatusInp) error {
	if err := in.Filter(ctx); err != nil {
		return err
	}
	c := dao.YoubanPublishRelayChannel.Columns()
	var row relayChannelRecord
	if err := dao.YoubanPublishRelayChannel.Ctx(ctx).Where(c.Id, in.Id).Scan(&row); err != nil {
		return err
	}
	if row.Id == 0 {
		return gerror.New("中转频道不存在")
	}
	if err := s.validateRelayOwner(ctx, row.TenantId, row.TgAccountId, 0); err != nil {
		return err
	}
	if row.OperationStatus == relayconsts.RelayStatusPending || row.OperationStatus == relayconsts.RelayStatusCreating || row.OperationStatus == relayconsts.RelayStatusChecking {
		return gerror.New("中转频道任务处理中，请稍后操作")
	}
	if in.Status == relayChannelStatusDisabled {
		result, err := dao.YoubanPublishRelayChannel.Ctx(ctx).Where(c.Id, in.Id).WhereNotIn(c.OperationStatus, []string{relayconsts.RelayStatusPending, relayconsts.RelayStatusCreating, relayconsts.RelayStatusChecking}).Data(do.YoubanPublishRelayChannel{Status: in.Status}).Update()
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return gerror.New("中转频道任务处理中，请稍后操作")
		}
		return nil
	}
	if row.BotId <= 0 {
		return gerror.New("请先编辑中转频道并选择推送Bot")
	}
	if row.ChannelId == "" {
		return gerror.New("频道创建尚未确认，请先检查创建任务")
	}
	result, err := dao.YoubanPublishRelayChannel.Ctx(ctx).Where(c.Id, in.Id).WhereNotIn(c.OperationStatus, []string{relayconsts.RelayStatusPending, relayconsts.RelayStatusCreating, relayconsts.RelayStatusChecking}).Data(do.YoubanPublishRelayChannel{OperationKind: "enable", OperationStatus: relayconsts.RelayStatusPending}).Update()
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return gerror.New("中转频道任务处理中，请稍后操作")
	}
	return s.submitRelayControl(ctx, in.Id)
}

func resolveRelayBot(ctx context.Context, api *tg.Client, username string) (*tg.InputUser, error) {
	result, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: strings.TrimPrefix(strings.TrimSpace(username), "@")})
	if err != nil {
		return nil, gerror.Wrap(err, "解析中转Bot失败")
	}
	return resolvedBotInputUser(result)
}
