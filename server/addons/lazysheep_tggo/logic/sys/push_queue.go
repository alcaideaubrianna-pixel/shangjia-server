package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	botmodels "github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/model"
	lsysin "hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
	"hotgo/internal/library/queue"
)

const (
	pushNoteTopic         = "lazysheep_tggo:push_note"
	pushTaskStatusReady   = 1
	pushTaskStatusDoing   = 2
	pushTaskStatusDone    = 3
	pushTaskStatusRetry   = 4
	pushTaskStatusDead    = 5
	pushTaskStatusUnknown = 6
	pushTaskMaxAttempts   = 5
	pushRetryScanLimit    = 50
	pushReadyScanLimit    = 200
	pushGlobalWorkers     = 24
	pushChatInterval      = 10 * time.Second
	pushDoingTimeout      = 15 * time.Minute
	pushLogStatusSuccess  = 1
	pushLogStatusFailed   = 2
	pushLogStatusSkipped  = 3
	pushDedupStatusHold   = 1
	pushDedupStatusDone   = 2
	pushDedupStatusFailed = 3
	pushDedupTTL          = 180 * 24 * time.Hour
	pushQueuePausedKey    = "lazysheep_tggo:push_queue:paused"
)

var (
	pushQueueLoopOnce sync.Once
	pushWorkerSem     = make(chan struct{}, pushGlobalWorkers)
	pushChatLimiters  sync.Map
)

type pushChatLimiter struct {
	sync.Mutex
	last          time.Time
	nextAvailable time.Time
	running       bool
}

type pushTaskRecord struct {
	Id          int64       `json:"id" orm:"id"`
	BotKey      string      `json:"botKey" orm:"bot_key"`
	BindingKey  string      `json:"bindingKey" orm:"binding_key"`
	SourceUrl   string      `json:"sourceUrl" orm:"source_url"`
	NoteId      int64       `json:"noteId" orm:"note_id"`
	ContentId   int64       `json:"contentId" orm:"content_id"`
	ChatId      int64       `json:"chatId" orm:"chat_id"`
	Status      int         `json:"status" orm:"status"`
	Attempts    int         `json:"attempts" orm:"attempts"`
	MaxAttempts int         `json:"maxAttempts" orm:"max_attempts"`
	LastError   string      `json:"lastError" orm:"last_error"`
	CreatedAt   *gtime.Time `json:"createdAt" orm:"created_at"`
}

func (s *sLazySheepTGGo) StartPushQueueLoop(ctx context.Context) {
	pushQueueLoopOnce.Do(func() {
		if err := s.ensurePushQueueTable(ctx); err != nil {
			g.Log().Warningf(ctx, "初始化推送任务表失败，重试调度器稍后继续尝试 err:%+v", err)
		}
		if err := s.ensurePushLogTable(ctx); err != nil {
			g.Log().Warningf(ctx, "初始化推送日志表失败 err:%+v", err)
		}
		if err := s.ensurePushDedupTable(ctx); err != nil {
			g.Log().Warningf(ctx, "初始化推送去重表失败 err:%+v", err)
		}
		go s.runPushQueueRetryLoop(ctx)
	})
}

func (s *sLazySheepTGGo) runPushQueueRetryLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	g.Log().Info(ctx, "懒羊羊TGGo推送队列重试调度器已启动")
	s.dispatchPushTasks(ctx)
	for {
		select {
		case <-ctx.Done():
			g.Log().Info(ctx, "懒羊羊TGGo推送队列重试调度器已停止")
			return
		case <-ticker.C:
			s.dispatchPushTasks(ctx)
		}
	}
}

func (s *sLazySheepTGGo) dispatchPushTasks(ctx context.Context) {
	if pushQueuePaused(ctx) {
		return
	}
	if err := s.ensurePushQueueTable(ctx); err != nil {
		g.Log().Warningf(ctx, "初始化推送任务表失败，跳过本轮推送重试扫描 err:%+v", err)
		return
	}
	_ = s.ensurePushLogTable(ctx)
	_ = s.ensurePushDedupTable(ctx)
	s.recoverStalePushTasks(ctx)
	var records []*pushTaskRecord
	if err := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").
		Where("status", pushTaskStatusReady).
		OrderAsc("created_at").
		Limit(pushReadyScanLimit).
		Scan(&records); err != nil {
		if isTableNotExistError(err) {
			if ensureErr := s.ensurePushQueueTable(ctx); ensureErr != nil {
				g.Log().Warningf(ctx, "推送任务表不存在且自动创建失败 err:%+v", ensureErr)
			}
			return
		}
		g.Log().Warningf(ctx, "扫描推送重试任务失败 err:%+v", err)
		return
	}
	var retryRecords []*pushTaskRecord
	if err := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").
		Where("status", pushTaskStatusRetry).
		WhereLTE("next_retry_at", gtime.Now()).
		OrderAsc("next_retry_at").
		Limit(pushRetryScanLimit).
		Scan(&retryRecords); err != nil {
		g.Log().Warningf(ctx, "扫描推送重试任务失败 err:%+v", err)
		return
	}
	records = append(records, retryRecords...)
	records = selectDispatchablePushRecords(records)
	for _, record := range records {
		task := pushTaskFromRecord(record)
		if task == nil {
			continue
		}
		s.DispatchPushNoteTask(ctx, task)
	}
}

