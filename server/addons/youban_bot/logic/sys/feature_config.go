package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/model/input/sysin"
)

const featureCacheTTL = 30 * time.Second
const featureDefaultSyncTTL = 5 * time.Minute

func (s *sSysBot) AdminFeatureList(ctx context.Context, in *sysin.FeatureListInp) (list []*sysin.FeatureModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.FeatureListInp{}
	}
	if err = s.syncFeatureDefaults(ctx); err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(featureTable).Safe().Ctx(ctx).WhereNull("deleted_at")
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(feature_key LIKE ? OR name LIKE ? OR command LIKE ? OR description LIKE ?)", like, like, like, like)
	}
	var rows []*botFeatureRow
	if err = mod.Page(in.Page, in.PerPage).OrderAsc("sort").OrderAsc("id").ScanAndCount(&rows, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取Bot插件列表失败")
	}
	list = make([]*sysin.FeatureModel, 0, len(rows))
	for _, row := range rows {
		configJson := normalizeFeatureConfigJson(row.FeatureKey, row.ConfigJson)
		item := &sysin.FeatureModel{Id: row.Id, FeatureKey: row.FeatureKey, Name: row.Name, Command: row.Command, Description: row.Description, ConfigJson: configJson, Sort: row.Sort, Status: row.Status, ConfigSchema: featureConfigSchema(row.FeatureKey), ConfigValues: featureConfigValues(configJson)}
		if row.CreatedAt != nil {
			item.CreatedAt = row.CreatedAt.String()
		}
		if row.UpdatedAt != nil {
			item.UpdatedAt = row.UpdatedAt.String()
		}
		list = append(list, item)
	}
	return
}

