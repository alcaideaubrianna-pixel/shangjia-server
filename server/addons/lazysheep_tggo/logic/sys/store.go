// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/model"
	lsysin "hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/model/entity"
)

func (s *sLazySheepTGGo) loadState(ctx context.Context) (res *model.State, err error) {
	res = model.NewState()
	res.Normalize()

	var bots []*entity.AddonLazysheepTggoBot
	if err = dao.AddonLazysheepTggoBot.Ctx(ctx).OrderAsc(dao.AddonLazysheepTggoBot.Columns().Id).Scan(&bots); err != nil {
		return nil, gerror.Wrap(err, "获取机器人配置失败")
	}
	for _, row := range bots {
		if row == nil {
			continue
		}
		key := row.BotKey
		if key == "" {
			key = fmt.Sprintf("%d", row.Id)
		}
		res.Bots[key] = &model.BotConfig{
			Id:            row.Id,
			Key:           key,
			Role:          row.Role,
			MemberId:      row.MemberId,
			Token:         row.Token,
			DisplayName:   row.BotName,
			Username:      row.Username,
			WebhookSecret: row.WebhookSecret,
			WebhookPath:   row.WebhookPath,
			Enabled:       row.Enabled > 0,
			AutoPull:      row.AutoPull > 0,
			AutoForward:   row.AutoForward > 0,
			ReviewEnabled: row.ReviewEnabled > 0,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
		}
		g.Log().Debugf(ctx, "读取机器人配置 botKey:%s role:%s enabled:%t", key, row.Role, row.Enabled > 0)
		s.fillBotRuntimeStatus(key, res.Bots[key])
		res.Settings = &model.Settings{
			AllowVerify:   row.AllowVerify > 0,
			AllowLocation: row.AllowLocation > 0,
			MemberVerify:  memberVerifyLabel(row.MemberVerify),
			MemberPoints:  fmt.Sprintf("%d", row.MemberPoints),
			SignFollow:    row.SignFollow > 0,
		}
	}

	var users []*entity.AddonLazysheepTggoUser
	if err = dao.AddonLazysheepTggoUser.Ctx(ctx).OrderAsc(dao.AddonLazysheepTggoUser.Columns().Id).Scan(&users); err != nil {
		return nil, gerror.Wrap(err, "获取 Telegram 用户失败")
	}
	for _, row := range users {
		if row == nil {
			continue
		}
		res.Users[int64(row.TelegramId)] = &model.UserRecord{
			TelegramID:   int64(row.TelegramId),
			BotKey:       "",
			Username:     row.Username,
			FirstName:    row.FirstName,
			LastName:     row.LastName,
			LanguageCode: row.LanguageCode,
			IsBot:        row.IsBot > 0,
			MemberLevel:  row.MemberLevel,
			Points:       row.Points,
			Status:       row.Status,
			LastActiveAt: row.LastActiveAt,
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		}
	}

	var bindings []*entity.AddonLazysheepTggoBinding
	if err = dao.AddonLazysheepTggoBinding.Ctx(ctx).OrderAsc(dao.AddonLazysheepTggoBinding.Columns().Id).Scan(&bindings); err != nil {
		return nil, gerror.Wrap(err, "获取绑定关系失败")
	}
	for _, row := range bindings {
		if row == nil {
			continue
		}
		key := row.BindingKey
		if key == "" {
			key = fmt.Sprintf("%d:%d:%d:%s", row.BotId, row.ReviewChatId, row.PublishChatId, row.SourceUrl)
		}
		res.Bindings[key] = &model.BindingRecord{
			ID:              row.Id,
			Key:             key,
			BotKey:          row.BotKey,
			SourceURL:       row.SourceUrl,
			SourceToken:     row.SourceToken,
			ReviewChatID:    int64(row.ReviewChatId),
			PublishChatID:   int64(row.PublishChatId),
			LastPullID:      row.LastPullId,
			LastCursor:      row.LastCursor,
			Status:          statusLabel(row.Status),
			AutoPush:        row.AutoPush > 0,
			VerifyEnabled:   row.VerifyEnabled > 0,
			LocationEnabled: row.LocationEnabled > 0,
			PluginState:     decodePluginState(row.PluginSettings),
			CreatedAt:       row.CreatedAt,
			UpdatedAt:       row.UpdatedAt,
		}
	}
	if err = s.loadPlugins(ctx, res); err != nil {
		return nil, err
	}
	if err = s.loadGlobal(ctx, res); err != nil {
		return nil, err
	}
	if err = s.loadBotPlugins(ctx, res); err != nil {
		return nil, err
	}

	res.Normalize()
	return
}

