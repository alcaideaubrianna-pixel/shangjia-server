package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultFullPushExpandWorkerCount = 4
	defaultFullPushPageSize          = 100
	defaultFullPushCandidateCount    = 20
	defaultFullPushSchedulerInterval = 2 * time.Second
	defaultFullPushExpandLeaseTTL    = 60 * time.Second
	defaultFullPushPendingJobLimit   = 300
)

func fullPushExpandWorkerCount(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.fullPush.expandWorkerCount", defaultFullPushExpandWorkerCount).Int()
	if value < 1 {
		return 1
	}
	if value > 32 {
		return 32
	}
	return value
}

func fullPushPageSize(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.fullPush.pageSize", defaultFullPushPageSize).Int()
	if value < 20 {
		return 20
	}
	if value > 500 {
		return 500
	}
	return value
}

func fullPushCandidateCount(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.fullPush.candidateCount", defaultFullPushCandidateCount).Int()
	if value < 1 {
		return 1
	}
	if value > 200 {
		return 200
	}
	return value
}

func fullPushSchedulerInterval(ctx context.Context) time.Duration {
	seconds := g.Cfg().MustGet(ctx, "youbanPublish.fullPush.schedulerIntervalSeconds", int(defaultFullPushSchedulerInterval/time.Second)).Int()
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 60 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func fullPushExpandLeaseTTL(ctx context.Context) time.Duration {
	seconds := g.Cfg().MustGet(ctx, "youbanPublish.fullPush.expandLeaseSeconds", int(defaultFullPushExpandLeaseTTL/time.Second)).Int()
	if seconds < 10 {
		seconds = 10
	}
	maxSeconds := int((10 * time.Minute) / time.Second)
	if seconds > maxSeconds {
		seconds = maxSeconds
	}
	return time.Duration(seconds) * time.Second
}

func fullPushPendingJobLimit(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.fullPush.pendingJobLimit", defaultFullPushPendingJobLimit).Int()
	if value < 50 {
		return 50
	}
	if value > 2000 {
		return 2000
	}
	return value
}
