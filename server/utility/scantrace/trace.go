// Package scantrace provides diagnostic context shared by scan entry and search.
// It must never change business deadlines, retries or search results.
package scantrace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

type contextKey struct{}

var privateURL = regexp.MustCompile(`https?://[^\s"<>]+`)
var privateToken = regexp.MustCompile(`[0-9]{6,}:[A-Za-z0-9_-]{20,}`)

// Record emits a structured stage summary without changing business results.
// Callers should pass counts/statuses, never tokens, URLs or media payloads.
func Record(ctx context.Context, stage string, fields g.Map, err error) {
	base, enabled := ctx.Value(contextKey{}).(g.Map)
	if !enabled {
		return
	}
	data := g.Map{}
	for key, value := range base {
		data[key] = value
	}
	for key, value := range fields {
		data[key] = value
	}
	data["log_type"], data["stage"], data["event"] = "bot_scan_stage", stage, "summary"
	data["instance"], _ = os.Hostname()
	write(ctx, data, err)
}

func write(ctx context.Context, data g.Map, err error) {
	if deadline, ok := ctx.Deadline(); ok {
		data["remainingMs"] = time.Until(deadline).Milliseconds()
	}
	if err != nil {
		data["error"] = privateToken.ReplaceAllString(privateURL.ReplaceAllString(err.Error(), "[url]"), "[token]")
		data["timeout"] = errors.Is(err, context.DeadlineExceeded)
	}
	if ctx.Err() != nil {
		data["contextError"] = ctx.Err().Error()
	}
	body, marshalErr := json.Marshal(data)
	if marshalErr != nil {
		return
	}
	if err != nil || data["event"] == "slow" {
		g.Log().Warningf(ctx, "Bot扫图链路 %s", body)
	} else {
		g.Log().Infof(ctx, "Bot扫图链路 %s", body)
	}
}

// With copies fields so concurrent media stages cannot mutate their parent.
func With(ctx context.Context, fields g.Map) context.Context {
	data := g.Map{}
	if base, ok := ctx.Value(contextKey{}).(g.Map); ok {
		for key, value := range base {
			data[key] = value
		}
	}
	for key, value := range fields {
		data[key] = value
	}
	return context.WithValue(ctx, contextKey{}, data)
}

// Begin emits a searchable single-line JSON record and one bounded slow warning.
// Call the returned function on every exit, including errors.
func Begin(ctx context.Context, stage string) func(error) {
	base, enabled := ctx.Value(contextKey{}).(g.Map)
	if !enabled {
		return func(error) {}
	}
	fields := g.Map{"log_type": "bot_scan_stage", "stage": stage, "stageId": guid.S()}
	for key, value := range base {
		fields[key] = value
	}
	fields["instance"], _ = os.Hostname()
	started := time.Now()
	var ended atomic.Bool
	emit := func(event string, err error) {
		data := g.Map{}
		for key, value := range fields {
			data[key] = value
		}
		data["event"], data["durationMs"] = event, time.Since(started).Milliseconds()
		write(ctx, data, err)
	}
	emit("start", nil)
	timer := time.AfterFunc(5*time.Second, func() {
		if !ended.Load() {
			emit("slow", nil)
		}
	})
	return func(err error) {
		if ended.Swap(true) {
			return
		}
		timer.Stop()
		emit("end", err)
	}
}
