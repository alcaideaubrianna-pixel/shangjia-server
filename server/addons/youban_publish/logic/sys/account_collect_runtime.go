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
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
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
	tenantId    int64
	configMu    sync.RWMutex
	signature   string
	sources     []accountCollectSourceRuntime
	listeners   []accountListenPlanRuntime
	cancel      context.CancelFunc
	done        chan struct{}
	clientMu    sync.RWMutex
	client      *telegram.Client
	messages    chan accountCollectMessageTask
	operations  chan accountCollectOperationTask
	mediaSlots  chan struct{}
	operationMu sync.RWMutex

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
	ctx      context.Context
	run      accountCollectOperation
	done     chan error
	parallel bool
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
		case <-s.accountRuntimeRefresh:
			supervisor.sync(ctx, s)
		case <-ticker.C:
			supervisor.sync(ctx, s)
		}
	}
}

func (s *sSysPublish) refreshAccountCollectSupervisor() {
	if s == nil || s.accountRuntimeRefresh == nil {
		return
	}
	select {
	case s.accountRuntimeRefresh <- struct{}{}:
	default:
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
		if worker := m.workers[tgAccountId]; worker != nil && !worker.isDone() {
			if worker.updateConfig(signature, group.Sources, group.Listeners) {
				g.Log().Infof(ctx, "账号采集配置已热更新 tgAccountId:%d sources:%d listeners:%d listenerTargets:%d", tgAccountId, len(group.Sources), len(group.Listeners), accountListenerTargetCount(group.Listeners))
			}
			continue
		}
		if worker := m.workers[tgAccountId]; worker != nil {
			worker.stop()
		}
		if !service.accountCollectCircuitShouldStart(tgAccountId) {
			continue
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
		if err := ensureTenantVipTables(ctx); err != nil {
			return nil, err
		}
		var rows []accountCollectSourceRuntime
		err := g.DB().Model(pdao.YoubanPublishCollectSource.Table()+" s").Safe().Ctx(ctx).
			InnerJoin(publishTgAccountTable+" ta", "ta.id=s.tg_account_id").
			InnerJoin(pdao.YoubanPublishTenantVip.Table()+" vip", "vip.tenant_id=s.tenant_id AND vip.status=1 AND vip.level>0 AND vip.deleted_at IS NULL").
			Fields("s.id,s.tenant_id,s.account_id,s.tg_account_id,s.source_chat_id,s.source_username,s.history_collect_enabled,s.history_collect_mode,s.history_collect_days").
			Where("s.source_type", sysin.CollectSourceTypeAccount).
			Where("s.collect_enabled", 1).
			Where("s.status", 1).
			WhereGT("s.tg_account_id", 0).
			Where("ta.status", sysin.PublishTgAccountStatusAuthorized).
			WhereNot("ta.session_key", "").
			Where("(vip.expired_at IS NULL OR vip.expired_at>?)", gtime.Now()).
			WhereNull("s.deleted_at").
			WhereNull("ta.deleted_at").
			OrderAsc("s.tg_account_id").
			OrderAsc("s.id").
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
	return groups, nil
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
		mediaSlots:      make(chan struct{}, accountCollectMediaConcurrency(ctx)),
		listenerGroups:  make(map[string]*listenerMessageGroup),
		listenerSenders: make(map[string]listenerMessageSenderInfo),
	}
	service.registerAccountCollectWorker(worker)
	go worker.run(workerCtx)
	return worker
}

func (w *accountCollectWorker) run(ctx context.Context) {
	defer func() {
		w.service.unregisterAccountCollectWorker(w)
		w.failPendingOperations(newCollectMediaRetryError("TG账号共享连接启动中断，等待自动重试", accountCollectConnectionRetryDelay))
		close(w.done)
	}()
	defer w.clearListenerGroups()
	accountLock, err := acquireTelegramAccountClientLeaseWait(ctx, w.tgAccountId, func(waitErr error) {
		g.Log().Infof(ctx, "账号采集 worker 等待旧实例连接租约释放 tgAccountId:%d err:%+v", w.tgAccountId, waitErr)
	})
	if err != nil {
		if !isContextDone(ctx) {
			g.Log().Warningf(ctx, "账号采集 worker 获取集群连接租约失败 tgAccountId:%d err:%+v", w.tgAccountId, err)
		}
		return
	}
	g.Log().Infof(ctx, "账号采集 worker 已取得集群连接租约 tgAccountId:%d", w.tgAccountId)
	defer func() { _ = accountLock.Unlock(context.Background()) }()
	sources, listeners := w.configSnapshot()
	g.Log().Infof(ctx, "账号采集 worker 启动 tgAccountId:%d sources:%d listeners:%d listenerTargets:%d", w.tgAccountId, len(sources), len(listeners), accountListenerTargetCount(listeners))
	defer g.Log().Infof(context.Background(), "账号采集 worker 停止 tgAccountId:%d", w.tgAccountId)
	if err := w.runGotdDispatcher(ctx); err != nil && !isContextDone(ctx) {
		w.service.openAccountCollectCircuit(ctx, w.tgAccountId, err)
		if isTelegramPermanentAccountAuthError(err) {
			w.service.handleTgAccountPermanentAuthError(context.Background(), w.tgAccountId, 0, telegramPermanentAccountAuthMessage(err), err)
		}
		g.Log().Warningf(ctx, "账号采集 worker 异常 tgAccountId:%d err:%+v", w.tgAccountId, err)
	}
}

func (w *accountCollectWorker) updateConfig(signature string, sources []accountCollectSourceRuntime, listeners []accountListenPlanRuntime) bool {
	if w == nil {
		return false
	}
	w.configMu.Lock()
	defer w.configMu.Unlock()
	if w.signature == signature {
		return false
	}
	w.signature = signature
	w.sources = append([]accountCollectSourceRuntime(nil), sources...)
	w.listeners = append([]accountListenPlanRuntime(nil), listeners...)
	return true
}

func (w *accountCollectWorker) configSnapshot() ([]accountCollectSourceRuntime, []accountListenPlanRuntime) {
	if w == nil {
		return nil, nil
	}
	w.configMu.RLock()
	defer w.configMu.RUnlock()
	return append([]accountCollectSourceRuntime(nil), w.sources...), append([]accountListenPlanRuntime(nil), w.listeners...)
}

func (w *accountCollectWorker) clearListenerGroups() {
	if w == nil {
		return
	}
	w.listenerGroupMu.Lock()
	defer w.listenerGroupMu.Unlock()
	for key, group := range w.listenerGroups {
		if group != nil && group.timer != nil {
			group.timer.Stop()
		}
		delete(w.listenerGroups, key)
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
	w.tenantId = item.TenantId
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
		w.clientMu.Lock()
		w.client = client
		w.clientMu.Unlock()
		w.service.closeAccountCollectCircuit(w.tgAccountId)
		defer func() {
			w.clientMu.Lock()
			if w.client == client {
				w.client = nil
			}
			w.clientMu.Unlock()
		}()
		go w.runMessageLoop(runCtx)
		go w.runOperationLoop(runCtx, client)
		g.Log().Infof(runCtx, "账号采集 dispatcher 已连接 tgAccountId:%d", w.tgAccountId)
		<-runCtx.Done()
		return runCtx.Err()
	})
}

