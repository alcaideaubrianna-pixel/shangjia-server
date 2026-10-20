package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/utility/sensitive"
)

type collectProgressContextKey struct{}
type collectProgressReporter func(context.Context, string, string)

// Progress is diagnostic only. It never changes the state used by recovery.
func reportCollectProgress(ctx context.Context, stage, message string) {
	if report, ok := ctx.Value(collectProgressContextKey{}).(collectProgressReporter); ok {
		report(ctx, stage, message)
	}
}

func (s *sSysPublish) collectProgressContext(ctx context.Context, eventID, mediaID int64) context.Context {
	if eventID <= 0 {
		return ctx
	}
	ec := pdao.YoubanPublishCollectEvent.Columns()
	owner, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(eventID).Fields(ec.TenantId, ec.AccountId).One()
	if err != nil || owner.IsEmpty() {
		g.Log().Warningf(ctx, "读取采集进度归属失败 eventId:%d err:%v", eventID, err)
		return ctx
	}
	return context.WithValue(ctx, collectProgressContextKey{}, collectProgressReporter(func(callCtx context.Context, stage, message string) {
		if mediaID > 0 {
			message = fmt.Sprintf("媒体 #%d：%s", mediaID, message)
		}
		s.appendCollectEventLogWithOwner(callCtx, eventID, owner[ec.TenantId].Int64(), owner[ec.AccountId].Int64(), "progress", stage, sensitive.Diagnostic(message), fmt.Sprintf("mediaId=%d", mediaID))
		g.Log().Info(callCtx, "采集处理阶段更新", g.Map{"eventId": eventID, "mediaId": mediaID, "stage": stage, "message": sensitive.Diagnostic(message)})
	}))
}

func collectProgressLabel(stage string) string {
	labels := map[string]string{
		"waiting_owner": "等待媒体处理配额", "waiting_interval": "等待账号下载节流", "waiting_commit": "等待资料提交锁", "extract": "解析资料内容", "commit": "资料入库与分发",
		"queued": "等待处理队列", "scheduled": "等待定时重试", "claimed": "已投递处理队列", "lease_expired": "处理任务租约过期",
		"processing": "资料分组与规则检查", "waiting": "等待条件满足", "waiting_lock": "等待事件处理锁", "failed": "处理失败",
		"waiting_slot": "等待下载并发槽", "get_file": "获取Telegram文件信息", "download": "下载媒体文件", "account_download": "账号媒体任务执行中",
		"fingerprint": "计算媒体指纹", "upload": "上传云存储", "persist": "写回媒体结果", "media_queued": "等待媒体处理队列",
		"media_ready": "单个媒体处理完成", "ready": "媒体已就绪，等待后续处理", "finished": "处理结束",
		"dedupe": "匹配重复", "ignored": "匹配忽略",
	}
	if label := labels[stage]; label != "" {
		return label
	}
	return stage
}

func finishCollectProgress(p *sysin.CollectEventProgressModel, now time.Time) {
	p.Label = collectProgressLabel(p.Stage)
	if p.Since != nil && !p.Since.IsZero() {
		p.ElapsedSeconds = int64(now.Sub(p.Since.Time).Seconds())
		if p.ElapsedSeconds < 0 {
			p.ElapsedSeconds = 0
		}
	}
	// A slow phase is not proof of a deadlock. Expose its age without claiming it is stuck.
	p.NoProgressWarning = p.ElapsedSeconds >= 120 && p.Stage != "finished" && p.Stage != "dedupe" && p.Stage != "ignored" && p.Stage != "scheduled"
}

