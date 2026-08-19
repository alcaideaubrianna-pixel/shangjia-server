package sys

import (
	"context"

	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model"
)

// telegramMediaUploadContext supplies the module identity required by HotGo's
// attachment repository when a Telegram update is handled outside HTTP.
// Keep an existing request context unchanged so normal API uploads retain
// their caller module and tenant metadata.
func telegramMediaUploadContext(ctx context.Context) context.Context {
	if contexts.GetModule(ctx) != "" {
		return ctx
	}
	return context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{
		Module:    consts.AppApi,
		AddonName: "youban_chat",
		User:      contexts.GetUser(ctx),
	})
}