func (s *sLazySheepTGGo) saveState(ctx context.Context, state *model.State) error {
	if state == nil {
		state = model.NewState()
	}
	state.Normalize()

	if err := dao.AddonLazysheepTggoBot.Transaction(ctx, func(ctx context.Context, tx gdb.TX) (err error) {
		for key, item := range state.Bots {
			if item == nil {
				continue
			}
			if err = s.upsertBot(ctx, key, item, state.Settings); err != nil {
				return err
			}
		}
		for key, item := range state.Users {
			if item == nil {
				continue
			}
			if err = s.upsertUser(ctx, key, item); err != nil {
				return err
			}
		}
		for key, item := range state.Bindings {
			if item == nil {
				continue
			}
			if err = s.upsertBinding(ctx, key, item); err != nil {
				return err
			}
		}
		if err = s.savePlugins(ctx, state.Plugins); err != nil {
			return err
		}
		if err = s.saveGlobal(ctx, state.Global); err != nil {
			return err
		}
		if err = s.saveBotPlugins(ctx, state.Bots); err != nil {
			return err
		}
		return
	}); err != nil {
		return err
	}
	return s.syncWebhooksAfterSave(ctx, state)
}

func (s *sLazySheepTGGo) upsertBot(ctx context.Context, key string, item *model.BotConfig, settings *model.Settings) error {
	s.normalizeBotConfig(key, item)
	g.Log().Debugf(ctx, "落库机器人配置 botKey:%s role:%s enabled:%t username:%s", key, item.Role, item.Enabled, item.Username)

	allowVerify := true
	allowLocation := true
	memberVerify := 0
	memberPoints := 0
	signFollow := false
	if settings != nil {
		allowVerify = settings.AllowVerify
		allowLocation = settings.AllowLocation
		memberVerify = memberVerifyValue(settings.MemberVerify)
		memberPoints, _ = strconv.Atoi(settings.MemberPoints)
		signFollow = settings.SignFollow
	}

	cols := dao.AddonLazysheepTggoBot.Columns()
	row := g.Map{
		cols.BotKey:        key,
		cols.Role:          item.Role,
		cols.MemberId:      item.MemberId,
		cols.Token:         item.Token,
		cols.BotName:       item.DisplayName,
		cols.Username:      item.Username,
		cols.WebhookSecret: item.WebhookSecret,
		cols.WebhookPath:   item.WebhookPath,
		cols.Enabled:       boolToInt(item.Enabled),
		cols.AutoPull:      boolToInt(item.AutoPull),
		cols.AutoForward:   boolToInt(item.AutoForward),
		cols.ReviewEnabled: boolToInt(item.ReviewEnabled),
		cols.AllowVerify:   boolToInt(allowVerify),
		cols.AllowLocation: boolToInt(allowLocation),
		cols.MemberVerify:  memberVerify,
		cols.MemberPoints:  memberPoints,
		cols.SignFollow:    boolToInt(signFollow),
		cols.Status:        1,
		cols.UpdatedAt:     gtime.Now(),
	}
	if item.CreatedAt != nil {
		row[cols.CreatedAt] = item.CreatedAt
	} else {
		row[cols.CreatedAt] = gtime.Now()
	}
	mod := dao.AddonLazysheepTggoBot.DB().Model(dao.AddonLazysheepTggoBot.Table()).Ctx(ctx)
	_, err := upsertByKey(ctx, mod, cols.BotKey, key, row)
	return err
}

