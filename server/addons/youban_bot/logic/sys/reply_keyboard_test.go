package sys

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-telegram/bot/models"
)

func TestReplyKeyboardDeliveryVersion(t *testing.T) {
	keyboard := &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{{Text: "开始使用"}, {Text: "立即注册"}},
		},
		IsPersistent:          true,
		ResizeKeyboard:        true,
		InputFieldPlaceholder: "请选择菜单",
	}

	version := replyKeyboardDeliveryVersion(keyboard)
	if version == "" {
		t.Fatal("keyboard version should not be empty")
	}
	if got := replyKeyboardDeliveryVersion(keyboard); got != version {
		t.Fatalf("keyboard version should be deterministic, got %q and %q", version, got)
	}

	changedKeyboard := &models.ReplyKeyboardMarkup{
		Keyboard:              [][]models.KeyboardButton{{{Text: "开始使用"}}},
		IsPersistent:          true,
		ResizeKeyboard:        true,
		InputFieldPlaceholder: "请选择菜单",
	}
	if got := replyKeyboardDeliveryVersion(changedKeyboard); got == version {
		t.Fatal("keyboard content change should produce a new version")
	}
	if got := replyKeyboardDeliveryVersion(&models.ReplyKeyboardRemove{RemoveKeyboard: true}); got == version {
		t.Fatal("keyboard removal should produce a different version")
	}
}

func TestIsReplyKeyboardMarkup(t *testing.T) {
	if !isReplyKeyboardMarkup(&models.ReplyKeyboardMarkup{}) {
		t.Fatal("reply keyboard should be recognized")
	}
	if !isReplyKeyboardMarkup(&models.ReplyKeyboardRemove{RemoveKeyboard: true}) {
		t.Fatal("reply keyboard removal should be recognized")
	}
	if isReplyKeyboardMarkup(&models.InlineKeyboardMarkup{}) {
		t.Fatal("inline keyboard should not be recognized as persistent reply keyboard")
	}
	if isReplyKeyboardMarkup(nil) {
		t.Fatal("nil markup should not be recognized")
	}
}

func TestIsTelegramCommand(t *testing.T) {
	for _, text := range []string{"/start", " /unknown argument", "/menu@sample_bot"} {
		if !isTelegramCommand(text) {
			t.Fatalf("%q should be recognized as Telegram command", text)
		}
	}
	for _, text := range []string{"", "开始使用", "hello /start"} {
		if isTelegramCommand(text) {
			t.Fatalf("%q should not be recognized as Telegram command", text)
		}
	}
}

func TestCompactReplyKeyboardLabels(t *testing.T) {
	for _, feature := range botFeatures {
		if feature == nil {
			continue
		}
		label := compactReplyKeyboardLabel(feature, feature.Description())
		if utf8.RuneCountInString(label) > 4 {
			t.Errorf("%s: label too long %q", feature.Key(), label)
		}
	}
}
func TestReplyKeyboardRowsByLabelLength(t *testing.T) {
	for _, test := range []struct {
		name   string
		labels []string
		sizes  []int
	}{
		{"short", []string{"资料管理", "快速推送", "扫图搜索", "账户管理"}, []int{3, 1}},
		{"six characters", []string{"资料管理", "双向机器管理", "扫图搜索"}, []int{2, 1}},
		{"long starts new row", []string{"资料管理", "快速推送", "双向机器管理", "账户管理"}, []int{2, 2}},
		{"custom long", []string{"这是自定义长按钮", "扫图搜索"}, []int{1, 1}},
		{"empty", nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buttons []models.KeyboardButton
			for _, label := range test.labels {
				buttons = append(buttons, models.KeyboardButton{Text: label})
			}
			rows := replyKeyboardRows(buttons)
			if len(rows) != len(test.sizes) {
				t.Fatalf("rows=%v", rows)
			}
			index := 0
			for i, row := range rows {
				if len(row) != test.sizes[i] {
					t.Fatalf("row %d: %v", i, row)
				}
				for _, button := range row {
					if button.Text != test.labels[index] {
						t.Fatal("changed button order or label")
					}
					index++
				}
			}
		})
	}
}