func (s *sLazySheepTGGo) recoverStalePushTasks(ctx context.Context) {
	_, err := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").
		Where("status", pushTaskStatusDoing).
		WhereLT("started_at", gtime.New(time.Now().Add(-pushDoingTimeout))).
		Data(g.Map{
			"status":        pushTaskStatusRetry,
			"last_error":    "推送任务执行超时，已重新调度",
			"next_retry_at": gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		g.Log().Warningf(ctx, "恢复超时推送任务失败 err:%+v", err)
	}
}

func (s *sLazySheepTGGo) PushQueueMonitor(ctx context.Context, in *lsysin.PushQueueMonitorInp) (*lsysin.PushQueueMonitorModel, error) {
	if err := s.ensurePushQueueTable(ctx); err != nil {
		return nil, err
	}
	_ = s.ensurePushLogTable(ctx)
	limit := 50
	botKey := ""
	var chatID int64
	if in != nil {
		botKey = strings.TrimSpace(in.BotKey)
		chatID = in.ChatID
		if in.Limit > 0 && in.Limit <= 200 {
			limit = in.Limit
		}
	}
	res := &lsysin.PushQueueMonitorModel{
		Paused:     pushQueuePaused(ctx),
		Summary:    initPushQueueStatusCounts(),
		Channels:   []*lsysin.PushQueueChannelModel{},
		Recent:     []*lsysin.PushQueueTaskModel{},
		FailedLogs: []*lsysin.PushQueueLogModel{},
	}
	if err := s.fillPushQueueSummary(ctx, res, botKey, chatID); err != nil {
		return nil, err
	}
	if err := s.fillPushQueueChannels(ctx, res, botKey, chatID, limit); err != nil {
		return nil, err
	}
	if err := s.fillPushQueueRecent(ctx, res, botKey, chatID, limit); err != nil {
		return nil, err
	}
	if err := s.fillPushQueueFailedLogs(ctx, res, botKey, chatID, limit); err != nil {
		return nil, err
	}
	s.enrichPushQueueMonitorLabels(ctx, res)
	return res, nil
}

func (s *sLazySheepTGGo) UpdatePushQueueControl(ctx context.Context, in *lsysin.PushQueueControlInp) error {
	paused := false
	if in != nil {
		switch strings.ToLower(strings.TrimSpace(in.Action)) {
		case "pause", "stop", "disable":
			paused = true
		case "resume", "start", "enable":
			paused = false
		default:
			paused = in.Paused
		}
	}
	if paused {
		return cache.Instance().Set(ctx, pushQueuePausedKey, "1", 0)
	}
	_, err := cache.Instance().Remove(ctx, pushQueuePausedKey)
	return err
}

func (s *sLazySheepTGGo) fillPushQueueSummary(ctx context.Context, res *lsysin.PushQueueMonitorModel, botKey string, chatID int64) error {
	var rows []struct {
		Status int `json:"status" orm:"status"`
		Count  int `json:"count" orm:"count"`
	}
	mod := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").Fields("status, COUNT(*) AS count").Group("status")
	mod = filterPushQueueMonitorModel(mod, botKey, chatID)
	if err := mod.Scan(&rows); err != nil {
		return gerror.Wrap(err, "查询推送队列汇总失败")
	}
	index := make(map[int]*lsysin.PushQueueStatusCount, len(res.Summary))
	for _, item := range res.Summary {
		index[item.Status] = item
	}
	for _, row := range rows {
		if item, ok := index[row.Status]; ok {
			item.Count = row.Count
			continue
		}
		res.Summary = append(res.Summary, &lsysin.PushQueueStatusCount{
			Status: row.Status,
			Label:  pushQueueStatusLabel(row.Status),
			Count:  row.Count,
		})
	}
	return nil
}

func (s *sLazySheepTGGo) fillPushQueueChannels(ctx context.Context, res *lsysin.PushQueueMonitorModel, botKey string, chatID int64, limit int) error {
	var rows []struct {
		BotKey     string      `json:"botKey" orm:"bot_key"`
		BindingKey string      `json:"bindingKey" orm:"binding_key"`
		ChatID     int64       `json:"chatId" orm:"chat_id"`
		Ready      int         `json:"ready" orm:"ready"`
		Doing      int         `json:"doing" orm:"doing"`
		Retry      int         `json:"retry" orm:"retry"`
		Done       int         `json:"done" orm:"done"`
		Dead       int         `json:"dead" orm:"dead"`
		Unknown    int         `json:"unknown" orm:"unknown"`
		Backlog    int         `json:"backlog" orm:"backlog"`
		LastError  string      `json:"lastError" orm:"last_error"`
		OldestAt   *gtime.Time `json:"oldestAt" orm:"oldest_at"`
	}
	mod := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").
		Fields("bot_key,binding_key,chat_id," +
			"SUM(CASE WHEN status=1 THEN 1 ELSE 0 END) AS ready," +
			"SUM(CASE WHEN status=2 THEN 1 ELSE 0 END) AS doing," +
			"SUM(CASE WHEN status=3 THEN 1 ELSE 0 END) AS done," +
			"SUM(CASE WHEN status=4 THEN 1 ELSE 0 END) AS retry," +
			"SUM(CASE WHEN status=5 THEN 1 ELSE 0 END) AS dead," +
			"SUM(CASE WHEN status=6 THEN 1 ELSE 0 END) AS unknown," +
			"SUM(CASE WHEN status IN (1,2,4) THEN 1 ELSE 0 END) AS backlog," +
			"MAX(last_error) AS last_error,MIN(created_at) AS oldest_at").
		Group("bot_key,binding_key,chat_id").
		OrderDesc("backlog").
		Limit(limit)
	mod = filterPushQueueMonitorModel(mod, botKey, chatID)
	if err := mod.Scan(&rows); err != nil {
		return gerror.Wrap(err, "查询推送频道队列失败")
	}
	for _, row := range rows {
		res.Channels = append(res.Channels, &lsysin.PushQueueChannelModel{
			BotKey:     row.BotKey,
			BindingKey: row.BindingKey,
			ChatID:     row.ChatID,
			Ready:      row.Ready,
			Doing:      row.Doing,
			Retry:      row.Retry,
			Done:       row.Done,
			Dead:       row.Dead,
			Unknown:    row.Unknown,
			Backlog:    row.Backlog,
			LastError:  limitPushLogError(row.LastError),
			OldestAt:   formatPushQueueTime(row.OldestAt),
		})
	}
	return nil
}

func (s *sLazySheepTGGo) fillPushQueueRecent(ctx context.Context, res *lsysin.PushQueueMonitorModel, botKey string, chatID int64, limit int) error {
	var rows []struct {
		Id          int64       `json:"id" orm:"id"`
		BotKey      string      `json:"botKey" orm:"bot_key"`
		BindingKey  string      `json:"bindingKey" orm:"binding_key"`
		SourceURL   string      `json:"sourceUrl" orm:"source_url"`
		NoteID      int64       `json:"noteId" orm:"note_id"`
		ContentID   int64       `json:"contentId" orm:"content_id"`
		ChatID      int64       `json:"chatId" orm:"chat_id"`
		Status      int         `json:"status" orm:"status"`
		Attempts    int         `json:"attempts" orm:"attempts"`
		MaxAttempts int         `json:"maxAttempts" orm:"max_attempts"`
		LastError   string      `json:"lastError" orm:"last_error"`
		CreatedAt   *gtime.Time `json:"createdAt" orm:"created_at"`
		StartedAt   *gtime.Time `json:"startedAt" orm:"started_at"`
		FinishedAt  *gtime.Time `json:"finishedAt" orm:"finished_at"`
		NextRetryAt *gtime.Time `json:"nextRetryAt" orm:"next_retry_at"`
	}
	mod := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").OrderDesc("id").Limit(limit)
	mod = filterPushQueueMonitorModel(mod, botKey, chatID)
	if err := mod.Scan(&rows); err != nil {
		return gerror.Wrap(err, "查询最近推送任务失败")
	}
	for _, row := range rows {
		res.Recent = append(res.Recent, &lsysin.PushQueueTaskModel{
			Id:          row.Id,
			BotKey:      row.BotKey,
			BindingKey:  row.BindingKey,
			SourceURL:   row.SourceURL,
			NoteID:      row.NoteID,
			ContentID:   row.ContentID,
			ChatID:      row.ChatID,
			Status:      row.Status,
			StatusLabel: pushQueueStatusLabel(row.Status),
			Attempts:    row.Attempts,
			MaxAttempts: row.MaxAttempts,
			LastError:   limitPushLogError(row.LastError),
			CreatedAt:   formatPushQueueTime(row.CreatedAt),
			StartedAt:   formatPushQueueTime(row.StartedAt),
			FinishedAt:  formatPushQueueTime(row.FinishedAt),
			NextRetryAt: formatPushQueueTime(row.NextRetryAt),
		})
	}
	return nil
}

func (s *sLazySheepTGGo) fillPushQueueFailedLogs(ctx context.Context, res *lsysin.PushQueueMonitorModel, botKey string, chatID int64, limit int) error {
	var rows []struct {
		Id         int64       `json:"id" orm:"id"`
		TaskID     int64       `json:"taskId" orm:"task_id"`
		BotKey     string      `json:"botKey" orm:"bot_key"`
		BindingKey string      `json:"bindingKey" orm:"binding_key"`
		NoteID     int64       `json:"noteId" orm:"note_id"`
		ContentID  int64       `json:"contentId" orm:"content_id"`
		ChatID     int64       `json:"chatId" orm:"chat_id"`
		Status     int         `json:"status" orm:"status"`
		Attempt    int         `json:"attempt" orm:"attempt"`
		ElapsedMs  int64       `json:"elapsedMs" orm:"elapsed_ms"`
		Error      string      `json:"error" orm:"error"`
		CreatedAt  *gtime.Time `json:"createdAt" orm:"created_at"`
	}
	mod := g.DB().Model("hg_addon_lazysheep_tggo_push_log").
		Where("status", pushLogStatusFailed).
		OrderDesc("id").
		Limit(limit)
	if botKey != "" {
		mod = mod.Where("bot_key", botKey)
	}
	if chatID != 0 {
		mod = mod.Where("chat_id", chatID)
	}
	if err := mod.Scan(&rows); err != nil {
		if isTableNotExistError(err) {
			return nil
		}
		return gerror.Wrap(err, "查询推送失败日志失败")
	}
	for _, row := range rows {
		res.FailedLogs = append(res.FailedLogs, &lsysin.PushQueueLogModel{
			Id:         row.Id,
			TaskID:     row.TaskID,
			BotKey:     row.BotKey,
			BindingKey: row.BindingKey,
			NoteID:     row.NoteID,
			ContentID:  row.ContentID,
			ChatID:     row.ChatID,
			Status:     row.Status,
			Attempt:    row.Attempt,
			ElapsedMs:  row.ElapsedMs,
			Error:      limitPushLogError(row.Error),
			CreatedAt:  formatPushQueueTime(row.CreatedAt),
		})
	}
	return nil
}

func (s *sLazySheepTGGo) enqueuePushNote(ctx context.Context, botKey string, binding *model.BindingRecord, noteID int64, contentID int64, chatID int64) (*lsysin.PushNoteTask, bool, error) {
	if binding == nil {
		return nil, false, gerror.New("绑定关系为空")
	}
	targetChatID := pushTargetChatID(binding, chatID)
	if targetChatID == 0 {
		return nil, false, nil
	}
	if existing, err := s.existingPushNoteTask(ctx, botKey, binding, noteID, targetChatID); err != nil {
		return nil, false, err
	} else if existing != nil {
		g.Log().Debugf(ctx, "推送任务已存在，跳过重复入队 bot:%s binding:%s noteId:%d task:%d status:%d", botKey, binding.Key, noteID, existing.Id, existing.Status)
		recordPushTaskLog(ctx, pushTaskFromRecord(existing), pushLogStatusSkipped, existing.Attempts, 0, 0, "推送任务已存在，跳过重复入队")
		return pushTaskFromRecord(existing), false, nil
	}
	fingerprints, err := s.pushDedupFingerprints(ctx, contentID, noteID)
	if err != nil {
		return nil, false, err
	}
	if seen, err := s.pushDedupSeenAny(ctx, botKey, binding.Key, targetChatID, fingerprints); err != nil {
		return nil, false, err
	} else if seen {
		task := &lsysin.PushNoteTask{BotKey: botKey, BindingKey: binding.Key, SourceURL: binding.SourceURL, NoteID: noteID, ContentID: contentID, ChatID: targetChatID}
		recordPushTaskLog(ctx, task, pushLogStatusSkipped, 0, 0, 0, "频道内重复内容，跳过推送")
		return task, false, nil
	}
	reserved, err := s.pushDedupReserveAny(ctx, botKey, binding.Key, targetChatID, noteID, contentID, fingerprints)
	if err != nil {
		return nil, false, err
	}
	if !reserved {
		task := &lsysin.PushNoteTask{BotKey: botKey, BindingKey: binding.Key, SourceURL: binding.SourceURL, NoteID: noteID, ContentID: contentID, ChatID: targetChatID}
		recordPushTaskLog(ctx, task, pushLogStatusSkipped, 0, 0, 0, "频道内重复内容，跳过推送")
		return task, false, nil
	}
	now := gtime.Now()
	row := g.Map{
		"bot_key":      botKey,
		"binding_key":  binding.Key,
		"source_url":   binding.SourceURL,
		"note_id":      noteID,
		"content_id":   contentID,
		"chat_id":      targetChatID,
		"status":       pushTaskStatusReady,
		"attempts":     0,
		"max_attempts": pushTaskMaxAttempts,
		"created_at":   now,
		"updated_at":   now,
	}
	taskID, err := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").Data(row).InsertAndGetId()
	if err != nil {
		if !isTableNotExistError(err) {
			return nil, false, gerror.Wrap(err, "创建推送任务失败")
		}
		if ensureErr := s.ensurePushQueueTable(ctx); ensureErr != nil {
			return nil, false, gerror.Wrap(ensureErr, "初始化推送任务表失败")
		}
		taskID, err = g.DB().Model("hg_addon_lazysheep_tggo_push_queue").Data(row).InsertAndGetId()
		if err != nil {
			return nil, false, gerror.Wrap(err, "创建推送任务失败")
		}
	}
	task := &lsysin.PushNoteTask{
		TaskID:     taskID,
		BotKey:     botKey,
		BindingKey: binding.Key,
		SourceURL:  binding.SourceURL,
		NoteID:     noteID,
		ContentID:  contentID,
		ChatID:     targetChatID,
		Attempt:    0,
		QueuedAt:   time.Now().Format(time.RFC3339),
	}
	if err = queue.Push(pushNoteTopic, task); err != nil {
		nextRetryAt := gtime.New(time.Now().Add(time.Duration(pushRetryDelay(0)) * time.Second))
		_, _ = g.DB().Model("hg_addon_lazysheep_tggo_push_queue").WherePri(taskID).Data(g.Map{
			"status":        pushTaskStatusRetry,
			"last_error":    err.Error(),
			"next_retry_at": nextRetryAt,
			"updated_at":    gtime.Now(),
		}).Update()
		g.Log().Warningf(ctx, "推送任务首次投递失败，已转入重试 task:%d err:%+v", taskID, err)
	}
	if err = s.pushDedupRememberAny(ctx, botKey, binding.Key, targetChatID, noteID, contentID, fingerprints, taskID, 1); err != nil {
		g.Log().Warningf(ctx, "记录推送去重失败 task:%d err:%+v", taskID, err)
	}
	return task, true, nil
}

func (s *sLazySheepTGGo) existingPushNoteTask(ctx context.Context, botKey string, binding *model.BindingRecord, noteID int64, chatID int64) (*pushTaskRecord, error) {
	if binding == nil || noteID <= 0 || chatID == 0 {
		return nil, nil
	}
	var record *pushTaskRecord
	if err := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").
		Where("bot_key", botKey).
		Where("binding_key", binding.Key).
		Where("note_id", noteID).
		Where("chat_id", chatID).
		WhereIn("status", []int{pushTaskStatusReady, pushTaskStatusDoing, pushTaskStatusDone, pushTaskStatusRetry}).
		OrderDesc("id").
		Scan(&record); err != nil {
		if isTableNotExistError(err) {
			return nil, nil
		}
		return nil, gerror.Wrap(err, "查询推送任务幂等状态失败")
	}
	return record, nil
}

func pushTargetChatID(binding *model.BindingRecord, fallbackChatID int64) int64 {
	if binding == nil {
		return fallbackChatID
	}
	targetChatID := binding.PublishChatID
	if !binding.AutoPush && binding.ReviewChatID != 0 {
		targetChatID = binding.ReviewChatID
	}
	if targetChatID == 0 {
		targetChatID = fallbackChatID
	}
	return targetChatID
}

func (s *sLazySheepTGGo) pushDedupFingerprint(ctx context.Context, contentID int64, noteID int64) (string, error) {
	fingerprints, err := s.pushDedupFingerprints(ctx, contentID, noteID)
	if err != nil {
		return "", err
	}
	if len(fingerprints) > 0 {
		return fingerprints[0], nil
	}
	return "", nil
}

func (s *sLazySheepTGGo) pushMediaDedupFingerprint(ctx context.Context, noteID int64) (string, error) {
	fingerprints, err := s.pushMediaDedupFingerprints(ctx, noteID)
	if err != nil {
		return "", err
	}
	if len(fingerprints) == 0 {
		return "", nil
	}
	return fingerprints[0], nil
}

func (s *sLazySheepTGGo) pushDedupFingerprints(ctx context.Context, contentID int64, noteID int64) ([]string, error) {
	return s.pushMediaDedupFingerprints(ctx, noteID)
}

func (s *sLazySheepTGGo) pushMediaDedupFingerprints(ctx context.Context, noteID int64) ([]string, error) {
	if noteID <= 0 {
		return nil, nil
	}
	if err := s.ensureNoteAssetPHashField(ctx); err != nil {
		g.Log().Warningf(ctx, "确保笔记资源感知哈希字段失败 err:%+v", err)
	}
	var rows []struct {
		AssetType  string `json:"assetType" orm:"asset_type"`
		SourceUrl  string `json:"sourceUrl" orm:"source_url"`
		MediaPHash string `json:"mediaPHash" orm:"media_phash"`
	}
	if err := g.DB().Model("hg_addon_lazysheep_tggo_note_asset").
		Fields("asset_type,source_url,media_phash").
		Where("note_id", noteID).
		Where("source_url !=", "").
		WhereNull("deleted_at").
		OrderAsc("sort").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "查询笔记媒体去重指纹失败")
	}
	phashes := make([]string, 0, len(rows))
	videoURLs := make([]string, 0)
	imageCount := 0
	missingImagePHash := false
	urls := make([]string, 0, len(rows))
	for _, row := range rows {
		assetType := strings.TrimSpace(row.AssetType)
		switch assetType {
		case "image":
			imageCount++
			if phash := strings.TrimSpace(row.MediaPHash); phash != "" {
				phashes = append(phashes, phash)
			} else {
				missingImagePHash = true
			}
		case "video", "verify_video":
			if text := normalizeDedupMediaURL(row.SourceUrl); text != "" {
				videoURLs = append(videoURLs, text)
			}
		}
		if text := normalizeDedupMediaURL(row.SourceUrl); text != "" {
			urls = append(urls, text)
		}
	}
	fingerprints := make([]string, 0, 2)
	if len(urls) > 0 {
		sort.Strings(urls)
		sum := sha256.Sum256([]byte(strings.Join(urls, "\n")))
		fingerprints = append(fingerprints, "media:"+hex.EncodeToString(sum[:]))
	}
	if imageCount > 0 && !missingImagePHash && len(phashes) == imageCount {
		sort.Strings(phashes)
		sort.Strings(videoURLs)
		sum := sha256.Sum256([]byte("phash\n" + strings.Join(phashes, "\n") + "\nvideo\n" + strings.Join(videoURLs, "\n")))
		fingerprints = append(fingerprints, "media-phash:"+hex.EncodeToString(sum[:]))
	}
	return uniquePushFingerprints(fingerprints), nil
}

