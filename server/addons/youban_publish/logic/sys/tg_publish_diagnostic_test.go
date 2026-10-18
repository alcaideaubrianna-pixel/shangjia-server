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

	_ "hotgo/addons/youban_tg_bot_gateway/logic/sys"
	"hotgo/internal/bootstrap/envconfig"
	"hotgo/internal/library/cache"
)

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
