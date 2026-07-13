package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

const accountCollectSupervisorInterval = 15 * time.Second

type accountCollectSupervisor struct {
	workers map[int64]*accountCollectWorker
}

type accountCollectSourceRuntime struct {
	Id                    int64  `json:"id"`
	TenantId              int64  `json:"tenantId"`
	AccountId             int64  `json:"accountId"`
	TgAccountId           int64  `json:"tgAccountId"`
	SourceChatId          string `json:"sourceChatId"`
	SourceUsername        string `json:"sourceUsername"`
	HistoryCollectEnabled int    `json:"historyCollectEnabled"`
	HistoryCollectMode    string `json:"historyCollectMode"`
	HistoryCollectDays    int    `json:"historyCollectDays"`
}

type accountCollectWorker struct {
	service     *sSysPublish
	tgAccountId int64
	signature   string
	sources     []accountCollectSourceRuntime
	listeners   []accountListenPlanRuntime
	cancel      context.CancelFunc
	done        chan struct{}
	messages    chan accountCollectMessageTask
	operations  chan accountCollectOperationTask

	listenerGroupMu  sync.Mutex
	listenerGroups   map[string]*listenerMessageGroup
	listenerSenderMu sync.Mutex
	listenerSenders  map[string]listenerMessageSenderInfo
}

type accountCollectMessageTask struct {
	source accountCollectSourceRuntime
	msg    *tg.Message
	chatId string
}

type accountCollectOperationTask struct {
	ctx  context.Context
	run  accountCollectOperation
	done chan error
}

type accountCollectOperation func(context.Context, *telegram.Client) error

func (s *sSysPublish) runAccountCollectSupervisor(ctx context.Context) {
	supervisor := &accountCollectSupervisor{workers: make(map[int64]*accountCollectWorker)}
	defer supervisor.stopAll()
	supervisor.sync(ctx, s)
	ticker := time.NewTicker(accountCollectSupervisorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			supervisor.sync(ctx, s)
		}
	}
}

func (m *accountCollectSupervisor) sync(ctx context.Context, service *sSysPublish) {
	groups, err := service.enabledAccountMonitorGroups(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取账号监听源失败：%+v", err)
		return
	}
	active := make(map[int64]struct{}, len(groups))
	for tgAccountId, group := range groups {
		if tgAccountId <= 0 {
			continue
		}
		active[tgAccountId] = struct{}{}
		signature := accountMonitorGroupSignature(group.Sources, group.Listeners)
		if worker := m.workers[tgAccountId]; worker != nil && worker.signature == signature && !worker.isDone() {
			continue
		}
		if worker := m.workers[tgAccountId]; worker != nil {
			worker.stop()
		}
		m.workers[tgAccountId] = startAccountCollectWorker(ctx, service, tgAccountId, signature, group.Sources, group.Listeners)
	}
	for tgAccountId, worker := range m.workers {
		if _, ok := active[tgAccountId]; ok {
			continue
		}
		worker.stop()
		delete(m.workers, tgAccountId)
	}
}

func (m *accountCollectSupervisor) stopAll() {
	for tgAccountId, worker := range m.workers {
		worker.stop()
		delete(m.workers, tgAccountId)
	}
}

func (s *sSysPublish) enabledAccountCollectSources(ctx context.Context) (map[int64][]accountCollectSourceRuntime, error) {
	groups := make(map[int64][]accountCollectSourceRuntime)
	if s.collectGlobalEnabled(ctx) {
		if err := ensureCollectSourceColumns(ctx); err != nil {
			return nil, err
		}
		var rows []accountCollectSourceRuntime
		err := pdao.YoubanPublishCollectSource.Ctx(ctx).
			Fields("id,tenant_id,account_id,tg_account_id,source_chat_id,source_username,history_collect_enabled,history_collect_mode,history_collect_days").
			Where("source_type", sysin.CollectSourceTypeAccount).
			Where("collect_enabled", 1).
			Where("status", 1).
			WhereGT("tg_account_id", 0).
			WhereNull("deleted_at").
			OrderAsc("tg_account_id").
			OrderAsc("id").
			Scan(&rows)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			row.HistoryCollectEnabled, row.HistoryCollectMode, row.HistoryCollectDays = sysin.NormalizeCollectHistoryConfig(
				sysin.CollectSourceTypeAccount,
				row.HistoryCollectEnabled,
				row.HistoryCollectMode,
				row.HistoryCollectDays,
			)
			groups[row.TgAccountId] = append(groups[row.TgAccountId], row)
		}
	}
	if enabled, err := s.autoDeleteEnabled(ctx); err != nil {
		return nil, err
	} else if enabled {
		ids, err := s.authorizedTgAccountIds(ctx)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, ok := groups[id]; !ok {
				groups[id] = []accountCollectSourceRuntime{}
			}
		}
	}
	return groups, nil
}

