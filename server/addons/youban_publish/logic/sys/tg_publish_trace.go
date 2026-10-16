package sys

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gtrace"
	"github.com/gogf/gf/v2/os/gctx"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/hibiken/asynq"
)

type telegramPublishTraceContextKey struct{}

// This context is diagnostic only: no leases, task states or retry decisions
// depend on it. jobId joins executions across queues; executionId distinguishes
// retries and concurrent consumers even when the incoming trace is reused.
func telegramPublishTraceContext(ctx context.Context, jobId int64, fields ...g.Map) context.Context {
	ctx = gctx.WithSpan(ctx, "telegram.publish")
	base := g.Map{}
	if previous, ok := ctx.Value(telegramPublishTraceContextKey{}).(g.Map); ok {
		for key, value := range previous {
			base[key] = value
		}
	} else {
		instance, _ := os.Hostname()
		taskId, _ := asynq.GetTaskID(ctx)
		queueName, _ := asynq.GetQueueName(ctx)
		retryCount, _ := asynq.GetRetryCount(ctx)
		base = g.Map{"executionId": guid.S(), "instance": instance, "pid": os.Getpid(),
			"taskId": taskId, "queueName": queueName, "queueRetryCount": retryCount}
	}
	base["jobId"] = jobId
	base["traceId"] = gtrace.GetTraceID(ctx)
	for _, extra := range fields {
		for key, value := range extra {
			base[key] = value
		}
	}
	return context.WithValue(ctx, telegramPublishTraceContextKey{}, base)
}

func telegramPublishJobTraceContext(ctx context.Context, job telegramJobRecord) context.Context {
	return telegramPublishTraceContext(ctx, job.Id, g.Map{
		"tenantId": job.TenantId, "accountId": job.AccountId, "profileId": job.ProfileId,
		"channelId": job.ChannelId, "botId": job.BotId, "targetChatId": job.TargetChatId,
		"jobStatus": job.Status, "dispatchStatus": job.DispatchStatus, "sendPhase": job.SendPhase,
	})
}

type telegramPublishStage struct {
	ctx     context.Context
	fields  g.Map
	started time.Time
	timer   *time.Timer
	ended   atomic.Bool
}

func beginTelegramPublishStage(ctx context.Context, stage string, fields ...g.Map) *telegramPublishStage {
	base, enabled := ctx.Value(telegramPublishTraceContextKey{}).(g.Map)
	if !enabled {
		return &telegramPublishStage{}
	}
	data := g.Map{"log_type": "telegram_publish_stage", "stage": stage, "stageId": guid.S()}
	for key, value := range base {
		data[key] = value
	}
	for _, extra := range fields {
		for key, value := range extra {
			data[key] = value
		}
	}
	step := &telegramPublishStage{ctx: ctx, fields: data, started: time.Now()}
	step.log("start", nil, nil)
	// One warning per slow stage bounds log volume and adds no database/Redis
	// traffic. Stop on every normal/error/panic exit; never cancel business work.
	step.timer = time.AfterFunc(30*time.Second, func() {
		if !step.ended.Load() {
			step.log("slow", nil, nil)
		}
	})
	return step
}

func (s *telegramPublishStage) End(err error, fields ...g.Map) {
	if s.timer == nil {
		return
	}
	if s.ended.Swap(true) {
		return
	}
	s.timer.Stop()
	event := "end"
	if err != nil {
		event = "error"
	}
	s.log(event, err, fields)
}

// Must be deferred directly so recover observes a panic. Re-panic preserves
// Asynq's existing failure/retry behavior rather than silently swallowing it.
func (s *telegramPublishStage) Finish(err *error) {
	if recovered := recover(); recovered != nil {
		if s.timer != nil && !s.ended.Swap(true) {
			s.timer.Stop()
			s.log("panic", fmt.Errorf("%v", recovered), []g.Map{{"stack": string(debug.Stack())}})
		}
		panic(recovered)
	}
	s.End(*err)
}

func (s *telegramPublishStage) log(event string, err error, fields []g.Map) {
	data := g.Map{}
	for key, value := range s.fields {
		data[key] = value
	}
	for _, extra := range fields {
		for key, value := range extra {
			data[key] = value
		}
	}
	data["event"] = event
	data["durationMs"] = time.Since(s.started).Milliseconds()
	if deadline, ok := s.ctx.Deadline(); ok {
		data["deadlineRemainingMs"] = time.Until(deadline).Milliseconds()
	}
	if ctxErr := s.ctx.Err(); ctxErr != nil {
		data["contextError"] = ctxErr.Error()
	}
	if err != nil {
		data["errorType"] = fmt.Sprintf("%T", err)
		data["error"] = telegramPublishLogError(err)
	}
	if event == "error" || event == "slow" || event == "panic" {
		g.Log().Warning(s.ctx, "TG推送链路", data)
	} else {
		g.Log().Info(s.ctx, "TG推送链路", data)
	}
}

var (
	telegramPublishLogURLPattern   = regexp.MustCompile(`(?i)(?:https?|socks5h?|postgres(?:ql)?|redis)://[^\s<>"']+`)
	telegramPublishLogTokenPattern = regexp.MustCompile(`\b\d{5,}:[A-Za-z0-9_-]{20,}\b`)
)

func telegramPublishLogError(err error) string {
	if err == nil {
		return ""
	}
	message := telegramPublishLogURLPattern.ReplaceAllString(err.Error(), "[URL redacted]")
	message = telegramPublishLogTokenPattern.ReplaceAllString(message, "[TOKEN redacted]")
	message = strings.ReplaceAll(strings.ReplaceAll(message, "\n", " "), "\r", " ")
	if len(message) > 2048 {
		message = message[:2048]
	}
	return message
}

func telegramPublishMediaLogFields(media *telegramMediaItem) g.Map {
	if media == nil {
		return g.Map{"mediaMissing": true}
	}
	mode := "upload"
	if _, ok := telegramCopyMediaRefFromFileId(media.TgFileId); ok && !telegramMediaRequiresSanitizedUpload(media) {
		mode = "copy"
	} else if telegramMediaUsesReusableFileId(media) {
		mode = "file_id"
	} else if telegramMediaCanUseRemoteURL(media) {
		mode = "url"
	}
	host := ""
	if parsed, err := url.Parse(media.FileUrl); err == nil {
		host = parsed.Hostname()
	}
	return g.Map{"mediaId": media.Id, "mediaType": media.MediaType, "purpose": media.Purpose,
		"inputMode": mode, "sourceHost": host, "antiScanEnabled": media.AntiScanEnabled,
		"antiScanMode": media.AntiScanMode, "forceUpload": media.ForceUpload}
}
