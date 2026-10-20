package sysin

import (
	"context"
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
	invalidOwner := valid
	invalidOwner.BotId = 0
	if invalidOwner.Filter(ctx) == nil {
		t.Fatal("missing bot accepted")
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
