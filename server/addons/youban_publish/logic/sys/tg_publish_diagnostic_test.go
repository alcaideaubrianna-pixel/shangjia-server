package sys

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	_ "hotgo/addons/youban_tg_bot_gateway/logic/sys"
	"hotgo/internal/bootstrap/envconfig"
	"hotgo/internal/library/cache"
)

func TestTelegramPublishTraceTimingAndOperationCategory(t *testing.T) {
	start := gtime.New("2026-11-13 10:00:00")
	end := gtime.New("2026-11-13 10:00:01.250")
	if got := telegramPublishElapsedMs(start, end); got != 1250 {
		t.Fatalf("dispatchAgeMs=%d, want 1250", got)
	}
	if got := telegramPublishElapsedMs(end, start); got != 0 {
		t.Fatalf("negative duration=%d, want 0", got)
	}
	cases := map[string]string{
		"collect:1:2":       "collect",
		"cycle_batch:1:2:3": "cycle_batch",
		"profile_cycle:1":   "cycle",
		"quick_push:1":      "manual",
		"edit:1":            "edit",
		"unknown":           "other",
	}
	for operation, want := range cases {
		if got := telegramPublishOperationCategory(operation); got != want {
			t.Errorf("operation=%q category=%q, want %q", operation, got, want)
		}
	}
}

// Explicitly opt-in. This invokes preparation only: no queue consumers,
// account sessions, attempts, Telegram sends or job state mutations.
func TestTelegramPublishLivePreparation(t *testing.T) {
	jobId, _ := strconv.ParseInt(os.Getenv("YOUBAN_PUBLISH_DIAG_JOB_ID"), 10, 64)
	if jobId <= 0 {
		t.Skip("set YOUBAN_PUBLISH_DIAG_JOB_ID to inspect one existing job")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	envconfig.Apply(ctx)
	cache.SetAdapter(ctx)
	ctx = telegramPublishTraceContext(ctx, jobId, g.Map{"diagnostic": true, "sendEnabled": false})
	s := NewSysPublish()
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		t.Fatal(telegramPublishLogError(err))
	}
	ctx = telegramPublishJobTraceContext(ctx, job)
	var token, caption string
	var display, verify []*telegramMediaItem
	steps := []struct {
		name string
		run  func() error
	}{
		{"bot_token", func() (err error) { token, err = s.telegramJobBotToken(ctx, job.BotId, job.TenantId); return }},
		{"caption", func() (err error) { caption, err = s.telegramJobCaption(ctx, job); return }},
		{"display_media", func() (err error) { display, err = s.telegramJobMedia(ctx, job, "display"); return }},
		{"media_selection", func() (err error) { display, err = s.selectTelegramDisplayMediaForTenant(ctx, job, display); return }},
		{"verify_media", func() (err error) { verify, err = s.telegramJobMedia(ctx, job, "verify"); return }},
		{"bot_client", func() error { _, err := s.telegramMediaBot(ctx, token); return err }},
		{"content_protection", func() (err error) {
			caption, err = s.applyTelegramJobContentProtection(ctx, job, caption, display, verify)
			return
		}},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			started := time.Now()
			if err := step.run(); err != nil {
				t.Fatal(telegramPublishLogError(err))
			}
			t.Logf("jobId=%d stage=%s duration=%s", jobId, step.name, time.Since(started))
		})
		if t.Failed() {
			return
		}
	}
	t.Logf("preparation completed jobId=%d botConfigured=%t captionBytes=%d display=%d verify=%d", jobId, token != "", len(caption), len(display), len(verify))
}
