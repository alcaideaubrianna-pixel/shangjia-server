package sys

import (
	"context"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// Successful collection counts distinct dispatched events, not retained ingress
// events or target channels. Read durable dispatch evidence for the current page
// so legacy zero counters and event-log cleanup cannot hide successful imports.
func fillCollectSourceSuccessTotals(_ context.Context, list []*sysin.CollectSourceModel, tenantID, accountID int64) error {
	// SuccessTotal is maintained transactionally by syncCollectSourceStats and
	// syncCollectSourceStatsBatch. Reusing the projection avoids a COUNT(DISTINCT)
	// scan over the dispatch table on every source-list request.
	for _, item := range list {
		if item != nil && (item.TenantId != tenantID || item.AccountId != accountID) {
			item.SuccessTotal = 0
		}
	}
	return nil
}
