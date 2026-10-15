package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	twdao "hotgo/addons/youban_two_way_bot/internal/dao"
	"hotgo/addons/youban_two_way_bot/internal/model/entity"
	"hotgo/addons/youban_two_way_bot/model/input/sysin"
)

func (s *sSysTwoWayBot) AdminBotList(ctx context.Context, in *sysin.BotListInp) (list []*sysin.BotModel, totalCount int, err error) {
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.BotListInp{}
	}
	columns := twdao.YoubanBotRegistry.Columns()
	mod := twdao.YoubanBotRegistry.Ctx(ctx).
		Where(columns.TenantId, account.TenantId).
		Where(columns.AccountId, account.Id).Where(columns.BotType, "two_way").
		WhereNull(columns.DeletedAt)
	if strings.TrimSpace(in.Keyword) != "" {
		keyword := "%" + strings.TrimSpace(in.Keyword) + "%"
		mod = mod.Where("("+columns.BotName+" LIKE ? OR "+columns.BotUsername+" LIKE ?)", keyword, keyword)
	}
	if in.Status != nil &&
		(*in.Status == sysin.TwoWayBotStatusEnabled || *in.Status == sysin.TwoWayBotStatusDisabled) {
		status := *in.Status
		if status == sysin.TwoWayBotStatusDisabled {
			status = 2
		}
		mod = mod.Where(columns.Status, status)
	}
	var rows []*entity.YoubanBotRegistry
	if err = mod.Page(in.GetPage(), in.GetPerPage()).OrderDesc(columns.Id).ScanAndCount(&rows, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "读取双向机器人失败")
	}
	tgAccountNames, err := loadTgAccountNames(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	list = make([]*sysin.BotModel, 0, len(rows))
	for _, row := range rows {
		list = append(list, botModel(row, tgAccountNames[row.TgAccountId]))
	}
	return list, totalCount, nil
}

func (s *sSysTwoWayBot) AdminBotSave(ctx context.Context, in *sysin.BotSaveInp) error {
	if in == nil {
		return gerror.New("参数不能为空")
	}
	if err := in.Filter(); err != nil {
		return err
	}
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if err = ensureTgAccountBelongsTenant(ctx, in.TgAccountId, account.TenantId); err != nil {
		return err
	}
	var old *entity.YoubanBotRegistry
	if in.Id > 0 {
		old, err = s.ownedBotByID(ctx, in.Id, account)
		if err != nil {
			return err
		}
	}
	token := strings.TrimSpace(in.BotToken)
	if token == "" && old != nil {
		token = old.BotToken
	}
	botUserId, botUsername, botDisplayName, err := s.creationToken(ctx, token)
	if err != nil {
		return err
	}
	if old != nil && old.AccountId != account.Id {
		return gerror.New("无权编辑其他账户的Bot")
	}
	if old != nil && old.BotUserId != botUserId {
		return gerror.New("不能把现有Bot更换为另一个机器人，请新增Bot")
	}
	count, err := twdao.YoubanBotRegistry.Ctx(ctx).Unscoped().Where(twdao.YoubanBotRegistry.Columns().BotUserId, botUserId).WhereNot(twdao.YoubanBotRegistry.Columns().Id, in.Id).Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return gerror.New("该Telegram Bot已存在，请编辑原Bot，不能重复创建")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = botDisplayName
	}
	if name == "" {
		name = botUsername
	}
	if name == "" {
		name = "双向机器人"
	}
	now := gtime.Now()
	setupStatus := sysin.TwoWayBotSetupReady
	supergroupId := strings.TrimSpace(in.SupergroupId)
	supergroupTitle := strings.TrimSpace(in.SupergroupTitle)
	inviteLink := strings.TrimSpace(in.InviteLink)
	supergroupAccessHash := ""
	if in.Id <= 0 {
		tgAccount, err := tgAccountById(ctx, in.TgAccountId, account.TenantId)
		if err != nil {
			return err
		}
		setup, err := s.creationGroup(ctx, account, tgAccount, in.ExistingGroupId, botUsername)
		if err != nil {
			return err
		}
		supergroupId = setup.SupergroupId
		supergroupAccessHash = fmt.Sprintf("%d", setup.AccessHash)
		supergroupTitle = setup.Title
		inviteLink = setup.InviteLink
	} else if old != nil {
		if in.Status == nil {
			status := old.Status
			if status == 2 {
				status = sysin.TwoWayBotStatusDisabled
			}
			in.Status = &status
		}
		supergroupAccessHash = old.SupergroupAccessHash
		if supergroupId == "" {
			supergroupId = old.SupergroupId
		}
		if supergroupTitle == "" {
			supergroupTitle = old.SupergroupTitle
		}
		if inviteLink == "" {
			inviteLink = old.InviteLink
		}
	}
	status := sysin.TwoWayBotStatusEnabled
	if in.Status != nil {
		status = *in.Status
	}
	if status == sysin.TwoWayBotStatusDisabled {
		status = 2
	}
	data := g.Map{
		"tenant_id":              account.TenantId,
		"account_id":             account.Id,
		"tg_account_id":          in.TgAccountId,
		"bot_name":               name,
		"welcome_message":        in.WelcomeMessage,
		"bot_token":              token,
		"bot_user_id":            botUserId,
		"bot_type":               "two_way",
		"bot_username":           botUsername,
		"supergroup_id":          supergroupId,
		"supergroup_access_hash": supergroupAccessHash,
		"supergroup_title":       supergroupTitle,
		"invite_link":            inviteLink,
		"setup_status":           setupStatus,
		"last_setup_at":          now,
		"status":                 status,
		"error_message":          "",
		"updated_at":             now,
	}
	if in.Id > 0 {
		if _, err = twdao.YoubanBotRegistry.Ctx(ctx).WherePri(in.Id).Where("tenant_id", account.TenantId).Where("account_id", account.Id).Where("bot_type", "two_way").Data(data).Update(); err != nil {
			return gerror.Wrap(err, "更新双向机器人失败")
		}
	} else {
		data["created_by"] = account.Id
		data["created_at"] = now
		if in.Id, err = twdao.YoubanBotRegistry.Ctx(ctx).Data(data).InsertAndGetId(); err != nil {
			return gerror.Wrap(err, "创建双向机器人失败")
		}
	}
	if supergroupId != "" {
		if _, err := s.ownedBotByID(ctx, in.Id, account); err != nil {
			return err
		}
		return s.creationSync(ctx, in.Id, account.TenantId)
	}
	return nil
}

