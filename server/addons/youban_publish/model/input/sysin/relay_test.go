package sysin

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRelayChannelInputValidation(t *testing.T) {
	ctx := context.Background()
	var missing *RelayChannelCreateInp
	if missing.Filter(ctx) == nil {
		t.Fatal("nil input accepted")
	}
	valid := RelayChannelCreateInp{TenantId: 1, TgAccountId: 2, BotId: 3, Title: "  中转频道  "}
	if err := valid.Filter(ctx); err != nil || valid.Capacity != 2 || valid.Title != "中转频道" {
		t.Fatalf("default/trim: %+v %v", valid, err)
	}
	for _, capacity := range []int{-1, 21} {
		in := valid
		in.Capacity = capacity
		if in.Filter(ctx) == nil {
			t.Fatalf("invalid capacity accepted: %d", capacity)
		}
	}
	for _, owner := range []struct {
		tenantID, accountID, botID int64
	}{
		{0, 2, 0}, {1, 0, 0}, {1, 2, -1},
	} {
		invalidOwner := valid
		invalidOwner.TenantId, invalidOwner.TgAccountId, invalidOwner.BotId = owner.tenantID, owner.accountID, owner.botID
		if invalidOwner.Filter(ctx) == nil {
			t.Fatalf("invalid relay owner accepted: %+v", owner)
		}
	}
	if (&RelayChannelStatusInp{Id: 1, Status: "anything"}).Filter(ctx) == nil {
		t.Fatal("invalid status accepted")
	}
	if (&RelayChannelStatusInp{Id: 1, Status: "enabled"}).Filter(ctx) != nil {
		t.Fatal("valid status rejected")
	}
	if (&RelayChannelSaveInp{TenantId: 1, TgAccountId: 2, BotId: 3, TargetChatId: "  "}).Filter(ctx) == nil {
		t.Fatal("blank channel accepted")
	}
}

func TestRelayChannelSharedBotIsOptional(t *testing.T) {
	ctx := context.Background()
	// The deployed create form sends only tenantId, tgAccountId and title.
	var create RelayChannelCreateInp
	if err := json.Unmarshal([]byte(`{"tenantId":1,"tgAccountId":2,"title":"共享中转频道"}`), &create); err != nil {
		t.Fatal(err)
	}
	if err := create.Filter(ctx); err != nil || create.BotId != 0 || create.Capacity != 2 {
		t.Fatalf("shared relay create rejected or assigned a Bot: %+v err=%v", create, err)
	}
	save := RelayChannelSaveInp{TenantId: 1, TgAccountId: 2, TargetChatId: "-1001234567890"}
	if err := save.Filter(ctx); err != nil || save.BotId != 0 || save.Capacity != 2 {
		t.Fatalf("shared relay save rejected or assigned a Bot: %+v err=%v", save, err)
	}
	save.BotId = -1
	if save.Filter(ctx) == nil {
		t.Fatal("negative Bot ID accepted by save")
	}
	// Existing clients may continue to configure a specific Bot.
	save.BotId = 3
	if err := save.Filter(ctx); err != nil || save.BotId != 3 {
		t.Fatalf("explicit Bot binding rejected or changed: %+v err=%v", save, err)
	}
}
