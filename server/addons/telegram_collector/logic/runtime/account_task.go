package runtime

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/utility/telegrambudget"
)

const (
	accountTaskPollInterval = 2 * time.Second
	accountTaskLeaseTTL     = 30 * time.Minute
	accountTaskWorkerCount  = 8
)

type accountTaskRetryDelay interface {
	AccountTaskRetryDelay() time.Duration
}

var accountRuntimeMeter = otel.Meter("hotgo/addons/telegram_collector/runtime")

func RunAccountTaskLoop(ctx context.Context, client *telegram.Client, lease *sysin.AccountLease, operationGate *sync.Mutex) {
	runAccountTaskLoop(ctx, client, lease, operationGate, nil)
}

func runAccountTaskLoop(ctx context.Context, client *telegram.Client, lease *sysin.AccountLease, operationGate *sync.Mutex, resetSession context.CancelFunc) {
	if client == nil || lease == nil {
		return
	}
	if operationGate == nil {
		operationGate = &sync.Mutex{}
	}
	mediaFallbackGate := &sync.Mutex{}
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runAccountTaskDispatcher(loopCtx, accountTaskWorkerCount,
		func(ctx context.Context, limit int) ([]*sysin.AccountTask, bool) {
			return claimAccountTasks(ctx, lease, limit)
		},
		func(ctx context.Context, task *sysin.AccountTask) {
			processAccountTask(ctx, client, lease, operationGate, mediaFallbackGate, resetSession, task)
		},
		func(ctx context.Context, delay time.Duration) {
			if waiter, ok := collectorservice.AccountTasks().(interface {
				WaitForWork(context.Context, int64, time.Duration)
			}); ok {
				waiter.WaitForWork(ctx, lease.AccountID, delay)
				return
			}
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
			case <-timer.C:
			}
		},
		func() {
			cancel()
			if ctx.Err() == nil && resetSession != nil {
				resetSession()
			}
		},
		func(tasks []*sysin.AccountTask) {
			// Claim already acquired durable leases. Return tasks that never
			// reached a worker rather than waiting for their execution TTL.
			cleanupCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), telegrambudget.Finalize(ctx))
			defer stop()
			for _, task := range tasks {
				if cleanupCtx.Err() != nil {
					return
				}
				if task == nil {
					continue
				}
				if err := collectorservice.AccountTasks().Fail(cleanupCtx, &sysin.AccountTaskFailure{
					TaskID: task.ID, Lease: lease, Cause: context.Canceled, NotStarted: true,
				}); err != nil {
					g.Log().Warningf(cleanupCtx, "归还未分发Telegram账号任务失败 taskId:%d err:%+v", task.ID, err)
				}
			}
		})
}