func (s *sSysBot) AdminFeatureSave(ctx context.Context, in *sysin.FeatureSaveInp) (err error) {
	if in == nil {
		return gerror.New("插件配置不能为空")
	}
	if err = s.syncFeatureDefaults(ctx); err != nil {
		return err
	}
	featureKey := strings.TrimSpace(in.FeatureKey)
	if featureKey == "" {
		return gerror.New("插件Key不能为空")
	}
	in.ConfigJson = normalizeFeatureConfigJson(in.FeatureKey, in.ConfigJson)
	if strings.TrimSpace(in.ConfigJson) != "" && !json.Valid([]byte(strings.TrimSpace(in.ConfigJson))) {
		return gerror.New("配置JSON格式不正确")
	}
	if featureKey == (bindFeature{}).Key() {
		config := featureConfigValues(in.ConfigJson)
		config["successText"] = sanitizeTelegramHTML(fmt.Sprintf("%v", config["successText"]))
		buttons, validateErr := normalizeTelegramURLButtons(config["successButtons"])
		if validateErr != nil {
			return validateErr
		}
		config["successButtons"] = buttons
		data, _ := json.Marshal(config)
		in.ConfigJson = string(data)
	}
	if featureKey == (startFeature{}).Key() {
		config := featureConfigValues(in.ConfigJson)
		config["replyText"] = sanitizeTelegramHTML(fmt.Sprintf("%v", config["replyText"]))
		if strings.TrimSpace(fmt.Sprintf("%v", config["welcomeImage"])) != "" && telegramHTMLTextLength(fmt.Sprintf("%v", config["replyText"])) > telegramPhotoCaptionMaxLength {
			return gerror.Newf("配置欢迎图片时，欢迎文案不能超过%d个字符", telegramPhotoCaptionMaxLength)
		}
		data, _ := json.Marshal(config)
		in.ConfigJson = string(data)
	}
	if featureKey == inlinePromotionFeatureKey {
		config := featureConfigValues(in.ConfigJson)
		config["messageText"] = sanitizeTelegramHTML(fmt.Sprintf("%v", config["messageText"]))
		config["startParameter"] = normalizeInlinePromotionStartParameter(fmt.Sprintf("%v", config["startParameter"]))
		config["targetFeatureKey"] = normalizeInlinePromotionTargetFeatureKey(fmt.Sprintf("%v", config["targetFeatureKey"]))
		config["targetCommandArgs"] = strings.TrimSpace(fmt.Sprintf("%v", config["targetCommandArgs"]))
		if err = s.normalizeInlinePromotionImageConfig(ctx, config); err != nil {
			return err
		}
		data, _ := json.Marshal(config)
		in.ConfigJson = string(data)
	}
	status := in.Status
	if status == 0 {
		status = 1
	}
	data := g.Map{"name": strings.TrimSpace(in.Name), "command": strings.TrimPrefix(strings.TrimSpace(in.Command), "/"), "description": strings.TrimSpace(in.Description), "config_json": strings.TrimSpace(in.ConfigJson), "sort": in.Sort, "status": status, "updated_at": gtime.Now()}
	if in.Id > 0 {
		_, err = g.DB().Model(featureTable).Safe().Ctx(ctx).Where("id", in.Id).Where("feature_key", featureKey).Data(data).Update()
	} else {
		data["feature_key"] = featureKey
		data["created_at"] = gtime.Now()
		_, err = g.DB().Model(featureTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存Bot插件失败")
	}
	s.clearFeatureCache()
	if featureKey == inlinePromotionFeatureKey {
		config := featureConfigValues(in.ConfigJson)
		g.Log().Infof(ctx, "Inline宣传配置已保存 title:%s messageLength:%d target:%s", inlinePromotionConfigString(config, "title"), len(inlinePromotionConfigString(config, "messageText")), inlinePromotionConfigString(config, "targetFeatureKey"))
	} else {
		g.Log().Infof(ctx, "Bot插件配置已刷新 featureKey:%s", featureKey)
	}
	if err = s.syncAllTelegramBotMenus(ctx); err != nil {
		g.Log().Warningf(ctx, "同步Telegram菜单失败 err:%+v", err)
		return gerror.Wrap(err, "插件已保存，但同步机器人菜单失败")
	}
	return nil
}

func (s *sSysBot) syncFeatureDefaults(ctx context.Context) error {
	if s.featureDefaultsSynced() {
		return nil
	}
	changed := false
	for index, feature := range botFeatures {
		if feature == nil || strings.TrimSpace(feature.Key()) == "" {
			continue
		}
		count, err := g.DB().Model(featureTable).Safe().Ctx(ctx).Where("feature_key", feature.Key()).WhereNull("deleted_at").Count()
		if err != nil {
			return gerror.Wrap(err, "同步Bot插件默认配置失败")
		}
		if count > 0 {
			continue
		}
		_, err = g.DB().Model(featureTable).Safe().Ctx(ctx).Data(g.Map{"feature_key": feature.Key(), "name": feature.Description(), "command": strings.TrimPrefix(feature.Command(), "/"), "description": feature.Description(), "config_json": normalizeFeatureConfigJson(feature.Key(), ""), "sort": (index + 1) * 10, "status": 1, "created_at": gtime.Now(), "updated_at": gtime.Now()}).Insert()
		if err != nil {
			return gerror.Wrap(err, "写入Bot插件默认配置失败")
		}
		changed = true
	}
	s.markFeatureDefaultsSynced(changed)
	return nil
}

func (s *sSysBot) featureDefaultsSynced() bool {
	s.featureMu.RLock()
	defer s.featureMu.RUnlock()
	return !s.featureDefaultsAt.IsZero() && time.Since(s.featureDefaultsAt) < featureDefaultSyncTTL
}

func (s *sSysBot) markFeatureDefaultsSynced(changed bool) {
	s.featureMu.Lock()
	defer s.featureMu.Unlock()
	s.featureDefaultsAt = time.Now()
	if changed {
		s.features = nil
		s.featureAt = time.Time{}
	}
}

func (s *sSysBot) featureConfig(ctx context.Context, key string) (*botFeatureRow, bool) {
	configs, err := s.featureConfigs(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取Bot插件配置失败 err:%+v", err)
		return nil, true
	}
	row, ok := configs[strings.TrimSpace(key)]
	if !ok || row == nil {
		return nil, true
	}
	return row, row.Status == 1
}

func (s *sSysBot) featureConfigs(ctx context.Context) (map[string]*botFeatureRow, error) {
	if err := s.syncFeatureDefaults(ctx); err != nil {
		return nil, err
	}
	s.featureMu.RLock()
	if s.features != nil && time.Since(s.featureAt) < featureCacheTTL {
		defer s.featureMu.RUnlock()
		return s.features, nil
	}
	s.featureMu.RUnlock()
	s.featureMu.Lock()
	defer s.featureMu.Unlock()
	if s.features != nil && time.Since(s.featureAt) < featureCacheTTL {
		return s.features, nil
	}
	var rows []*botFeatureRow
	if err := g.DB().Model(featureTable).Safe().Ctx(ctx).WhereNull("deleted_at").OrderAsc("sort").OrderAsc("id").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取Bot插件配置失败")
	}
	configs := make(map[string]*botFeatureRow, len(rows))
	for _, row := range rows {
		if row != nil && strings.TrimSpace(row.FeatureKey) != "" {
			configs[row.FeatureKey] = row
		}
	}
	s.features = configs
	s.featureAt = time.Now()
	return configs, nil
}