func (w *accountCollectWorker) failPendingOperations(err error) {
	if w == nil || w.operations == nil {
		return
	}
	for {
		select {
		case task := <-w.operations:
			if task.done != nil {
				select {
				case task.done <- err:
				default:
				}
			}
		default:
			return
		}
	}
}

func (w *accountCollectWorker) runtimeClient() *telegram.Client {
	if w == nil {
		return nil
	}
	w.clientMu.RLock()
	defer w.clientMu.RUnlock()
	return w.client
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
			if !task.parallel {
				task.done <- w.runOperation(ctx, client, task)
				continue
			}
			select {
			case w.mediaSlots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			go func() {
				defer func() { <-w.mediaSlots }()
				task.done <- w.runOperation(ctx, client, task)
			}()
		}
	}
}

func (w *accountCollectWorker) runOperation(ctx context.Context, client *telegram.Client, task accountCollectOperationTask) error {
	if task.run == nil {
		return nil
	}
	if task.parallel {
		w.operationMu.RLock()
		defer w.operationMu.RUnlock()
	} else {
		w.operationMu.Lock()
		defer w.operationMu.Unlock()
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
	result := make(chan error, 1)
	go func() {
		result <- task.run(runCtx, client)
	}()
	if task.ctx == nil {
		return <-result
	}
	select {
	case err := <-result:
		return err
	case <-task.ctx.Done():
		return task.ctx.Err()
	}
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
	return s.executeAccountCollectOperationMode(ctx, tgAccountId, timeout, run, false)
}

func (s *sSysPublish) executeAccountCollectMediaOperation(ctx context.Context, tgAccountId int64, timeout time.Duration, run accountCollectOperation) (bool, error) {
	return s.executeAccountCollectOperationMode(ctx, tgAccountId, timeout, run, true)
}

func (s *sSysPublish) accountCollectRuntimeClient(tgAccountId int64) (*telegram.Client, error) {
	if tgAccountId <= 0 {
		return nil, gerror.New("账号采集缺少TG账号")
	}
	s.restoreAccountCollectCircuit(context.Background(), tgAccountId)
	if err := s.accountCollectCircuitError(tgAccountId); err != nil {
		return nil, err
	}
	s.accountRuntimeMu.Lock()
	worker := s.accountRuntimes[tgAccountId]
	s.accountRuntimeMu.Unlock()
	if worker == nil {
		return nil, newCollectMediaRetryError("TG账号采集客户端暂不可用，等待连接恢复后重试", accountCollectConnectionRetryDelay)
	}
	client := worker.runtimeClient()
	if client == nil {
		return nil, newCollectMediaRetryError("TG账号采集客户端正在重连，当前媒体等待连接恢复", accountCollectConnectionRetryDelay)
	}
	return client, nil
}

func (s *sSysPublish) restartAccountCollectWorker(ctx context.Context, tgAccountId int64, reason error) {
	if tgAccountId <= 0 {
		return
	}
	s.accountRuntimeMu.Lock()
	worker := s.accountRuntimes[tgAccountId]
	s.accountRuntimeMu.Unlock()
	if worker == nil || worker.cancel == nil {
		return
	}
	g.Log().Warningf(ctx, "账号采集连接需要重建 tgAccountId:%d err:%+v", tgAccountId, reason)
	worker.cancel()
	go func() {
		select {
		case <-worker.done:
			s.refreshAccountCollectSupervisor()
		case <-time.After(5 * time.Second):
			s.refreshAccountCollectSupervisor()
		}
	}()
}

func (s *sSysPublish) executeAccountCollectOperationMode(ctx context.Context, tgAccountId int64, timeout time.Duration, run accountCollectOperation, parallel bool) (bool, error) {
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
		ctx:      runCtx,
		run:      run,
		done:     make(chan error, 1),
		parallel: parallel,
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
		// A single media download timing out must not stop the account's
		// dispatcher. The operation receives the canceled task context and
		// will release its media slot when it returns; other downloads can
		// continue using the same Telegram client.
		return true, runCtx.Err()
	}
}

func accountCollectMediaConcurrency(ctx context.Context) int {
	concurrency := g.Cfg().MustGet(ctx, "youbanPublish.collect.mediaFileConcurrency", 2).Int()
	if concurrency < 1 {
		return 1
	}
	if concurrency > 16 {
		return 16
	}
	return concurrency
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
	if err = validateAccountCollectTgAccount(item); err != nil {
		return nil, err
	}
	return item, nil
}

func validateAccountCollectTgAccount(item *accountCollectTgAccount) error {
	if item == nil || item.Id <= 0 {
		return gerror.New("账号采集TG账号不存在或已被删除")
	}
	if strings.TrimSpace(item.Status) != sysin.PublishTgAccountStatusAuthorized {
		return gerror.New("账号采集TG账号未授权或已失效")
	}
	if strings.TrimSpace(item.SessionKey) == "" {
		return gerror.New("账号采集TG账号未登录")
	}
	return nil
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
		AllowCDN:       true,
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