func (s *sSysPublish) autoDeleteEnabled(ctx context.Context) (bool, error) {
	conf, err := service.SysConfig().AutoDeleteConfigView(ctx, &sysin.AutoDeleteConfigViewInp{})
	if err != nil {
		return false, err
	}
	return conf != nil && conf.AutoDeleteConfig != nil && conf.Enabled == 1, nil
}

func (s *sSysPublish) authorizedTgAccountIds(ctx context.Context) ([]int64, error) {
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Fields("id").
		Where("status", sysin.PublishTgAccountStatusAuthorized).
		WhereNot("session_key", "").
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取自动删除监听TG账号失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			ids = append(ids, row.Id)
		}
	}
	return ids, nil
}

func startAccountCollectWorker(ctx context.Context, service *sSysPublish, tgAccountId int64, signature string, sources []accountCollectSourceRuntime, listeners []accountListenPlanRuntime) *accountCollectWorker {
	workerCtx, cancel := context.WithCancel(ctx)
	worker := &accountCollectWorker{
		service:         service,
		tgAccountId:     tgAccountId,
		signature:       signature,
		sources:         append([]accountCollectSourceRuntime(nil), sources...),
		listeners:       append([]accountListenPlanRuntime(nil), listeners...),
		cancel:          cancel,
		done:            make(chan struct{}),
		messages:        make(chan accountCollectMessageTask, 4096),
		operations:      make(chan accountCollectOperationTask, 256),
		listenerGroups:  make(map[string]*listenerMessageGroup),
		listenerSenders: make(map[string]listenerMessageSenderInfo),
	}
	go worker.run(workerCtx)
	return worker
}

func (w *accountCollectWorker) run(ctx context.Context) {
	defer close(w.done)
	g.Log().Infof(ctx, "账号采集 worker 启动 tgAccountId:%d sources:%d listeners:%d listenerTargets:%d", w.tgAccountId, len(w.sources), len(w.listeners), accountListenerTargetCount(w.listeners))
	defer g.Log().Infof(context.Background(), "账号采集 worker 停止 tgAccountId:%d", w.tgAccountId)
	if err := w.runGotdDispatcher(ctx); err != nil && !isContextDone(ctx) {
		if isTelegramAuthKeyUnregistered(err) {
			if item, itemErr := w.service.accountCollectTgAccount(context.Background(), w.tgAccountId); itemErr == nil {
				w.service.expireTgAccountSession(context.Background(), item.Id, item.TenantId, 0, tgAccountSessionExpiredMessage)
			}
		}
		g.Log().Warningf(ctx, "账号采集 worker 异常 tgAccountId:%d err:%+v", w.tgAccountId, err)
	}
}

func accountListenerTargetCount(listeners []accountListenPlanRuntime) int {
	count := 0
	for _, listener := range listeners {
		count += len(listener.Targets)
	}
	return count
}

func (w *accountCollectWorker) runGotdDispatcher(ctx context.Context) error {
	if w.service == nil {
		return gerror.New("账号采集服务未初始化")
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return err
	}
	item, err := w.service.accountCollectTgAccount(ctx, w.tgAccountId)
	if err != nil {
		return err
	}
	dispatcher := tg.NewUpdateDispatcher()
	w.bindGotdHandlers(dispatcher)
	client, err := w.service.newAccountCollectClient(ctx, conf, item, dispatcher)
	if err != nil {
		return err
	}
	return client.Run(ctx, func(runCtx context.Context) error {
		if _, err := client.Self(runCtx); err != nil {
			return err
		}
		w.service.registerAccountCollectWorker(w)
		defer w.service.unregisterAccountCollectWorker(w)
		go w.runMessageLoop(runCtx)
		go w.runOperationLoop(runCtx, client)
		g.Log().Infof(runCtx, "账号采集 dispatcher 已连接 tgAccountId:%d", w.tgAccountId)
		<-runCtx.Done()
		return runCtx.Err()
	})
}

func (w *accountCollectWorker) runMessageLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-w.messages:
			w.ingestGotdMessage(ctx, task.source, task.msg, task.chatId)
		}
	}
}

func (w *accountCollectWorker) runOperationLoop(ctx context.Context, client *telegram.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-w.operations:
			task.done <- w.runOperation(ctx, client, task)
		}
	}
}

