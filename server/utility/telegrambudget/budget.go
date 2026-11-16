// Package telegrambudget defines shared Telegram execution budgets.
// Durations are read at execution time; existing durable job deadlines are never
// recalculated here. Environment overrides are applied by bootstrap/envconfig.
package telegrambudget

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	DefaultJob            = 30 * time.Minute
	DefaultDisplay        = 10 * time.Minute
	DefaultVerify         = 10 * time.Minute
	DefaultBotRequest     = 10 * time.Minute
	DefaultAccount        = 10 * time.Minute
	DefaultAccountGate    = 30 * time.Second
	DefaultFinalize       = 10 * time.Second
	DefaultResponse       = 5 * time.Second
	DefaultShutdown       = 30 * time.Second
	DefaultEnqueue        = 5 * time.Second
	DefaultPreview        = 30 * time.Second
	DefaultPoster         = 5 * time.Second
	DefaultSanitize       = 10 * time.Minute
	DefaultCleanup        = 15 * time.Second
	DefaultLeaseOperation = 3 * time.Second
	DefaultWebhook        = 15 * time.Second
)

type setting struct {
	key      string
	fallback time.Duration
	minimum  time.Duration
	maximum  time.Duration
}

var (
	job            = setting{"sendBudget", DefaultJob, 2 * time.Minute, 30 * time.Minute}
	display        = setting{"displayTimeout", DefaultDisplay, time.Second, 30 * time.Minute}
	verify         = setting{"verifyTimeout", DefaultVerify, time.Second, 30 * time.Minute}
	botRequest     = setting{"botRequestTimeout", DefaultBotRequest, time.Second, 30 * time.Minute}
	account        = setting{"accountTimeout", DefaultAccount, time.Second, 30 * time.Minute}
	accountGate    = setting{"accountGateTimeout", DefaultAccountGate, time.Second, 5 * time.Minute}
	finalize       = setting{"finalizeTimeout", DefaultFinalize, time.Second, time.Minute}
	response       = setting{"responseTimeout", DefaultResponse, time.Second, time.Minute}
	shutdown       = setting{"shutdownTimeout", DefaultShutdown, time.Second, 5 * time.Minute}
	enqueue        = setting{"enqueueTimeout", DefaultEnqueue, time.Second, time.Minute}
	preview        = setting{"previewTimeout", DefaultPreview, time.Second, 5 * time.Minute}
	poster         = setting{"posterTimeout", DefaultPoster, time.Second, time.Minute}
	sanitize       = setting{"sanitizeTimeout", DefaultSanitize, time.Second, 30 * time.Minute}
	cleanup        = setting{"cleanupTimeout", DefaultCleanup, time.Second, 5 * time.Minute}
	leaseOperation = setting{"leaseOperationTimeout", DefaultLeaseOperation, 100 * time.Millisecond, time.Minute}
	webhook        = setting{"webhookWait", DefaultWebhook, time.Second, 5 * time.Minute}
	warningMu      sync.Mutex
	// Only one entry per known setting; this is log throttling, not business state.
	warnings = map[string]time.Time{}
)

func (s setting) duration(ctx context.Context) time.Duration {
	key := "youbanPublish.telegram." + s.key
	raw := strings.TrimSpace(g.Cfg().MustGet(ctx, key, s.fallback.String()).String())
	value, err := time.ParseDuration(raw)
	if err == nil && value >= s.minimum && value <= s.maximum {
		return value
	}
	warningMu.Lock()
	now := time.Now()
	logWarning := now.Sub(warnings[key]) >= time.Minute
	if logWarning {
		warnings[key] = now
	}
	warningMu.Unlock()
	if logWarning {
		// Do not echo an arbitrary invalid value into logs.
		g.Log().Warning(ctx, "Telegram时间配置无效，使用默认值", g.Map{
			"event": "telegram_budget_invalid", "configKey": key,
			"default": s.fallback.String(), "minimum": s.minimum.String(), "maximum": s.maximum.String(),
		})
	}
	return s.fallback
}

func Job(ctx context.Context) time.Duration            { return job.duration(ctx) }
func Display(ctx context.Context) time.Duration        { return display.duration(ctx) }
func Verify(ctx context.Context) time.Duration         { return verify.duration(ctx) }
func BotRequest(ctx context.Context) time.Duration     { return botRequest.duration(ctx) }
func Account(ctx context.Context) time.Duration        { return account.duration(ctx) }
func AccountGate(ctx context.Context) time.Duration    { return accountGate.duration(ctx) }
func Finalize(ctx context.Context) time.Duration       { return finalize.duration(ctx) }
func Response(ctx context.Context) time.Duration       { return response.duration(ctx) }
func Shutdown(ctx context.Context) time.Duration       { return shutdown.duration(ctx) }
func Enqueue(ctx context.Context) time.Duration        { return enqueue.duration(ctx) }
func Preview(ctx context.Context) time.Duration        { return preview.duration(ctx) }
func Poster(ctx context.Context) time.Duration         { return poster.duration(ctx) }
func Sanitize(ctx context.Context) time.Duration       { return sanitize.duration(ctx) }
func Cleanup(ctx context.Context) time.Duration        { return cleanup.duration(ctx) }
func LeaseOperation(ctx context.Context) time.Duration { return leaseOperation.duration(ctx) }
func Webhook(ctx context.Context) time.Duration        { return webhook.duration(ctx) }

// TaskTimeout leaves finalization time outside the durable send budget.
func TaskTimeout(ctx context.Context) time.Duration { return Job(ctx) + Finalize(ctx) }
