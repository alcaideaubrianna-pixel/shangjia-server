package runtime

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	collectorsys "hotgo/addons/telegram_collector/logic/sys"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

const accountRuntimeSyncInterval = 15 * time.Second
const accountRuntimeOperationConcurrency = 4
const accountRuntimePriorityOperationConcurrency = 1
const accountTaskFinalizationConcurrency = 8
const accountTaskMaintenanceBatchSize = 4
const accountTaskMaintenanceTimeout = 5 * time.Second

type accountRuntime struct {
	mu                  sync.Mutex
	cancel              context.CancelFunc
	done                chan struct{}
	refresh             chan struct{}
	workers             map[int64]*accountWorker
	maintenanceAccounts []int64
	maintenanceCursor   int64
}

type accountWorker struct {
	selfUserID             atomic.Int64
	bindingMu              sync.RWMutex
	binding                *sysin.AccountRuntimeBinding
	session                collectorservice.AccountRuntimeSession
	cancel                 context.CancelFunc
	done                   chan struct{}
	operations             chan accountOperationTask
	priorityOperations     chan accountOperationTask
	operationSlots         chan struct{}
	priorityOperationSlots chan struct{}
}

type accountOperationTask struct {
	ctx  context.Context
	run  collectorservice.AccountOperation
	done chan error
}

func init() {
	collectorservice.RegisterAccountRuntime(&accountRuntime{refresh: make(chan struct{}, 1), workers: make(map[int64]*accountWorker)})
}

func (r *accountRuntime) Start(ctx context.Context) {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	done := make(chan struct{})
	r.done = done
	r.mu.Unlock()
	r.startRemoteExecutor(runtimeCtx)
	go r.run(runtimeCtx, done)
}

func (r *accountRuntime) Stop() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.cancel, r.done = nil, nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
}

func (r *accountRuntime) Refresh() {
	select {
	case r.refresh <- struct{}{}:
	default:
	}
}

func (r *accountRuntime) Restart(accountID int64) {
	if accountID <= 0 {
		return
	}
	r.mu.Lock()
	worker := r.workers[accountID]
	delete(r.workers, accountID)
	r.mu.Unlock()
	if worker != nil {
		worker.stop()
	}
	r.Refresh()
}

func (r *accountRuntime) run(ctx context.Context, done chan struct{}) {
	defer func() {
		r.stopAll()
		close(done)
	}()
	r.sync(ctx)
	syncTicker := time.NewTicker(accountRuntimeSyncInterval)
	recoveryTicker := time.NewTicker(30 * time.Second)
	defer syncTicker.Stop()
	defer recoveryTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.refresh:
			r.sync(ctx)
		case <-syncTicker.C:
			r.sync(ctx)
		case <-recoveryTicker.C:
			r.recoverAndObserveAccountTasks(ctx)
		}
	}
}

// Maintenance rotates registered account scopes independently of live workers.
// Never discover accounts through task-table scans or global queue aggregates.
func (r *accountRuntime) recoverAndObserveAccountTasks(ctx context.Context) {
	r.recoverAndObserveAccountTasksWith(ctx, collectorservice.AccountTasks())
}

func (r *accountRuntime) recoverAndObserveAccountTasksWith(ctx context.Context, tasks collectorservice.IAccountTasks) {
	scopeCtx, cancelScopes := context.WithTimeout(ctx, accountTaskMaintenanceTimeout)
	accountIDs, err := tasks.MaintenanceAccounts(scopeCtx, accountTaskMaintenanceBatchSize)
	cancelScopes()
	if err != nil {
		g.Log().Warningf(ctx, "读取Telegram账号维护范围失败，使用本地已知账户 err:%+v", err)
		accountIDs = r.nextMaintenanceAccounts()
	}
	for _, accountID := range accountIDs {
		if ctx.Err() != nil {
			return
		}
		r.recoverAccountTasks(ctx, tasks, accountID)
	}
}

// Redis outage fallback retains known bindings after their workers stop.
// Select at most one small batch per tick without walking all workers.
func (r *accountRuntime) nextMaintenanceAccounts() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	accounts := r.maintenanceAccounts
	if len(accounts) == 0 {
		return nil
	}
	start := sort.Search(len(accounts), func(i int) bool { return accounts[i] > r.maintenanceCursor })
	selected := make([]int64, 0, accountTaskMaintenanceBatchSize)
	for i := 0; i < len(accounts) && i < accountTaskMaintenanceBatchSize; i++ {
		id := accounts[(start+i)%len(accounts)]
		r.maintenanceCursor = id
		selected = append(selected, id)
	}
	return selected
}

