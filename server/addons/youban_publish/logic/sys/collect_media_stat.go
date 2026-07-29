package sys

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) recordCollectMediaPerformance(ctx context.Context, event gdb.Record, startedAt time.Time, taskErr error) {
	if event.IsEmpty() || event["id"].Int64() <= 0 {
		return
	}
	rows, err := g.DB().Model("hg_youban_publish_collect_event_media").Safe().Ctx(ctx).
		Fields("cache_status", "download_duration_ms", "download_bytes", "download_attempts", "cache_hit", "download_error_type", "error_message").
		Where("event_id", event["id"].Int64()).All()
	if err != nil {
		g.Log().Warningf(ctx, "写入采集媒体性能统计读取媒体失败 eventId:%d err:%+v", event["id"].Int64(), err)
		return
	}
	mediaTotal := len(rows)
	if mediaTotal == 0 {
		return
	}
	finishedAt := time.Now()
	successCount, failedCount, pendingCount, cacheHitCount, retryCount := 0, 0, 0, 0, 0
	var bytes int64
	durations := make([]int64, 0, len(rows))
	errorsByType := make(map[string]int)
	for _, row := range rows {
		status := strings.TrimSpace(row["cache_status"].String())
		switch status {
		case collectMediaCacheReady:
			successCount++
		case collectMediaCacheFailed:
			failedCount++
		default:
			pendingCount++
		}
		if row["cache_hit"].Int() > 0 {
			cacheHitCount++
		}
		attempts := row["download_attempts"].Int()
		if attempts > 1 {
			retryCount += attempts - 1
		}
		bytes += row["download_bytes"].Int64()
		if duration := row["download_duration_ms"].Int64(); duration > 0 {
			durations = append(durations, duration)
		}
		errorType := strings.TrimSpace(row["download_error_type"].String())
		if errorType == "" && status == collectMediaCacheFailed {
			errorType = collectMediaErrorType(row["error_message"].String())
		}
		if errorType != "" {
			errorsByType[errorType]++
		}
	}
	if taskErr != nil && failedCount == 0 && pendingCount == 0 {
		errorsByType[collectMediaErrorType(taskErr.Error())]++
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	durationMs := finishedAt.Sub(startedAt).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}
	p50Ms, p95Ms := percentileDuration(durations, 0.50), percentileDuration(durations, 0.95)
	throughput := float64(0)
	if durationMs > 0 {
		throughput = float64(bytes) * 8 / float64(durationMs) / 1000
	}
	successRate := float64(successCount) / float64(mediaTotal)
	failureRate := float64(failedCount) / float64(mediaTotal)
	errorJSON, _ := json.Marshal(errorsByType)
	status := "ready"
	if failedCount > 0 || taskErr != nil {
		status = collectMediaCacheFailed
	} else if pendingCount > 0 {
		status = collectMediaCachePending
	}
	data := g.Map{
		"tenant_id":          event["tenant_id"].Int64(),
		"account_id":         event["account_id"].Int64(),
		"tg_account_id":      event["tg_account_id"].Int64(),
		"source_id":          event["source_id"].Int64(),
		"event_id":           event["id"].Int64(),
		"status":             status,
		"media_total":        mediaTotal,
		"success_count":      successCount,
		"failed_count":       failedCount,
		"pending_count":      pendingCount,
		"cache_hit_count":    cacheHitCount,
		"retry_count":        retryCount,
		"bytes":              bytes,
		"duration_ms":        durationMs,
		"p50_ms":             p50Ms,
		"p95_ms":             p95Ms,
		"throughput_mbps":    throughput,
		"success_rate":       successRate,
		"failure_rate":       failureRate,
		"error_summary_json": string(errorJSON),
		"started_at":         gtime.NewFromTime(startedAt),
		"finished_at":        gtime.NewFromTime(finishedAt),
		"updated_at":         gtime.Now(),
	}
	statModel := g.DB().Model("hg_youban_publish_collect_media_stat").Safe().Ctx(ctx).Where("event_id", event["id"].Int64())
	if count, countErr := statModel.Count(); countErr == nil && count > 0 {
		if _, updateErr := statModel.Data(data).Update(); updateErr != nil {
			g.Log().Warningf(ctx, "更新采集媒体性能统计失败 eventId:%d err:%+v", event["id"].Int64(), updateErr)
		}
		return
	}
	data["created_at"] = gtime.Now()
	if _, insertErr := g.DB().Model("hg_youban_publish_collect_media_stat").Safe().Ctx(ctx).Data(data).Insert(); insertErr != nil {
		g.Log().Warningf(ctx, "新增采集媒体性能统计失败 eventId:%d err:%+v", event["id"].Int64(), insertErr)
	}
}

func collectMediaErrorType(message string) string {
	message = strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(message, "flood_wait"), strings.Contains(message, "too many requests"), strings.Contains(message, "限流"):
		return "rate_limit"
	case strings.Contains(message, "auth_bytes_invalid"):
		return "auth_bytes_invalid"
	case strings.Contains(message, "file_migrate"):
		return "file_migrate"
	case strings.Contains(message, "dc is closed"):
		return "dc_closed"
	case strings.Contains(message, "context canceled"):
		return "context_canceled"
	case strings.Contains(message, "timeout"), strings.Contains(message, "deadline exceeded"):
		return "timeout"
	case message != "":
		return "other"
	default:
		return ""
	}
}
