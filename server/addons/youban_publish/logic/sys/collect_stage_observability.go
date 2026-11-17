package sys

import (
	"context"
	"time"

	"hotgo/internal/library/collectobserve"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gtrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type collectPipelineInstruments struct {
	count    metric.Int64Counter
	duration metric.Float64Histogram
}

func newCollectPipelineInstruments(meter metric.Meter) collectPipelineInstruments {
	count, _ := meter.Int64Counter("xiaohuiji.collect.pipeline.operations",
		metric.WithDescription("Collection stage observations; retries are separate attempts"))
	duration, _ := meter.Float64Histogram("xiaohuiji.collect.pipeline.duration_seconds",
		metric.WithUnit("s"), metric.WithDescription("Collection stage execution time; nested stages must not be summed"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120))
	return collectPipelineInstruments{count: count, duration: duration}
}

func collectSourceAttemptWaitReason(pending bool, err error) string {
	if err != nil {
		return "error"
	}
	if pending {
		return "rescheduled"
	}
	return "none"
}

// bindCollectEventObservation only consumes the record already read by the
// business operation. received_at is Telegram's date and is not an ingress clock.
func bindCollectEventObservation(ctx context.Context, event gdb.Record) context.Context {
	return collectobserve.With(ctx, collectobserve.Metadata{
		TenantID: event["tenant_id"].Int64(), AccountID: event["account_id"].Int64(),
		SourceID: event["source_id"].Int64(), EventID: event["id"].Int64(),
		BotID: event["bot_id"].Int64(), TgAccountID: event["tg_account_id"].Int64(),
		ChatID: event["source_chat_id"].String(), SourceType: event["source_type"].String(),
	})
}

func collectContextTraceAttributes(ctx context.Context) []attribute.KeyValue {
	meta := collectobserve.From(ctx)
	return []attribute.KeyValue{
		attribute.String("source_type", collectobserve.SourceType(meta.SourceType)),
		attribute.String("collection_mode", collectobserve.Mode(meta.Mode)),
		attribute.Int64("tenant.id", meta.TenantID), attribute.Int64("account.id", meta.AccountID),
		attribute.Int64("collect.source_id", meta.SourceID), attribute.String("collect.chat_id", meta.ChatID),
		attribute.String("collect.canonical_chat_id", canonicalCollectSourceChatID(meta.ChatID)),
		attribute.Int64("collect.bot_id", meta.BotID), attribute.Int64("collect.tg_account_id", meta.TgAccountID),
		attribute.Int64("collect.event_id", meta.EventID), attribute.Int64("collect.upstream_event_id", meta.UpstreamEventID),
		attribute.Int64("collect.delivery_id", meta.DeliveryID), attribute.Int64("collect.rule_id", meta.RuleID),
		attribute.Int64("profile.id", meta.ProfileID),
	}
}

// startCollectPipelineStage creates a real parent span while the operation is
// running. Each caller must finish exactly once, including deferred/error exits.
func startCollectPipelineStage(ctx context.Context, stage collectPipelineStage) (context.Context, func(collectPipelineResult, error, ...attribute.KeyValue)) {
	startedAt := time.Now()
	normalized := collectPipelineMetricAttributes(stage, collectPipelineUnknown)
	attrs := append(collectContextTraceAttributes(ctx), normalized...)
	ctx, span := otel.Tracer("hotgo/addons/youban_publish").Start(ctx,
		"collect."+normalized[0].Value.AsString(), trace.WithAttributes(attrs...))
	return ctx, func(result collectPipelineResult, err error, extra ...attribute.KeyValue) {
		finishedAt := time.Now()
		recordCollectPipelineMetrics(ctx, publishObserveMeter, stage, result, finishedAt.Sub(startedAt))
		span.SetAttributes(collectPipelineMetricAttributes(stage, result)...)
		span.SetAttributes(extra...)
		span.SetAttributes(attribute.String("error.type", collectPipelineErrorType(err)))
		if result == collectPipelineFailed || (err != nil && result != collectPipelineRetryable && result != collectPipelineDuplicate && result != collectPipelineIgnored) {
			span.SetStatus(codes.Error, "collection stage failed")
		}
		span.End(trace.WithTimestamp(finishedAt))
		logCollectPipelineObservation(ctx, stage, result, startedAt, finishedAt, err, extra...)
	}
}

func logCollectPipelineObservation(ctx context.Context, stage collectPipelineStage, result collectPipelineResult, startedAt, finishedAt time.Time, err error, attrs ...attribute.KeyValue) {
	// Counters/histograms retain every attempt. Fast successful substeps do
	// not need an additional SLS row on top of the event terminal record.
	if err == nil && result == collectPipelineCompleted && finishedAt.Sub(startedAt) < 100*time.Millisecond && stage != collectPipelineProcess && stage != collectPipelineSourceAttempt {
		return
	}
	meta := collectobserve.From(ctx)
	normalized := collectPipelineMetricAttributes(stage, result)
	fields := g.Map{
		"log_type": "telegram_collect_pipeline", "event": "collect_stage_completed",
		"stage": normalized[0].Value.AsString(), "result": normalized[1].Value.AsString(),
		"durationMs": finishedAt.Sub(startedAt).Milliseconds(), "durationUs": finishedAt.Sub(startedAt).Microseconds(),
		"sourceType": collectobserve.SourceType(meta.SourceType), "collectionMode": collectobserve.Mode(meta.Mode),
		"tenantId": meta.TenantID, "accountId": meta.AccountID, "sourceId": meta.SourceID,
		"sourceChatId": meta.ChatID, "canonicalChatId": canonicalCollectSourceChatID(meta.ChatID), "botId": meta.BotID, "tgAccountId": meta.TgAccountID,
		"collectEventId": meta.EventID, "upstreamEventId": meta.UpstreamEventID, "deliveryId": meta.DeliveryID,
		"ruleId": meta.RuleID, "profileId": meta.ProfileID, "traceId": gtrace.GetTraceID(ctx),
		"errorType": collectPipelineErrorType(err),
	}
	for _, attr := range attrs {
		// Only bounded diagnostic facts are copied, never text/error messages,
		// URLs, tokens, or caller-controlled label names.
		switch attr.Key {
		case "collect.event_id":
			fields["collectEventId"] = attr.Value.AsInt64()
		case "profile.id":
			fields["profileId"] = attr.Value.AsInt64()
		case "collect.rule_id":
			fields["ruleId"] = attr.Value.AsInt64()
		case "collect.wait_reason", "collect.match_count", "collect.row_count", "collect.reused", "collect.wait_sample":
			fields[string(attr.Key)] = attr.Value.AsInterface()
		}
	}
	if result == collectPipelineFailed || (err != nil && result != collectPipelineRetryable && result != collectPipelineDuplicate && result != collectPipelineIgnored) {
		g.Log().Warning(ctx, "采集阶段执行失败", fields)
		return
	}
	g.Log().Info(ctx, "采集阶段执行完成", fields)
}
