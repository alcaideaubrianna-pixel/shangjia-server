package sys

import (
	"context"
	"github.com/go-telegram/bot/models"
	"hotgo/addons/youban_bot/model/input/sysin"
	"testing"
	"time"
)

func TestConfiguredKeyboardRowsPreservesPermissionsAndOrder(t *testing.T) {
	items := []replyKeyboardItem{{Key: "start", Button: models.KeyboardButton{Text: "开始使用"}}, {Key: "info", Button: models.KeyboardButton{Text: "聊天信息"}}}
	layout := [][]replyKeyboardConfigButton{{{FeatureKey: "profile_manage", Style: "danger"}, {FeatureKey: "info", Style: "success"}}, {{FeatureKey: "start", Style: "primary"}}}
	rows := configuredKeyboardRows(items, layout)
	if len(rows) != 2 || len(rows[0]) != 1 || rows[0][0].Text != "聊天信息" || rows[0][0].Style != "success" || rows[1][0].Style != "primary" {
		t.Fatalf("unexpected authorized layout: %+v", rows)
	}
	if len(configuredKeyboardRows(items, [][]replyKeyboardConfigButton{})) != 0 {
		t.Fatal("explicit empty configuration must hide all buttons")
	}
	if len(configuredKeyboardRows(items, nil)) == 0 {
		t.Fatal("nil configuration must use defaults")
	}
}

func TestReplyKeyboardConfigValidation(t *testing.T) {
	for _, value := range []interface{}{"not an array", []interface{}{[]interface{}{}}, [][]replyKeyboardConfigButton{{{FeatureKey: "unknown"}}}, [][]replyKeyboardConfigButton{{{FeatureKey: "start", Style: "purple"}}}, [][]replyKeyboardConfigButton{{{FeatureKey: "start"}, {FeatureKey: "start"}}}} {
		if _, err := parseReplyKeyboardLayout(value); err == nil {
			t.Fatalf("invalid configuration accepted: %#v", value)
		}
	}
	layout, err := parseReplyKeyboardLayout([][]replyKeyboardConfigButton{{{FeatureKey: "start", Style: "primary"}}})
	if err != nil || len(layout) != 1 {
		t.Fatalf("valid configuration rejected: %v", err)
	}
}

func TestReplyKeyboardColorChangesDeliveryVersion(t *testing.T) {
	markup := &models.ReplyKeyboardMarkup{Keyboard: [][]models.KeyboardButton{{{Text: "开始使用"}}}}
	old := replyKeyboardDeliveryVersion(markup)
	markup.Keyboard[0][0].Style = "danger"
	if old == replyKeyboardDeliveryVersion(markup) {
		t.Fatal("color change must refresh keyboard")
	}
}

func TestStartReservedAndDuplicateCommands(t *testing.T) {
	bot := &sSysBot{features: map[string]*botFeatureRow{"start": {Command: "legacy", Status: 2}, "info": {Command: "custominfo", Status: 1}}, featureAt: time.Now(), featureDefaultsAt: time.Now()}
	ctx := context.Background()
	if _, enabled := bot.featureConfig(ctx, "start"); !enabled {
		t.Fatal("default start must remain enabled")
	}
	if command := bot.featureCommand(ctx, startFeature{}); command != "start" {
		t.Fatal(command)
	}
	for _, in := range []*sysin.FeatureSaveInp{
		{FeatureKey: "info", Command: "start", Status: 1},
		{FeatureKey: "start", Command: "renamed", Status: 1},
		{FeatureKey: "start", Command: "start", Status: 2},
		{FeatureKey: "account_bind", Command: "CUSTOMINFO", Status: 1},
	} {
		if err := bot.validateFeatureCommand(ctx, in); err == nil {
			t.Fatalf("invalid command accepted: %+v", in)
		}
	}
	if err := bot.validateFeatureCommand(ctx, &sysin.FeatureSaveInp{FeatureKey: "start", Command: "start", Status: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyKeyboardRemovesPreviousLayout(t *testing.T) {
	markup := replyKeyboardMarkupOrRemove(nil)
	remove, ok := markup.(*models.ReplyKeyboardRemove)
	if !ok || !remove.RemoveKeyboard {
		t.Fatal("empty menu must remove previously delivered keyboard")
	}
}