func uniquePushFingerprints(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func pushDedupKey(botKey string, chatID int64, fingerprint string) string {
	return fmt.Sprintf("lazysheep_tggo:push:dedup:%s:%d:%s", strings.TrimSpace(botKey), chatID, strings.TrimSpace(fingerprint))
}

func (s *sLazySheepTGGo) pushDedupSeen(ctx context.Context, botKey, bindingKey string, chatID int64, fingerprint string) (bool, error) {
	if botKey == "" || chatID == 0 || fingerprint == "" {
		return false, nil
	}
	cacheKey := pushDedupKey(botKey, chatID, fingerprint)
	if val, err := cache.Instance().Get(ctx, cacheKey); err != nil {
		return false, gerror.Wrap(err, "查询推送去重缓存失败")
	} else if !val.IsNil() && val.String() == "done" {
		return true, nil
	} else if !val.IsNil() && val.String() != "" {
		_, _ = cache.Instance().Remove(ctx, cacheKey)
	}
	if err := s.ensurePushDedupTable(ctx); err != nil {
		return false, err
	}
	var row *struct {
		Status int   `json:"status" orm:"status"`
		TaskID int64 `json:"taskId" orm:"task_id"`
	}
	if err := g.DB().Model("hg_addon_lazysheep_tggo_push_dedup").
		Fields("status,task_id").
		Where("bot_key", botKey).
		Where("chat_id", chatID).
		Where("fingerprint", fingerprint).
		WhereIn("status", []int{pushDedupStatusHold, pushDedupStatusDone}).
		OrderDesc("updated_at").
		Scan(&row); err != nil {
		return false, gerror.Wrap(err, "查询推送去重记录失败")
	}
	if row != nil && row.Status == pushDedupStatusDone {
		_ = cache.Instance().Set(ctx, cacheKey, "done", pushDedupTTL)
		return true, nil
	}
	if row != nil && row.Status == pushDedupStatusHold {
		released, releaseErr := s.releaseDeadPushDedupHold(ctx, botKey, chatID, fingerprint, row.TaskID)
		if releaseErr != nil {
			return false, releaseErr
		}
		if released {
			return false, nil
		}
	}
	return row != nil, nil
}

func (s *sLazySheepTGGo) pushDedupSeenAny(ctx context.Context, botKey, bindingKey string, chatID int64, fingerprints []string) (bool, error) {
	for _, fingerprint := range uniquePushFingerprints(fingerprints) {
		seen, err := s.pushDedupSeen(ctx, botKey, bindingKey, chatID, fingerprint)
		if err != nil || seen {
			return seen, err
		}
	}
	return false, nil
}

func (s *sLazySheepTGGo) pushDedupReserve(ctx context.Context, botKey, bindingKey string, chatID int64, noteID, contentID int64, fingerprint string) (bool, error) {
	if botKey == "" || chatID == 0 || fingerprint == "" {
		return true, nil
	}
	if err := s.ensurePushDedupTable(ctx); err != nil {
		return false, err
	}
	now := gtime.Now()
	data := g.Map{
		"bot_key":     botKey,
		"binding_key": bindingKey,
		"chat_id":     chatID,
		"note_id":     noteID,
		"content_id":  contentID,
		"fingerprint": fingerprint,
		"task_id":     0,
		"status":      pushDedupStatusHold,
		"created_at":  now,
		"updated_at":  now,
	}
	if _, err := g.DB().Model("hg_addon_lazysheep_tggo_push_dedup").Data(data).Insert(); err != nil {
		if isDuplicateKeyError(err) {
			var row *struct {
				Status int   `json:"status" orm:"status"`
				TaskID int64 `json:"taskId" orm:"task_id"`
			}
			if scanErr := g.DB().Model("hg_addon_lazysheep_tggo_push_dedup").
				Fields("status,task_id").
				Where("bot_key", botKey).
				Where("chat_id", chatID).
				Where("fingerprint", fingerprint).
				Scan(&row); scanErr != nil {
				return false, gerror.Wrap(scanErr, "查询推送去重状态失败")
			}
			if row != nil && row.Status == pushDedupStatusHold {
				released, releaseErr := s.releaseDeadPushDedupHold(ctx, botKey, chatID, fingerprint, row.TaskID)
				if releaseErr != nil {
					return false, releaseErr
				}
				if !released {
					return false, nil
				}
			}
			if row != nil && (row.Status == pushDedupStatusFailed || row.Status == pushDedupStatusHold) {
				_, updateErr := g.DB().Model("hg_addon_lazysheep_tggo_push_dedup").
					Where("bot_key", botKey).
					Where("chat_id", chatID).
					Where("fingerprint", fingerprint).
					Data(data).
					Update()
				if updateErr != nil {
					return false, gerror.Wrap(updateErr, "更新失败推送去重记录失败")
				}
				_, _ = cache.Instance().Remove(ctx, pushDedupKey(botKey, chatID, fingerprint))
				return true, nil
			}
			if row != nil && row.Status == pushDedupStatusDone {
				_ = cache.Instance().Set(ctx, pushDedupKey(botKey, chatID, fingerprint), "done", pushDedupTTL)
			}
			return false, nil
		}
		return false, gerror.Wrap(err, "写入推送去重记录失败")
	}
	_, _ = cache.Instance().Remove(ctx, pushDedupKey(botKey, chatID, fingerprint))
	return true, nil
}

func (s *sLazySheepTGGo) releaseDeadPushDedupHold(ctx context.Context, botKey string, chatID int64, fingerprint string, taskID int64) (bool, error) {
	if strings.TrimSpace(botKey) == "" || chatID == 0 || strings.TrimSpace(fingerprint) == "" || taskID <= 0 {
		return false, nil
	}
	count, err := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").
		Where("id", taskID).
		Where("status", pushTaskStatusDead).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "查询推送死信任务失败")
	}
	if count == 0 {
		return false, nil
	}
	if _, err = g.DB().Model("hg_addon_lazysheep_tggo_push_dedup").
		Where("bot_key", botKey).
		Where("chat_id", chatID).
		Where("fingerprint", fingerprint).
		Where("status", pushDedupStatusHold).
		Data(g.Map{"status": pushDedupStatusFailed, "updated_at": gtime.Now()}).
		Update(); err != nil {
		return false, gerror.Wrap(err, "释放死信推送去重占位失败")
	}
	_, _ = cache.Instance().Remove(ctx, pushDedupKey(botKey, chatID, fingerprint))
	return true, nil
}

