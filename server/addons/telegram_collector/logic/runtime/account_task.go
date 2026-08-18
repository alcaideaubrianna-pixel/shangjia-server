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
)

const (
	accountTaskPollInterval = 2 * time.Second
	accountTaskLeaseTTL     = 30 * time.Minute
)

type accountTaskRetryDelay interface {
	AccountTaskRetryDelay() time.Duration
}

var accountRuntimeMeter = otel.Meter("hotgo/addons/telegram_collector/runtime")

func RunAccountTaskLoop(ctx context.Context, client *telegram.Client, lease *sysin.AccountLease, operationGate sync.Locker) {
	if client == nil || lease == nil {
		return
	}
	ticker := time.NewTicker(accountTaskPollInterval)
	defer ticker.Stop()
	for {
		keepRunning, handled := processAccountTask(ctx, client, lease, operationGate)
		if !keepRunning {
			return
		}
		if handled {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processAccountTask(ctx context.Context, client *telegram.Client, lease *sysin.AccountLease, operationGate sync.Locker) (bool, bool) {
	renewed, err := collectorservice.AccountLeaseManager().Renew(ctx, lease, 0)
	if err != nil {
		g.Log().Warningf(ctx, "续期Telegram账号任务租约失败 tgAccountId:%d err:%+v", lease.AccountID, err)
		return false, false
	}
	if !renewed {
		g.Log().Warningf(ctx, "Telegram账号任务租约已失效，停止领取任务 tgAccountId:%d epoch:%d", lease.AccountID, lease.Epoch)
		return false, false
	}
	tasks, err := collectorservice.AccountTasks().Claim(ctx, lease, 1, accountTaskLeaseTTL)
	if err != nil {
		g.Log().Warningf(ctx, "领取Telegram账号任务失败 tgAccountId:%d err:%+v", lease.AccountID, err)
		return true, false
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		startedAt := time.Now()
		resultStatus := "completed"
		handler := collectorservice.AccountTaskHandlerFor(task.TaskType)
		if handler == nil {
			resultStatus = "failed"
			err = collectorservice.AccountTasks().Fail(ctx, &sysin.AccountTaskFailure{
				TaskID: task.ID, Lease: lease, RetryDelay: time.Minute,
				Cause: gerror.New("Telegram账号任务处理器不存在：" + task.TaskType),
			})
		} else {
			taskCtx, cancel := context.WithTimeout(ctx, accountTaskTimeout(task.TaskType))
			stopRenew := keepAccountTaskLeaseAlive(taskCtx, cancel, lease)
			if operationGate != nil {
				operationGate.Lock()
			}
			result, handleErr := handler.HandleAccountTask(taskCtx, client, task)
			if operationGate != nil {
				operationGate.Unlock()
			}
			taskCtxErr := taskCtx.Err()
			stopRenew()
			cancel()
			if handleErr != nil && errors.Is(taskCtxErr, context.DeadlineExceeded) {
				g.Log().Warningf(ctx, "Telegram账号任务处理超时 taskId:%d type:%s tgAccountId:%d timeout:%s attempt:%d/%d err:%+v", task.ID, task.TaskType, task.AccountID, accountTaskTimeout(task.TaskType), task.AttemptCount, task.MaxAttempts, taskCtxErr)
			}
			if handleErr == nil {
				err = collectorservice.AccountTasks().Complete(ctx, task.ID, lease, result)
			} else {
				g.Log().Warningf(ctx, "Telegram账号任务处理失败 taskId:%d type:%s tgAccountId:%d duration:%s err:%+v", task.ID, task.TaskType, task.AccountID, time.Since(startedAt).Round(time.Millisecond), handleErr)
				resultStatus = "failed"
				err = collectorservice.AccountTasks().Fail(ctx, &sysin.AccountTaskFailure{
					TaskID: task.ID, Lease: lease, Cause: handleErr, RetryDelay: retryDelay(handleErr),
				})
			}
		}
		if err != nil {
			resultStatus = "commit_failed"
			g.Log().Warningf(ctx, "提交Telegram账号任务结果失败 taskId:%d type:%s err:%+v", task.ID, task.TaskType, err)
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
	if retryErr, ok := err.(accountTaskRetryDelay); ok {
		if delay := retryErr.AccountTaskRetryDelay(); delay > 0 {
			return delay
		}
	}
	return 5 * time.Second
}

func accountTaskTimeout(taskType string) time.Duration {
	switch taskType {
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
		return 5 * time.Minute
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
