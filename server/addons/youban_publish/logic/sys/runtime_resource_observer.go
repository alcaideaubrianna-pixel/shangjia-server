package sys

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const runtimeResourceObserveInterval = time.Minute

func startRuntimeResourceObserver(ctx context.Context, config publishRuntimeConfig) {
	if !config.Account && !config.PushWorker {
		return
	}
	go func() {
		logRuntimeResources(ctx, config, "started")
		ticker := time.NewTicker(runtimeResourceObserveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				logRuntimeResources(context.Background(), config, "stopped")
				return
			case <-ticker.C:
				logRuntimeResources(ctx, config, "heartbeat")
			}
		}
	}()
}

func logRuntimeResources(ctx context.Context, config publishRuntimeConfig, event string) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	g.Log().Info(ctx, "运行角色资源水位", g.Map{
		"event":            event,
		"roles":            config.Roles,
		"goHeapAllocMiB":   bytesToMiB(stats.HeapAlloc),
		"goHeapInuseMiB":   bytesToMiB(stats.HeapInuse),
		"goSysMiB":         bytesToMiB(stats.Sys),
		"goGoroutines":     runtime.NumGoroutine(),
		"cgroupCurrentMiB": cgroupBytesToMiB(readRuntimeMetric("/sys/fs/cgroup/memory.current")),
		"cgroupMaxMiB":     cgroupBytesToMiB(readRuntimeMetric("/sys/fs/cgroup/memory.max")),
		"cgroupEvents":     readRuntimeMetric("/sys/fs/cgroup/memory.events"),
	})
}

func readRuntimeMetric(path string) string {
	value, err := os.ReadFile(path)
	if err != nil {
		return "unavailable"
	}
	return strings.TrimSpace(string(value))
}

func bytesToMiB(value uint64) uint64 {
	return value / 1024 / 1024
}

func cgroupBytesToMiB(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "max" || value == "unavailable" {
		return value
	}
	var bytes uint64
	if _, err := fmt.Sscan(value, &bytes); err != nil {
		return value
	}
	return fmt.Sprintf("%d", bytesToMiB(bytes))
}
