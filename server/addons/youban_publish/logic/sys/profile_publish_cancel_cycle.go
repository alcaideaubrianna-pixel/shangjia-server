package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// Cycle creation locks its Run before profile_state. Retire the Run in a
// separate transaction, before the state/Job settlement lock, to preserve that
// order. The committed cancellation tombstone already fences Job generation.
func (s *sSysPublish) stopCanceledProfileCycleRun(ctx context.Context, intent profilePublishCancel) error {
	if !strings.HasPrefix(intent.OperationNo, "profile_cycle:") {
		return nil // Channel batch runs include other profiles; never cancel the batch.
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		run, err := tx.Model(publishProfileCycleRunTable).Ctx(ctx).
			Where("run_key", strings.TrimPrefix(intent.OperationNo, "profile_cycle:")).
			Where("tenant_id", intent.TenantID).Where("account_id", intent.AccountID).
			Where("profile_id", intent.ProfileID).Fields("id").LockUpdate().One()
		if err != nil || run.IsEmpty() {
			return err
		}
		if _, err := tx.Model(publishProfileCycleRunTable).Ctx(ctx).
			Where("id", run["id"].Int64()).Where("tenant_id", intent.TenantID).
			Where("account_id", intent.AccountID).Where("profile_id", intent.ProfileID).
			Data(g.Map{"status": "canceled", "error_message": "资料本轮发布已取消"}).Update(); err != nil {
			return err
		}
		_, err = tx.Model(publishProfileCycleRunChannelTable).Ctx(ctx).
			Where("run_id", run["id"].Int64()).Where("tenant_id", intent.TenantID).
			Where("account_id", intent.AccountID).Where("profile_id", intent.ProfileID).
			Data(g.Map{"status": "superseded", "error_message": "资料本轮发布已取消"}).Update()
		return err
	})
}