// accountTaskWorkerLoop retains the single-worker entry for isolated callers.
// Account runtime uses the shared dispatcher above, never eight polling loops.
func accountTaskWorkerLoop(ctx context.Context, client *telegram.Client, lease *sysin.AccountLease, operationGate, mediaFallbackGate *sync.Mutex, resetSession context.CancelFunc, workerID int) {
	for ctx.Err() == nil {
		valid, handled := processAccountTask(ctx, client, lease, operationGate, mediaFallbackGate, resetSession)
		if !valid {
			if ctx.Err() == nil && resetSession != nil {
				resetSession()
			}
			return
		}
		if handled {
			continue
		}
		timer := time.NewTimer(accountTaskPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// One dispatcher owns polling and account lease renewal; execution remains
// concurrent. Slots prevent claiming work that would wait behind busy workers.
// While saturated, the account owner's existing lock watchdog and each running
// task's keepAccountTaskLeaseAlive continue renewal independently of polling.
func runAccountTaskDispatcher(ctx context.Context, concurrency int,
	claim func(context.Context, int) ([]*sysin.AccountTask, bool),
	execute func(context.Context, *sysin.AccountTask),
	wait func(context.Context, time.Duration), lostLease func(), abandoned ...func([]*sysin.AccountTask)) {
	if concurrency < 1 {
		return
	}
	jobs := make(chan *sysin.AccountTask)
	slots := make(chan struct{}, concurrency)
	for i := 0; i < concurrency; i++ {
		slots <- struct{}{}
	}
	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for task := range jobs {
				execute(ctx, task)
				// Shutdown must not wait for the dispatcher to drain slot tokens.
				select {
				case slots <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	defer workers.Wait()
	defer close(jobs)
	delay := accountTaskPollInterval
	for {
		select {
		case <-ctx.Done():
			return
		case <-slots:
		}
		capacity := 1
		for capacity < concurrency {
			select {
			case <-slots:
				capacity++
			default:
				goto claimedCapacity
			}
		}
	claimedCapacity:
		if ctx.Err() != nil {
			return
		}
		tasks, valid := claim(ctx, capacity)
		if !valid {
			lostLease()
			return
		}
		for i := len(tasks); i < capacity; i++ {
			slots <- struct{}{}
		}
		if len(tasks) == 0 {
			wait(ctx, delay)
			delay = min(delay*2, 8*time.Second)
			continue
		}
		delay = accountTaskPollInterval
		for index, task := range tasks {
			select {
			case <-ctx.Done():
				if len(abandoned) > 0 && abandoned[0] != nil {
					abandoned[0](tasks[index:])
				}
				return
			case jobs <- task:
			}
		}
	}
}

func claimAccountTasks(ctx context.Context, lease *sysin.AccountLease, limit int) ([]*sysin.AccountTask, bool) {
	renewed, err := collectorservice.AccountLeaseManager().Renew(ctx, lease, 0)
	if err != nil || !renewed {
		g.Log().Warningf(ctx, "Telegram账号任务租约续期失败 tgAccountId:%d err:%+v", lease.AccountID, err)
		return nil, false
	}
	tasks, err := collectorservice.AccountTasks().Claim(ctx, lease, limit, accountTaskLeaseTTL)
	if err != nil {
		g.Log().Warningf(ctx, "领取Telegram账号任务失败 tgAccountId:%d err:%+v", lease.AccountID, err)
		// A batch may have claimed tasks before a later row failed. Execute
		// those tasks instead of abandoning their live leases.
		return tasks, true
	}
	return tasks, true
}

func processAccountTask(ctx context.Context, client *telegram.Client, lease *sysin.AccountLease, operationGate, mediaFallbackGate *sync.Mutex, resetSession context.CancelFunc, supplied ...*sysin.AccountTask) (bool, bool) {
	tasks := supplied
	if len(tasks) == 0 {
		var valid bool
		tasks, valid = claimAccountTasks(ctx, lease, 1)
		if !valid {
			return false, false
		}
	}
	var err error
	for _, task := range tasks {
		if task == nil {
			continue
		}
		startedAt := time.Now()
		resultStatus := "completed"
		var handleErr error
		detailedLog := accountTaskLifecycleLogEnabled(task.TaskType)
		if detailedLog {
			g.Log().Info(ctx, "Telegram账号任务开始执行", g.Map{
				"taskId": task.ID, "taskType": task.TaskType, "taskKey": task.TaskKey,
				"tgAccountId": task.AccountID, "attempt": task.AttemptCount, "maxAttempts": task.MaxAttempts,
				"leaseEpoch": lease.Epoch,
			})
		}
		handler := collectorservice.AccountTaskHandlerFor(task.TaskType)
		if handler == nil {
			resultStatus = "failed"
			handleErr = gerror.New("Telegram账号任务处理器不存在：" + task.TaskType)
			err = collectorservice.AccountTasks().Fail(ctx, &sysin.AccountTaskFailure{
				TaskID: task.ID, Lease: lease, RetryDelay: time.Minute,
				Cause: handleErr,
			})
		} else {
			taskCtx, cancel := context.WithCancel(ctx)
			gateTimeout := 30 * time.Second
			if task.TaskType == sysin.AccountTaskTypeMessageMediaFallback {
				gateTimeout = telegrambudget.AccountGate(ctx)
			}
			waitCtx, waitCancel := context.WithTimeout(taskCtx, gateTimeout)
			stopRenew := keepAccountTaskLeaseAlive(taskCtx, cancel, lease)
			gate := accountTaskOperationGate(task.TaskType, operationGate, mediaFallbackGate)
			waitStarted := time.Now()
			gateErr := lockAccountTaskOperation(waitCtx, gate)
			waitCancel()
			g.Log().Infof(ctx, "Telegram账号任务发送锁等待结束 taskId:%d tgAccountId:%d waitMs:%d err:%v", task.ID, task.AccountID, time.Since(waitStarted).Milliseconds(), gateErr)
			var result *sysin.AccountMediaDownloadResult
			handleErr = gateErr
			var executionErr error
			if gateErr == nil {
				executionCtx, executionCancel := context.WithTimeout(taskCtx, accountTaskTimeoutContext(ctx, task.TaskType))
				stopWatchdog := watchAccountTaskTimeout(executionCtx, task, lease, resetSession)
				result, handleErr = handler.HandleAccountTask(executionCtx, client, task)
				executionErr = executionCtx.Err()
				if executionErr != nil {
					g.Log().Warning(ctx, "Telegram账号任务执行上下文结束", g.Map{
						"taskId": task.ID, "taskType": task.TaskType, "taskKey": task.TaskKey,
						"tgAccountId": task.AccountID, "attempt": task.AttemptCount,
						"duration":     time.Since(startedAt).Round(time.Millisecond).String(),
						"contextError": executionErr.Error(), "handlerError": handleErr,
					})
				}
				stopWatchdog()
				executionCancel()
				if gate != nil {
					gate.Unlock()
				}
			}
			taskCtxErr := executionErr
			stopRenew()
			cancel()
			if handleErr != nil && errors.Is(taskCtxErr, context.DeadlineExceeded) {
				g.Log().Warningf(ctx, "Telegram账号任务处理超时 taskId:%d type:%s tgAccountId:%d timeout:%s attempt:%d/%d err:%+v", task.ID, task.TaskType, task.AccountID, accountTaskTimeoutContext(ctx, task.TaskType), task.AttemptCount, task.MaxAttempts, taskCtxErr)
			}
			if handleErr == nil {
				if completion := collectorservice.AccountTaskCompletionHandlerFor(task.TaskType); completion != nil {
					handleErr = completion.HandleAccountTaskCompletion(ctx, task, result)
				}
			}
			if handleErr == nil {
				err = collectorservice.AccountTasks().Complete(ctx, task.ID, lease, result)
			} else {
				g.Log().Warningf(ctx, "Telegram账号任务处理失败 taskId:%d type:%s tgAccountId:%d duration:%s err:%+v", task.ID, task.TaskType, task.AccountID, time.Since(startedAt).Round(time.Millisecond), handleErr)
				notStarted := accountTaskFailureNotStarted(gateErr, handleErr)
				if provider := collectorservice.AccountRuntimeProviderInstance(); provider != nil && !notStarted {
					provider.ObserveAccountTaskError(ctx, task, handleErr)
				}
				resultStatus = "failed"
				if notStarted {
					resultStatus = "deferred"
				}
				err = collectorservice.AccountTasks().Fail(ctx, &sysin.AccountTaskFailure{
					TaskID: task.ID, Lease: lease, Cause: handleErr, RetryDelay: retryDelay(handleErr),
					NotStarted: notStarted,
				})
				if err == nil && accountTaskFailureShouldFinalize(task, notStarted) {
					task.Status = sysin.AccountTaskStatusFinalizing
					task.ErrorMessage = handleErr.Error()
					finalizeAccountTaskFailure(ctx, task)
				}
			}
		}
		if err != nil {
			resultStatus = "commit_failed"
			g.Log().Warningf(ctx, "提交Telegram账号任务结果失败 taskId:%d type:%s err:%+v", task.ID, task.TaskType, err)
		}
		if detailedLog {
			g.Log().Info(ctx, "Telegram账号任务执行结束", g.Map{
				"taskId": task.ID, "taskType": task.TaskType, "taskKey": task.TaskKey,
				"tgAccountId": task.AccountID, "attempt": task.AttemptCount, "maxAttempts": task.MaxAttempts,
				"result": resultStatus, "duration": time.Since(startedAt).Round(time.Millisecond).String(),
				"handlerError": handleErr, "commitError": err,
			})
		}
		counter, _ := accountRuntimeMeter.Int64Counter("telegram_account_task_total")
		counter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("task_type", task.TaskType),
			attribute.String("result", resultStatus),
		))
		histogram, _ := accountRuntimeMeter.Float64Histogram("telegram_account_task_duration_seconds")
		histogram.Record(ctx, time.Since(startedAt).Seconds(), metric.WithAttributes(
			attribute.String("task_type", task.TaskType),
			attribute.String("result", resultStatus),
		))
	}
	return true, len(tasks) > 0
}

func accountTaskFailureNotStarted(gateErr, handlerErr error) bool {
	if gateErr != nil {
		return true
	}
	var deferred sysin.AccountTaskAdmissionDeferred
	return errors.As(handlerErr, &deferred) && deferred.AccountTaskNotStarted()
}

func accountTaskFailureShouldFinalize(task *sysin.AccountTask, notStarted bool) bool {
	return task != nil && !notStarted && task.AttemptCount >= task.MaxAttempts
}

func accountTaskLifecycleLogEnabled(taskType string) bool {
	switch taskType {
	case sysin.AccountTaskTypeMessagePushInline,
		sysin.AccountTaskTypeMessageReconcile,
		sysin.AccountTaskTypeMessageMediaFallback,
		sysin.AccountTaskTypeMessageDeleteFallback:
		return true
	default:
		return false
	}
}

func watchAccountTaskTimeout(ctx context.Context, task *sysin.AccountTask, lease *sysin.AccountLease, resetSession context.CancelFunc) func() {
	stop := make(chan struct{})
	var stopOnce sync.Once
	if !accountTaskRequiresSessionResetOnTimeout(task) || resetSession == nil {
		return func() { stopOnce.Do(func() { close(stop) }) }
	}
	go func() {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			// Reset first. A stuck protocol RPC can also delay the task-state
			// write, and waiting for that write would leave the session wedged.
			resetSession()
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), telegrambudget.Finalize(ctx))
			defer cancel()
			cause := gerror.Newf("Telegram账号任务执行中断（%v），已重置协议号连接", ctx.Err())
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				cause = gerror.Newf("Telegram账号任务超过%s，已重置协议号连接", accountTaskTimeoutContext(ctx, task.TaskType))
			}
			if err := collectorservice.AccountTasks().Fail(cleanupCtx, &sysin.AccountTaskFailure{
				TaskID: task.ID, Lease: lease, Cause: cause, RetryDelay: retryDelay(cause),
			}); err != nil {
				g.Log().Warningf(cleanupCtx, "Telegram账号任务中断状态写回失败 taskId:%d type:%s tgAccountId:%d cause:%v err:%+v", task.ID, task.TaskType, task.AccountID, ctx.Err(), err)
			} else {
				g.Log().Warningf(cleanupCtx, "Telegram账号任务调用中断，已重置连接并写回失败 taskId:%d type:%s tgAccountId:%d timeout:%s cause:%v", task.ID, task.TaskType, task.AccountID, accountTaskTimeoutContext(ctx, task.TaskType), ctx.Err())
			}
		}
	}()
	return func() { stopOnce.Do(func() { close(stop) }) }
}