func (s *sSysPublish) fillCollectEventProgress(ctx context.Context, list []*sysin.CollectEventModel) error {
	ids := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil {
			ids = append(ids, item.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	// Page-bounded indexed reads, never a scan across the event log history.
	logDAO := pdao.YoubanPublishCollectEventLog
	lc := logDAO.Columns()
	latest := logDAO.Ctx(ctx).Fields("MAX("+lc.Id+")").WhereIn(lc.EventId, ids).Where(lc.Stage, "progress").Group(lc.EventId + "," + lc.MetaText)
	logs, err := logDAO.Ctx(ctx).WhereIn(lc.Id, latest).OrderAsc(lc.Id).All()
	if err != nil {
		return err
	}
	progress := make(map[int64]*sysin.CollectEventProgressModel, len(logs))
	mediaStages := make(map[int64]string, len(logs))
	for _, row := range logs {
		var mediaID int64
		if _, scanErr := fmt.Sscanf(row[lc.MetaText].String(), "mediaId=%d", &mediaID); scanErr == nil && mediaID > 0 {
			mediaStages[mediaID] = row[lc.Status].String()
		}
		progress[row[lc.EventId].Int64()] = &sysin.CollectEventProgressModel{Stage: row[lc.Status].String(), Message: row[lc.Message].String(), Since: row[lc.CreatedAt].GTime()}
	}
	outboxes, err := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).WhereIn("event_id", ids).Fields("event_id,status,available_at,updated_at,lease_expires_at,last_error").All()
	if err != nil && !isMissingCollectProcessOutboxError(err) {
		return err
	}
	mediaDAO := pdao.YoubanPublishCollectEventMedia
	mc := mediaDAO.Columns()
	mediaRows, mediaErr := mediaDAO.Ctx(ctx).WhereIn(mc.EventId, ids).Fields(mc.Id, mc.EventId, mc.CacheStatus, mc.DownloadAttempts, mc.DownloadBytes, mc.UpdatedAt, mc.NextRetryAt, mc.ErrorMessage).OrderAsc(mc.Id).All()
	if mediaErr != nil {
		return mediaErr
	}
	byEvent := make(map[int64][]*sysin.CollectMediaProgressModel)
	for _, row := range mediaRows {
		label := "等待媒体任务执行"
		switch row[mc.CacheStatus].String() {
		case collectMediaCacheReady:
			label = "已缓存"
		case collectMediaCacheDownloading:
			label = "正在获取、下载或转存，具体步骤见阶段日志"
		case collectMediaCacheForwarding:
			label = "正在转发备份"
		case collectMediaCacheFailed:
			label = "处理失败"
		}
		if stage := mediaStages[row[mc.Id].Int64()]; stage != "" && (row[mc.CacheStatus].String() == collectMediaCacheDownloading || row[mc.CacheStatus].String() == collectMediaCachePending) {
			label = collectProgressLabel(stage)
		}
		retryAt := row[mc.NextRetryAt].GTime()
		if retryAt != nil && !retryAt.IsZero() {
			label = "等待重试"
		}
		id := row[mc.EventId].Int64()
		byEvent[id] = append(byEvent[id], &sysin.CollectMediaProgressModel{Id: row[mc.Id].Int64(), Label: label, Attempts: row[mc.DownloadAttempts].Int(), Bytes: row[mc.DownloadBytes].Int64(), UpdatedAt: row[mc.UpdatedAt].GTime(), NextRetryAt: retryAt, Message: sensitive.Diagnostic(row[mc.ErrorMessage].String())})
	}
	now := time.Now()
	byOutbox := make(map[int64]gdb.Record, len(outboxes))
	for _, row := range outboxes {
		byOutbox[row["event_id"].Int64()] = row
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		p := collectEventProgressView(item, progress[item.Id], byOutbox[item.Id], now)
		p.Media = byEvent[item.Id]
		item.Progress = p
	}
	return nil
}

func collectEventProgressView(item *sysin.CollectEventModel, latest *sysin.CollectEventProgressModel, outbox gdb.Record, now time.Time) *sysin.CollectEventProgressModel {
	p := latest
	if p == nil {
		p = &sysin.CollectEventProgressModel{Stage: "queued", Message: "等待处理任务；尚无详细阶段记录", Since: item.CreatedAt}
	}
	// Outbox timestamps are scheduling timestamps, not worker execution heartbeats.
	for _, row := range []gdb.Record{outbox} {
		if row.IsEmpty() || row["event_id"].Int64() != item.Id {
			continue
		}
		if p.Stage == "waiting" {
			p.NextRetryAt = row["available_at"].GTime()
			break
		}
		if p.Since != nil && !row["updated_at"].GTime().After(p.Since) {
			break
		}
		switch row["status"].String() {
		case "pending":
			p = &sysin.CollectEventProgressModel{Stage: "queued", Message: "等待调度器投递到处理队列", Since: row["updated_at"].GTime(), NextRetryAt: row["available_at"].GTime()}
			if p.NextRetryAt != nil && p.NextRetryAt.After(gtime.New(now)) {
				p.Stage = "scheduled"
				p.Message = "等待计划执行时间"
			}
		case "processing":
			p = &sysin.CollectEventProgressModel{Stage: "claimed", Message: "任务已投递；队列领取不代表工作线程已开始执行", Since: row["updated_at"].GTime()}
			if expiry := row["lease_expires_at"].GTime(); expiry != nil && expiry.Before(gtime.New(now)) {
				p.Stage = "lease_expired"
				p.Message = "任务租约已过期，等待调度恢复"
			}
		}
		if reason := row["last_error"].String(); reason != "" {
			p.Message += "；最近投递错误：" + sensitive.Diagnostic(reason)
		}
		break
	}
	switch item.Status {
	case sysin.CollectEventStatusIgnored:
		stage := "ignored"
		message := sensitive.Diagnostic(item.ErrorMessage)
		if collectEventIgnoreType(item.Status, item.ErrorMessage) == sysin.CollectEventIgnoreTypeDedupe {
			stage = "dedupe"
		}
		if message == "" {
			message = collectProgressLabel(stage)
		}
		p = &sysin.CollectEventProgressModel{Stage: stage, Message: message, Since: item.ProcessedAt}
	case sysin.CollectEventStatusProcessed, sysin.CollectEventStatusDispatched:
		p = &sysin.CollectEventProgressModel{Stage: "finished", Message: "采集事件已结束", Since: item.ProcessedAt}
	case sysin.CollectEventStatusFailed:
		p = &sysin.CollectEventProgressModel{Stage: "failed", Message: sensitive.Diagnostic(item.ErrorMessage), Since: p.Since}
	default:
		if item.MediaCacheStatus == "cached" && (p.Stage == "download" || p.Stage == "upload" || p.Stage == "persist" || p.Stage == "get_file") {
			p.Stage = "ready"
			p.Message = "媒体已缓存，等待后续规则处理"
		}
	}
	finishCollectProgress(p, now)
	return p
}