func (s *sLazySheepTGGo) deleteBot(ctx context.Context, in *lsysin.BotDeleteInp) error {
	if in == nil || in.Key == "" {
		return gerror.New("机器人标识不能为空")
	}
	cols := dao.AddonLazysheepTggoBot.Columns()
	bindingCols := dao.AddonLazysheepTggoBinding.Columns()
	var target *entity.AddonLazysheepTggoBot
	query := dao.AddonLazysheepTggoBot.Ctx(ctx).Where(cols.BotKey, in.Key)
	if id, parseErr := strconv.ParseInt(strings.TrimSpace(in.Key), 10, 64); parseErr == nil && id > 0 {
		query = dao.AddonLazysheepTggoBot.Ctx(ctx).Where(cols.BotKey, in.Key).WhereOr(cols.Id, id)
	}
	if err := query.Scan(&target); err != nil {
		return gerror.Wrap(err, "查询机器人失败")
	}
	if target == nil || target.Id == 0 {
		return gerror.New("机器人不存在或已删除")
	}
	s.stopRuntimeBot(in.Key, target.BotKey)
	if strings.TrimSpace(target.Token) != "" {
		if err := s.cleanupDeletedTelegramBotState(ctx, target.BotKey, target.Token); err != nil {
			g.Log().Warningf(ctx, "同步清理 Telegram 机器人状态失败 bot:%s err:%+v", target.BotKey, err)
		}
	}
	if err := dao.AddonLazysheepTggoBot.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		bindingQuery := dao.AddonLazysheepTggoBinding.Ctx(ctx).Where(bindingCols.BotKey, target.BotKey)
		if target.Id > 0 {
			bindingQuery = bindingQuery.WhereOr(bindingCols.BotId, target.Id)
		}
		if _, deleteErr := bindingQuery.Delete(); deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除机器人绑定失败")
		}
		if _, deleteErr := dao.AddonLazysheepTggoBot.Ctx(ctx).Where(cols.Id, target.Id).Delete(); deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除机器人失败")
		}
		s.stopRuntimeBot(in.Key, target.BotKey)
		return nil
	}); err != nil {
		return err
	}
	if target.MemberId > 0 && strings.TrimSpace(target.Token) != "" {
		if err := s.sendTelegramTextRemoveKeyboard(ctx, target.Token, target.MemberId, formatBotDeleteNotice(target.BotName, target.Username)); err != nil {
			g.Log().Warningf(ctx, "删除机器人后通知创建者失败 bot:%s memberID:%d err:%+v", in.Key, target.MemberId, err)
		}
	}
	return nil
}

func (s *sLazySheepTGGo) stopRuntimeBot(keys ...string) {
	s.runtime.mu.Lock()
	defer s.runtime.mu.Unlock()
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if rt := s.runtime.runtimes[key]; rt != nil && rt.cancel != nil {
			rt.cancel()
		}
		delete(s.runtime.runtimes, key)
	}
}

func (s *sLazySheepTGGo) cleanupDeletedTelegramBotState(ctx context.Context, botKey, token string) error {
	httpClient, err := s.telegramHTTPClient(ctx)
	if err != nil {
		return err
	}
	client, err := bot.New(strings.TrimSpace(token), bot.WithHTTPClient(telegramHTTPTimeout-time.Second, httpClient))
	if err != nil {
		return err
	}
	if _, err = client.DeleteWebhook(ctx, &bot.DeleteWebhookParams{DropPendingUpdates: true}); err != nil {
		g.Log().Warningf(ctx, "删除 Telegram webhook 失败 bot:%s err:%+v", botKey, err)
	}
	scopes := []models.BotCommandScope{
		nil,
		&models.BotCommandScopeDefault{},
		&models.BotCommandScopeAllPrivateChats{},
		&models.BotCommandScopeAllGroupChats{},
		&models.BotCommandScopeAllChatAdministrators{},
	}
	for _, scope := range scopes {
		if _, err = client.DeleteMyCommands(ctx, &bot.DeleteMyCommandsParams{Scope: scope}); err != nil {
			g.Log().Warningf(ctx, "清理 Telegram 命令菜单失败 bot:%s err:%+v", botKey, err)
		}
	}
	if _, err = client.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
		MenuButton: &models.MenuButtonDefault{Type: models.MenuButtonTypeDefault},
	}); err != nil {
		g.Log().Warningf(ctx, "恢复 Telegram 菜单按钮默认状态失败 bot:%s err:%+v", botKey, err)
	}
	return nil
}

