package sys

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// These instruments use the application's global provider. The meter argument
// keeps tests isolated without replacing global telemetry or starting exporters.
type publishTransportInstruments struct {
	requests        metric.Int64Counter
	duration        metric.Float64Histogram
	fallback        metric.Int64Counter
	accountPhases   metric.Int64Counter
	accountDuration metric.Float64Histogram
}

func newPublishTransportInstruments(meter metric.Meter) *publishTransportInstruments {
	requests, _ := meter.Int64Counter("xiaohuiji.publish.api.requests", metric.WithDescription("Completed concrete Telegram client calls; excludes preparation, retry wrappers and queue submissions. Outcomes describe the call, not the final job."))
	duration, _ := meter.Float64Histogram("xiaohuiji.publish.api.duration",
		metric.WithUnit("s"), metric.WithDescription("Elapsed seconds per concrete Telegram client call, excluding media preparation and wrapper retries; upload_mode describes prepared primary media."),
		metric.WithExplicitBucketBoundaries(.005, .01, .05, .1, .5, 1, 5, 10, 30, 60, 120, 300, 600, 1200))
	fallback, _ := meter.Int64Counter("xiaohuiji.publish.fallback.events", metric.WithDescription("Media fallback branch entries and account handoff lifecycle events; queued, delayed and rejected handoffs remain unknown until a phase result is known."))
	accountPhases, _ := meter.Int64Counter("xiaohuiji.publish.account.phase.attempts",
		metric.WithDescription("Account media phase send invocations; one phase may issue multiple underlying RPCs, retries count again"))
	accountDuration, _ := meter.Float64Histogram("xiaohuiji.publish.account.phase.duration",
		metric.WithUnit("s"), metric.WithDescription("Account media phase send duration, excluding task queue residence; not per-RPC latency"),
		metric.WithExplicitBucketBoundaries(.005, .01, .05, .1, .5, 1, 5, 10, 30, 60, 120, 300, 600, 1200))
	return &publishTransportInstruments{requests: requests, duration: duration, fallback: fallback,
		accountPhases: accountPhases, accountDuration: accountDuration}
}

var publishTransportMetrics = sync.OnceValue(func() *publishTransportInstruments {
	return newPublishTransportInstruments(otel.Meter("hotgo/youban_publish/transport"))
})

func publishTransportBounded(value, fallback string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}

func publishTransportResult(err error, confirmed bool) (string, string) {
	if err == nil {
		if confirmed {
			return "success", "none"
		}
		return "unknown", "unknown"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled", "canceled"
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) || isTelegramAPITimeout(err) {
		return "timeout", "timeout"
	}
	if isTelegramAmbiguousDeliveryError(err) {
		return "unknown", "ambiguous"
	}
	switch telegramAPIStatusCode(err) {
	case 429:
		return "failed", "rate_limited"
	case 401, 403:
		return "failed", "permission"
	case 400, 404, 413:
		return "failed", "rejected"
	}
	return "failed", "other"
}

func publishTransportAttributes(method, purpose, transport, mode, outcome, errorClass string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("method", publishTransportBounded(method, "other", "sendMessage", "sendPhoto", "sendVideo", "sendMediaGroup", "copyMessage", "copyMessages")),
		attribute.String("purpose", publishTransportBounded(purpose, "unknown", "display", "verify", "mixed")),
		attribute.String("transport", publishTransportBounded(transport, "unknown", "bot", "account")),
		attribute.String("upload_mode", publishTransportBounded(mode, "not_applicable", "file_id", "url", "binary", "copy", "mixed", "not_applicable")),
		attribute.String("outcome", publishTransportBounded(outcome, "unknown", "success", "failed", "timeout", "canceled", "unknown")),
		attribute.String("error_class", publishTransportBounded(errorClass, "other", "none", "unknown", "ambiguous", "timeout", "canceled", "rate_limited", "permission", "rejected", "other")),
	}
}

func (m *publishTransportInstruments) request(ctx context.Context, method, purpose, transport, mode string, elapsed time.Duration, err error, confirmed bool) {
	outcome, errorClass := publishTransportResult(err, confirmed)
	attrs := publishTransportAttributes(method, purpose, transport, mode, outcome, errorClass)
	// Separate bounded dimensions: request totals omit upload_mode; duration
	// omits error_class. No identifiers, raw methods or error text are labels.
	// Duration has at most 7 methods x 4 purposes x 3 transports x 6 upload
	// modes x 5 outcomes (2,520 label sets); actual send combinations are sparse.
	// mixed purpose denotes a relay package containing display and verify media.
	if m.requests != nil {
		m.requests.Add(ctx, 1, metric.WithAttributes(attrs[0], attrs[1], attrs[2], attrs[4], attrs[5]))
	}
	if m.duration != nil {
		m.duration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs[:5]...))
	}
}

// Call only immediately around a concrete client invocation, never around a
// retry wrapper, preparation, queue submission, or an aggregate send operation.
func observePublishTransportRequest(ctx context.Context, method, purpose, mode string, started time.Time, err error, confirmed bool) {
	elapsed := time.Since(started)
	publishTransportMetrics().request(ctx, method, purpose, "bot", mode, elapsed, err, confirmed)
	observePublishTransportDiagnostics(ctx, method, purpose, "bot", mode, elapsed, err, confirmed)
}

