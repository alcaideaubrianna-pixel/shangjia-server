package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

const accountRuntimeSyncInterval = 15 * time.Second
const accountRuntimeOperationConcurrency = 4
const accountRuntimePriorityOperationConcurrency = 1

type accountRuntime struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	refresh chan struct{}
	workers map[int64]*accountWorker
}

type accountWorker struct {
	bindingMu              sync.RWMutex
	binding                *sysin.AccountRuntimeBinding
	session                collectorservice.AccountRuntimeSession
	cancel                 context.CancelFunc
	done                   chan struct{}
	operations             chan accountOperationTask
	priorityOperations     chan accountOperationTask
	messages               chan accountMessageTask
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

func (r *accountRuntime) recoverAndObserveAccountTasks(ctx context.Context) {
	tasks := collectorservice.AccountTasks()
	if recovered, err := tasks.RecoverExpired(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复超时Telegram账号任务失败：%+v", err)
	} else if recovered > 0 {
		g.Log().Infof(ctx, "已恢复超时Telegram账号任务 count:%d", recovered)
	}
	stats, err := tasks.ActiveStatusStats(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "统计Telegram账号任务状态失败：%+v", err)
		return
	}
	now := time.Now()
	depth, _ := accountRuntimeMeter.Int64Histogram("telegram_account_task_queue_depth")
	age, _ := accountRuntimeMeter.Float64Histogram("telegram_account_task_oldest_age_seconds")
	for _, stat := range stats {
		attributes := metric.WithAttributes(attribute.String("status", stat.Status))
		depth.Record(ctx, stat.Total, attributes)
		oldest := stat.OldestCreatedAt
		if stat.Status == sysin.AccountTaskStatusProcessing {
			oldest = stat.OldestUpdatedAt
		}
		if oldest != nil {
			seconds := now.Sub(*oldest).Seconds()
			if seconds < 0 {
				seconds = 0
			}
			age.Record(ctx, seconds, attributes)
		}
	}
}

func (r *accountRuntime) sync(ctx context.Context) {
	provider := collectorservice.AccountRuntimeProviderInstance()
	if provider == nil {
		return
	}
	bindings, err := provider.ListAccountRuntimes(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取Telegram账号运行配置失败：%+v", err)
		return
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
			operations: make(chan accountOperationTask, 256), priorityOperations: make(chan accountOperationTask, 64), messages: make(chan accountMessageTask, 4096),
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
		return
	}
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
	defer collectorservice.AccountLeaseManager().Release(context.Background(), lease)
	startCounter, _ := accountRuntimeMeter.Int64Counter("telegram_account_runtime_starts_total")
	startCounter.Add(ctx, 1)
	dispatcher := tg.NewUpdateDispatcher()
	w.bindAccountMessageHandlers(dispatcher)
	client, err := w.session.NewAccountRuntimeClient(ctx, dispatcher)
	if err != nil {
		w.session.HandleAccountRuntimeError(ctx, err)
		return
	}
	err = client.Run(ctx, func(runCtx context.Context) error {
		if _, selfErr := client.Self(runCtx); selfErr != nil {
			return selfErr
		}
		w.session.StartAccountRuntime(runCtx, client)
		go w.runAccountMessageLoop(runCtx)
		go w.runOperations(runCtx, client)
		go w.runPriorityOperations(runCtx, client)
		go RunAccountTaskLoop(runCtx, client, lease, nil)
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
