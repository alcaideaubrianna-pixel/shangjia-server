package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
)

const CollectRepairSuspendedStatus = "suspended_realtime_relief"

// CollectRepairCandidate describes a proposed association, not content approval.
var collectRepairSuspendedStatuses = []string{CollectRepairSuspendedStatus, "suspended_verify_repair"}

func isCollectRepairSuspended(status string) bool {
	for _, suspended := range collectRepairSuspendedStatuses {
		if status == suspended {
			return true
		}
	}
	return false
}

type CollectRepairCandidate struct {
	DisplayID int64  `json:"displayId"`
	VerifyID  int64  `json:"verifyId"`
	SourceID  int64  `json:"sourceId"`
	TenantID  int64  `json:"tenantId"`
	AccountID int64  `json:"accountId"`
	MessageID int64  `json:"messageId"`
	ChatID    string `json:"chatId"`
}

type CollectRepairMedia struct {
	ID      int64  `json:"id"`
	EventID int64  `json:"eventId"`
	Kind    string `json:"kind"`
	URL     string `json:"-"`
	Status  string `json:"status"`
}

// ScanSuspendedCollectRepair is read-only and uses the existing classifier.
// Association is scoped to tenant, owner, Telegram account and original channel.
func ScanSuspendedCollectRepair(ctx context.Context, tenantID, accountID int64, since, until time.Time, limit int, sourceIDs ...int64) ([]CollectRepairCandidate, []int64, error) {
	if limit <= 0 || limit > 100000 || (!since.IsZero() && !until.After(since)) {
		return nil, nil, gerror.New("修复范围不合法或超过100000条")
	}
	cols := pdao.YoubanPublishCollectEvent.Columns()
	model := pdao.YoubanPublishCollectEvent.Ctx(ctx).WhereIn(cols.Status, collectRepairSuspendedStatuses)
	if len(sourceIDs) > 0 && sourceIDs[0] > 0 {
		model = model.Where(cols.SourceId, sourceIDs[0])
	}
	if tenantID > 0 {
		model = model.Where(cols.TenantId, tenantID)
	}
	if accountID > 0 {
		model = model.Where(cols.AccountId, accountID)
	}
	if !since.IsZero() {
		model = model.WhereGTE(cols.CreatedAt, since).WhereLT(cols.CreatedAt, until)
	}
	// Exclude large raw payloads: pairing only needs text, origin and timestamps.
	rows, err := model.Fields(cols.Id, cols.TenantId, cols.AccountId, cols.TgAccountId, cols.SourceId, cols.SourceChatId, cols.SourceMessageId, cols.RawText, cols.ReceivedAt, cols.CreatedAt).OrderAsc(cols.TenantId).OrderAsc(cols.AccountId).OrderAsc(cols.SourceId).
		OrderAsc(cols.SourceChatId).OrderAsc(cols.SourceMessageId).OrderAsc(cols.Id).Limit(limit + 1).All()
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取挂起事件失败")
	}
	if len(rows) > limit {
		return nil, nil, gerror.New("挂起事件超过批次限制，请缩小范围")
	}
	ids := make([]int64, 0, len(rows))
	groups := make(map[string][]gdb.Record)
	for _, row := range rows {
		ids = append(ids, row[cols.Id].Int64())
		key := fmt.Sprintf("%d:%d:%d:%d:%s", row[cols.TenantId].Int64(), row[cols.AccountId].Int64(), row[cols.TgAccountId].Int64(), row[cols.SourceId].Int64(), row[cols.SourceChatId].String())
		groups[key] = append(groups[key], row)
	}
	s := NewSysPublish()
	media, err := s.collectEventMediaItemsByEvent(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	pairs := make([]CollectRepairCandidate, 0)
	matched := map[int64]bool{}
	for _, group := range groups {
		for _, pair := range pairCollectMaterialMessages(collectMaterialEventViews(group, media)) {
			display, verify := group[pair.DisplayIndex], group[pair.VerifyIndex]
			pairs = append(pairs, CollectRepairCandidate{DisplayID: display[cols.Id].Int64(), VerifyID: verify[cols.Id].Int64(), SourceID: display[cols.SourceId].Int64(), TenantID: display[cols.TenantId].Int64(), AccountID: display[cols.AccountId].Int64(), MessageID: display[cols.SourceMessageId].Int64(), ChatID: display[cols.SourceChatId].String()})
			matched[display[cols.Id].Int64()] = true
			matched[verify[cols.Id].Int64()] = true
		}
	}
	unresolved := []int64{}
	for _, id := range ids {
		if !matched[id] {
			unresolved = append(unresolved, id)
		}
	}
	return pairs, unresolved, nil
}