func (s *sSysBot) clearFeatureCache() {
	s.featureMu.Lock()
	defer s.featureMu.Unlock()
	s.features = nil
	s.featureAt = time.Time{}
}

func (s *sSysBot) dispatchFeature(ctx context.Context, botId int64, msg *models.Message, text string) (bool, error) {
	command, args := botCommandAndArgs(text)
	labelText := strings.TrimSpace(text)
	for _, feature := range botFeatures {
		if feature == nil {
			continue
		}
		row, enabled := s.featureConfig(ctx, feature.Key())
		if !enabled {
			continue
		}
		if command != "" && s.featureCommandMatches(ctx, feature, command) {
			return feature.Handle(ctx, s, &botFeatureContext{BotId: botId, Msg: msg, Text: text, Args: args})
		}
		if command == "" && s.matchFeatureLabel(ctx, row, feature, labelText) {
			return feature.Handle(ctx, s, &botFeatureContext{BotId: botId, Msg: msg, Text: text, Args: ""})
		}
		if command == "" {
			if matcher, ok := feature.(botFeatureTextMatcher); ok && matcher.Match(ctx, s, row, labelText) {
				return feature.Handle(ctx, s, &botFeatureContext{BotId: botId, Msg: msg, Text: text, Args: ""})
			}
		}
	}
	return false, nil
}

func (s *sSysBot) featureCommandMatches(ctx context.Context, feature botFeature, command string) bool {
	if feature == nil || strings.TrimSpace(command) == "" {
		return false
	}
	return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(command), "/"), strings.TrimPrefix(s.featureCommand(ctx, feature), "/"))
}

func (s *sSysBot) matchFeatureLabel(ctx context.Context, row *botFeatureRow, feature botFeature, text string) bool {
	name := ""
	if row != nil {
		name = row.Name
	}
	return featureLabelMatches(text, s.replyKeyboardLabel(ctx, feature), name, s.featureCommand(ctx, feature))
}

func featureLabelMatches(text string, keyboardLabel string, featureName string, command string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if strings.EqualFold(text, strings.TrimSpace(keyboardLabel)) || strings.EqualFold(text, strings.TrimSpace(featureName)) {
		return true
	}
	return strings.EqualFold(strings.TrimPrefix(text, "/"), strings.TrimPrefix(strings.TrimSpace(command), "/"))
}

func botCommandAndArgs(text string) (command string, args string) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", ""
	}
	first := strings.TrimSpace(fields[0])
	if !strings.HasPrefix(first, "/") {
		return "", ""
	}
	if at := strings.Index(first, "@"); at > 0 {
		first = first[:at]
	}
	command = strings.ToLower(strings.TrimPrefix(first, "/"))
	if len(fields) > 1 {
		args = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), fields[0]))
	}
	return
}

func (s *sSysBot) featureCommand(ctx context.Context, feature botFeature) string {
	if row, _ := s.featureConfig(ctx, feature.Key()); row != nil && strings.TrimSpace(row.Command) != "" {
		return strings.TrimPrefix(strings.TrimSpace(row.Command), "/")
	}
	return strings.TrimPrefix(strings.TrimSpace(feature.Command()), "/")
}

func (s *sSysBot) featureDescription(ctx context.Context, feature botFeature) string {
	if row, _ := s.featureConfig(ctx, feature.Key()); row != nil && strings.TrimSpace(row.Description) != "" {
		return strings.TrimSpace(row.Description)
	}
	return strings.TrimSpace(feature.Description())
}

