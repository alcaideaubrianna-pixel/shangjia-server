package sys

import (
	"context"
	"hotgo/internal/consts"
	"hotgo/internal/model"
	"testing"
)

func TestRelayAdminRejectsUnknownIdentity(t *testing.T) {
	s := new(sSysPublish)
	if _, _, err := s.relayAdminScope(context.Background(), 0); err == nil {
		t.Fatal("anonymous access accepted")
	}
	ctx := context.WithValue(context.Background(), consts.ContextHTTPKey, &model.Context{User: &model.Identity{Id: 1, App: "unknown"}})
	if _, _, err := s.relayAdminScope(ctx, 0); err == nil {
		t.Fatal("unknown app with super-admin numeric ID accepted")
	}
}
