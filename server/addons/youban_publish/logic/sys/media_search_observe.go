package sys

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	botMediaSearchMetricsOnce sync.Once
	botMediaSearchStageCount  metric.Int64Counter
	botMediaSearchStageSum    metric.Float64Counter
)

func initBotMediaSearchMetrics() {
	botMediaSearchMetricsOnce.Do(func() {
		meter := otel.Meter("hotgo/addons/youban_publish/media_search")
		botMediaSearchStageCount, _ = meter.Int64Counter("xiaohuiji.bot.scan.fingerprint.stage.duration.count")
		botMediaSearchStageSum, _ = meter.Float64Counter("xiaohuiji.bot.scan.fingerprint.stage.duration.sum", metric.WithUnit("s"))
	})
}

func observeBotMediaSearchStage(ctx context.Context, stage string, startedAt time.Time, stageErr error) {
	initBotMediaSearchMetrics()
	result := "success"
	if stageErr != nil {
		result = "failed"
	}
	attrs := metric.WithAttributes(attribute.String("stage", stage), attribute.String("result", result))
	if botMediaSearchStageCount != nil {
		botMediaSearchStageCount.Add(ctx, 1, attrs)
	}
	if botMediaSearchStageSum != nil {
		botMediaSearchStageSum.Add(ctx, time.Since(startedAt).Seconds(), attrs)
	}
}