func (r *accountRuntime) recoverAccountTasks(ctx context.Context, tasks collectorservice.IAccountTasks, accountID int64) {
	ctx, cancel := context.WithTimeout(ctx, accountTaskMaintenanceTimeout)
	defer cancel()
	if recovered, err := tasks.RecoverExpired(ctx, accountID, 100); err != nil {
		g.Log().Warningf(ctx, "恢复超时Telegram账号任务失败 accountId:%d err:%+v", accountID, err)
	} else if recovered > 0 {
		g.Log().Infof(ctx, "已恢复超时Telegram账号任务 accountId:%d count:%d", accountID, recovered)
	}
	for taskType, policy := range collectorservice.AccountTaskPolicies() {
		if ctx.Err() != nil {
			return
		}
		expired, err := tasks.ExpirePending(ctx, accountID, taskType, time.Now().Add(-policy.PendingTTL), 100)
		if err != nil {
			g.Log().Warningf(ctx, "终止超期Telegram账号任务失败 accountId:%d type:%s err:%+v", accountID, taskType, err)
		} else if expired > 0 {
			g.Log().Warningf(ctx, "Telegram账号任务进入失败收尾 accountId:%d type:%s count:%d", accountID, taskType, expired)
		}
	}
	finalizing, err := tasks.ListFinalizing(ctx, accountID, 100)
	if err != nil {
		g.Log().Warningf(ctx, "读取待收尾Telegram账号任务失败 accountId:%d err:%+v", accountID, err)
		return
	}
	finalizeAccountTaskFailures(ctx, finalizing)
}

// Shared across passes: callbacks still running under independent cleanup
// budgets must not be started again or exceed the runtime concurrency limit.
var accountTaskFinalizationSlots = make(chan struct{}, accountTaskFinalizationConcurrency)
var accountTaskFinalizationActive sync.Map

func finalizeAccountTaskFailures(ctx context.Context, tasks []*sysin.AccountTask) {
	finalizeAccountTaskBatch(ctx, tasks, accountTaskFinalizationSlots, &accountTaskFinalizationActive, finalizeAccountTaskFailure)
}

func finalizeAccountTaskBatch(ctx context.Context, tasks []*sysin.AccountTask, slots chan struct{}, active *sync.Map, finalize func(context.Context, *sysin.AccountTask)) {
	var workers sync.WaitGroup
launch:
	for _, task := range tasks {
		if ctx.Err() != nil {
			break
		}
		if task == nil || task.ID <= 0 {
			continue
		}
		if _, loaded := active.LoadOrStore(task.ID, struct{}{}); loaded {
			continue
		}
		select {
		case <-ctx.Done():
			active.Delete(task.ID)
			break launch
		case slots <- struct{}{}:
		}
		if ctx.Err() != nil {
			<-slots
			active.Delete(task.ID)
			break
		}
		workers.Add(1)
		go func(task *sysin.AccountTask) {
			defer workers.Done()
			defer active.Delete(task.ID)
			defer func() { <-slots }()
			if ctx.Err() == nil {
				finalize(ctx, task)
			}
		}(task)
	}
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	select {
	case <-ctx.Done():
	case <-done:
	}
}