func (s *sLazySheepTGGo) pushDedupReserveAny(ctx context.Context, botKey, bindingKey string, chatID int64, noteID, contentID int64, fingerprints []string) (bool, error) {
	fingerprints = uniquePushFingerprints(fingerprints)
	if len(fingerprints) == 0 {
		return true, nil
	}
	for _, fingerprint := range fingerprints {
		reserved, err := s.pushDedupReserve(ctx, botKey, bindingKey, chatID, noteID, contentID, fingerprint)
		if err != nil || !reserved {
			return reserved, err
		}
	}
	return true, nil
}

func (s *sLazySheepTGGo) pushDedupRemember(ctx context.Context, botKey, bindingKey string, chatID int64, noteID, contentID int64, fingerprint string, taskID int64, status int) error {
	if botKey == "" || chatID == 0 || fingerprint == "" {
		return nil
	}
	if err := s.ensurePushDedupTable(ctx); err != nil {
		return err
	}
	now := gtime.Now()
	result, err := g.DB().Model("hg_addon_lazysheep_tggo_push_dedup").
		Where("bot_key", botKey).
		Where("chat_id", chatID).
		Where("fingerprint", fingerprint).
		Data(g.Map{
			"binding_key": bindingKey,
			"note_id":     noteID,
			"content_id":  contentID,
			"task_id":     taskID,
			"status":      status,
			"updated_at":  now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新推送去重记录失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		data := g.Map{
			"bot_key":     botKey,
			"binding_key": bindingKey,
			"chat_id":     chatID,
			"note_id":     noteID,
			"content_id":  contentID,
			"fingerprint": fingerprint,
			"task_id":     taskID,
			"status":      status,
			"created_at":  now,
			"updated_at":  now,
		}
		if _, err = g.DB().Model("hg_addon_lazysheep_tggo_push_dedup").Data(data).Insert(); err != nil && !isDuplicateKeyError(err) {
			return gerror.Wrap(err, "写入推送去重记录失败")
		}
	}
	if status == pushDedupStatusDone {
		return cache.Instance().Set(ctx, pushDedupKey(botKey, chatID, fingerprint), "done", pushDedupTTL)
	}
	_, err = cache.Instance().Remove(ctx, pushDedupKey(botKey, chatID, fingerprint))
	return err
}

func (s *sLazySheepTGGo) pushDedupRememberAny(ctx context.Context, botKey, bindingKey string, chatID int64, noteID, contentID int64, fingerprints []string, taskID int64, status int) error {
	for _, fingerprint := range uniquePushFingerprints(fingerprints) {
		if err := s.pushDedupRemember(ctx, botKey, bindingKey, chatID, noteID, contentID, fingerprint, taskID, status); err != nil {
			return err
		}
	}
	return nil
}

func (s *sLazySheepTGGo) pushDedupMarkDone(ctx context.Context, botKey string, chatID int64, contentID int64, noteID int64) error {
	fingerprints, err := s.pushDedupFingerprints(ctx, contentID, noteID)
	if err != nil {
		return err
	}
	if botKey == "" || chatID == 0 || len(fingerprints) == 0 {
		return nil
	}
	_, err = g.DB().Model("hg_addon_lazysheep_tggo_push_dedup").
		Where("bot_key", botKey).
		Where("chat_id", chatID).
		WhereIn("fingerprint", uniquePushFingerprints(fingerprints)).
		Data(g.Map{"status": pushDedupStatusDone, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return err
	}
	for _, fingerprint := range uniquePushFingerprints(fingerprints) {
		_ = cache.Instance().Set(ctx, pushDedupKey(botKey, chatID, fingerprint), "done", pushDedupTTL)
	}
	return nil
}

func (s *sLazySheepTGGo) pushDedupMarkFailed(ctx context.Context, task *lsysin.PushNoteTask) error {
	if task == nil || task.BotKey == "" || task.ChatID == 0 {
		return nil
	}
	fingerprints, err := s.pushDedupFingerprints(ctx, task.ContentID, task.NoteID)
	if err != nil {
		return err
	}
	fingerprints = uniquePushFingerprints(fingerprints)
	if len(fingerprints) == 0 {
		return nil
	}
	if _, err = g.DB().Model("hg_addon_lazysheep_tggo_push_dedup").
		Where("bot_key", task.BotKey).
		Where("chat_id", task.ChatID).
		WhereIn("fingerprint", fingerprints).
		Where("status !=", pushDedupStatusDone).
		Data(g.Map{"status": pushDedupStatusFailed, "updated_at": gtime.Now()}).
		Update(); err != nil {
		return gerror.Wrap(err, "标记推送去重失败状态失败")
	}
	for _, fingerprint := range fingerprints {
		_, _ = cache.Instance().Remove(ctx, pushDedupKey(task.BotKey, task.ChatID, fingerprint))
	}
	return nil
}

func recordPushTaskLog(ctx context.Context, task *lsysin.PushNoteTask, status int, attempt int, elapsedMs int64, messageID int, errText string) {
	if task == nil {
		return
	}
	now := gtime.Now()
	_, err := g.DB().Model("hg_addon_lazysheep_tggo_push_log").Data(g.Map{
		"task_id":     task.TaskID,
		"bot_key":     task.BotKey,
		"binding_key": task.BindingKey,
		"note_id":     task.NoteID,
		"content_id":  task.ContentID,
		"chat_id":     task.ChatID,
		"status":      status,
		"attempt":     attempt,
		"message_id":  messageID,
		"elapsed_ms":  elapsedMs,
		"error":       limitPushLogError(errText),
		"created_at":  now,
	}).Insert()
	if err != nil {
		g.Log().Warningf(ctx, "写入推送日志失败 task:%d err:%+v", task.TaskID, err)
	}
}

func (s *sLazySheepTGGo) recordPushedMessages(ctx context.Context, task *lsysin.PushNoteTask, messages []*botmodels.Message) error {
	if task == nil || len(messages) == 0 {
		return nil
	}
	if err := s.ensurePushMessageTable(ctx); err != nil {
		return err
	}
	now := gtime.Now()
	for _, msg := range messages {
		if msg == nil || msg.ID == 0 {
			continue
		}
		data := g.Map{
			"task_id":        task.TaskID,
			"bot_key":        task.BotKey,
			"binding_key":    task.BindingKey,
			"note_id":        task.NoteID,
			"content_id":     task.ContentID,
			"chat_id":        task.ChatID,
			"message_id":     msg.ID,
			"media_group_id": strings.TrimSpace(msg.MediaGroupID),
			"status":         1,
			"created_at":     now,
			"updated_at":     now,
		}
		if _, err := g.DB().Model("hg_addon_lazysheep_tggo_push_message").Data(data).Insert(); err != nil {
			if isDuplicateKeyError(err) {
				_, updateErr := g.DB().Model("hg_addon_lazysheep_tggo_push_message").
					Where("bot_key", task.BotKey).
					Where("chat_id", task.ChatID).
					Where("message_id", msg.ID).
					Data(g.Map{
						"task_id":        task.TaskID,
						"binding_key":    task.BindingKey,
						"note_id":        task.NoteID,
						"content_id":     task.ContentID,
						"media_group_id": strings.TrimSpace(msg.MediaGroupID),
						"status":         1,
						"deleted_at":     nil,
						"updated_at":     now,
					}).
					Update()
				if updateErr != nil {
					return gerror.Wrap(updateErr, "更新已推送消息记录失败")
				}
				continue
			}
			return gerror.Wrap(err, "写入已推送消息记录失败")
		}
	}
	return nil
}

func (s *sLazySheepTGGo) clearPushChannelState(ctx context.Context, botKey string, binding *model.BindingRecord, chatID int64) error {
	targetChatID := pushTargetChatID(binding, chatID)
	if strings.TrimSpace(botKey) == "" || targetChatID == 0 {
		return nil
	}
	if err := s.ensurePushLogTable(ctx); err != nil {
		return err
	}
	if err := s.ensurePushDedupTable(ctx); err != nil {
		return err
	}
	if err := s.ensurePushMessageTable(ctx); err != nil {
		return err
	}
	if _, err := g.DB().Model("hg_addon_lazysheep_tggo_push_log").
		Where("bot_key", botKey).
		Where("chat_id", targetChatID).
		Delete(); err != nil {
		return gerror.Wrap(err, "清理频道推送日志失败")
	}
	if _, err := g.DB().Model("hg_addon_lazysheep_tggo_push_dedup").
		Where("bot_key", botKey).
		Where("chat_id", targetChatID).
		Delete(); err != nil {
		return gerror.Wrap(err, "清理频道推送去重失败")
	}
	if _, err := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").
		Where("bot_key", botKey).
		Where("chat_id", targetChatID).
		Delete(); err != nil {
		return gerror.Wrap(err, "清理频道推送队列失败")
	}
	return clearPushDedupCache(ctx, botKey, targetChatID)
}

func (s *sLazySheepTGGo) deleteBindingPushedMessages(ctx context.Context, botKey string, binding *model.BindingRecord, chatID int64) (deleted int, failed int, err error) {
	targetChatID := pushTargetChatID(binding, chatID)
	if strings.TrimSpace(botKey) == "" || binding == nil || targetChatID == 0 {
		return 0, 0, nil
	}
	if err = s.ensurePushMessageTable(ctx); err != nil {
		return 0, 0, err
	}
	if err = s.ensurePushBotRuntime(ctx, botKey); err != nil {
		return 0, 0, err
	}
	rt := s.runtime.get(botKey)
	if rt == nil || rt.client == nil {
		return 0, 0, gerror.New("机器人运行实例不存在，请先启动机器人")
	}
	var rows []struct {
		Id        int64 `json:"id" orm:"id"`
		MessageID int   `json:"messageId" orm:"message_id"`
	}
	if err = g.DB().Model("hg_addon_lazysheep_tggo_push_message").
		Fields("id,message_id").
		Where("bot_key", botKey).
		Where("binding_key", binding.Key).
		Where("chat_id", targetChatID).
		Where("status", 1).
		OrderDesc("message_id").
		Scan(&rows); err != nil {
		return 0, 0, gerror.Wrap(err, "查询频道已推送消息失败")
	}
	for _, row := range rows {
		if row.MessageID == 0 {
			continue
		}
		_, deleteErr := rt.client.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    targetChatID,
			MessageID: row.MessageID,
		})
		now := gtime.Now()
		if deleteErr != nil {
			failed++
			_, _ = g.DB().Model("hg_addon_lazysheep_tggo_push_message").WherePri(row.Id).Data(g.Map{
				"status":     3,
				"updated_at": now,
			}).Update()
			g.Log().Warningf(ctx, "删除频道消息失败 bot:%s chat:%d message:%d err:%+v", botKey, targetChatID, row.MessageID, deleteErr)
			continue
		}
		deleted++
		_, _ = g.DB().Model("hg_addon_lazysheep_tggo_push_message").WherePri(row.Id).Data(g.Map{
			"status":     2,
			"deleted_at": now,
			"updated_at": now,
		}).Update()
	}
	return deleted, failed, nil
}

