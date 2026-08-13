package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

type publishAccountRuntimeProvider struct{ publish *sSysPublish }

type publishAccountRuntimePayload struct {
	Listeners []accountListenPlanRuntime
}

func (p *publishAccountRuntimeProvider) ListAccountRuntimes(ctx context.Context) ([]*collectorin.AccountRuntimeBinding, error) {
	groups, err := p.publish.enabledAccountMonitorGroups(ctx)
	if err != nil {
		return nil, err
	}
	accountIDs, err := p.publish.authorizedTgAccountRuntimeIDs(ctx)
	if err != nil {
		return nil, err
	}
	for _, accountID := range accountIDs {
		if accountID > 0 {
			if _, exists := groups[accountID]; !exists {
				groups[accountID] = accountMonitorGroupRuntime{}
			}
		}
	}
	bindings := make([]*collectorin.AccountRuntimeBinding, 0, len(groups))
	for accountID, group := range groups {
		if accountID <= 0 || !p.publish.accountCollectCircuitShouldStart(accountID) {
			continue
		}
		bindings = append(bindings, &collectorin.AccountRuntimeBinding{
			AccountID: accountID,
			Signature: accountMonitorGroupSignature(group.Sources, group.Listeners),
			Sources:   collectorAccountRuntimeSources(group.Sources),
			Payload:   &publishAccountRuntimePayload{Listeners: group.Listeners},
		})
	}
	return bindings, nil
}

func (s *sSysPublish) authorizedTgAccountRuntimeIDs(ctx context.Context) ([]int64, error) {
	var rows []struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Fields("id").
		Where("status", publishsysin.PublishTgAccountStatusAuthorized).
		WhereNot("session_key", "").
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取常驻Telegram账号失败")
	}
	result := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			result = append(result, row.Id)
		}
	}
	return result, nil
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
		listeners:       append([]accountListenPlanRuntime(nil), payload.Listeners...),
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
	return w.updateConfig(binding.Signature, payload.Listeners)
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

func (w *accountCollectWorker) HandleAccountRuntimeMessage(ctx context.Context, entities tg.Entities, message *tg.Message, chatIDs []string) {
	if w == nil || message == nil {
		return
	}
	if gotdMessageGroupedId(message) != "" {
		w.bufferListenerGroupedMessage(ctx, entities, message, chatIDs)
		return
	}
	w.handleListenerMessage(ctx, entities, message, chatIDs)
}

func collectorAccountRuntimeSources(sources []accountCollectSourceRuntime) []collectorin.AccountRuntimeSource {
	result := make([]collectorin.AccountRuntimeSource, 0, len(sources))
	for _, source := range sources {
		result = append(result, collectorin.AccountRuntimeSource{
			TenantID: source.TenantId, AccountID: source.AccountId, SourceID: source.Id, ChatID: source.SourceChatId,
		})
	}
	return result
}
