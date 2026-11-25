package sys

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	collectDedupeDecisions, _ = publishObserveMeter.Int64Counter(
		"xiaohuiji.collect.dedupe.decisions",
		metric.WithDescription("采集资料去重判断次数"),
	)
	collectDedupeReplacements, _ = publishObserveMeter.Int64Counter(
		"xiaohuiji.collect.dedupe.replacements",
		metric.WithDescription("采集资料重复覆盖次数"),
	)
	collectDedupeLSHCandidates, _ = publishObserveMeter.Int64Counter(
		"xiaohuiji.collect.dedupe.lsh.candidates",
		metric.WithDescription("采集资料LSH候选媒体数量"),
	)
	collectDedupeLSHMatches, _ = publishObserveMeter.Int64Counter(
		"xiaohuiji.collect.dedupe.lsh.matches",
		metric.WithDescription("采集资料LSH匹配资料数量"),
	)
	collectDedupeLSHMatchedImages, _ = publishObserveMeter.Int64Counter(
		"xiaohuiji.collect.dedupe.lsh.matched_images",
		metric.WithDescription("采集资料LSH匹配图片数量"),
	)
	collectDedupeLSHDuration, _ = publishObserveMeter.Float64Histogram(
		"xiaohuiji.collect.dedupe.lsh.duration_seconds",
		metric.WithDescription("采集资料LSH查询和匹配耗时"),
	)
)

func observeCollectDedupeDecision(ctx context.Context, result string) {
	collectDedupeDecisions.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func observeCollectDedupeReplacement(ctx context.Context, result string) {
	collectDedupeReplacements.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func observeCollectDedupeLSH(ctx context.Context, duration time.Duration, candidateCount, matchCount, matchedImageCount int, result string) {
	attrs := metric.WithAttributes(attribute.String("result", result))
	if candidateCount > 0 {
		collectDedupeLSHCandidates.Add(ctx, int64(candidateCount), attrs)
	}
	if matchCount > 0 {
		collectDedupeLSHMatches.Add(ctx, int64(matchCount), attrs)
	}
	if matchedImageCount > 0 {
		collectDedupeLSHMatchedImages.Add(ctx, int64(matchedImageCount), attrs)
	}
	collectDedupeLSHDuration.Record(ctx, duration.Seconds(), attrs)
}