func (r *accountRuntime) sync(ctx context.Context) {
	provider := collectorservice.AccountRuntimeProviderInstance()
	if provider == nil {
		collectorsys.ObserveAccountInventory(ctx, nil, false)
		return
	}
	bindings, err := provider.ListAccountRuntimes(ctx)
	collectorsys.ObserveAccountInventory(ctx, bindings, err == nil)
	if err != nil {
		g.Log().Warningf(ctx, "读取Telegram账号运行配置失败：%+v", err)
		return
	}
	for _, binding := range bindings {
		if binding != nil && binding.AccountID > 0 {
			if err := collectorservice.AccountTasks().RegisterMaintenanceAccount(ctx, binding.AccountID); err != nil {
				g.Log().Warningf(ctx, "登记已绑定Telegram账号维护范围失败 accountId:%d err:%+v", binding.AccountID, err)
			}
		}
	}
	active := make(map[int64]struct{}, len(bindings))
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, binding := range bindings {
		if binding == nil || binding.AccountID <= 0 {
			continue
		}
		active[binding.AccountID] = struct{}{}
		if worker := r.workers[binding.AccountID]; worker != nil && !worker.isDone() {
			worker.updateBinding(binding)
			worker.session.UpdateAccountRuntime(binding)
			continue
		}
		if old := r.workers[binding.AccountID]; old != nil {
			old.stop()
		}
		session, openErr := provider.OpenAccountRuntime(ctx, binding)
		if openErr != nil {
			g.Log().Warningf(ctx, "创建Telegram账号运行会话失败 accountId:%d err:%+v", binding.AccountID, openErr)
			continue
		}
		workerCtx, cancel := context.WithCancel(ctx)
		worker := &accountWorker{
			binding: binding, session: session, cancel: cancel, done: make(chan struct{}),
			operations: make(chan accountOperationTask, 256), priorityOperations: make(chan accountOperationTask, 64),
			operationSlots: make(chan struct{}, accountRuntimeOperationConcurrency), priorityOperationSlots: make(chan struct{}, accountRuntimePriorityOperationConcurrency),
		}
		r.workers[binding.AccountID] = worker
		workerGauge, _ := accountRuntimeMeter.Int64UpDownCounter("telegram_account_runtime_workers")
		workerGauge.Add(ctx, 1)
		go worker.run(workerCtx)
	}
	for accountID, worker := range r.workers {
		if _, ok := active[accountID]; ok {
			continue
		}
		worker.stop()
		delete(r.workers, accountID)
	}
	known := make(map[int64]struct{}, len(r.maintenanceAccounts)+len(bindings))
	for _, id := range r.maintenanceAccounts {
		known[id] = struct{}{}
	}
	for _, binding := range bindings {
		if binding != nil && binding.AccountID > 0 {
			known[binding.AccountID] = struct{}{}
		}
	}
	r.maintenanceAccounts = r.maintenanceAccounts[:0]
	for id := range known {
		r.maintenanceAccounts = append(r.maintenanceAccounts, id)
	}
	sort.Slice(r.maintenanceAccounts, func(i, j int) bool { return r.maintenanceAccounts[i] < r.maintenanceAccounts[j] })

}

func (r *accountRuntime) stopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, worker := range r.workers {
		worker.stop()
		delete(r.workers, id)
	}
}

func (r *accountRuntime) Execute(ctx context.Context, accountID int64, timeout time.Duration, operation collectorservice.AccountOperation) (bool, error) {
	return r.execute(ctx, accountID, timeout, operation, false)
}

func (r *accountRuntime) ExecutePriority(ctx context.Context, accountID int64, timeout time.Duration, operation collectorservice.AccountOperation) (bool, error) {
	return r.execute(ctx, accountID, timeout, operation, true)
}

