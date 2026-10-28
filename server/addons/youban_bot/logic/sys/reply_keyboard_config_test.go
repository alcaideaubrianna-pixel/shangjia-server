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
		{FeatureKey: "info", Command: "///START", Status: 1},
		{FeatureKey: "info", Command: " / / /start", Status: 1},
		{FeatureKey: "start", Command: "renamed", Status: 1},
		{FeatureKey: "start", Command: "start", Status: 2},
		{FeatureKey: "account_bind", Command: "CUSTOMINFO", Status: 1},
	} {
		if err := validateFeatureCommand(in, bot.features); err == nil {
			t.Fatalf("invalid command accepted: %+v", in)
		}
	}
	if err := validateFeatureCommand(&sysin.FeatureSaveInp{FeatureKey: "start", Command: "start", Status: 1}, bot.features); err != nil {
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

func TestEmptyCommandCannotRestoreOccupiedDefault(t *testing.T) {
	bot := &sSysBot{features: map[string]*botFeatureRow{"info": {Command: "custominfo", Status: 1}, "account_bind": {Command: "info", Status: 1}}, featureAt: time.Now(), featureDefaultsAt: time.Now()}
	if err := validateFeatureCommand(&sysin.FeatureSaveInp{FeatureKey: "info", Command: "", Status: 1}, bot.features); err == nil {
		t.Fatal("clearing command restored /info already owned by account_bind")
	}
}

func TestTypedNilKeyboardIsNotDelivered(t *testing.T) {
	var keyboard *models.ReplyKeyboardMarkup
	if isReplyKeyboardMarkup(keyboard) {
		t.Fatal("typed nil keyboard must not be recorded as a delivered keyboard")
	}
	var removal *models.ReplyKeyboardRemove
	if isReplyKeyboardMarkup(removal) {
		t.Fatal("typed nil removal must not be recorded as delivered")
	}
}

func TestNormalizeReplyKeyboardMarkupKeepsIntent(t *testing.T) {
	if normalizeReplyKeyboardMarkup(nil) != nil {
		t.Fatal("absent markup must preserve existing menu")
	}
	inline := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "link", URL: "https://example.com"}}}}
	if normalizeReplyKeyboardMarkup(inline) != inline {
		t.Fatal("inline keyboard changed")
	}
	keyboard := &models.ReplyKeyboardMarkup{Keyboard: [][]models.KeyboardButton{{{Text: "menu"}}}}
	if normalizeReplyKeyboardMarkup(keyboard) != keyboard {
		t.Fatal("nonempty keyboard changed")
	}
	var empty *models.ReplyKeyboardMarkup
	removal, ok := normalizeReplyKeyboardMarkup(empty).(*models.ReplyKeyboardRemove)
	if !ok || !removal.RemoveKeyboard {
		t.Fatal("typed nil keyboard must send explicit removal")
	}
}

func TestEmptyCommandPersistsEffectiveDefault(t *testing.T) {
	in := &sysin.FeatureSaveInp{FeatureKey: "info", Command: "", Status: 1}
	if err := validateFeatureCommand(in, map[string]*botFeatureRow{}); err != nil {
		t.Fatal(err)
	}
	if in.Command != "info" {
		t.Fatalf("empty fallback was not made explicit: %q", in.Command)
	}
	// Legacy start aliases no longer own commands: dispatch always uses /start.
	if err := validateFeatureCommand(&sysin.FeatureSaveInp{FeatureKey: "info", Command: "legacy"}, map[string]*botFeatureRow{"start": {Command: "legacy"}}); err != nil {
		t.Fatal(err)
	}
}

func TestFeatureCommandNormalizationIsStable(t *testing.T) {
	for _, input := range []string{"info", " /info ", "///INFO", " / / /start", "", "/"} {
		normalized := normalizeFeatureCommand(input)
		if second := normalizeFeatureCommand(normalized); second != normalized {
			t.Fatalf("unstable command %q: %q -> %q", input, normalized, second)
		}
	}
}
