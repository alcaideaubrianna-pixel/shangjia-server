package sys

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"hotgo/addons/youban_tg_bot_gateway/service"

	"github.com/go-telegram/bot/models"
)

type exclusiveFailureFeature struct{ active atomic.Bool }

func (*exclusiveFailureFeature) Key() string   { return "exclusive-failure-test" }
func (*exclusiveFailureFeature) Priority() int { return 99999 }
func (*exclusiveFailureFeature) Menus(context.Context, service.BotContext) (service.FeatureMenus, error) {
	return service.FeatureMenus{}, nil
}
func (f *exclusiveFailureFeature) HandleUpdate(context.Context, service.BotContext, *models.Update) (bool, error) {
	if !f.active.Load() {
		return false, nil
	}
	return true, errors.New("durable relay storage failed")
}
func TestDispatchDoesNotFallThroughClaimedFeatureFailure(t *testing.T) {
	feature := &exclusiveFailureFeature{}
	feature.active.Store(true)
	service.RegisterFeature(feature)
	defer feature.active.Store(false)
	provider := &recordingGatewayProvider{}
	service.RegisterProvider(provider)
	gateway := NewGateway()
	gateway.bindings["exclusive"] = []service.BotBinding{{Owner: provider.Name()}}
	if err := gateway.dispatch(context.Background(), "exclusive", &models.Update{ID: 7}); err == nil {
		t.Fatal("claimed failure was acknowledged")
	}
	if provider.called {
		t.Fatal("failed relay message reached legacy handlers")
	}
}
