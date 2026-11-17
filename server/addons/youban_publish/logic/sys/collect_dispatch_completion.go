package sys

import (
	"fmt"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

// Callers pass internal table aliases only. A single channel receipt does not
// finish a multi-channel rule; every expected target must have its own receipt.
func collectDispatchAllJobsSentCondition(alias string) string {
	// Preserve exact-account receipts, including legacy raw0 -> raw0.
	// A raw0 dispatch may acknowledge a positive-account job only when that
	// exact tenant/profile has an active state owned by the job's account.
	// The correlated lookup starts with profile_id; never infer ownership
	// from the source, a different profile, or an arbitrary account in tenant.
	accountReceipt := fmt.Sprintf(`(j.account_id=%s.account_id OR (
   %s.account_id=0 AND j.account_id>0
   AND EXISTS (SELECT 1 FROM %s receipt_owner
    WHERE receipt_owner.profile_id=j.profile_id AND receipt_owner.tenant_id=j.tenant_id
     AND receipt_owner.account_id=j.account_id AND receipt_owner.deleted_at IS NULL)
  ))`, alias, alias, publishProfileStateTable)
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM %s expected WHERE expected.dispatch_id=%s.id)
AND NOT EXISTS (
 SELECT 1 FROM %s expected WHERE expected.dispatch_id=%s.id
 AND NOT EXISTS (
  SELECT 1 FROM %s j
  WHERE j.tenant_id=%s.tenant_id AND %s
   AND j.profile_id=%s.profile_id AND j.collect_event_id=%s.event_id
   AND j.channel_id=expected.channel_id AND j.operation_no=CONCAT('collect:',%s.id)
   AND j.status='sent' AND j.dispatch_status='done'
 ))`, collectDispatchChannelTable, alias, collectDispatchChannelTable, alias,
		publishTgJobTable, alias, accountReceipt, alias, alias, alias)
}

// Parent completion is a summary of every persisted branch, never the first
// success callback. Unknown or failed branch states deliberately prevent it.
func collectEventAllDispatchesFinishedCondition(alias string) string {
	table := pdao.YoubanPublishCollectDispatch.Table()
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM %s branch WHERE branch.event_id=%s.id)
AND NOT EXISTS (SELECT 1 FROM %s branch WHERE branch.event_id=%s.id
 AND (branch.status IS NULL OR branch.status NOT IN ('sent','skipped')))`, table, alias, table, alias)
}