// PrepareSuspendedCollectRepair downloads through the existing leased account
// runtime. Its task keys intentionally bypass normal collect completion hooks,
// so no ordinary media queue or publication is triggered while held.
func PrepareSuspendedCollectRepair(ctx context.Context, eventID int64, download bool) ([]CollectRepairMedia, error) {
	s := NewSysPublish()
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(eventID).One()
	if err != nil {
		return nil, err
	}
	if event.IsEmpty() || !isCollectRepairSuspended(event["status"].String()) {
		return nil, gerror.New("目标事件不处于修复挂起状态")
	}
	rows, err := s.collectEventMediaRows(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows) != event["media_count"].Int() {
		return nil, gerror.New("原消息媒体组不完整，必须回读后修复")
	}
	result := make([]CollectRepairMedia, 0, len(rows))
	for _, row := range rows {
		if download && (row.CacheStatus != "ready" || row.FileUrl == "") {
			if event["tg_account_id"].Int64() <= 0 {
				cached, downloadErr := s.downloadBotTelegramMedia(ctx, event["tenant_id"].Int64(), event["bot_id"].Int64(), collectMediaRowsToItems([]*collectEventMediaRow{row}, "")[0])
				if downloadErr != nil {
					return nil, gerror.Wrap(downloadErr, "修复BOT媒体下载失败")
				}
				if cached == nil || cached.FileUrl == "" {
					return nil, gerror.New("修复BOT媒体下载结果为空")
				}
				cols := pdao.YoubanPublishCollectEventMedia.Columns()
				_, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where(cols.Id, row.Id).Data(pdo.YoubanPublishCollectEventMedia{
					CacheStatus: "ready", FileUrl: cached.FileUrl, StoragePath: cached.Path, PosterUrl: cached.Item.PosterUrl, ErrorMessage: "",
				}).Update()
				if err != nil {
					return nil, gerror.Wrap(err, "写入BOT修复媒体缓存失败")
				}
				row.FileUrl, row.CacheStatus = cached.FileUrl, "ready"
				result = append(result, CollectRepairMedia{ID: row.Id, EventID: eventID, Kind: row.MediaType, URL: row.FileUrl, Status: row.CacheStatus})
				continue
			}
			item := collectorMediaItemFromCollect(collectMediaRowsToItems([]*collectEventMediaRow{row}, "")[0])
			// Resolve each original message again rather than trusting its old
			// file reference. No Telegram session is opened by this process.
			if row.SourceMessageRef != "" {
				item.FileID = row.SourceMessageRef
			}
			item.SourceMediaID = 0
			item.SourceFileReference = nil
			task, taskErr := collectorservice.AccountTasks().SubmitAndWait(ctx, &collectorin.AccountTaskSubmit{
				TenantID: event["tenant_id"].Int64(), AccountID: event["tg_account_id"].Int64(), MediaOwnerAccountID: event["account_id"].Int64(),
				TaskType: collectorin.AccountTaskTypeMediaDownload, TaskKey: fmt.Sprintf("suspended-repair-media:%d", row.Id), Priority: collectorin.EventPriorityRealtime, Media: &item, MaxAttempts: 5,
			}, time.Second)
			if taskErr != nil {
				return nil, gerror.Wrap(taskErr, "修复媒体下载失败")
			}
			if task == nil || task.Status != collectorin.AccountTaskStatusCompleted || task.MediaResult.ErrorCode != "" || task.MediaResult.FileURL == "" {
				return nil, gerror.Newf("修复媒体未下载成功 mediaId:%d", row.Id)
			}
			cols := pdao.YoubanPublishCollectEventMedia.Columns()
			_, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where(cols.Id, row.Id).Data(pdo.YoubanPublishCollectEventMedia{
				CacheStatus: "ready", FileUrl: task.MediaResult.FileURL, StoragePath: task.MediaResult.StoragePath, PosterUrl: task.MediaResult.PosterURL, ErrorMessage: "",
				SourceFileId: task.MediaResult.Media.FileID, SourceKind: task.MediaResult.Media.SourceKind,
				SourceMediaId: task.MediaResult.Media.SourceMediaID, SourceAccessHash: task.MediaResult.Media.SourceAccessHash,
				SourceFileReference: task.MediaResult.Media.SourceFileReference, SourceThumbSize: task.MediaResult.Media.SourceThumbSize,
				SourceDcId: task.MediaResult.Media.SourceDCID, SourceSize: task.MediaResult.Media.SourceSize,
			}).Update()
			if err != nil {
				return nil, gerror.Wrap(err, "写入修复媒体缓存失败")
			}
			row.FileUrl = task.MediaResult.FileURL
			row.CacheStatus = "ready"
		}
		result = append(result, CollectRepairMedia{ID: row.Id, EventID: eventID, Kind: row.MediaType, URL: row.FileUrl, Status: row.CacheStatus})
	}
	if download {
		if err = s.syncCollectEventMediaSnapshot(ctx, eventID); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ReleaseSuspendedCollectRepair must be called only after external content
// approval and actual media decode. All group changes happen atomically.
func ReleaseSuspendedCollectRepair(ctx context.Context, tenantID, accountID int64, pair CollectRepairCandidate) error {
	s := NewSysPublish()
	err := pdao.YoubanPublishCollectEvent.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		cols := pdao.YoubanPublishCollectEvent.Columns()
		rows, err := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).WhereIn(cols.Id, []int64{pair.DisplayID, pair.VerifyID}).Where(cols.TenantId, tenantID).Where(cols.AccountId, accountID).OrderAsc(cols.Id).LockUpdate().All()
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			return gerror.New("修复资料归属不一致")
		}
		byID := map[int64]gdb.Record{}
		for _, row := range rows {
			byID[row[cols.Id].Int64()] = row
			if !isCollectRepairSuspended(row[cols.Status].String()) {
				return gerror.New("资料状态已改变，拒绝解除挂起")
			}
		}
		display, verify := byID[pair.DisplayID], byID[pair.VerifyID]
		for _, field := range []string{cols.SourceId, cols.SourceChatId, cols.TgAccountId} {
			if display[field].String() != verify[field].String() {
				return gerror.New("展示和验证来源不一致")
			}
		}
		if display[cols.SourceId].Int64() != pair.SourceID || verify[cols.SourceMessageId].Int64() <= display[cols.SourceMessageId].Int64() {
			return gerror.New("修复配对消息顺序不合法")
		}
		if parent := verify["material_parent_event_id"].Int64(); parent > 0 && parent != pair.DisplayID {
			return gerror.New("验证视频已关联其他资料")
		}
		media, err := s.collectEventMediaItemsByEvent(ctx, []int64{pair.DisplayID, pair.VerifyID})
		if err != nil {
			return err
		}
		proposed := pairCollectMaterialMessages(collectMaterialEventViews([]gdb.Record{display, verify}, media))
		if len(proposed) != 1 {
			return gerror.New("修复资料分类或验证窗口不合法")
		}
		for _, event := range []gdb.Record{display, verify} {
			mcols := pdao.YoubanPublishCollectEventMedia.Columns()
			count, err := tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).Where(mcols.EventId, event[cols.Id].Int64()).Where(mcols.CacheStatus, "ready").Where(mcols.FileUrl+" <> ?", "").Count()
			if err != nil {
				return err
			}
			if count <= 0 || count != event[cols.MediaCount].Int() {
				return gerror.New("展示或验证媒体组未完整下载")
			}
		}
		roleColumns := struct {
			Role   string `orm:"material_role"`
			Parent int64  `orm:"material_parent_event_id"`
			Group  string `orm:"material_group_status"`
		}{Role: collectMaterialRoleDisplay, Group: "paired"}
		if _, err = tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).Where(cols.Id, pair.DisplayID).Data(roleColumns).Update(); err != nil {
			return err
		}
		roleColumns.Role = collectMaterialRoleVerify
		roleColumns.Parent = pair.DisplayID
		if _, err = tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).Where(cols.Id, pair.VerifyID).Data(roleColumns).Update(); err != nil {
			return err
		}
		for _, id := range []int64{pair.DisplayID, pair.VerifyID} {
			if _, err = tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).Where(cols.Id, id).WhereIn(cols.Status, collectRepairSuspendedStatuses).Data(pdo.YoubanPublishCollectEvent{Status: "media_ready", ErrorMessage: ""}).Update(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return gerror.Wrap(err, "解除修复资料挂起失败")
	}
	s.appendCollectEventLog(ctx, pair.DisplayID, "repair", "verified", "来源顺序关联及媒体完整解码通过", fmt.Sprintf("verifyEventId=%d", pair.VerifyID))
	_, err = RequeueCollectEvents(ctx, []int64{pair.DisplayID})
	return err
}