func (r *accountRuntime) execute(ctx context.Context, accountID int64, timeout time.Duration, operation collectorservice.AccountOperation, priority bool) (bool, error) {
	if accountID <= 0 || operation == nil {
		return false, nil
	}
	r.mu.Lock()
	worker := r.workers[accountID]
	r.mu.Unlock()
	if worker == nil || worker.isDone() {
		return false, nil
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	task := accountOperationTask{ctx: runCtx, run: operation, done: make(chan error, 1)}
	queue := worker.operations
	if priority {
		queue = worker.priorityOperations
	}
	select {
	case queue <- task:
	case <-runCtx.Done():
		return true, runCtx.Err()
	}
	select {
	case err := <-task.done:
		return true, err
	case <-runCtx.Done():
		return true, runCtx.Err()
	}
}

func (w *accountWorker) run(ctx context.Context) {
	defer func() {
		workerGauge, _ := accountRuntimeMeter.Int64UpDownCounter("telegram_account_runtime_workers")
		workerGauge.Add(context.Background(), -1)
		close(w.done)
	}()
	defer w.session.StopAccountRuntime()
	leaseTTL := time.Duration(g.Cfg().MustGet(ctx, "telegramCollector.account.leaseSeconds", 120).Int()) * time.Second
	if leaseTTL < 30*time.Second {
		leaseTTL = 30 * time.Second
	}
	binding := w.bindingSnapshot()
	if binding == nil || binding.AccountID <= 0 {
		g.Log().Warning(ctx, "Telegram账号运行绑定为空，跳过账号Worker")
		return
	}
	g.Log().Info(ctx, "Telegram账号Worker启动", g.Map{"accountId": binding.AccountID})
	lease, acquired, err := collectorservice.AccountLeaseManager().Acquire(ctx, binding.AccountID, accountRuntimeInstanceID(), leaseTTL)
	leaseResult := "acquired"
	if err != nil {
		leaseResult = "error"
	} else if !acquired {
		leaseResult = "held_by_other_instance"
	}
	leaseCounter, _ := accountRuntimeMeter.Int64Counter("telegram_account_runtime_lease_acquire_total")
	leaseCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", leaseResult)))
	if err != nil || !acquired {
		if err != nil {
			g.Log().Warningf(ctx, "获取Telegram账号运行租约失败 accountId:%d err:%+v", binding.AccountID, err)
		}
		return
	}
	g.Log().Info(ctx, "Telegram账号运行租约已获取", g.Map{"accountId": binding.AccountID, "epoch": lease.Epoch})
	defer collectorservice.AccountLeaseManager().Release(context.Background(), lease)
	if recovered, recoverErr := collectorservice.AccountTasks().RecoverExpired(ctx, binding.AccountID, 100); recoverErr != nil {
		g.Log().Warningf(ctx, "账号Worker启动恢复过期任务失败 accountId:%d err:%+v", binding.AccountID, recoverErr)
	} else if recovered > 0 {
		g.Log().Infof(ctx, "账号Worker启动恢复过期任务完成 accountId:%d count:%d", binding.AccountID, recovered)
	}
	startCounter, _ := accountRuntimeMeter.Int64Counter("telegram_account_runtime_starts_total")
	startCounter.Add(ctx, 1)
	dispatcher := tg.NewUpdateDispatcher()
	w.bindAccountMessageHandlers(dispatcher)
	client, err := w.session.NewAccountRuntimeClient(ctx, w.channelAccessUpdateHandler(dispatcher))
	if err != nil {
		g.Log().Warningf(ctx, "创建Telegram账号协议客户端失败 accountId:%d err:%+v", binding.AccountID, err)
		w.session.HandleAccountRuntimeError(ctx, err)
		return
	}
	err = client.Run(ctx, func(runCtx context.Context) error {
		self, selfErr := client.Self(runCtx)
		if selfErr != nil {
			return selfErr
		}
		w.selfUserID.Store(self.ID)
		g.Log().Info(ctx, "Telegram账号协议连接已建立", g.Map{"accountId": binding.AccountID, "selfUserId": self.ID})
		w.session.StartAccountRuntime(runCtx, client)
		go w.runOperations(runCtx, client)
		go w.runPriorityOperations(runCtx, client)
		go runAccountTaskLoop(runCtx, client, lease, nil, w.cancel)
		g.Log().Info(ctx, "Telegram账号任务循环已启动", g.Map{"accountId": binding.AccountID})
		<-runCtx.Done()
		return runCtx.Err()
	})
	if err != nil && ctx.Err() == nil {
		w.session.HandleAccountRuntimeError(ctx, err)
	}
}

func (w *accountWorker) runPriorityOperations(ctx context.Context, client *telegram.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-w.priorityOperations:
			w.runOperation(ctx, client, task, w.priorityOperationSlots)
		}
	}
}

func (w *accountWorker) updateBinding(binding *sysin.AccountRuntimeBinding) {
	if w == nil || binding == nil {
		return
	}
	w.bindingMu.Lock()
	w.binding = binding
	w.bindingMu.Unlock()
}

func (w *accountWorker) bindingSnapshot() *sysin.AccountRuntimeBinding {
	if w == nil {
		return nil
	}
	w.bindingMu.RLock()
	defer w.bindingMu.RUnlock()
	if w.binding == nil {
		return nil
	}
	copyBinding := *w.binding
	copyBinding.Sources = append([]sysin.AccountRuntimeSource(nil), w.binding.Sources...)
	return &copyBinding
}

func (w *accountWorker) runOperations(ctx context.Context, client *telegram.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-w.operations:
			w.runOperation(ctx, client, task, w.operationSlots)
		}
	}
}

func (w *accountWorker) runOperation(ctx context.Context, client *telegram.Client, task accountOperationTask, slots chan struct{}) {
	go func() {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-slots }()
		runCtx := ctx
		if task.ctx != nil {
			runCtx = task.ctx
		}
		err := task.run(runCtx, client)
		select {
		case task.done <- err:
		default:
		}
	}()
}

func (w *accountWorker) stop() {
	if w != nil && w.cancel != nil {
		w.cancel()
	}
}
func (w *accountWorker) isDone() bool {
	if w == nil {
		return true
	}
	select {
	case <-w.done:
		return true
	default:
		return false
	}
}

func accountRuntimeInstanceID() string {
	for _, key := range []string{"RAILWAY_REPLICA_ID", "RAILWAY_DEPLOYMENT_ID", "HOSTNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return fmt.Sprintf("%s:%d", value, os.Getpid())
		}
	}
	return fmt.Sprintf("local:%d", os.Getpid())
}