func (s *sSysBot) syncAllTelegramBotMenus(ctx context.Context) error {
	bots, err := s.enabledBots(ctx)
	if err != nil {
		return err
	}
	errorsList := make([]error, 0)
	for _, item := range bots {
		if item != nil && strings.TrimSpace(item.BotToken) != "" {
			if err = s.syncTelegramBotMenu(ctx, item.BotToken); err != nil {
				g.Log().Warningf(ctx, "同步Telegram菜单失败 botId:%d username:%s err:%+v", item.Id, item.BotUsername, err)
				errorsList = append(errorsList, fmt.Errorf("bot %s: %w", firstNonEmpty(item.BotUsername, fmt.Sprintf("%d", item.Id)), err))
				continue
			}
			g.Log().Infof(ctx, "同步Telegram菜单成功 botId:%d username:%s", item.Id, item.BotUsername)
		}
	}
	return errors.Join(errorsList...)
}

func (s *sSysBot) syncTelegramBotMenu(ctx context.Context, botToken string) error {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	commands := make([]models.BotCommand, 0, len(botFeatures))
	for _, feature := range botFeatures {
		if feature == nil {
			continue
		}
		if !s.featureMenuVisible(ctx, feature.Key()) {
			continue
		}
		cmd := strings.TrimPrefix(s.featureCommand(ctx, feature), "/")
		if cmd == "" {
			continue
		}
		commands = append(commands, models.BotCommand{Command: cmd, Description: s.featureDescription(ctx, feature)})
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	if len(commands) == 0 {
		_, err = bot.DeleteMyCommands(callCtx, &tgbot.DeleteMyCommandsParams{Scope: &models.BotCommandScopeDefault{}})
		return err
	}
	if _, err = bot.SetMyCommands(callCtx, &tgbot.SetMyCommandsParams{Commands: commands, Scope: &models.BotCommandScopeDefault{}}); err != nil {
		return err
	}
	_, err = bot.SetChatMenuButton(callCtx, &tgbot.SetChatMenuButtonParams{MenuButton: &models.MenuButtonCommands{Type: models.MenuButtonTypeCommands}})
	return err
}

func featureMenuSchema() *sysin.FeatureConfigSchema {
	return &sysin.FeatureConfigSchema{Field: "menuVisible", Label: "菜单可见", Component: "switch", Default: 1, Placeholder: "关闭后不在机器人底部菜单显示"}
}

func appendFeatureMenuSchema(list []*sysin.FeatureConfigSchema) []*sysin.FeatureConfigSchema {
	for _, item := range list {
		if item != nil && item.Field == "menuVisible" {
			return list
		}
	}
	return append([]*sysin.FeatureConfigSchema{featureMenuSchema()}, list...)
}

func featureConfigSchema(key string) []*sysin.FeatureConfigSchema {
	for _, feature := range botFeatures {
		if feature != nil && feature.Key() == key {
			if provider, ok := feature.(botFeatureConfigProvider); ok {
				return appendFeatureMenuSchema(provider.ConfigSchema())
			}
		}
	}
	return appendFeatureMenuSchema([]*sysin.FeatureConfigSchema{})
}

func featureConfigValues(configJson string) map[string]interface{} {
	values := map[string]interface{}{}
	if strings.TrimSpace(configJson) == "" {
		return values
	}
	_ = json.Unmarshal([]byte(configJson), &values)
	return values
}

func normalizeFeatureConfigJson(key string, configJson string) string {
	values := featureConfigValues(configJson)
	for _, field := range featureConfigSchema(key) {
		if field == nil || strings.TrimSpace(field.Field) == "" {
			continue
		}
		if _, ok := values[field.Field]; !ok {
			values[field.Field] = field.Default
		}
	}
	if len(values) == 0 {
		return "{}"
	}
	bs, _ := json.Marshal(values)
	return string(bs)
}

func (s *sSysBot) featureConfigValue(ctx context.Context, key string, field string) string {
	row, _ := s.featureConfig(ctx, key)
	values := map[string]interface{}{}
	if row != nil {
		values = featureConfigValues(row.ConfigJson)
	}
	if value, ok := values[field]; ok && value != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
	for _, item := range featureConfigSchema(key) {
		if item != nil && item.Field == field && item.Default != nil {
			return strings.TrimSpace(fmt.Sprintf("%v", item.Default))
		}
	}
	return ""
}

func (s *sSysBot) featureMenuVisible(ctx context.Context, key string) bool {
	row, enabled := s.featureConfig(ctx, key)
	if !enabled {
		return false
	}
	if row == nil {
		return true
	}
	values := featureConfigValues(row.ConfigJson)
	value, ok := values["menuVisible"]
	if !ok {
		return true
	}
	return configBool(value)
}
