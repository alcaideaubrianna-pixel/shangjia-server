package sys

import (
	"context"
	"encoding/json"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_bot/model/input/sysin"
)

type replyKeyboardConfigButton struct {
	FeatureKey string `json:"featureKey"`
	Style      string `json:"style,omitempty"`
}

func replyKeyboardConfigSchema() []*sysin.FeatureConfigSchema {
	options := make([]*sysin.FeatureConfigOption, 0, len(botFeatures))
	for _, feature := range botFeatures {
		if feature.Key() == "two_way_relay" || feature.Key() == "notify" || feature.Key() == "super_notify" {
			continue
		}
		options = append(options, &sysin.FeatureConfigOption{Label: feature.Description(), Value: feature.Key()})
	}
	styles := []*sysin.FeatureConfigOption{{Label: "默认", Value: ""}, {Label: "蓝色", Value: "primary"}, {Label: "绿色", Value: "success"}, {Label: "红色", Value: "danger"}}
	return []*sysin.FeatureConfigSchema{
		{Field: "keyboardUnbound", Label: "绑定前菜单", Component: "reply_keyboard", Default: nil, Options: options, StyleOptions: styles},
		{Field: "keyboardBound", Label: "绑定后菜单", Component: "reply_keyboard", Default: nil, Options: options, StyleOptions: styles},
	}
}

func parseReplyKeyboardLayout(value interface{}) ([][]replyKeyboardConfigButton, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var rows [][]replyKeyboardConfigButton
	if err = json.Unmarshal(data, &rows); err != nil {
		return nil, gerror.New("底部菜单必须是二维按钮数组")
	}
	if len(rows) > 12 {
		return nil, gerror.New("底部菜单最多12行")
	}
	known := map[string]bool{}
	for _, f := range botFeatures {
		known[f.Key()] = true
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if len(row) == 0 || len(row) > 4 {
			return nil, gerror.New("底部菜单每行需要1至4个按钮")
		}
		for _, button := range row {
			if !known[button.FeatureKey] || button.FeatureKey == "notify" || button.FeatureKey == "super_notify" || button.FeatureKey == "two_way_relay" {
				return nil, gerror.New("底部菜单包含无效插件")
			}
			if seen[button.FeatureKey] {
				return nil, gerror.New("底部菜单不能重复添加同一插件")
			}
			seen[button.FeatureKey] = true
			switch button.Style {
			case "", "primary", "success", "danger":
			default:
				return nil, gerror.New("按钮颜色仅支持默认、蓝色、绿色、红色")
			}
		}
	}
	return rows, nil
}

func validateReplyKeyboardConfig(config map[string]interface{}) error {
	for _, key := range []string{"keyboardUnbound", "keyboardBound"} {
		if _, err := parseReplyKeyboardLayout(config[key]); err != nil {
			return err
		}
	}
	return nil
}

func configuredKeyboardRows(items []replyKeyboardItem, layout [][]replyKeyboardConfigButton) [][]models.KeyboardButton {
	if layout == nil {
		return groupedReplyKeyboardRows(items)
	}
	allowed := map[string]models.KeyboardButton{}
	for _, item := range items {
		allowed[item.Key] = item.Button
	}
	rows := make([][]models.KeyboardButton, 0, len(layout))
	for _, row := range layout {
		var buttons []models.KeyboardButton
		for _, entry := range row {
			if button, ok := allowed[entry.FeatureKey]; ok {
				button.Style = entry.Style
				buttons = append(buttons, button)
			}
		}
		if len(buttons) > 0 {
			rows = append(rows, buttons)
		}
	}
	return rows
}

func (s *sSysBot) configuredReplyKeyboardRows(ctx context.Context, items []replyKeyboardItem) [][]models.KeyboardButton {
	row, _ := s.featureConfig(ctx, "start")
	if row == nil {
		return groupedReplyKeyboardRows(items)
	}
	config := featureConfigValues(row.ConfigJson)
	if config["keyboardBound"] == nil && config["keyboardUnbound"] == nil {
		return groupedReplyKeyboardRows(items)
	}
	key := "keyboardUnbound"
	userID := telegramUserIdFromCtx(ctx)
	if userID != "" {
		// Match the publishing account binding used by feature visibility checks.
		binding, err := s.bindingByTelegram(ctx, sysin.BotAppApi, userID)
		if err != nil {
			return groupedReplyKeyboardRows(items)
		}
		if binding != nil && binding.AccountId > 0 {
			key = "keyboardBound"
		}
	}
	layout, err := parseReplyKeyboardLayout(config[key])
	if err != nil {
		return groupedReplyKeyboardRows(items)
	}
	return configuredKeyboardRows(items, layout)
}
