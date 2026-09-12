package sys

import (
	"errors"
	"reflect"
	"testing"

	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestMergeAutoDeleteStrings(t *testing.T) {
	got := mergeAutoDeleteStrings(
		[]string{"系统默认", "重复词"},
		[]string{" 自定义 ", "重复词", "系统默认"},
	)
	want := []string{"系统默认", "重复词", "自定义"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeAutoDeleteStrings() = %#v, want %#v", got, want)
	}
}

func TestDefaultAutoDeleteConfigIsEnabled(t *testing.T) {
	if defaultAutoDeleteConfig().Enabled != 1 {
		t.Fatal("automatic deletion should be enabled by default")
	}
}

func TestTenantAutoDeleteConfigFromLegacyIsEnabledByDefault(t *testing.T) {
	conf := tenantAutoDeleteConfigFromLegacy(&model.AutoDeleteConfig{Enabled: 1})
	if conf.Enabled != 1 || !conf.LegacyMigrated {
		t.Fatalf("legacy migration = %#v", conf)
	}
}

func TestChannelAutoDeleteBotIdsPrioritizesIncomingConfiguredBot(t *testing.T) {
	got := channelAutoDeleteBotIds(`[6,8,10]`, 8)
	if !reflect.DeepEqual(got, []int64{8, 6, 10}) {
		t.Fatalf("channelAutoDeleteBotIds() = %#v", got)
	}
}

func TestTelegramAutoDeleteTaskResult(t *testing.T) {
	bot := &sysin.BotModel{Id: 30}
	if err := telegramAutoDeleteTaskResult(bot, nil); err != nil {
		t.Fatalf("successful delete returned error: %v", err)
	}
	if err := telegramAutoDeleteTaskResult(bot, errors.New("Bad Request: message to delete not found")); err != nil {
		t.Fatalf("already deleted message must not retry: %v", err)
	}
	if err := telegramAutoDeleteTaskResult(bot, errors.New("connection reset by peer")); err == nil {
		t.Fatal("transient Telegram failure must be returned to Asynq")
	}
	if err := telegramAutoDeleteTaskResult(nil, nil); err == nil {
		t.Fatal("missing channel Bot must be returned to Asynq")
	}
}

func TestChannelBotPermissionSummaryRequiresSendAndDelete(t *testing.T) {
	raw := encodeChannelBotPermissionStates([]*sysin.ChannelCheckBotModel{{
		BotId:             1,
		BotName:           "上架 Bot",
		CanSendMessage:    1,
		CanDeleteMessages: 0,
		Status:            "warning",
		Message:           "Bot 已加入频道但没有删除消息权限",
	}})
	status, message := channelBotPermissionSummary(raw)
	if status != "error" || message == "" {
		t.Fatalf("channelBotPermissionSummary() = %q, %q", status, message)
	}
	state := channelBotPermissionStateForBot(raw, 1)
	if state == nil || state.CanDeleteMessages != 0 {
		t.Fatalf("channelBotPermissionStateForBot() = %#v", state)
	}
}
