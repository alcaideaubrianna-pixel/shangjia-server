package runtime

import (
	"context"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"

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

func RunAccountTaskLoop(ctx context.Context, client *telegram.Client, lease *sysin.AccountLease, operationGate sync.Locker) {
	if client == nil || lease == nil {
		return
	}
	ticker := time.NewTicker(accountTaskPollInterval)
	defer ticker.Stop()
	for {
		if !processAccountTask(ctx, client, lease, operationGate) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processAccountTask(ctx context.Context, client *telegram.Client, lease *sysin.AccountLease, operationGate sync.Locker) bool {
	renewed, err := collectorservice.AccountLeaseManager().Renew(ctx, lease, 0)
	if err != nil {
		g.Log().Warningf(ctx, "续期Telegram账号任务租约失败 tgAccountId:%d err:%+v", lease.AccountID, err)
		return false
	}
	if !renewed {
		g.Log().Warningf(ctx, "Telegram账号任务租约已失效，停止领取任务 tgAccountId:%d epoch:%d", lease.AccountID, lease.Epoch)
		return false
	}
	tasks, err := collectorservice.AccountTasks().Claim(ctx, lease, 1, accountTaskLeaseTTL)
	if err != nil {
		g.Log().Warningf(ctx, "领取Telegram账号任务失败 tgAccountId:%d err:%+v", lease.AccountID, err)
		return true
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		handler := collectorservice.AccountTaskHandlerFor(task.TaskType)
		if handler == nil {
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
			stopRenew()
			cancel()
			if handleErr == nil {
				err = collectorservice.AccountTasks().Complete(ctx, task.ID, lease, result)
			} else {
				err = collectorservice.AccountTasks().Fail(ctx, &sysin.AccountTaskFailure{
					TaskID: task.ID, Lease: lease, Cause: handleErr, RetryDelay: retryDelay(handleErr),
				})
			}
		}
		if err != nil {
			g.Log().Warningf(ctx, "提交Telegram账号任务结果失败 taskId:%d type:%s err:%+v", task.ID, task.TaskType, err)
		}
	}
	return true
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
	case sysin.AccountTaskTypeMediaDownload:
		return 10 * time.Minute
	default:
		return 5 * time.Minute
	}
}