func (s *sLazySheepTGGo) deleteBindingNotesNotInFingerprints(ctx context.Context, botKey string, binding *model.BindingRecord, chatID int64, keepURL map[string]struct{}, keepPush map[string]struct{}) (removedNotes int, deletedMessages int, failedMessages int, err error) {
	if binding == nil || len(keepURL) == 0 {
		return 0, 0, 0, nil
	}
	targetChatID := pushTargetChatID(binding, chatID)
	if strings.TrimSpace(botKey) == "" || targetChatID == 0 {
		return 0, 0, 0, nil
	}
	bindingID, err := s.resolveBindingID(ctx, binding.Key)
	if err != nil {
		return 0, 0, 0, err
	}
	var rows []struct {
		Id        int64 `json:"id" orm:"id"`
		ContentID int64 `json:"contentId" orm:"content_id"`
	}
	if err = g.DB().Model("hg_addon_lazysheep_tggo_note").
		Fields("id,content_id").
		Where("binding_id", bindingID).
		Scan(&rows); err != nil {
		return 0, 0, 0, gerror.Wrap(err, "查询频道笔记失败")
	}
	removeNoteIDs := make([]int64, 0)
	for _, row := range rows {
		fingerprint, fpErr := s.noteURLFingerprintByNoteID(ctx, row.Id)
		if fpErr != nil {
			return 0, 0, 0, fpErr
		}
		if fingerprint == "" {
			continue
		}
		if _, ok := keepURL[fingerprint]; ok {
			continue
		}
		pushFingerprints, fpErr := s.pushMediaDedupFingerprints(ctx, row.Id)
		if fpErr != nil {
			return 0, 0, 0, fpErr
		}
		keepByPushFingerprint := false
		for _, pushFingerprint := range pushFingerprints {
			if _, ok := keepPush[pushFingerprint]; ok {
				keepByPushFingerprint = true
				break
			}
		}
		if keepByPushFingerprint {
			continue
		}
		hasPending, pendingErr := s.noteHasUnfinishedPushTask(ctx, botKey, binding.Key, targetChatID, row.Id)
		if pendingErr != nil {
			return 0, 0, 0, pendingErr
		}
		if hasPending {
			continue
		}
		removeNoteIDs = append(removeNoteIDs, row.Id)
	}
	if len(removeNoteIDs) == 0 {
		return 0, 0, 0, nil
	}
	deletedMessages, failedMessages, err = s.deletePushedMessagesByNoteIDs(ctx, botKey, binding.Key, targetChatID, removeNoteIDs)
	if err != nil {
		return 0, deletedMessages, failedMessages, err
	}
	if err = s.deleteNoteRows(ctx, removeNoteIDs); err != nil {
		return 0, deletedMessages, failedMessages, err
	}
	return len(removeNoteIDs), deletedMessages, failedMessages, nil
}