func (s *sLazySheepTGGo) upsertUser(ctx context.Context, key int64, item *model.UserRecord) error {
	cols := dao.AddonLazysheepTggoUser.Columns()
	row := g.Map{
		cols.TelegramId:   key,
		"bot_key":         item.BotKey,
		cols.Username:     item.Username,
		cols.FirstName:    item.FirstName,
		cols.LastName:     item.LastName,
		cols.LanguageCode: item.LanguageCode,
		cols.IsBot:        boolToInt(item.IsBot),
		cols.LastActiveAt: gtime.Now(),
		cols.Status:       1,
		cols.UpdatedAt:    gtime.Now(),
	}
	if item.CreatedAt != nil {
		row[cols.CreatedAt] = item.CreatedAt
	} else {
		row[cols.CreatedAt] = gtime.Now()
	}
	if item.BotKey == "" {
		return nil
	}
	existing, err := dao.AddonLazysheepTggoUser.Ctx(ctx).
		Fields(cols.Id).
		Where("bot_key", item.BotKey).
		Where(cols.TelegramId, key).
		Value()
	if err != nil {
		return gerror.Wrap(err, "查询Telegram用户失败")
	}
	if existing.IsNil() {
		count, err := dao.AddonLazysheepTggoUser.Ctx(ctx).Where("bot_key", item.BotKey).Count()
		if err != nil {
			return gerror.Wrap(err, "统计Telegram用户失败")
		}
		if count == 0 {
			row[cols.MemberLevel] = 9
		}
		_, err = dao.AddonLazysheepTggoUser.Ctx(ctx).Data(row).Insert()
		return err
	}
	adminCount, err := dao.AddonLazysheepTggoUser.Ctx(ctx).
		Where("bot_key", item.BotKey).
		WhereGTE(cols.MemberLevel, 9).
		Count()
	if err != nil {
		return gerror.Wrap(err, "统计机器人管理员失败")
	}
	if adminCount == 0 {
		row[cols.MemberLevel] = 9
	}
	_, err = dao.AddonLazysheepTggoUser.Ctx(ctx).Where(cols.Id, existing.Int64()).Data(row).Update()
	return err
}

