// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/lazysheep_tggo/logic/shared"
	lsysin "hotgo/addons/lazysheep_tggo/model/input/sysin"
)

var (
	errPullNoteLimitReached = errors.New("pull note limit reached")
	errPullOldCursorReached = errors.New("pull old cursor reached")
)

type pullTimer struct {
	ctx     context.Context
	enabled bool
	started time.Time
	last    time.Time
	steps   []lsysin.PullMonitorStep
}

func newPullTimer(ctx context.Context) *pullTimer {
	now := time.Now()
	mode := strings.TrimSpace(g.Cfg().MustGet(ctx, "system.mode").String())
	return &pullTimer{
		ctx:     ctx,
		enabled: mode == "develop",
		started: now,
		last:    now,
	}
}

func (t *pullTimer) Report(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	now := time.Now()
	step := now.Sub(t.last).Round(time.Millisecond)
	total := now.Sub(t.started).Round(time.Millisecond)
	t.last = now
	t.steps = append(t.steps, lsysin.PullMonitorStep{
		Name:      text,
		StepMs:    step.Milliseconds(),
		ElapsedMs: total.Milliseconds(),
	})
	if !t.enabled {
		shared.ReportPullProgress(t.ctx, text)
		return
	}
	shared.ReportPullProgress(t.ctx, fmt.Sprintf("%s\n耗时：本步 %s，总计 %s", text, step, total))
}

func (t *pullTimer) Append(text string) string {
	if !t.enabled {
		return text
	}
	return fmt.Sprintf("%s\n耗时：总计 %s", text, time.Since(t.started).Round(time.Millisecond))
}

func (t *pullTimer) Steps() []lsysin.PullMonitorStep {
	if t == nil {
		return nil
	}
	return append([]lsysin.PullMonitorStep{}, t.steps...)
}