func (s *sLazySheepTGGo) noteHasUnfinishedPushTask(ctx context.Context, botKey, bindingKey string, chatID int64, noteID int64) (bool, error) {
	if strings.TrimSpace(botKey) == "" || strings.TrimSpace(bindingKey) == "" || chatID == 0 || noteID <= 0 {
		return false, nil
	}
	count, err := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").
		Where("bot_key", botKey).
		Where("binding_key", bindingKey).
		Where("chat_id", chatID).
		Where("note_id", noteID).
		WhereIn("status", []int{pushTaskStatusReady, pushTaskStatusDoing, pushTaskStatusRetry, pushTaskStatusDead}).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "查询笔记推送任务状态失败")
	}
	return count > 0, nil
}

func (s *sLazySheepTGGo) noteURLFingerprintByNoteID(ctx context.Context, noteID int64) (string, error) {
	if noteID <= 0 {
		return "", nil
	}
	var rows []struct {
		SourceUrl string `json:"sourceUrl" orm:"source_url"`
	}
	if err := g.DB().Model("hg_addon_lazysheep_tggo_note_asset").
		Fields("source_url").
		Where("note_id", noteID).
		Where("source_url !=", "").
		WhereNull("deleted_at").
		OrderAsc("source_url").
		Scan(&rows); err != nil {
		return "", gerror.Wrap(err, "查询笔记 URL 指纹失败")
	}
	urls := make([]string, 0, len(rows))
	for _, row := range rows {
		if text := normalizeDedupMediaURL(row.SourceUrl); text != "" {
			urls = append(urls, text)
		}
	}
	urls = sortedNonEmptyStrings(urls)
	if len(urls) == 0 {
		return "", nil
	}
	raw, _ := json.Marshal(noteMediaFingerprint{Kind: "media", URLs: urls})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s *sLazySheepTGGo) deletePushedMessagesByNoteIDs(ctx context.Context, botKey string, bindingKey string, chatID int64, noteIDs []int64) (deleted int, failed int, err error) {
	if len(noteIDs) == 0 {
		return 0, 0, nil
	}
	if err = s.ensurePushMessageTable(ctx); err != nil {
		return 0, 0, err
	}
	if err = s.ensurePushBotRuntime(ctx, botKey); err != nil {
		return 0, 0, err
	}
	rt := s.runtime.get(botKey)
	if rt == nil || rt.client == nil {
		return 0, 0, gerror.New("机器人运行实例不存在，请先启动机器人")
	}
	var rows []struct {
		Id        int64 `json:"id" orm:"id"`
		MessageID int   `json:"messageId" orm:"message_id"`
	}
	if err = g.DB().Model("hg_addon_lazysheep_tggo_push_message").
		Fields("id,message_id").
		Where("bot_key", botKey).
		Where("binding_key", bindingKey).
		Where("chat_id", chatID).
		WhereIn("note_id", noteIDs).
		Where("status", 1).
		OrderDesc("message_id").
		Scan(&rows); err != nil {
		return 0, 0, gerror.Wrap(err, "查询待删除频道消息失败")
	}
	for _, row := range rows {
		if row.MessageID == 0 {
			continue
		}
		_, deleteErr := rt.client.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: row.MessageID,
		})
		now := gtime.Now()
		if deleteErr != nil {
			failed++
			_, _ = g.DB().Model("hg_addon_lazysheep_tggo_push_message").WherePri(row.Id).Data(g.Map{
				"status":     3,
				"updated_at": now,
			}).Update()
			g.Log().Warningf(ctx, "删除下架频道消息失败 bot:%s chat:%d message:%d err:%+v", botKey, chatID, row.MessageID, deleteErr)
			continue
		}
		deleted++
		_, _ = g.DB().Model("hg_addon_lazysheep_tggo_push_message").WherePri(row.Id).Data(g.Map{
			"status":     2,
			"deleted_at": now,
			"updated_at": now,
		}).Update()
	}
	return deleted, failed, nil
}

func (s *sLazySheepTGGo) deleteNoteRows(ctx context.Context, noteIDs []int64) error {
	if len(noteIDs) == 0 {
		return nil
	}
	if _, err := g.DB().Model("hg_addon_lazysheep_tggo_note_asset").WhereIn("note_id", noteIDs).Delete(); err != nil {
		return gerror.Wrap(err, "删除下架笔记资源失败")
	}
	if _, err := g.DB().Model("hg_addon_lazysheep_tggo_note_item").WhereIn("note_id", noteIDs).Delete(); err != nil {
		return gerror.Wrap(err, "删除下架笔记项失败")
	}
	if _, err := g.DB().Model("hg_addon_lazysheep_tggo_note").WhereIn("id", noteIDs).Delete(); err != nil {
		return gerror.Wrap(err, "删除下架笔记失败")
	}
	return nil
}

func clearPushDedupCache(ctx context.Context, botKey string, chatID int64) error {
	keys, err := cache.Instance().Keys(ctx)
	if err != nil {
		return gerror.Wrap(err, "列出推送去重缓存失败")
	}
	prefix := pushDedupKey(botKey, chatID, "")
	removeKeys := make([]interface{}, 0)
	for _, key := range keys {
		text := strings.TrimSpace(fmt.Sprint(key))
		if strings.HasPrefix(text, prefix) {
			removeKeys = append(removeKeys, key)
		}
	}
	if len(removeKeys) == 0 {
		return nil
	}
	_, err = cache.Instance().Remove(ctx, removeKeys...)
	return err
}

func limitPushLogError(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 1000 {
		return text
	}
	return text[:1000]
}

func (s *sLazySheepTGGo) ensurePushBotRuntime(ctx context.Context, botKey string) error {
	botKey = strings.TrimSpace(botKey)
	if botKey == "" {
		return gerror.New("机器人标识为空")
	}
	if rt := s.runtime.get(botKey); rt != nil && rt.client != nil {
		return nil
	}
	mutex := lock.NewConfig(30*time.Second, 200*time.Millisecond).Mutex("lazysheep_tggo:push_bot_runtime:" + botKey)
	if err := mutex.Lock(ctx); err != nil {
		return gerror.Wrap(err, "等待机器人运行实例启动失败")
	}
	defer func() {
		if err := mutex.Unlock(ctx); err != nil && !gerror.Is(err, lock.ErrNotExist) {
			g.Log().Warningf(ctx, "释放机器人运行实例锁失败 bot:%s err:%+v", botKey, err)
		}
	}()
	if rt := s.runtime.get(botKey); rt != nil && rt.client != nil {
		return nil
	}
	if err := s.SyncBot(ctx, botKey); err != nil {
		return gerror.Wrap(err, "启动机器人运行实例失败")
	}
	if rt := s.runtime.get(botKey); rt != nil && rt.client != nil {
		return nil
	}
	return gerror.New("机器人运行实例暂未就绪")
}

func (s *sLazySheepTGGo) DispatchPushNoteTask(ctx context.Context, task *lsysin.PushNoteTask) {
	if task == nil || task.TaskID <= 0 {
		return
	}
	if !tryAcquirePushChat(task.BotKey, task.ChatID) {
		return
	}
	select {
	case pushWorkerSem <- struct{}{}:
		go func() {
			defer func() {
				releasePushChat(task.BotKey, task.ChatID)
				<-pushWorkerSem
			}()
			if err := s.HandlePushNoteTask(ctx, task); err != nil {
				g.Log().Warningf(ctx, "异步推送任务执行失败 task:%d err:%+v", task.TaskID, err)
			}
		}()
	default:
		releasePushChat(task.BotKey, task.ChatID)
	}
}

