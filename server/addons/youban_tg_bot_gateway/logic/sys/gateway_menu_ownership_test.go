package sys

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot/models"
	"hotgo/addons/youban_tg_bot_gateway/service"
)

type menuOwnershipFeature struct {
	menus service.FeatureMenus
	calls int
}

func (*menuOwnershipFeature) Key() string   { return "menu-ownership-test" }
func (*menuOwnershipFeature) Priority() int { return 1 }
func (f *menuOwnershipFeature) Menus(context.Context, service.BotContext) (service.FeatureMenus, error) {
	f.calls++
	return f.menus, nil
}
func (*menuOwnershipFeature) HandleUpdate(context.Context, service.BotContext, *models.Update) (bool, error) {
	return false, nil
}

func TestExclusiveMenuOwnsCommands(t *testing.T) {
	previous := &menuOwnershipFeature{menus: service.FeatureMenus{Managed: true, Items: []service.MenuItem{{Command: "cooperation", Description: "平台合作"}}}}
	relay := &menuOwnershipFeature{menus: service.FeatureMenus{Managed: true, Exclusive: true, Items: []service.MenuItem{{Command: "start", Description: "开始双向聊天"}}}}
	legacy := &menuOwnershipFeature{menus: service.FeatureMenus{Managed: true, Items: []service.MenuItem{{Command: "start", Description: "开始使用"}}}}
	commands, _, managed, err := gatewayMenuCommands(t.Context(), service.BotContext{}, []service.Feature{previous, relay, legacy})
	if err != nil || !managed || len(commands) != 1 || commands[0].Description != "开始双向聊天" || legacy.calls != 0 {
		t.Fatalf("exclusive menu = %+v managed=%v legacyCalls=%d err=%v", commands, managed, legacy.calls, err)
	}
}

func TestOrdinaryMenuConflictStillRejected(t *testing.T) {
	a := &menuOwnershipFeature{menus: service.FeatureMenus{Managed: true, Items: []service.MenuItem{{Command: "start", Description: "开始使用"}}}}
	b := &menuOwnershipFeature{menus: service.FeatureMenus{Managed: true, Items: []service.MenuItem{{Command: "start", Description: "另一个入口"}}}}
	if _, _, _, err := gatewayMenuCommands(t.Context(), service.BotContext{}, []service.Feature{a, b}); err == nil {
		t.Fatal("ordinary conflict must remain an error")
	}
}

func TestCanceledGatewayDoesNotReachHandlers(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	feature := &menuOwnershipFeature{}
	if _, _, _, err := gatewayMenuCommands(ctx, service.BotContext{}, []service.Feature{feature}); !errors.Is(err, context.Canceled) || feature.calls != 0 {
		t.Fatalf("canceled menu calls=%d err=%v", feature.calls, err)
	}
	if err := NewGateway().dispatch(ctx, "missing", &models.Update{ID: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled dispatch error=%v", err)
	}
}
