package sys

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
)

const (
	pullMonitorDelay             = 3 * time.Minute
	pullMonitorDefaultRange      = 30 * time.Minute
	pullMonitorMaxQueryRange     = 72 * time.Hour
	pullMonitorCacheTTL          = 7 * 24 * time.Hour
	pullMonitorMaxRecentEvents   = 5000
	pullMonitorPendingKey        = "lazysheep_tggo:pull_monitor:pending"
	pullMonitorSnapshotKey       = "lazysheep_tggo:pull_monitor:snapshot"
	pullMonitorLockKey           = "lazysheep_tggo:pull_monitor:lock"
	pullMonitorAggregatorTick    = 15 * time.Second
	pullMonitorBucketIntervalMin = int64(60)
)

var pullMonitorAggregatorOnce sync.Once

func (s *sLazySheepTGGo) StartPullMonitorAggregator(ctx context.Context) {
	pullMonitorAggregatorOnce.Do(func() {
		go s.runPullMonitorAggregator(ctx)
	})
}

func (s *sLazySheepTGGo) runPullMonitorAggregator(ctx context.Context) {
	ticker := time.NewTicker(pullMonitorAggregatorTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = aggregatePullMonitorEvents(ctx)
		}
	}
}

func recordPullMonitorEvent(ctx context.Context, event *sysin.PullMonitorEvent) {
	if event == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	event.VisibleAtUnix = time.Now().Add(pullMonitorDelay).Unix()
	mutex := lock.NewConfig(10*time.Second, 100*time.Millisecond).Mutex(pullMonitorLockKey)
	if err := mutex.Lock(writeCtx); err != nil {
		g.Log().Warningf(ctx, "记录拉取监控加锁失败 trace:%s err:%+v", event.TraceID, err)
		return
	}
	defer unlockPullMonitor(writeCtx, mutex)
	pending := loadPullMonitorPending(writeCtx)
	pending = append(pending, event)
	if err := cache.Instance().Set(writeCtx, pullMonitorPendingKey, pending, pullMonitorCacheTTL); err != nil {
		g.Log().Warningf(ctx, "写入拉取监控缓存失败 trace:%s err:%+v", event.TraceID, err)
	}
}

func aggregatePullMonitorEvents(ctx context.Context) error {
	mutex := lock.NewConfig(10*time.Second, 100*time.Millisecond).Mutex(pullMonitorLockKey)
	if err := mutex.TryLock(ctx); err != nil {
		if gerror.Is(err, lock.ErrLockFailed) {
			return nil
		}
		return err
	}
	defer unlockPullMonitor(ctx, mutex)
	return aggregatePullMonitorEventsLocked(ctx)
}

func aggregatePullMonitorEventsLocked(ctx context.Context) error {
	pending := loadPullMonitorPending(ctx)
	if len(pending) == 0 {
		return nil
	}
	now := time.Now().Unix()
	remaining := make([]*sysin.PullMonitorEvent, 0, len(pending))
	ready := make([]*sysin.PullMonitorEvent, 0, len(pending))
	for _, item := range pending {
		if item == nil {
			continue
		}
		if item.VisibleAtUnix > now {
			remaining = append(remaining, item)
			continue
		}
		ready = append(ready, item)
	}
	if len(ready) == 0 {
		return nil
	}
	snapshot := loadPullMonitorSnapshot(ctx)
	for _, item := range ready {
		mergePullMonitorEvent(snapshot, item)
	}
	prunePullMonitorSnapshot(snapshot, time.Now().Add(-pullMonitorCacheTTL).Unix())
	_ = cache.Instance().Set(ctx, pullMonitorPendingKey, remaining, pullMonitorCacheTTL)
	return cache.Instance().Set(ctx, pullMonitorSnapshotKey, snapshot, pullMonitorCacheTTL)
}