func accountTaskRequiresSessionResetOnTimeout(task *sysin.AccountTask) bool {
	if task == nil {
		return false
	}
	switch task.TaskType {
	case sysin.AccountTaskTypeMessagePushInline,
		sysin.AccountTaskTypeMessageReconcile,
		sysin.AccountTaskTypeMessageMediaFallback,
		sysin.AccountTaskTypeMessageDeleteFallback:
		return true
	default:
		return false
	}
}

// Media fallback uses its own gate: large albums stay serialized per account
// without blocking reconciliation and other control operations. Media downloads
// retain their existing independent transfer concurrency.
func accountTaskOperationGate(taskType string, operationGate, mediaFallbackGate *sync.Mutex) *sync.Mutex {
	switch taskType {
	case sysin.AccountTaskTypeMediaDownload:
		return nil
	case sysin.AccountTaskTypeMessageMediaFallback:
		return mediaFallbackGate
	default:
		return operationGate
	}
}

// A queued operation must not block shutdown or enter its handler with an
// already expired context. Do not spawn a goroutine that could acquire the
// mutex after the caller has returned.
func lockAccountTaskOperation(ctx context.Context, gate *sync.Mutex) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if gate == nil {
		return nil
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if gate.TryLock() {
			if err := ctx.Err(); err != nil {
				gate.Unlock()
				return err
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func finalizeAccountTaskFailure(ctx context.Context, task *sysin.AccountTask) {
	if task == nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), telegrambudget.Cleanup(ctx))
	defer cancel()
	g.Log().Warning(cleanupCtx, "Telegram账号任务开始终态失败回调", g.Map{
		"taskId": task.ID, "taskType": task.TaskType, "taskKey": task.TaskKey,
		"tgAccountId": task.AccountID, "attempt": task.AttemptCount, "maxAttempts": task.MaxAttempts,
		"error": task.ErrorMessage,
	})
	if err := handleAccountTaskFinalFailure(cleanupCtx, task); err != nil {
		g.Log().Warningf(cleanupCtx, "Telegram账号任务终态失败回调执行失败，稍后重试 taskId:%d type:%s err:%+v", task.ID, task.TaskType, err)
		return
	}
	if err := collectorservice.AccountTasks().FinalizeFailure(cleanupCtx, task.ID); err != nil {
		g.Log().Warningf(cleanupCtx, "Telegram账号任务终态确认失败，稍后重试 taskId:%d type:%s err:%+v", task.ID, task.TaskType, err)
		return
	}
	g.Log().Info(cleanupCtx, "Telegram账号任务终态失败已确认", g.Map{
		"taskId": task.ID, "taskType": task.TaskType, "taskKey": task.TaskKey,
		"tgAccountId": task.AccountID,
	})
}

func handleAccountTaskFinalFailure(ctx context.Context, task *sysin.AccountTask) error {
	handler := collectorservice.AccountTaskFailureHandlerFor(task.TaskType)
	if handler == nil {
		return nil
	}
	return handler.HandleAccountTaskFailure(ctx, task, gerror.New(task.ErrorMessage))
}

func keepAccountTaskLeaseAlive(ctx context.Context, cancel context.CancelFunc, lease *sysin.AccountLease) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				renewed, err := collectorservice.AccountLeaseManager().Renew(ctx, lease, 0)
				if err != nil || !renewed {
					g.Log().Warningf(ctx, "Telegram账号任务执行期间租约续期失败 tgAccountId:%d epoch:%d err:%+v", lease.AccountID, lease.Epoch, err)
					cancel()
					return
				}
			}
		}
	}()
	return func() { close(stop) }
}