func (s *sSysTwoWayBot) AdminBotSettings(ctx context.Context, in *sysin.BotSettingsInp) error {
	if err := in.Filter(); err != nil {
		return err
	}
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	columns := twdao.YoubanBotRegistry.Columns()
	result, err := twdao.YoubanBotRegistry.Ctx(ctx).
		Where(columns.Id, in.Id).
		Where(columns.TenantId, account.TenantId).
		Where(columns.AccountId, account.Id).Where(columns.BotType, "two_way").
		WhereNull(columns.DeletedAt).
		Data(g.Map{
			columns.BotName:        in.Name,
			columns.WelcomeMessage: in.WelcomeMessage,
			columns.UpdatedAt:      gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "保存双向机器人设置失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return gerror.New("双向机器人不存在")
	}
	return nil
}

func (s *sSysTwoWayBot) AdminBotDelete(ctx context.Context, in *sysin.BotDeleteInp) error {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择机器人")
	}
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	_, err = twdao.YoubanBotRegistry.Ctx(ctx).
		WhereIn("id", in.Ids).
		Where("tenant_id", account.TenantId).Where("account_id", account.Id).Where("bot_type", "two_way").
		Data(g.Map{"deleted_at": gtime.Now(), "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "删除双向机器人失败")
	}
	return nil
}

func (s *sSysTwoWayBot) AdminBotRefreshWebhook(ctx context.Context, in *sysin.BotActionInp) error {
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if _, err := s.ownedBotByID(ctx, in.Id, account); err != nil {
		return err
	}
	return s.refreshWebhookById(ctx, in.Id, account.TenantId)
}

func (s *sSysTwoWayBot) AdminBotSetup(ctx context.Context, in *sysin.BotActionInp) error {
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	row, err := s.ownedBotByID(ctx, in.Id, account)
	if err != nil {
		return err
	}
	return s.refreshWebhookById(ctx, row.Id, account.TenantId)
}

func (s *sSysTwoWayBot) refreshWebhookById(ctx context.Context, id int64, tenantId int64) error {
	return s.syncRuntimeById(ctx, id, tenantId)
}

func (s *sSysTwoWayBot) syncRuntimeById(ctx context.Context, id int64, tenantId int64) error {
	row, err := s.botById(ctx, id, tenantId)
	if err != nil {
		return err
	}
	if strings.TrimSpace(row.SupergroupId) == "" {
		return gerror.New("请先配置管理群ID")
	}
	if err = gatewayservice.Gateway().Refresh(ctx); err != nil {
		return gerror.Wrap(err, "刷新TG Bot Gateway失败")
	}
	return s.updateWebhookStatus(ctx, row.Id, sysin.TwoWayBotWebhookReady, "")
}

func (s *sSysTwoWayBot) validateBotToken(ctx context.Context, token string) (userId string, username string, displayName string, err error) {
	bot, err := s.telegramBot(ctx, token)
	if err != nil {
		return "", "", "", err
	}
	user, err := bot.GetMe(ctx)
	if err != nil {
		return "", "", "", gerror.Wrap(err, "校验Bot Token失败")
	}
	if user == nil || !user.IsBot {
		return "", "", "", gerror.New("Token不是Telegram Bot")
	}
	username = strings.TrimPrefix(user.Username, "@")
	displayName = strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	if displayName == "" {
		displayName = username
	}
	return fmt.Sprintf("%d", user.ID), username, displayName, nil
}

func (s *sSysTwoWayBot) botById(ctx context.Context, id int64, tenantId int64) (*entity.YoubanBotRegistry, error) {
	if id <= 0 {
		return nil, gerror.New("请选择机器人")
	}
	columns := twdao.YoubanBotRegistry.Columns()
	var row *entity.YoubanBotRegistry
	err := twdao.YoubanBotRegistry.Ctx(ctx).
		Where(columns.Id, id).
		Where(columns.TenantId, tenantId).Where(columns.BotType, "two_way").
		WhereNull(columns.DeletedAt).
		Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取双向机器人失败")
	}
	if row == nil || row.Id <= 0 {
		return nil, gerror.New("双向机器人不存在")
	}
	return row, nil
}