func (s *sLazySheepTGGo) PullMonitor(ctx context.Context, in *sysin.PullMonitorInp) (*sysin.PullMonitorModel, error) {
	if err := aggregatePullMonitorEvents(ctx); err != nil {
		return nil, err
	}
	botKey := ""
	section := ""
	if in != nil {
		botKey = strings.TrimSpace(in.BotKey)
		section = strings.ToLower(strings.TrimSpace(in.Section))
	}
	snapshot := loadPullMonitorSnapshot(ctx)
	startAt, endAt := pullMonitorTimeRange(in)
	res := &sysin.PullMonitorModel{
		Bindings: []*sysin.PullMonitorBindingSummary{},
		Buckets:  []*sysin.PullMonitorBucket{},
		Recent:   []*sysin.PullMonitorEvent{},
	}
	if section != "" {
		return s.enrichPullMonitorLabels(ctx, buildPullMonitorSection(snapshot, botKey, section, startAt, endAt, res)), nil
	}
	if botKey == "" {
		res.Buckets = filterPullMonitorBuckets(snapshot.Buckets, startAt, endAt)
		res.Summary = summaryFromPullMonitorBuckets(res.Buckets)
		for _, item := range snapshot.Recent {
			if !pullMonitorEventMatched(item, "", startAt, endAt) {
				continue
			}
			res.Recent = append(res.Recent, item)
		}
		res.Bindings = buildPullMonitorBindingSummaries(res.Recent)
		return s.enrichPullMonitorLabels(ctx, res), nil
	}
	var elapsedTotal int64
	for _, item := range snapshot.Recent {
		if !pullMonitorEventMatched(item, botKey, startAt, endAt) {
			continue
		}
		res.Recent = append(res.Recent, item)
		res.Summary.Total++
		elapsedTotal += item.ElapsedMs
		if item.Success {
			res.Summary.Success++
		} else {
			res.Summary.Failed++
		}
	}
	if res.Summary.Total > 0 {
		res.Summary.AvgElapsedMs = elapsedTotal / int64(res.Summary.Total)
	}
	res.Bindings = buildPullMonitorBindingSummaries(res.Recent)
	res.Buckets = buildPullMonitorBuckets(res.Recent, startAt, endAt)
	return s.enrichPullMonitorLabels(ctx, res), nil
}

func buildPullMonitorSection(snapshot *sysin.PullMonitorModel, botKey string, section string, startAt int64, endAt int64, res *sysin.PullMonitorModel) *sysin.PullMonitorModel {
	if snapshot == nil || res == nil {
		return res
	}
	switch section {
	case "overview":
		if botKey == "" {
			res.Buckets = filterPullMonitorBuckets(snapshot.Buckets, startAt, endAt)
			res.Summary = summaryFromPullMonitorBuckets(res.Buckets)
			return res
		}
		events := filterPullMonitorEvents(snapshot.Recent, botKey, startAt, endAt)
		res.Buckets = buildPullMonitorBuckets(events, startAt, endAt)
		res.Summary = summaryFromPullMonitorEvents(events)
	case "bindings":
		events := filterPullMonitorEvents(snapshot.Recent, botKey, startAt, endAt)
		res.Bindings = buildPullMonitorBindingSummaries(events)
	case "recent":
		res.Recent = filterPullMonitorEvents(snapshot.Recent, botKey, startAt, endAt)
	default:
		events := filterPullMonitorEvents(snapshot.Recent, botKey, startAt, endAt)
		res.Recent = events
		res.Bindings = buildPullMonitorBindingSummaries(events)
		res.Buckets = buildPullMonitorBuckets(events, startAt, endAt)
		res.Summary = summaryFromPullMonitorEvents(events)
	}
	return res
}

