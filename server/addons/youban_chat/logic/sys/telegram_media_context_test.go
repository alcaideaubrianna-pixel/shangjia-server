package sys

import (
	"context"
	"testing"

	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model"
)

func TestTelegramMediaUploadContextInitializesAsyncContext(t *testing.T) {
	ctx := telegramMediaUploadContext(context.Background())

	if module := contexts.GetModule(ctx); module != consts.AppApi {
		t.Fatalf("expected module %q, got %q", consts.AppApi, module)
	}
	if addon := contexts.GetAddonName(ctx); addon != "youban_chat" {
		t.Fatalf("expected addon youban_chat, got %q", addon)
	}
}

func TestTelegramMediaUploadContextPreservesExistingContext(t *testing.T) {
	existing := &model.Context{Module: consts.AppAdmin, AddonName: "existing"}
	ctx := context.WithValue(context.Background(), consts.ContextHTTPKey, existing)

	result := telegramMediaUploadContext(ctx)
	if contexts.Get(result) != existing {
		t.Fatal("expected the existing request context to be preserved")
	}
	if module := contexts.GetModule(result); module != consts.AppAdmin {
		t.Fatalf("expected module %q, got %q", consts.AppAdmin, module)
	}
}
