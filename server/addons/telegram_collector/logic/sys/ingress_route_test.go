package sys

import (
	"context"
	"testing"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type ingressPolicyFixture struct {
	decision sysin.BotIngressDecision
	scope    sysin.BotIngressScope
}

func (p *ingressPolicyFixture) HandleCollectorDelivery(context.Context, *sysin.CollectorDelivery) error {
	return nil
}
func (p *ingressPolicyFixture) ClassifyCollectorBotIngress(_ context.Context, s sysin.BotIngressScope) sysin.BotIngressDecision {
	p.scope = s
	return p.decision
}

func TestIngressPolicyOptionalAndStrict(t *testing.T) {
	old := collectorservice.CollectorDeliveryHandler()
	defer collectorservice.RegisterDeliveryHandler(old)
	collectorservice.RegisterDeliveryHandler(nil)
	if d := classifyBotIngress(context.Background(), 78, 351, -1004411799859); d.Defer() {
		t.Fatal("missing policy must preserve realtime")
	}
	p := &ingressPolicyFixture{decision: sysin.BotIngressDecision{Lane: sysin.IngressLaneBackground, Reason: sysin.IngressDeferralApproval}}
	collectorservice.RegisterDeliveryHandler(p)
	if d := classifyBotIngress(context.Background(), 78, 351, -1004411799859); !d.Defer() || !p.scope.Valid() {
		t.Fatal("explicit scoped decision was lost")
	}
	p.decision.Reason = sysin.IngressDeferralNone
	if d := classifyBotIngress(context.Background(), 78, 351, -1004411799859); d.Defer() {
		t.Fatal("missing reason must not move work")
	}
}
