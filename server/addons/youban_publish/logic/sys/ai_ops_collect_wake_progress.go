package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// Inspect exactly the immutable plan membership, never the entire source or
// pending table. Admission completion is separate from collection completion.
func (s *sSysPublish) aiOpsCollectWakeProgress(ctx context.Context, payload aiOpsRepairPlanPayload) (*sysin.AIOpsCollectWakeProgress, error) {
	ids := uniqueIds(payload.EventIDs)
	if payload.Action != aiOpsCollectWakeAction || payload.TenantID <= 0 || payload.AccountID <= 0 || payload.SourceID <= 0 ||
		len(ids) == 0 || len(ids) > 200 || len(ids) != len(payload.EventIDs) || len(payload.CollectWakeEvents) != len(ids) {
		return nil, gerror.New("业务采集唤醒进度缺少有效的限定事件范围")
	}
	expected := make(map[int64]*sysin.AIOpsCollectWakePreviewItem, len(ids))
	for _, item := range payload.CollectWakeEvents {
		if item == nil || item.EventId <= 0 || item.TenantId != payload.TenantID || item.OwnerAccountId != payload.AccountID ||
			item.SourceId != payload.SourceID || item.Identity == "" || !item.Decision.Valid() || expected[item.EventId] != nil {
			return nil, gerror.New("业务采集唤醒计划的事件身份不完整")
		}
		expected[item.EventId] = item
	}
	for _, id := range ids {
		if expected[id] == nil {
			return nil, gerror.New("业务采集唤醒计划的事件清单不一致")
		}
	}
	cols := pdao.YoubanPublishCollectEvent.Columns()
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields(cols.Id, cols.TenantId, cols.AccountId, cols.SourceId, cols.SourceChatId, cols.SourceMessageId,
			cols.SourceUniqueKey, cols.CreatedAt, cols.Status, cols.ProcessedAt).
		WhereIn(cols.Id, ids).Where(cols.TenantId, payload.TenantID).Where(cols.SourceId, payload.SourceID).
		WhereIn(cols.AccountId, []int64{0, payload.AccountID}).All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取业务采集唤醒进度失败")
	}
	progress := &sysin.AIOpsCollectWakeProgress{Total: len(ids), Missing: len(ids)}
	for _, row := range rows {
		item := expected[row[cols.Id].Int64()]
		if item == nil || item.AccountId != row[cols.AccountId].Int64() || collectWakeEventIdentity(row) != item.Identity {
			continue
		}
		progress.Missing--
		switch strings.TrimSpace(row[cols.Status].String()) {
		case sysin.CollectEventStatusIgnored:
			progress.Ignored++
		case sysin.CollectEventStatusDispatched:
			progress.Dispatched++
		case sysin.CollectEventStatusProcessed, "matched":
			progress.Processed++
		case sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusWaitingOrder,
			sysin.CollectEventStatusPrechecked, sysin.CollectEventStatusMediaPending, sysin.CollectEventStatusMediaReady:
			progress.Pending++
		default:
			// Includes failed and unsupported/suspended states: never claim a
			// drained backlog merely because the old worker cannot handle them.
			progress.Failed++
		}
	}
	return progress, nil
}