func (s *sLazySheepTGGo) upsertBinding(ctx context.Context, key string, item *model.BindingRecord) error {
	cols := dao.AddonLazysheepTggoBinding.Columns()
	botID, err := s.resolveBotID(ctx, item.BotKey)
	if err != nil {
		return err
	}
	row := g.Map{
		cols.BindingKey:      key,
		cols.BotId:           botID,
		cols.BotKey:          item.BotKey,
		cols.SourceUrl:       item.SourceURL,
		cols.SourceToken:     item.SourceToken,
		cols.ReviewChatId:    item.ReviewChatID,
		cols.PublishChatId:   item.PublishChatID,
		cols.AutoPush:        boolToInt(item.AutoPush),
		cols.ReviewEnabled:   1,
		cols.PublishEnabled:  1,
		cols.VerifyEnabled:   boolToInt(item.VerifyEnabled),
		cols.LocationEnabled: boolToInt(item.LocationEnabled),
		cols.PluginSettings:  encodePluginState(item.PluginState),
		cols.LastPullId:      item.LastPullID,
		cols.LastCursor:      item.LastCursor,
		cols.Status:          1,
		cols.UpdatedAt:       gtime.Now(),
	}
	if item.CreatedAt != nil {
		row[cols.CreatedAt] = item.CreatedAt
	} else {
		row[cols.CreatedAt] = gtime.Now()
	}
	if _, err = upsertByKey(ctx, dao.AddonLazysheepTggoBinding.Ctx(ctx), cols.BindingKey, key, row); err != nil {
		return err
	}
	if item.ReviewChatID != 0 || item.PublishChatID != 0 {
		chatIDs := make(map[int64]struct{}, 2)
		if item.ReviewChatID != 0 {
			chatIDs[item.ReviewChatID] = struct{}{}
		}
		if item.PublishChatID != 0 && item.PublishChatID != item.ReviewChatID {
			chatIDs[item.PublishChatID] = struct{}{}
		}
		for chatID := range chatIDs {
			data := g.Map{
				cols.Status:    2,
				cols.UpdatedAt: gtime.Now(),
			}
			if _, err = dao.AddonLazysheepTggoBinding.Ctx(ctx).
				Where(cols.BotKey, item.BotKey).
				WhereNot(cols.BindingKey, key).
				Where(cols.Status, 1).
				Where(cols.ReviewChatId, chatID).
				Data(data).
				Update(); err != nil {
				return gerror.Wrap(err, "清理频道审核旧绑定失败")
			}
			if _, err = dao.AddonLazysheepTggoBinding.Ctx(ctx).
				Where(cols.BotKey, item.BotKey).
				WhereNot(cols.BindingKey, key).
				Where(cols.Status, 1).
				Where(cols.PublishChatId, chatID).
				Data(data).
				Update(); err != nil {
				return gerror.Wrap(err, "清理频道发布旧绑定失败")
			}
		}
	}
	return nil
}

func encodePluginState(state map[string]any) string {
	if len(state) == 0 {
		return ""
	}
	data, err := json.Marshal(state)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodePluginState(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func (s *sLazySheepTGGo) resolveBotID(ctx context.Context, botKey string) (int, error) {
	if botKey == "" {
		return 0, nil
	}
	cols := dao.AddonLazysheepTggoBot.Columns()
	val, err := dao.AddonLazysheepTggoBot.Ctx(ctx).Fields(cols.Id).Where(cols.BotKey, botKey).Value()
	if err != nil {
		return 0, gerror.Wrap(err, "查询机器人失败")
	}
	if val.IsNil() {
		return 0, gerror.Newf("机器人不存在：%s", botKey)
	}
	return val.Int(), nil
}

func upsertByKey(ctx context.Context, mod *gdb.Model, field string, value any, row g.Map) (int64, error) {
	check := mod.Clone().Fields("id")
	existing, err := check.Where(field, value).Value()
	if err != nil {
		return 0, gerror.Wrap(err, "查询记录失败")
	}
	if !existing.IsNil() {
		if _, err = mod.Where(field, value).Data(row).Update(); err != nil {
			return 0, gerror.Wrap(err, "更新记录失败")
		}
		return existing.Int64(), nil
	}
	id, err := mod.Clone().Data(row).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "新增记录失败")
	}
	return id, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func statusLabel(v int) string {
	switch v {
	case 1:
		return "enabled"
	case 2:
		return "disabled"
	case 3:
		return "deleted"
	default:
		return "unknown"
	}
}

func memberVerifyValue(v string) int {
	switch v {
	case "member":
		return 1
	case "points":
		return 2
	default:
		return 0
	}
}

func memberVerifyLabel(v int) string {
	switch v {
	case 1:
		return "member"
	case 2:
		return "points"
	default:
		return "none"
	}
}

func (s *sLazySheepTGGo) fillBotRuntimeStatus(key string, item *model.BotConfig) {
	if item == nil {
		return
	}
	if !item.Enabled {
		item.RuntimeStatus = "disabled"
		item.RuntimeMessage = "未启用"
		return
	}
	rt := s.runtime.get(key)
	if rt == nil {
		item.RuntimeStatus = "pending"
		item.RuntimeMessage = ""
		return
	}
	if rt.status == "error" {
		item.RuntimeStatus = "error"
		item.RuntimeMessage = rt.lastError
		return
	}
	item.RuntimeStatus = "running"
	item.RuntimeMessage = "运行中"
}
