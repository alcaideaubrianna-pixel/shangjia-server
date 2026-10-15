package sys

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestGlobalAutoDeleteKeywordsBackfillDisabledByDefault(t *testing.T) {
	in := &sysin.GlobalAutoDeleteKeywordsSaveInp{
		Keywords:        []string{"异常提示"},
		BackfillStartAt: "2026-10-15 00:00:00",
		BackfillEndAt:   "2026-10-15 01:00:00",
	}
	if err := in.Filter(context.Background()); err != nil {
		t.Fatal(err)
	}
	if in.BackfillEnabled != 0 || in.BackfillStartAt != "" || in.BackfillEndAt != "" {
		t.Fatalf("disabled backfill retained scheduling fields: %#v", in)
	}
}

func TestGlobalAutoDeleteKeywordsBackfillRequiresBoundedRange(t *testing.T) {
	tests := []struct {
		name    string
		startAt string
		endAt   string
		wantErr bool
	}{
		{name: "valid", startAt: "2026-10-15 00:00:00", endAt: "2026-10-15 03:00:00"},
		{name: "missing start", endAt: "2026-10-15 01:00:00", wantErr: true},
		{name: "reversed", startAt: "2026-10-15 02:00:00", endAt: "2026-10-15 01:00:00", wantErr: true},
		{name: "over 3 hours", startAt: "2026-10-15 00:00:00", endAt: "2026-10-15 03:00:01", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := &sysin.GlobalAutoDeleteKeywordsSaveInp{
				BackfillEnabled: 1,
				BackfillStartAt: test.startAt,
				BackfillEndAt:   test.endAt,
			}
			err := in.Filter(context.Background())
			if (err != nil) != test.wantErr {
				t.Fatalf("Filter() error = %v, wantErr %t", err, test.wantErr)
			}
			if err == nil {
				if _, parseErr := time.Parse(time.RFC3339, in.BackfillStartAt); parseErr != nil {
					t.Fatalf("start time was not normalized: %q", in.BackfillStartAt)
				}
			}
		})
	}
}

func TestValidateAutoDeleteBackfillPayloadLimits(t *testing.T) {
	valid := autoDeleteBackfillQueuePayload{
		StartAt: "2026-10-15T00:00:00+08:00", EndAt: "2026-10-15T03:00:00+08:00",
		MaxMessages: autoDeleteBackfillMaxMessages,
	}
	if _, _, err := validateAutoDeleteBackfillPayload(valid); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	overLimit := valid
	overLimit.MaxMessages++
	if _, _, err := validateAutoDeleteBackfillPayload(overLimit); err == nil {
		t.Fatal("payload above hard scan limit was accepted")
	}
	overWindow := valid
	overWindow.EndAt = "2026-10-15T03:00:01+08:00"
	if _, _, err := validateAutoDeleteBackfillPayload(overWindow); err == nil {
		t.Fatal("payload above hard time window was accepted")
	}
}

func TestAutoDeleteQueuesAreIsolated(t *testing.T) {
	if tgQueueNameAutoDeleteRealtime == tgQueueNameAutoDeleteBackfill {
		t.Fatal("realtime and historical deletion must not share a queue")
	}
	if autoDeleteBackfillBatchSize > 20 || autoDeleteBackfillBatchInterval < 30*time.Second || autoDeleteBackfillDeleteSpacing < 2*time.Second {
		t.Fatal("historical deletion throttle was relaxed")
	}
}

func TestMarshalGlobalAutoDeleteKeywordsUsesJSON(t *testing.T) {
	raw, err := marshalGlobalAutoDeleteKeywords([]string{"处理异常", "录入失败"})
	if err != nil {
		t.Fatal(err)
	}
	if raw != `["处理异常","录入失败"]` {
		t.Fatalf("unexpected encoded keywords: %s", raw)
	}
	var decoded []string
	if err = json.Unmarshal([]byte(raw), &decoded); err != nil || len(decoded) != 2 {
		t.Fatalf("invalid keyword JSON: decoded=%v err=%v", decoded, err)
	}
}

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

func TestTelegramAutoDeleteFallbackTaskKey(t *testing.T) {
	taskKey := telegramAutoDeleteFallbackTaskKey(338, 522499)
	channelID, messageID, ok := parseTelegramAutoDeleteFallbackTaskKey(taskKey)
	if !ok || channelID != 338 || messageID != 522499 {
		t.Fatalf("parseTelegramAutoDeleteFallbackTaskKey(%q) = %d, %d, %t", taskKey, channelID, messageID, ok)
	}
	for _, invalid := range []string{"", "message-delete-fallback:1", "auto-delete-fallback:338", "auto-delete-fallback:0:1", "auto-delete-fallback:338:bad"} {
		if _, _, valid := parseTelegramAutoDeleteFallbackTaskKey(invalid); valid {
			t.Fatalf("invalid task key accepted: %q", invalid)
		}
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
