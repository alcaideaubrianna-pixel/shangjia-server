package sys

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/internal/library/contexts"
	"hotgo/utility/botregistry"
)

func (s *sSysBot) AdminBotList(ctx context.Context, in *sysin.BotListInp) (list []*sysin.BotModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.BotListInp{}
	}
	mod := g.DB().Model(botTable).Safe().Ctx(ctx).WhereNull("deleted_at").Where("bot_type", "main")
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	if in.IsOfficial == 1 {
		mod = mod.Where("is_official", 1)
	}
	if in.IsOfficial == 2 {
		mod = mod.Where("is_official", 0)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(bot_name LIKE ? OR bot_username LIKE ? OR remark LIKE ?)", like, like, like)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取Bot列表失败")
	}
	if list == nil {
		list = []*sysin.BotModel{}
	}
	for _, item := range list {
		if item != nil && isSystemErrorRemark(item.Remark) {
			item.Remark = ""
		}
	}
	return
}

func (s *sSysBot) AdminBotSave(ctx context.Context, in *sysin.BotSaveInp) (err error) {
	if in == nil {
		return gerror.New("Bot配置不能为空")
	}
	var old *sysin.BotModel
	if in.Id > 0 {
		old, err = s.botById(ctx, in.Id)
		if err != nil {
			return err
		}
	}
	if old != nil && old.BotType != "main" {
		return gerror.New("请从所属账户后台管理该Bot")
	}
	if in.Id > 0 && in.IsDefault == 1 {
		cfg, readErr := relayConfig(ctx, in.Id)
		if readErr == nil && cfg != nil {
			return gerror.New("该Bot已配置为双向机器人，不能设置为默认主机器人")
		}
	}
	if in.Id > 0 {
		cfg, readErr := relayConfig(ctx, in.Id)
		if readErr == nil && relayOwned(cfg) {
			return gerror.New("租户账户的双向Bot请从所属账户的双向机器人管理中操作")
		}
	}
	botToken := strings.TrimSpace(in.BotToken)
	if botToken == "" && old != nil {
		botToken = old.BotToken
	}
	if botToken == "" {
		return gerror.New("Bot Token不能为空")
	}
	var tgUser *models.User
	needValidateToken := old == nil || strings.TrimSpace(in.BotToken) != "" && strings.TrimSpace(in.BotToken) != strings.TrimSpace(old.BotToken)
	if needValidateToken {
		tgUser, err = s.telegramBotProfile(ctx, botToken)
		if err != nil {
			return gerror.Wrap(err, "校验Bot Token失败")
		}
	}
	status := in.Status
	if status == 0 {
		status = 1
	}
	if status != 1 && status != 2 {
		return gerror.New("Bot状态不合法")
	}
	botName := strings.TrimSpace(in.BotName)
	botUsername := ""
	if tgUser != nil {
		botUsername = strings.TrimPrefix(strings.TrimSpace(tgUser.Username), "@")
		if botName == "" {
			botName = telegramBotDisplayName(tgUser)
		}
	} else if old != nil {
		botUsername = old.BotUsername
		if botName == "" {
			botName = old.BotName
		}
	}
	isOfficial := 0
	if in.IsOfficial == 1 || in.IsDefault == 1 {
		isOfficial = 1
	}
	isDefault := 0
	if in.IsDefault == 1 {
		isDefault = 1
	}
	botUserID, idErr := botregistry.TelegramID(botToken)
	if idErr != nil {
		return idErr
	}
	data := g.Map{
		"bot_user_id": botUserID, "bot_type": "main",
		"bot_name": botName, "bot_username": botUsername, "bot_token": botToken,
		"is_official": isOfficial, "is_default": isDefault, "remark": strings.TrimSpace(in.Remark),
		"run_mode": normalizeRunMode(in.RunMode), "webhook_url": strings.TrimSpace(in.WebhookUrl),
		"status": status, "updated_by": contexts.GetUserId(ctx), "updated_at": gtime.Now(),
	}
	botId := in.Id
	if in.Id > 0 {
		_, err = g.DB().Model(botTable).Safe().Ctx(ctx).Where("id", in.Id).WhereNull("deleted_at").Data(data).Update()
	} else {
		data["created_by"] = contexts.GetUserId(ctx)
		data["created_at"] = gtime.Now()
		result, insertErr := g.DB().Model(botTable).Safe().Ctx(ctx).Data(data).Insert()
		if insertErr == nil && result != nil {
			botId, _ = result.LastInsertId()
		}
		err = insertErr
	}
	if err != nil {
		return gerror.Wrap(err, "保存Bot配置失败")
	}
	if isDefault == 1 && botId > 0 {
		if _, err = g.DB().Model(botTable).Safe().Ctx(ctx).WhereNot("id", botId).WhereNull("deleted_at").Data(g.Map{"is_default": 0, "updated_at": gtime.Now()}).Update(); err != nil {
			return gerror.Wrap(err, "更新默认Bot失败")
		}
	}
	s.clearTelegramBotCache()
	s.restartRuntime(ctx)
	return nil
}