func filterPullMonitorEvents(items []*sysin.PullMonitorEvent, botKey string, startAt int64, endAt int64) []*sysin.PullMonitorEvent {
	out := make([]*sysin.PullMonitorEvent, 0)
	for _, item := range items {
		if !pullMonitorEventMatched(item, botKey, startAt, endAt) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func summaryFromPullMonitorEvents(items []*sysin.PullMonitorEvent) sysin.PullMonitorSummary {
	var out sysin.PullMonitorSummary
	var elapsedTotal int64
	for _, item := range items {
		if item == nil {
			continue
		}
		out.Total++
		elapsedTotal += item.ElapsedMs
		if item.Success {
			out.Success++
		} else {
			out.Failed++
		}
	}
	if out.Total > 0 {
		out.AvgElapsedMs = elapsedTotal / int64(out.Total)
	}
	return out
}

func loadPullMonitorPending(ctx context.Context) []*sysin.PullMonitorEvent {
	val, err := cache.Instance().Get(ctx, pullMonitorPendingKey)
	if err != nil || val.IsNil() {
		return []*sysin.PullMonitorEvent{}
	}
	var out []*sysin.PullMonitorEvent
	_ = val.Scan(&out)
	if out == nil {
		return []*sysin.PullMonitorEvent{}
	}
	return out
}

func loadPullMonitorSnapshot(ctx context.Context) *sysin.PullMonitorModel {
	val, err := cache.Instance().Get(ctx, pullMonitorSnapshotKey)
	if err != nil || val.IsNil() {
		return &sysin.PullMonitorModel{
			Bindings: []*sysin.PullMonitorBindingSummary{},
			Recent:   []*sysin.PullMonitorEvent{},
		}
	}
	var out sysin.PullMonitorModel
	_ = val.Scan(&out)
	if out.Bindings == nil {
		out.Bindings = []*sysin.PullMonitorBindingSummary{}
	}
	if out.Buckets == nil {
		out.Buckets = []*sysin.PullMonitorBucket{}
	}
	if out.Recent == nil {
		out.Recent = []*sysin.PullMonitorEvent{}
	}
	return &out
}

func mergePullMonitorEvent(snapshot *sysin.PullMonitorModel, event *sysin.PullMonitorEvent) {
	if snapshot == nil || event == nil {
		return
	}
	if event.CreatedAtUnix == 0 {
		event.CreatedAtUnix = parsePullMonitorTime(event.CreatedAt)
	}
	snapshot.Recent = append([]*sysin.PullMonitorEvent{event}, snapshot.Recent...)
	if len(snapshot.Recent) > pullMonitorMaxRecentEvents {
		snapshot.Recent = snapshot.Recent[:pullMonitorMaxRecentEvents]
	}
	snapshot.Summary.Total++
	if event.Success {
		snapshot.Summary.Success++
	} else {
		snapshot.Summary.Failed++
	}
	if snapshot.Summary.Total > 0 {
		prevCount := int64(snapshot.Summary.Total - 1)
		snapshot.Summary.AvgElapsedMs = (snapshot.Summary.AvgElapsedMs*prevCount + event.ElapsedMs) / int64(snapshot.Summary.Total)
	}
	mergePullMonitorBucket(snapshot, event)
	snapshot.Bindings = buildPullMonitorBindingSummaries(snapshot.Recent)
}

func mergePullMonitorBucket(snapshot *sysin.PullMonitorModel, event *sysin.PullMonitorEvent) {
	if snapshot == nil || event == nil {
		return
	}
	ts := event.CreatedAtUnix
	if ts == 0 {
		ts = parsePullMonitorTime(event.CreatedAt)
	}
	if ts == 0 {
		return
	}
	slot := ts - ts%pullMonitorBucketIntervalMin
	var item *sysin.PullMonitorBucket
	for _, bucket := range snapshot.Buckets {
		if bucket == nil {
			continue
		}
		if bucket.TimeUnix == slot {
			item = bucket
			break
		}
	}
	if item == nil {
		item = &sysin.PullMonitorBucket{Time: time.Unix(slot, 0).Format("15:04"), TimeUnix: slot}
		snapshot.Buckets = append(snapshot.Buckets, item)
	}
	prevCount := int64(item.Total)
	item.Total++
	if event.Success {
		item.Success++
	} else {
		item.Failed++
	}
	item.Fetched += event.Fetched
	item.Stored += event.Stored
	item.Pushed += event.Pushed
	item.PushFailed += event.PushFailed
	mergePullMonitorBucketSteps(item, event.Steps)
	item.AvgElapsedMs = (item.AvgElapsedMs*prevCount + event.ElapsedMs) / int64(item.Total)
	sort.SliceStable(snapshot.Buckets, func(i, j int) bool {
		if snapshot.Buckets[i] == nil {
			return false
		}
		if snapshot.Buckets[j] == nil {
			return true
		}
		return snapshot.Buckets[i].TimeUnix < snapshot.Buckets[j].TimeUnix
	})
}

func unlockPullMonitor(ctx context.Context, mutex *lock.Lock) {
	if err := mutex.Unlock(ctx); err != nil && !gerror.Is(err, lock.ErrNotExist) {
		g.Log().Warningf(ctx, "释放拉取监控锁失败 err:%+v", err)
	}
}

func newPullMonitorEvent(ctx context.Context, in *sysin.PullInp, bindingKey string, sourceURL string, auto bool, started time.Time, steps []sysin.PullMonitorStep, summary *pullSummary, message string, err error) *sysin.PullMonitorEvent {
	event := &sysin.PullMonitorEvent{
		TraceID:       pullTraceID(ctx),
		BindingKey:    bindingKey,
		SourceURL:     sourceURL,
		Auto:          auto,
		Success:       err == nil,
		Message:       strings.TrimSpace(message),
		ElapsedMs:     time.Since(started).Milliseconds(),
		Steps:         steps,
		CreatedAt:     time.Now().Format("2006-01-02 15:04:05"),
		CreatedAtUnix: time.Now().Unix(),
	}
	if in != nil {
		event.BotKey = in.BotKey
		event.ChatID = in.ChatID
	}
	if err != nil {
		event.Error = err.Error()
	}
	if summary != nil {
		if event.Error == "" {
			event.Error = summary.ErrorText()
		}
		event.Fetched = summary.Fetched
		event.Stored = summary.Stored
		event.Pushed = summary.Pushed
		event.Deduped = summary.Deduped
		event.Skipped = summary.Skipped
		event.FailedCount = summary.Failed
		event.PushFailed = summary.PushFailed
	}
	return event
}

func pullMonitorTimeRange(in *sysin.PullMonitorInp) (int64, int64) {
	now := time.Now()
	endAt := now.Unix()
	startAt := now.Add(-pullMonitorDefaultRange).Unix()
	if in == nil {
		return startAt, endAt
	}
	if ts := parsePullMonitorTime(strings.TrimSpace(in.EndAt)); ts > 0 {
		endAt = ts
	}
	if ts := parsePullMonitorTime(strings.TrimSpace(in.StartAt)); ts > 0 {
		startAt = ts
	}
	if endAt <= 0 || endAt > now.Unix() {
		endAt = now.Unix()
	}
	minStart := endAt - int64(pullMonitorMaxQueryRange.Seconds())
	if startAt < minStart {
		startAt = minStart
	}
	if startAt >= endAt {
		startAt = endAt - int64(pullMonitorDefaultRange.Seconds())
	}
	return startAt, endAt
}

func pullMonitorEventMatched(event *sysin.PullMonitorEvent, botKey string, startAt, endAt int64) bool {
	if event == nil {
		return false
	}
	if botKey != "" && event.BotKey != botKey {
		return false
	}
	createdAt := event.CreatedAtUnix
	if createdAt == 0 {
		createdAt = parsePullMonitorTime(event.CreatedAt)
	}
	return createdAt >= startAt && createdAt <= endAt
}

func parsePullMonitorTime(text string) int64 {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, text, time.Local); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func buildPullMonitorBindingSummaries(events []*sysin.PullMonitorEvent) []*sysin.PullMonitorBindingSummary {
	items := make(map[string]*sysin.PullMonitorBindingSummary)
	for _, event := range events {
		if event == nil {
			continue
		}
		key := event.BotKey + ":" + event.BindingKey
		item := items[key]
		if item == nil {
			item = &sysin.PullMonitorBindingSummary{
				BotKey:     event.BotKey,
				BindingKey: event.BindingKey,
				SourceURL:  event.SourceURL,
				ChatID:     event.ChatID,
			}
			items[key] = item
		}
		prevCount := int64(item.Total)
		item.Total++
		if event.Success {
			item.Success++
		} else {
			item.Failed++
		}
		item.Fetched += event.Fetched
		item.Stored += event.Stored
		item.Pushed += event.Pushed
		item.FailedCount += event.FailedCount
		item.PushFailed += event.PushFailed
		item.LastStatus = event.Success
		item.LastError = event.Error
		item.LastAt = event.CreatedAt
		item.AvgElapsedMs = (item.AvgElapsedMs*prevCount + event.ElapsedMs) / int64(item.Total)
	}
	res := make([]*sysin.PullMonitorBindingSummary, 0, len(items))
	for _, item := range items {
		res = append(res, item)
	}
	sort.SliceStable(res, func(i, j int) bool {
		return res[i].LastAt > res[j].LastAt
	})
	return res
}

func buildPullMonitorBuckets(events []*sysin.PullMonitorEvent, startAt, endAt int64) []*sysin.PullMonitorBucket {
	buckets := make(map[int64]*sysin.PullMonitorBucket)
	for ts := startAt - startAt%pullMonitorBucketIntervalMin; ts <= endAt; ts += pullMonitorBucketIntervalMin {
		buckets[ts] = &sysin.PullMonitorBucket{
			Time:     time.Unix(ts, 0).Format("15:04"),
			TimeUnix: ts,
		}
	}
	elapsed := make(map[int64]int64)
	stepTotals := make(map[int64]map[string]int64)
	stepCounts := make(map[int64]map[string]int)
	for _, event := range events {
		if event == nil {
			continue
		}
		ts := event.CreatedAtUnix
		if ts == 0 {
			ts = parsePullMonitorTime(event.CreatedAt)
		}
		slot := ts - ts%pullMonitorBucketIntervalMin
		item := buckets[slot]
		if item == nil {
			item = &sysin.PullMonitorBucket{Time: time.Unix(slot, 0).Format("15:04"), TimeUnix: slot}
			buckets[slot] = item
		}
		item.Total++
		if event.Success {
			item.Success++
		} else {
			item.Failed++
		}
		item.Fetched += event.Fetched
		item.Stored += event.Stored
		item.Pushed += event.Pushed
		item.PushFailed += event.PushFailed
		elapsed[slot] += event.ElapsedMs
		item.AvgElapsedMs = elapsed[slot] / int64(item.Total)
		for _, step := range event.Steps {
			name := normalizePullMonitorStepName(step.Name)
			if name == "" {
				continue
			}
			if stepTotals[slot] == nil {
				stepTotals[slot] = make(map[string]int64)
				stepCounts[slot] = make(map[string]int)
			}
			stepTotals[slot][name] += step.StepMs
			stepCounts[slot][name]++
		}
	}
	for slot, item := range buckets {
		item.Steps = buildPullMonitorStepStats(stepTotals[slot], stepCounts[slot])
	}
	res := make([]*sysin.PullMonitorBucket, 0, len(buckets))
	for _, item := range buckets {
		res = append(res, item)
	}
	sort.SliceStable(res, func(i, j int) bool {
		return res[i].TimeUnix < res[j].TimeUnix
	})
	return res
}

func mergePullMonitorBucketSteps(bucket *sysin.PullMonitorBucket, steps []sysin.PullMonitorStep) {
	if bucket == nil || len(steps) == 0 {
		return
	}
	totals := make(map[string]int64)
	counts := make(map[string]int)
	for _, item := range bucket.Steps {
		if item == nil || item.Name == "" {
			continue
		}
		totals[item.Name] += item.AvgMs * int64(item.Count)
		counts[item.Name] += item.Count
	}
	for _, step := range steps {
		name := normalizePullMonitorStepName(step.Name)
		if name == "" {
			continue
		}
		totals[name] += step.StepMs
		counts[name]++
	}
	bucket.Steps = buildPullMonitorStepStats(totals, counts)
}

func buildPullMonitorStepStats(totals map[string]int64, counts map[string]int) []*sysin.PullMonitorStepStat {
	if len(totals) == 0 {
		return nil
	}
	names := make([]string, 0, len(totals))
	for name := range totals {
		names = append(names, name)
	}
	sort.Strings(names)
	res := make([]*sysin.PullMonitorStepStat, 0, len(names))
	for _, name := range names {
		count := counts[name]
		if count <= 0 {
			continue
		}
		res = append(res, &sysin.PullMonitorStepStat{Name: name, AvgMs: totals[name] / int64(count), Count: count})
	}
	return res
}

func normalizePullMonitorStepName(text string) string {
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return ""
	case strings.Contains(text, "代理配置"):
		return "配置代理"
	case strings.Contains(text, "已拉取第"):
		return "BangChat拉取"
	case strings.Contains(text, "正在处理"):
		return "处理消息"
	case strings.Contains(text, "旧笔记"):
		return "游标判断"
	case strings.Contains(text, "重复笔记"):
		return "去重判断"
	case strings.Contains(text, "入库并加入推送队列"):
		return "入库/入队"
	default:
		if len(text) > 24 {
			return text[:24]
		}
		return text
	}
}

func filterPullMonitorBuckets(buckets []*sysin.PullMonitorBucket, startAt, endAt int64) []*sysin.PullMonitorBucket {
	existing := make(map[int64]*sysin.PullMonitorBucket)
	for _, item := range buckets {
		if item == nil || item.TimeUnix < startAt || item.TimeUnix > endAt {
			continue
		}
		existing[item.TimeUnix] = item
	}
	res := make([]*sysin.PullMonitorBucket, 0)
	for ts := startAt - startAt%pullMonitorBucketIntervalMin; ts <= endAt; ts += pullMonitorBucketIntervalMin {
		if item := existing[ts]; item != nil {
			res = append(res, item)
			continue
		}
		res = append(res, &sysin.PullMonitorBucket{Time: time.Unix(ts, 0).Format("15:04"), TimeUnix: ts})
	}
	return res
}

func summaryFromPullMonitorBuckets(buckets []*sysin.PullMonitorBucket) sysin.PullMonitorSummary {
	var res sysin.PullMonitorSummary
	var elapsedTotal int64
	for _, item := range buckets {
		if item == nil {
			continue
		}
		res.Total += item.Total
		res.Success += item.Success
		res.Failed += item.Failed
		elapsedTotal += int64(item.Total) * item.AvgElapsedMs
	}
	if res.Total > 0 {
		res.AvgElapsedMs = elapsedTotal / int64(res.Total)
	}
	return res
}

func prunePullMonitorSnapshot(snapshot *sysin.PullMonitorModel, before int64) {
	if snapshot == nil || before <= 0 {
		return
	}
	recent := make([]*sysin.PullMonitorEvent, 0, len(snapshot.Recent))
	var elapsedTotal int64
	snapshot.Summary = sysin.PullMonitorSummary{}
	for _, event := range snapshot.Recent {
		if event == nil {
			continue
		}
		ts := event.CreatedAtUnix
		if ts == 0 {
			ts = parsePullMonitorTime(event.CreatedAt)
			event.CreatedAtUnix = ts
		}
		if ts < before {
			continue
		}
		recent = append(recent, event)
		snapshot.Summary.Total++
		elapsedTotal += event.ElapsedMs
		if event.Success {
			snapshot.Summary.Success++
		} else {
			snapshot.Summary.Failed++
		}
	}
	if snapshot.Summary.Total > 0 {
		snapshot.Summary.AvgElapsedMs = elapsedTotal / int64(snapshot.Summary.Total)
	}
	snapshot.Recent = recent
	snapshot.Bindings = buildPullMonitorBindingSummaries(snapshot.Recent)
	buckets := make([]*sysin.PullMonitorBucket, 0, len(snapshot.Buckets))
	for _, item := range snapshot.Buckets {
		if item == nil || item.TimeUnix < before {
			continue
		}
		buckets = append(buckets, item)
	}
	snapshot.Buckets = buckets
}