func TestGroupedReplyKeyboardRows(t *testing.T) {
	for _, test := range []struct {
		name string
		keys []string
		want [][]string
	}{
		{"administrator", []string{(helpFeature{}).Key(), (accountManagementFeature{}).Key(), (scanFeature{}).Key(), (profileFeature{}).Key(), (quickPushFeature{}).Key(), (collectManageFeature{}).Key(), (botManagerFeature{}).Key(), (relayManagementFeature{}).Key(), (adminFeature{}).Key(), (contactFeature{}).Key()},
			[][]string{{(profileFeature{}).Key(), (quickPushFeature{}).Key(), (scanFeature{}).Key()},
				{(collectManageFeature{}).Key(), (botManagerFeature{}).Key(), (relayManagementFeature{}).Key()},
				{(accountManagementFeature{}).Key()},
				{(adminFeature{}).Key()},
				{(helpFeature{}).Key(), (contactFeature{}).Key()}}},
		{"public", []string{(helpFeature{}).Key(), (instantRegisterFeature{}).Key(), (contactFeature{}).Key(), (scanFeature{}).Key()},
			[][]string{{(scanFeature{}).Key()}, {(instantRegisterFeature{}).Key()}, {(helpFeature{}).Key(), (contactFeature{}).Key()}}},
		{"uploader", []string{(botManagerFeature{}).Key(), (profileFeature{}).Key(), (scanFeature{}).Key()},
			[][]string{{(profileFeature{}).Key(), (scanFeature{}).Key()}, {(botManagerFeature{}).Key()}}},
		{"unknown preserved", []string{"future_plugin", (helpFeature{}).Key()}, [][]string{{(helpFeature{}).Key()}, {"future_plugin"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var items []replyKeyboardItem
			for _, key := range test.keys {
				items = append(items, replyKeyboardItem{Key: key, Button: models.KeyboardButton{Text: key}})
			}
			// Use short display labels while retaining keys for order assertions.
			for i := range items {
				items[i].Button.Text = fmt.Sprint(i)
			}
			rows := groupedReplyKeyboardRows(items)
			var actual [][]string
			for _, row := range rows {
				var keys []string
				for _, button := range row {
					for _, item := range items {
						if item.Button.Text == button.Text {
							keys = append(keys, item.Key)
						}
					}
				}
				actual = append(actual, keys)
			}
			if test.want != nil && !reflect.DeepEqual(actual, test.want) {
				t.Fatalf("got %v want %v", actual, test.want)
			}
			count := 0
			for _, row := range rows {
				count += len(row)
				if len(row) > 3 {
					t.Fatal("row too wide")
				}
			}
			if count != len(items) {
				t.Fatalf("lost or added unauthorized entries: %v", actual)
			}
		})
	}
}

func TestReplyKeyboardProductionLabelsAndConfiguration(t *testing.T) {
	bot := &sSysBot{
		features: map[string]*botFeatureRow{
			"bot_management": {Status: 1, Description: "上架BOT管理", ConfigJson: `{"menuVisible":1}`},
			"admin_panel":    {Status: 1, Description: "获取最新域名", ConfigJson: `{"menuVisible":1}`},
			"main_help":      {Status: 2, ConfigJson: `{"menuVisible":1}`},
			"account_bind":   {Status: 1, ConfigJson: `{"menuVisible":0}`},
			"notify":         {Status: 1, ConfigJson: `{"menuVisible":1}`},
			"super_notify":   {Status: 1, ConfigJson: `{"menuVisible":1}`},
		}, featureAt: time.Now(), featureDefaultsAt: time.Now(),
	}
	ctx := context.Background()
	if label := bot.replyKeyboardLabel(ctx, botManagerFeature{}); label != "机器人" {
		t.Fatal(label)
	}
	if label := bot.replyKeyboardLabel(ctx, adminFeature{}); label != "获取最新域名" {
		t.Fatal(label)
	}
	for _, key := range []string{"main_help", "account_bind", "notify", "super_notify"} {
		if bot.featureMenuVisible(ctx, key) {
			t.Fatalf("unexpected menu entry %s", key)
		}
	}
	for _, label := range []string{"上架BOT管理", "机器人"} {
		if !bot.matchFeatureLabel(ctx, bot.features["bot_management"], botManagerFeature{}, label) {
			t.Fatalf("button not routable: %s", label)
		}
	}
}