func (s *sSysBot) AdminBotDelete(ctx context.Context, in *sysin.BotDeleteInp) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	for _, id := range in.Ids {
		cfg, readErr := relayConfig(ctx, id)
		if readErr == nil && relayOwned(cfg) {
			return gerror.New("不能从全局Bot管理删除租户账户的双向Bot")
		}
	}

	_, err = g.DB().Model(botTable).Safe().Ctx(ctx).WhereIn("id", uniqueIds(in.Ids)).Data(g.Map{"deleted_by": contexts.GetUserId(ctx), "deleted_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除Bot配置失败")
	}
	s.clearTelegramBotCache()
	s.restartRuntime(ctx)
	return nil
}

func (s *sSysBot) AdminBotRefresh(ctx context.Context, in *sysin.BotRefreshInp) (list []*sysin.BotRefreshModel, err error) {
	if in == nil || len(in.Ids) == 0 {
		return nil, gerror.New("请选择要刷新的Bot")
	}
	var bots []*sysin.BotModel
	if err = g.DB().Model(botTable).Safe().Ctx(ctx).WhereIn("id", uniqueIds(in.Ids)).WhereNull("deleted_at").Scan(&bots); err != nil {
		return nil, gerror.Wrap(err, "读取Bot配置失败")
	}
	list = make([]*sysin.BotRefreshModel, 0, len(bots))
	for _, item := range bots {
		result := &sysin.BotRefreshModel{Id: item.Id, Status: item.Status}
		tgUser, refreshErr := s.telegramBotProfile(ctx, item.BotToken)
		if refreshErr != nil {
			result.Status = 2
			result.ErrorMessage = refreshErr.Error()
			_, _ = g.DB().Model(botTable).Safe().Ctx(ctx).Where("id", item.Id).Data(g.Map{"status": 2, "updated_by": contexts.GetUserId(ctx), "updated_at": gtime.Now()}).Update()
			list = append(list, result)
			continue
		}
		username := strings.TrimPrefix(strings.TrimSpace(tgUser.Username), "@")
		result.Status = 1
		result.BotUsername = username
		data := g.Map{"bot_name": telegramBotDisplayName(tgUser), "bot_username": username, "status": 1, "updated_by": contexts.GetUserId(ctx), "updated_at": gtime.Now()}
		if isSystemErrorRemark(item.Remark) {
			data["remark"] = ""
		}
		_, err = g.DB().Model(botTable).Safe().Ctx(ctx).Where("id", item.Id).Data(data).Update()
		if err != nil {
			return nil, gerror.Wrap(err, "刷新Bot状态失败")
		}
		list = append(list, result)
	}
	s.clearTelegramBotCache()
	s.restartRuntime(ctx)
	return list, nil
}

func (s *sSysBot) AdminBotRestart(ctx context.Context, in *sysin.BotRefreshInp) (list []*sysin.BotRefreshModel, err error) {
	list, err = s.AdminBotRefresh(ctx, in)
	if err != nil {
		return nil, err
	}
	s.restartRuntime(ctx)
	return list, nil
}