func retryDelay(err error) time.Duration {
	var retryErr accountTaskRetryDelay
	if errors.As(err, &retryErr) {
		if delay := retryErr.AccountTaskRetryDelay(); delay > 0 {
			return delay
		}
	}
	return 5 * time.Second
}

// accountTaskTimeout preserves the default-context helper for callers without a task context.
func accountTaskTimeout(taskType string) time.Duration {
	return accountTaskTimeoutContext(context.Background(), taskType)
}

func accountTaskTimeoutContext(ctx context.Context, taskType string) time.Duration {
	switch taskType {
	case sysin.AccountTaskTypeBotManager:
		return 30 * time.Second
	case sysin.AccountTaskTypeHistoryPage:
		return 25 * time.Minute
	case sysin.AccountTaskTypeMaterialImportHistoryPage:
		return 25 * time.Minute
	case sysin.AccountTaskTypeMediaDownload:
		return 10 * time.Minute
	case sysin.AccountTaskTypeUsernameResolveDiagnostic:
		return 30 * time.Second
	case sysin.AccountTaskTypeDialogCacheRefresh:
		return 30 * time.Minute
	case sysin.AccountTaskTypeMessagePushInline:
		return 2 * time.Minute
	case sysin.AccountTaskTypeMessageReconcile:
		return 2 * time.Minute
	case sysin.AccountTaskTypeMessageMediaFallback:
		return telegrambudget.Account(ctx)
	case sysin.AccountTaskTypeMessageDeleteFallback:
		return 30 * time.Second
	case sysin.AccountTaskTypeManagedBotUsernameCheck:
		return 30 * time.Second
	case sysin.AccountTaskTypeManagedBotCreate:
		return 90 * time.Second
	case sysin.AccountTaskTypeChannelMemberSync:
		return 2 * time.Hour
	case sysin.AccountTaskTypeTgAccountRefresh:
		return 30 * time.Second
	case sysin.AccountTaskTypeMessageRepair:
		return 25 * time.Minute
	case sysin.AccountTaskTypeMessageRepairScan:
		return 10 * time.Minute
	default:
		return 5 * time.Minute
	}
}