// Logs include existing causal IDs; metrics retain bounded dimensions only.
// Never attach request bodies, media sources or raw errors. One completion row
// per concrete call allows SLS diagnosis even when its trace was not sampled.
func observePublishTransportDiagnostics(ctx context.Context, method, purpose, transport, mode string, elapsed time.Duration, err error, confirmed bool) {
	outcome, errorClass := publishTransportResult(err, confirmed)
	attrs := publishTransportAttributes(method, purpose, transport, mode, outcome, errorClass)
	span := trace.SpanFromContext(ctx)
	traceID := ""
	if span.SpanContext().HasTraceID() {
		traceID = span.SpanContext().TraceID().String()
	}
	span.AddEvent("publish.api.request", trace.WithAttributes(append(attrs,
		attribute.Float64("duration_seconds", elapsed.Seconds()), attribute.String("trace_id", traceID))...))
	fields := g.Map{"log_type": "telegram_publish_api", "event": "publish_api_request", "durationSeconds": elapsed.Seconds(), "traceId": traceID}
	if base, ok := ctx.Value(telegramPublishTraceContextKey{}).(g.Map); ok {
		for _, key := range []string{"jobId", "profileId", "channelId", "tenantId", "accountId", "botId", "operationNo", "executionId", "attemptId"} {
			if value, exists := base[key]; exists {
				fields[key] = value
			}
		}
	}
	for _, attr := range attrs {
		fields[string(attr.Key)] = attr.Value.AsInterface()
	}
	if outcome != "success" || elapsed >= 5*time.Second {
		g.Log().Warning(ctx, "TG发布API调用异常或耗时较长", fields)
	} else {
		g.Log().Info(ctx, "TG发布API调用完成", fields)
	}
}

func (m *publishTransportInstruments) fallbackEvent(ctx context.Context, event, purpose, transport, mode, outcome, errorClass string) {
	if m.fallback == nil {
		return
	}
	attrs := publishTransportAttributes("other", purpose, transport, mode, outcome, errorClass)[1:]
	attrs = append(attrs, attribute.String("event", publishTransportBounded(event, "other",
		"requested", "queued", "submit_delayed", "rejected", "claimed", "completed", "recovered", "fallback_url", "fallback_upload", "fallback_source", "bot_switched")))
	m.fallback.Add(ctx, 1, metric.WithAttributes(attrs...))
}

func observePublishTransportFallback(ctx context.Context, event, purpose, transport, mode, outcome, errorClass string) {
	publishTransportMetrics().fallbackEvent(ctx, event, purpose, transport, mode, outcome, errorClass)
}

func (m *publishTransportInstruments) accountPhase(ctx context.Context, purpose string, elapsed time.Duration, err error, confirmed bool) {
	outcome, errorClass := publishTransportResult(err, confirmed)
	attrs := publishTransportAttributes("other", purpose, "account", "not_applicable", outcome, errorClass)
	if m.accountPhases != nil {
		m.accountPhases.Add(ctx, 1, metric.WithAttributes(attrs[1], attrs[4], attrs[5]))
	}
	if m.accountDuration != nil && elapsed >= 0 {
		m.accountDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs[1], attrs[4]))
	}
}

func publishTransportSourceMode(source string) string {
	switch {
	case strings.HasPrefix(source, "attach://"):
		return "binary"
	case strings.HasPrefix(source, "https://"), strings.HasPrefix(source, "http://"):
		return "url"
	case source != "":
		return "file_id"
	default:
		return "not_applicable"
	}
}

func publishTransportMessagesConfirmed(messages []*models.Message, expected int) bool {
	if expected <= 0 || len(messages) != expected {
		return false
	}
	for _, message := range messages {
		if message == nil || message.ID <= 0 {
			return false
		}
	}
	return true
}

func publishTransportCopiesConfirmed(messages []models.MessageID, expected int) bool {
	if expected <= 0 || len(messages) != expected {
		return false
	}
	for _, message := range messages {
		if message.ID <= 0 {
			return false
		}
	}
	return true
}

func publishTransportInputMode(input models.InputFile) string {
	switch input := input.(type) {
	case *models.InputFileUpload:
		return "binary"
	case *models.InputFileString:
		if input != nil {
			return publishTransportSourceMode(input.Data)
		}
	}
	return "not_applicable"
}

func publishTransportGroupMode(group []models.InputMedia) string {
	mode := ""
	for _, input := range group {
		itemMode := "not_applicable"
		switch input := input.(type) {
		case *models.InputMediaPhoto:
			if input != nil {
				itemMode = publishTransportSourceMode(input.Media)
			}
		case *models.InputMediaVideo:
			if input != nil {
				itemMode = publishTransportSourceMode(input.Media)
			}
		}
		if mode != "" && mode != itemMode {
			return "mixed"
		}
		mode = itemMode
	}
	return publishTransportBounded(mode, "not_applicable", "file_id", "url", "binary", "mixed")
}