func loadTgAccountNames(ctx context.Context, rows []*entity.YoubanBotRegistry) (map[int64]string, error) {
	ids := make([]int64, 0, len(rows))
	seen := map[int64]struct{}{}
	for _, row := range rows {
		if row == nil || row.TgAccountId <= 0 {
			continue
		}
		if _, ok := seen[row.TgAccountId]; ok {
			continue
		}
		seen[row.TgAccountId] = struct{}{}
		ids = append(ids, row.TgAccountId)
	}
	if len(ids) == 0 {
		return map[int64]string{}, nil
	}
	type tgAccountNameRow struct {
		Id               int64  `json:"id"`
		DisplayName      string `json:"display_name"`
		TelegramUsername string `json:"telegram_username"`
	}
	var items []*tgAccountNameRow
	if err := g.DB().Model("hg_youban_publish_tg_account").Safe().Ctx(ctx).
		Fields("id,display_name,telegram_username").
		WhereIn("id", ids).
		WhereNull("deleted_at").
		Scan(&items); err != nil {
		return nil, gerror.Wrap(err, "读取TG账号昵称失败")
	}
	out := make(map[int64]string, len(items))
	for _, item := range items {
		if item == nil || item.Id <= 0 {
			continue
		}
		name := strings.TrimSpace(item.DisplayName)
		if name == "" {
			name = strings.TrimPrefix(strings.TrimSpace(item.TelegramUsername), "@")
		}
		out[item.Id] = name
	}
	return out, nil
}

func botModel(row *entity.YoubanBotRegistry, tgAccountName string) *sysin.BotModel {
	if row == nil {
		return nil
	}
	status := row.Status
	if status == 2 {
		status = sysin.TwoWayBotStatusDisabled
	}
	return &sysin.BotModel{
		Id:                   row.Id,
		TenantId:             row.TenantId,
		AccountId:            row.AccountId,
		TgAccountId:          row.TgAccountId,
		TgAccountName:        strings.TrimSpace(tgAccountName),
		Name:                 row.BotName,
		WelcomeMessage:       row.WelcomeMessage,
		BotUserId:            row.BotUserId,
		BotUsername:          row.BotUsername,
		SupergroupId:         row.SupergroupId,
		SupergroupAccessHash: row.SupergroupAccessHash,
		SupergroupTitle:      row.SupergroupTitle,
		InviteLink:           row.InviteLink,
		SetupStatus:          row.SetupStatus,
		WebhookStatus:        row.WebhookStatus,
		Status:               status,
		ErrorMessage:         row.ErrorMessage,
		LastSetupAt:          row.LastSetupAt,
		LastWebhookAt:        row.LastWebhookAt,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

func (s *sSysTwoWayBot) ownedBotByID(ctx context.Context, id int64, account *publishAccount) (*entity.YoubanBotRegistry, error) {
	row, err := s.botById(ctx, id, account.TenantId)
	if err != nil {
		return nil, err
	}
	if row.AccountId != account.Id {
		return nil, gerror.New("无权管理其他账户的Bot")
	}
	return row, nil
}
