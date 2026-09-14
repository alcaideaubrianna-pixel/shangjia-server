package sys

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	hglock "hotgo/internal/library/hgrds/lock"
)

const duplicateScanGlobalLockKey = "youban_publish:duplicate_scan:worker"

func (s *sSysPublish) handleDuplicateScanTask(ctx context.Context, task *asynq.Task) error {
	var payload duplicateScanQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return gerror.Wrap(err, "解析重复资料扫描任务失败")
	}
	if payload.Token == "" || payload.Account.Id <= 0 || payload.Account.TenantId <= 0 {
		return gerror.New("重复资料扫描任务参数不完整")
	}
	if duplicateScanTaskSuperseded(ctx, payload.Token, &payload.Account, &payload.Input) {
		g.Log().Info(ctx, "跳过已被新任务替代的重复资料扫描", g.Map{
			"scanTaskId": payload.Token, "tenantId": payload.Account.TenantId, "accountId": payload.Account.Id,
		})
		return nil
	}
	lease := hglock.Mutex(duplicateScanGlobalLockKey)
	if err := lease.Lock(ctx); err != nil {
		return gerror.Wrap(err, "等待重复资料扫描执行槽失败")
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := lease.Unlock(unlockCtx); err != nil {
			g.Log().Warning(ctx, "释放重复资料扫描执行槽失败", g.Map{"scanTaskId": payload.Token, "err": err})
		}
	}()
	err := s.runDuplicateScan(ctx, payload.Token, &payload.Account, &payload.Input)
	if err != nil {
		if session, loadErr := loadDuplicateScanSession(ctx, payload.Token); loadErr == nil {
			retryCount, _ := asynq.GetRetryCount(ctx)
			maxRetry, _ := asynq.GetMaxRetry(ctx)
			session.Status = "queued"
			session.Error = "扫描遇到临时错误，系统正在自动重试"
			if retryCount >= maxRetry {
				session.Status = "failed"
				session.Error = "重复资料扫描失败，请重新发起扫描"
			}
			_ = saveDuplicateScanProgress(ctx, payload.Token, duplicateScanMetadata(session))
		}
		g.Log().Warning(ctx, "重复资料扫描任务失败", g.Map{
			"scanTaskId": payload.Token, "tenantId": payload.Account.TenantId,
			"accountId": payload.Account.Id, "err": err,
		})
		return err
	}
	g.Log().Info(ctx, "重复资料扫描任务完成", g.Map{
		"scanTaskId": payload.Token, "tenantId": payload.Account.TenantId, "accountId": payload.Account.Id,
	})
	return nil
}