// WaitCollectRepairDispatch prevents later repair groups overtaking an active send.
// Reviewing groups are not published; skipped groups produce no outgoing messages.
func WaitCollectRepairDispatch(ctx context.Context, eventID int64) error {
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	cols := pdao.YoubanPublishCollectDispatch.Columns()
	for {
		rows, err := pdao.YoubanPublishCollectDispatch.Ctx(waitCtx).Where(cols.EventId, eventID).All()
		if err != nil {
			return gerror.Wrap(err, "读取修复推送状态失败")
		}
		if len(rows) == 0 {
			eventCols := pdao.YoubanPublishCollectEvent.Columns()
			event, readErr := pdao.YoubanPublishCollectEvent.Ctx(waitCtx).WherePri(eventID).One()
			if readErr != nil {
				return gerror.Wrap(readErr, "读取修复事件终态失败")
			}
			if !event.IsEmpty() && event[eventCols.Status].String() == "ignored" {
				g.Log().Info(waitCtx, "修复事件已跳过，继续后续资料", g.Map{"eventId": eventID, "reason": event[eventCols.ErrorMessage].String()})
				return nil
			}
			return gerror.New("修复未产生入库派发记录，停止后续推送")
		}
		complete := true
		for _, row := range rows {
			switch row[cols.Status].String() {
			case "sent", "skipped", "reviewing":
			case "failed":
				return gerror.Newf("事件 %d 推送失败，停止后续推送", eventID)
			default:
				complete = false
			}
		}
		if complete {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return gerror.Wrap(waitCtx.Err(), "等待修复推送完成超时，停止后续推送")
		case <-ticker.C:
		}
	}
}
