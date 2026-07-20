package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

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
	columns := twdao.YoubanTwoWayBotBot.Columns()
	mod := twdao.YoubanTwoWayBotBot.Ctx(ctx).
		Where(columns.TenantId, account.TenantId).
		WhereNull(columns.DeletedAt)
	if strings.TrimSpace(in.Keyword) != "" {
		keyword := "%" + strings.TrimSpace(in.Keyword) + "%"
		mod = mod.WhereLike(columns.Name, keyword).WhereOrLike(columns.BotUsername, keyword)
	}
	if in.Status == sysin.TwoWayBotStatusEnabled || in.Status == sysin.TwoWayBotStatusDisabled {
		mod = mod.Where(columns.Status, in.Status)
	}
	var rows []*entity.YoubanTwoWayBotBot
	if err = mod.Page(in.Page, in.PerPage).OrderDesc(columns.Id).ScanAndCount(&rows, &totalCount, false); err != nil {
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
	var old *entity.YoubanTwoWayBotBot
	if in.Id > 0 {
		old, err = s.botById(ctx, in.Id, account.TenantId)
		if err != nil {
			return err
		}
	}
	token := strings.TrimSpace(in.BotToken)
	if token == "" && old != nil {
		token = old.BotToken
	}
	botUserId, botUsername, botDisplayName, err := s.validateBotToken(ctx, token)
	if err != nil {
		return err
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
		var setup *setupGroupResult
		if in.ExistingGroupId != "" {
			setup, err = s.setupExistingManagementGroup(ctx, account, tgAccount, in.ExistingGroupId, botUsername)
		} else {
			setup, err = s.createManagementGroup(ctx, account, tgAccount, botUsername)
		}
		if err != nil {
			return err
		}
		supergroupId = setup.SupergroupId
		supergroupAccessHash = fmt.Sprintf("%d", setup.AccessHash)
		supergroupTitle = setup.Title
		inviteLink = setup.InviteLink
	} else if old != nil {
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
	data := g.Map{
		"tenant_id":              account.TenantId,
		"account_id":             account.Id,
		"tg_account_id":          in.TgAccountId,
		"name":                   name,
		"bot_token":              token,
		"bot_user_id":            botUserId,
		"bot_username":           botUsername,
		"supergroup_id":          supergroupId,
		"supergroup_access_hash": supergroupAccessHash,
		"supergroup_title":       supergroupTitle,
		"invite_link":            inviteLink,
		"setup_status":           setupStatus,
		"last_setup_at":          now,
		"status":                 in.Status,
		"error_message":          "",
		"updated_at":             now,
	}
	if in.Id > 0 {
		if _, err = twdao.YoubanTwoWayBotBot.Ctx(ctx).WherePri(in.Id).Where("tenant_id", account.TenantId).Data(data).Update(); err != nil {
			return gerror.Wrap(err, "更新双向机器人失败")
		}
	} else {
		data["created_at"] = now
		if in.Id, err = twdao.YoubanTwoWayBotBot.Ctx(ctx).Data(data).InsertAndGetId(); err != nil {
			return gerror.Wrap(err, "创建双向机器人失败")
		}
	}
	if supergroupId != "" {
		return s.syncRuntimeById(ctx, in.Id, account.TenantId)
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
	_, err = twdao.YoubanTwoWayBotBot.Ctx(ctx).
		WhereIn("id", in.Ids).
		Where("tenant_id", account.TenantId).
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
	return s.refreshWebhookById(ctx, in.Id, account.TenantId)
}

func (s *sSysTwoWayBot) AdminBotSetup(ctx context.Context, in *sysin.BotActionInp) error {
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	row, err := s.botById(ctx, in.Id, account.TenantId)
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
	mode, conf, err := s.twoWayBotRuntimeMode(ctx)
	if err != nil {
		return err
	}
	if mode == "pull" || mode == "polling" {
		return s.enablePollingForBot(ctx, row)
	}
	return s.enableWebhookForBot(ctx, row, conf)
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

func (s *sSysTwoWayBot) botById(ctx context.Context, id int64, tenantId int64) (*entity.YoubanTwoWayBotBot, error) {
	if id <= 0 {
		return nil, gerror.New("请选择机器人")
	}
	columns := twdao.YoubanTwoWayBotBot.Columns()
	var row *entity.YoubanTwoWayBotBot
	err := twdao.YoubanTwoWayBotBot.Ctx(ctx).
		Where(columns.Id, id).
		Where(columns.TenantId, tenantId).
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

func loadTgAccountNames(ctx context.Context, rows []*entity.YoubanTwoWayBotBot) (map[int64]string, error) {
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

func botModel(row *entity.YoubanTwoWayBotBot, tgAccountName string) *sysin.BotModel {
	if row == nil {
		return nil
	}
	return &sysin.BotModel{
		Id:                   row.Id,
		TenantId:             row.TenantId,
		AccountId:            row.AccountId,
		TgAccountId:          row.TgAccountId,
		TgAccountName:        strings.TrimSpace(tgAccountName),
		Name:                 row.Name,
		BotUserId:            row.BotUserId,
		BotUsername:          row.BotUsername,
		SupergroupId:         row.SupergroupId,
		SupergroupAccessHash: row.SupergroupAccessHash,
		SupergroupTitle:      row.SupergroupTitle,
		InviteLink:           row.InviteLink,
		SetupStatus:          row.SetupStatus,
		WebhookStatus:        row.WebhookStatus,
		Status:               row.Status,
		ErrorMessage:         row.ErrorMessage,
		LastSetupAt:          row.LastSetupAt,
		LastWebhookAt:        row.LastWebhookAt,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}