func (w *accountCollectWorker) runOperation(ctx context.Context, client *telegram.Client, task accountCollectOperationTask) error {
	if task.run == nil {
		return nil
	}
	runCtx := ctx
	if task.ctx != nil {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithCancel(ctx)
		defer cancel()
		go func() {
			select {
			case <-task.ctx.Done():
				cancel()
			case <-runCtx.Done():
			}
		}()
	}
	return task.run(runCtx, client)
}

func (s *sSysPublish) registerAccountCollectWorker(worker *accountCollectWorker) {
	if worker == nil || worker.tgAccountId <= 0 {
		return
	}
	s.accountRuntimeMu.Lock()
	s.accountRuntimes[worker.tgAccountId] = worker
	s.accountRuntimeMu.Unlock()
}

func (s *sSysPublish) unregisterAccountCollectWorker(worker *accountCollectWorker) {
	if worker == nil || worker.tgAccountId <= 0 {
		return
	}
	s.accountRuntimeMu.Lock()
	if s.accountRuntimes[worker.tgAccountId] == worker {
		delete(s.accountRuntimes, worker.tgAccountId)
	}
	s.accountRuntimeMu.Unlock()
}

func (s *sSysPublish) executeAccountCollectOperation(ctx context.Context, tgAccountId int64, timeout time.Duration, run accountCollectOperation) (bool, error) {
	if tgAccountId <= 0 || run == nil {
		return false, nil
	}
	s.accountRuntimeMu.Lock()
	worker := s.accountRuntimes[tgAccountId]
	s.accountRuntimeMu.Unlock()
	if worker == nil {
		return false, nil
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	task := accountCollectOperationTask{
		ctx:  runCtx,
		run:  run,
		done: make(chan error, 1),
	}
	select {
	case worker.operations <- task:
	case <-runCtx.Done():
		return true, runCtx.Err()
	}
	select {
	case err := <-task.done:
		return true, err
	case <-runCtx.Done():
		worker.stop()
		return true, runCtx.Err()
	}
}

func (w *accountCollectWorker) stop() {
	if w == nil || w.cancel == nil {
		return
	}
	w.cancel()
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
	}
}

func (w *accountCollectWorker) isDone() bool {
	if w == nil || w.done == nil {
		return true
	}
	select {
	case <-w.done:
		return true
	default:
		return false
	}
}

func accountCollectSourceSignature(sources []accountCollectSourceRuntime) string {
	parts := make([]string, 0, len(sources))
	for _, item := range sources {
		parts = append(parts, fmt.Sprintf("%d:%s:%s:%d:%s:%d",
			item.Id,
			item.SourceChatId,
			item.SourceUsername,
			item.HistoryCollectEnabled,
			item.HistoryCollectMode,
			item.HistoryCollectDays,
		))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

type accountCollectTgAccount struct {
	Id         int64  `json:"id"`
	TenantId   int64  `json:"tenantId"`
	AccountId  int64  `json:"accountId"`
	SessionKey string `json:"sessionKey"`
	Status     string `json:"status"`
}

func (s *sSysPublish) accountCollectTgAccount(ctx context.Context, tgAccountId int64) (*accountCollectTgAccount, error) {
	var item *accountCollectTgAccount
	err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,session_key,status").
		Where("id", tgAccountId).
		WhereNull("deleted_at").
		Scan(&item)
	if err != nil {
		return nil, gerror.Wrap(err, "读取账号采集TG账号失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("账号采集TG账号不存在")
	}
	if strings.TrimSpace(item.SessionKey) == "" {
		return nil, gerror.New("账号采集TG账号未登录")
	}
	return item, nil
}

func (s *sSysPublish) newAccountCollectClient(ctx context.Context, conf *model.TelegramConfig, item *accountCollectTgAccount, dispatcher tg.UpdateDispatcher) (*telegram.Client, error) {
	if conf == nil || conf.AppId <= 0 || strings.TrimSpace(conf.AppHash) == "" {
		return nil, gerror.New("请先配置Telegram App ID和App Hash")
	}
	storage, err := s.telegramSessionStorage(item.SessionKey)
	if err != nil {
		return nil, err
	}
	options := telegram.Options{
		SessionStorage: storage,
		UpdateHandler:  dispatcher,
	}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		return nil, err
	} else if resolver != nil {
		options.Resolver = resolver
	}
	return telegram.NewClient(conf.AppId, conf.AppHash, options), nil
}

func isContextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
