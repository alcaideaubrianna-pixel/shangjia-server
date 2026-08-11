package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type publishAccountRuntimeProvider struct{ publish *sSysPublish }

type publishAccountRuntimePayload struct {
	Sources   []accountCollectSourceRuntime
	Listeners []accountListenPlanRuntime
}

func (p *publishAccountRuntimeProvider) ListAccountRuntimes(ctx context.Context) ([]*collectorin.AccountRuntimeBinding, error) {
	groups, err := p.publish.enabledAccountMonitorGroups(ctx)
	if err != nil {
		return nil, err
	}
	bindings := make([]*collectorin.AccountRuntimeBinding, 0, len(groups))
	for accountID, group := range groups {
		if accountID <= 0 || !p.publish.accountCollectCircuitShouldStart(accountID) {
			continue
		}
		bindings = append(bindings, &collectorin.AccountRuntimeBinding{
			AccountID: accountID,
			Signature: accountMonitorGroupSignature(group.Sources, group.Listeners),
			Payload:   &publishAccountRuntimePayload{Sources: group.Sources, Listeners: group.Listeners},
		})
	}
	return bindings, nil
}

func (p *publishAccountRuntimeProvider) OpenAccountRuntime(_ context.Context, binding *collectorin.AccountRuntimeBinding) (collectorservice.AccountRuntimeSession, error) {
	if binding == nil || binding.AccountID <= 0 {
		return nil, gerror.New("Telegram账号运行配置无效")
	}
	payload, ok := binding.Payload.(*publishAccountRuntimePayload)
	if !ok || payload == nil {
		return nil, gerror.New("Telegram账号运行配置载荷无效")
	}
	return &accountCollectWorker{
		service: p.publish, tgAccountId: binding.AccountID, signature: binding.Signature,
		sources:         append([]accountCollectSourceRuntime(nil), payload.Sources...),
		listeners:       append([]accountListenPlanRuntime(nil), payload.Listeners...),
		messages:        make(chan accountCollectMessageTask, 4096),
		listenerGroups:  make(map[string]*listenerMessageGroup),
		listenerSenders: make(map[string]listenerMessageSenderInfo),
	}, nil
}

func (w *accountCollectWorker) UpdateAccountRuntime(binding *collectorin.AccountRuntimeBinding) bool {
	if binding == nil {
		return false
	}
	payload, ok := binding.Payload.(*publishAccountRuntimePayload)
	if !ok || payload == nil {
		return false
	}
	return w.updateConfig(binding.Signature, payload.Sources, payload.Listeners)
}

func (w *accountCollectWorker) BindAccountRuntimeHandlers(dispatcher tg.UpdateDispatcher) {
	w.bindGotdHandlers(dispatcher)
}

func (w *accountCollectWorker) NewAccountRuntimeClient(ctx context.Context, dispatcher tg.UpdateDispatcher) (*telegram.Client, error) {
	account, err := w.service.accountCollectTgAccount(ctx, w.tgAccountId)
	if err != nil {
		return nil, err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, err
	}
	return w.service.newAccountCollectClient(ctx, conf, account, dispatcher)
}

func (w *accountCollectWorker) StartAccountRuntime(ctx context.Context, _ *telegram.Client) {
	w.service.closeAccountCollectCircuit(w.tgAccountId)
	go w.runMessageLoop(ctx)
}

func (w *accountCollectWorker) StopAccountRuntime() { w.clearListenerGroups() }

func (w *accountCollectWorker) HandleAccountRuntimeError(ctx context.Context, err error) {
	if err == nil || isContextDone(ctx) {
		return
	}
	w.service.openAccountCollectCircuit(ctx, w.tgAccountId, err)
	if isTelegramPermanentAccountAuthError(err) {
		w.service.handleTgAccountPermanentAuthError(context.Background(), w.tgAccountId, 0, telegramPermanentAccountAuthMessage(err), err)
	}
}