func (s *sLazySheepTGGo) HandlePushNoteTask(ctx context.Context, task *lsysin.PushNoteTask) error {
	if task == nil || task.TaskID <= 0 {
		return nil
	}
	mutex := lock.NewConfig(5*time.Minute, time.Second).Mutex(fmt.Sprintf("lazysheep_tggo:push_note:run:%d", task.TaskID))
	if err := mutex.TryLock(ctx); err != nil {
		if gerror.Is(err, lock.ErrLockFailed) {
			return nil
		}
		return err
	}
	defer func() {
		if err := mutex.Unlock(ctx); err != nil && !gerror.Is(err, lock.ErrNotExist) {
			g.Log().Warningf(ctx, "释放推送任务锁失败 task:%d err:%+v", task.TaskID, err)
		}
	}()
	record, err := loadPushTaskRecord(ctx, task.TaskID)
	if err != nil {
		return err
	}
	if record == nil || record.Status == pushTaskStatusDone || record.Status == pushTaskStatusDead || record.Status == pushTaskStatusUnknown {
		return nil
	}
	if record.Status != pushTaskStatusReady && record.Status != pushTaskStatusRetry {
		return nil
	}
	if err = s.ensurePushBotRuntime(ctx, task.BotKey); err != nil {
		nextRetryAt := gtime.New(time.Now().Add(60 * time.Second))
		_, _ = g.DB().Model("hg_addon_lazysheep_tggo_push_queue").WherePri(task.TaskID).Data(g.Map{
			"status":        pushTaskStatusRetry,
			"last_error":    err.Error(),
			"next_retry_at": nextRetryAt,
			"updated_at":    gtime.Now(),
		}).Update()
		return nil
	}
	now := gtime.Now()
	attempt := record.Attempts + 1
	if _, err = g.DB().Model("hg_addon_lazysheep_tggo_push_queue").WherePri(task.TaskID).Data(g.Map{
		"status":     pushTaskStatusDoing,
		"attempts":   attempt,
		"started_at": now,
		"updated_at": now,
	}).Update(); err != nil {
		return gerror.Wrap(err, "更新推送任务状态失败")
	}
	started := time.Now()
	binding, err := s.findBindingByKey(ctx, task.BotKey, task.BindingKey)
	if err != nil {
		pushErr := err
		elapsed := time.Since(started)
		_, _ = g.DB().Model("hg_addon_lazysheep_tggo_push_queue").WherePri(task.TaskID).Data(g.Map{
			"status":      pushTaskStatusDead,
			"last_error":  pushErr.Error(),
			"finished_at": gtime.Now(),
			"updated_at":  gtime.Now(),
		}).Update()
		recordPushQueueMonitorEvent(ctx, task, false, pushErr.Error(), elapsed)
		return pushErr
	}
	if err = waitPushChatTurn(ctx, task.BotKey, task.ChatID); err != nil {
		return err
	}
	record, err = loadPushTaskRecord(ctx, task.TaskID)
	if err != nil {
		return err
	}
	if record == nil || record.Status == pushTaskStatusDead || record.Status == pushTaskStatusUnknown {
		return nil
	}
	if seen, err := s.pushDedupSeenForTask(ctx, task); err != nil {
		g.Log().Warningf(ctx, "推送前检查频道去重状态失败 task:%d err:%+v", task.TaskID, err)
	} else if !seen {
		g.Log().Warningf(ctx, "推送前频道去重记录不存在，继续执行但保留任务幂等保护 task:%d bot:%s chat:%d note:%d", task.TaskID, task.BotKey, task.ChatID, task.NoteID)
	}
	messages, pushErr := s.pushCollectedNote(ctx, task.BotKey, binding, task.NoteID, task.ChatID)
	elapsed := time.Since(started)
	if pushErr == nil {
		messageID := firstTelegramMessageID(messages)
		if err := s.recordPushedMessages(ctx, task, messages); err != nil {
			g.Log().Warningf(ctx, "记录已推送消息失败 task:%d err:%+v", task.TaskID, err)
		}
		recordPushChatSuccess(task.BotKey, task.ChatID)
		_, err = g.DB().Model("hg_addon_lazysheep_tggo_push_queue").WherePri(task.TaskID).Data(g.Map{
			"status":      pushTaskStatusDone,
			"last_error":  "",
			"finished_at": gtime.Now(),
			"updated_at":  gtime.Now(),
		}).Update()
		_ = s.pushDedupMarkDone(ctx, task.BotKey, task.ChatID, task.ContentID, task.NoteID)
		recordPushTaskLog(ctx, task, pushLogStatusSuccess, attempt, elapsed.Milliseconds(), messageID, "")
		recordPushQueueMonitorEvent(ctx, task, true, "", elapsed)
		return err
	}
	pushErr = sanitizeTelegramBotError(pushErr)
	errText := strings.TrimSpace(pushErr.Error())
	recordPushChatError(task.BotKey, task.ChatID, pushErr)
	if isAmbiguousTelegramSendError(pushErr) {
		errText = "Telegram 发送结果未知，已停止自动重试以避免重复推送：" + errText
		_, _ = g.DB().Model("hg_addon_lazysheep_tggo_push_queue").WherePri(task.TaskID).Data(g.Map{
			"status":      pushTaskStatusUnknown,
			"last_error":  errText,
			"finished_at": gtime.Now(),
			"updated_at":  gtime.Now(),
		}).Update()
		recordPushTaskLog(ctx, task, pushLogStatusFailed, attempt, elapsed.Milliseconds(), 0, errText)
		recordPushQueueMonitorEvent(ctx, task, false, errText, elapsed)
		g.Log().Warningf(ctx, "Telegram 推送结果未知，停止自动重试避免重复发送 task:%d bot:%s chat:%d note:%d err:%+v", task.TaskID, task.BotKey, task.ChatID, task.NoteID, pushErr)
		return nil
	}
	if attempt >= record.MaxAttempts || !isRetriablePullError(pushErr) {
		_, _ = g.DB().Model("hg_addon_lazysheep_tggo_push_queue").WherePri(task.TaskID).Data(g.Map{
			"status":      pushTaskStatusDead,
			"last_error":  errText,
			"finished_at": gtime.Now(),
			"updated_at":  gtime.Now(),
		}).Update()
		if err := s.pushDedupMarkFailed(ctx, task); err != nil {
			g.Log().Warningf(ctx, "释放失败推送去重占位失败 task:%d err:%+v", task.TaskID, err)
		}
		recordPushTaskLog(ctx, task, pushLogStatusFailed, attempt, elapsed.Milliseconds(), 0, errText)
		recordPushQueueMonitorEvent(ctx, task, false, errText, elapsed)
		return pushErr
	}
	delay := pushRetryDelayForError(attempt, pushErr)
	nextRetryAt := gtime.New(time.Now().Add(time.Duration(delay) * time.Second))
	_, _ = g.DB().Model("hg_addon_lazysheep_tggo_push_queue").WherePri(task.TaskID).Data(g.Map{
		"status":        pushTaskStatusRetry,
		"last_error":    errText,
		"next_retry_at": nextRetryAt,
		"updated_at":    gtime.Now(),
	}).Update()
	recordPushTaskLog(ctx, task, pushLogStatusFailed, attempt, elapsed.Milliseconds(), 0, errText)
	task.Attempt = attempt
	task.QueuedAt = time.Now().Format(time.RFC3339)
	if err = delayOrPushQueue(ctx, pushNoteTopic, task, int64(delay)); err != nil {
		g.Log().Warningf(ctx, "推送任务延迟重投失败 task:%d delay:%d err:%+v", task.TaskID, delay, err)
	}
	recordPushQueueMonitorEvent(ctx, task, false, errText, elapsed)
	return nil
}

func firstTelegramMessageID(messages []*botmodels.Message) int {
	for _, msg := range messages {
		if msg != nil && msg.ID != 0 {
			return msg.ID
		}
	}
	return 0
}

func (s *sLazySheepTGGo) pushDedupSeenForTask(ctx context.Context, task *lsysin.PushNoteTask) (bool, error) {
	if task == nil {
		return false, nil
	}
	fingerprints, err := s.pushDedupFingerprints(ctx, task.ContentID, task.NoteID)
	if err != nil {
		return false, err
	}
	return s.pushDedupSeenAny(ctx, task.BotKey, task.BindingKey, task.ChatID, fingerprints)
}

func isAmbiguousTelegramSendError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "too many requests") || strings.Contains(text, "retry_after") {
		return false
	}
	sendMethod := strings.Contains(text, "sendmediagroup") ||
		strings.Contains(text, "sendphoto") ||
		strings.Contains(text, "sendvideo") ||
		strings.Contains(text, "sendmessage")
	if !sendMethod {
		return false
	}
	return strings.Contains(text, "timeout awaiting response headers") ||
		strings.Contains(text, "client.timeout exceeded while awaiting headers") ||
		strings.Contains(text, "context deadline exceeded") ||
		strings.Contains(text, "deadline exceeded")
}

func waitPushChatTurn(ctx context.Context, botKey string, chatID int64) error {
	if chatID == 0 {
		return nil
	}
	key := fmt.Sprintf("%s:%d", botKey, chatID)
	raw, _ := pushChatLimiters.LoadOrStore(key, &pushChatLimiter{})
	limiter := raw.(*pushChatLimiter)
	limiter.Lock()
	defer limiter.Unlock()
	wait := pushChatInterval - time.Since(limiter.last)
	if until := time.Until(limiter.nextAvailable); until > wait {
		wait = until
	}
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	limiter.last = time.Now()
	return nil
}

func tryAcquirePushChat(botKey string, chatID int64) bool {
	if chatID == 0 {
		return true
	}
	raw, _ := pushChatLimiters.LoadOrStore(fmt.Sprintf("%s:%d", botKey, chatID), &pushChatLimiter{})
	limiter := raw.(*pushChatLimiter)
	limiter.Lock()
	defer limiter.Unlock()
	if limiter.running {
		return false
	}
	if pushChatInterval-time.Since(limiter.last) > 0 {
		return false
	}
	if time.Until(limiter.nextAvailable) > 0 {
		return false
	}
	limiter.running = true
	return true
}

func selectDispatchablePushRecords(records []*pushTaskRecord) []*pushTaskRecord {
	if len(records) == 0 {
		return records
	}
	selected := make([]*pushTaskRecord, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		key := fmt.Sprintf("%s:%d", record.BotKey, record.ChatId)
		if _, ok := seen[key]; ok {
			continue
		}
		if !isPushChatDispatchable(record.BotKey, record.ChatId) {
			continue
		}
		seen[key] = struct{}{}
		selected = append(selected, record)
	}
	return selected
}

func isPushChatDispatchable(botKey string, chatID int64) bool {
	if chatID == 0 {
		return true
	}
	raw, _ := pushChatLimiters.LoadOrStore(fmt.Sprintf("%s:%d", botKey, chatID), &pushChatLimiter{})
	limiter := raw.(*pushChatLimiter)
	limiter.Lock()
	defer limiter.Unlock()
	if limiter.running {
		return false
	}
	if pushChatInterval-time.Since(limiter.last) > 0 {
		return false
	}
	return time.Until(limiter.nextAvailable) <= 0
}

func releasePushChat(botKey string, chatID int64) {
	if chatID == 0 {
		return
	}
	raw, ok := pushChatLimiters.Load(fmt.Sprintf("%s:%d", botKey, chatID))
	if !ok {
		return
	}
	limiter := raw.(*pushChatLimiter)
	limiter.Lock()
	limiter.running = false
	limiter.Unlock()
}

func recordPushChatSuccess(botKey string, chatID int64) {
	if chatID == 0 {
		return
	}
	raw, ok := pushChatLimiters.Load(fmt.Sprintf("%s:%d", botKey, chatID))
	if !ok {
		return
	}
	limiter := raw.(*pushChatLimiter)
	limiter.Lock()
	if !limiter.nextAvailable.IsZero() && time.Now().After(limiter.nextAvailable) {
		limiter.nextAvailable = time.Time{}
	}
	limiter.Unlock()
}

func recordPushChatError(botKey string, chatID int64, err error) {
	if chatID == 0 || err == nil {
		return
	}
	var rateErr *bot.TooManyRequestsError
	if !errors.As(err, &rateErr) {
		return
	}
	retryAfter := rateErr.RetryAfter + 1
	if retryAfter <= 0 {
		retryAfter = 30
	}
	raw, _ := pushChatLimiters.LoadOrStore(fmt.Sprintf("%s:%d", botKey, chatID), &pushChatLimiter{})
	limiter := raw.(*pushChatLimiter)
	limiter.Lock()
	limiter.nextAvailable = time.Now().Add(time.Duration(retryAfter) * time.Second)
	limiter.Unlock()
}

func (s *sLazySheepTGGo) findBindingByKey(ctx context.Context, botKey, bindingKey string) (*model.BindingRecord, error) {
	state, err := s.GetState(ctx)
	if err != nil {
		return nil, err
	}
	for _, binding := range state.Bindings {
		if binding == nil || binding.Key != bindingKey {
			continue
		}
		if botKey != "" && binding.BotKey != "" && binding.BotKey != botKey {
			continue
		}
		return binding, nil
	}
	return nil, gerror.New("推送任务绑定关系不存在或已删除")
}

func pushTaskFromRecord(record *pushTaskRecord) *lsysin.PushNoteTask {
	if record == nil || record.Id <= 0 {
		return nil
	}
	return &lsysin.PushNoteTask{
		TaskID:     record.Id,
		BotKey:     record.BotKey,
		BindingKey: record.BindingKey,
		SourceURL:  record.SourceUrl,
		NoteID:     record.NoteId,
		ContentID:  record.ContentId,
		ChatID:     record.ChatId,
		Attempt:    record.Attempts,
		QueuedAt:   time.Now().Format(time.RFC3339),
	}
}

func delayOrPushQueue(ctx context.Context, topic string, data interface{}, delay int64) error {
	if delay <= 0 {
		return queue.Push(topic, data)
	}
	if g.Cfg().MustGet(ctx, "queue.driver").String() == "disk" {
		return pushQueueAfter(ctx, topic, data, delay)
	}
	if err := queue.DelayPush(topic, data, delay); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "implement me") {
			return err
		}
		return pushQueueAfter(ctx, topic, data, delay)
	}
	return nil
}

func pushQueueAfter(ctx context.Context, topic string, data interface{}, delay int64) error {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(delay) * time.Second):
			if pushErr := queue.Push(topic, data); pushErr != nil {
				g.Log().Warningf(ctx, "延迟队列降级投递失败 topic:%s err:%+v", topic, pushErr)
			}
		}
	}()
	return nil
}

func loadPushTaskRecord(ctx context.Context, taskID int64) (*pushTaskRecord, error) {
	var record *pushTaskRecord
	if err := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").WherePri(taskID).Scan(&record); err != nil {
		return nil, gerror.Wrap(err, "查询推送任务失败")
	}
	return record, nil
}

func pushRetryDelay(attempt int) int {
	switch {
	case attempt <= 1:
		return 30
	case attempt == 2:
		return 120
	case attempt == 3:
		return 300
	case attempt == 4:
		return 900
	default:
		return 1800
	}
}

func pushRetryDelayForError(attempt int, err error) int {
	delay := pushRetryDelay(attempt)
	var rateErr *bot.TooManyRequestsError
	if errors.As(err, &rateErr) && rateErr.RetryAfter > 0 {
		rateDelay := rateErr.RetryAfter + 1
		if rateDelay > delay {
			return rateDelay
		}
	}
	return delay
}

func pushQueuePaused(ctx context.Context) bool {
	val, err := cache.Instance().Get(ctx, pushQueuePausedKey)
	return err == nil && !val.IsNil() && val.Bool()
}

func initPushQueueStatusCounts() []*lsysin.PushQueueStatusCount {
	return []*lsysin.PushQueueStatusCount{
		{Status: pushTaskStatusReady, Label: pushQueueStatusLabel(pushTaskStatusReady)},
		{Status: pushTaskStatusDoing, Label: pushQueueStatusLabel(pushTaskStatusDoing)},
		{Status: pushTaskStatusDone, Label: pushQueueStatusLabel(pushTaskStatusDone)},
		{Status: pushTaskStatusRetry, Label: pushQueueStatusLabel(pushTaskStatusRetry)},
		{Status: pushTaskStatusDead, Label: pushQueueStatusLabel(pushTaskStatusDead)},
	}
}

func pushQueueStatusLabel(status int) string {
	switch status {
	case pushTaskStatusReady:
		return "待推送"
	case pushTaskStatusDoing:
		return "推送中"
	case pushTaskStatusDone:
		return "成功"
	case pushTaskStatusRetry:
		return "重试中"
	case pushTaskStatusDead:
		return "失败"
	case pushTaskStatusUnknown:
		return "待确认"
	default:
		return "未知"
	}
}

func filterPushQueueMonitorModel(mod *gdb.Model, botKey string, chatID int64) *gdb.Model {
	if botKey != "" {
		mod = mod.Where("bot_key", botKey)
	}
	if chatID != 0 {
		mod = mod.Where("chat_id", chatID)
	}
	return mod
}

func formatPushQueueTime(value *gtime.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("Y-m-d H:i:s")
}

func recordPushQueueMonitorEvent(ctx context.Context, task *lsysin.PushNoteTask, success bool, errText string, elapsed time.Duration) {
	if task == nil {
		return
	}
	event := &lsysin.PullMonitorEvent{
		TraceID:       pullTraceID(ctx),
		BotKey:        task.BotKey,
		BindingKey:    task.BindingKey,
		SourceURL:     task.SourceURL,
		ChatID:        task.ChatID,
		Success:       success,
		Error:         errText,
		Message:       "推送任务完成",
		Pushed:        boolToInt(success),
		PushFailed:    boolToInt(!success),
		ElapsedMs:     elapsed.Milliseconds(),
		CreatedAt:     time.Now().Format("2006-01-02 15:04:05"),
		CreatedAtUnix: time.Now().Unix(),
	}
	recordPullMonitorEvent(ctx, event)
}

func isTableNotExistError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "1146") ||
		strings.Contains(text, "doesn't exist") ||
		strings.Contains(text, "does not exist") ||
		strings.Contains(text, "undefined_table")
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "1062") ||
		strings.Contains(text, "duplicate") ||
		strings.Contains(text, "unique constraint") ||
		strings.Contains(text, "duplicate key")
}
