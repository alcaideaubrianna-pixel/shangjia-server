package sys

import (
	"context"
	"errors"
	"sync"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	collectorInventorySourcesMetric = "telegram_collector_sources_loaded"
	collectorInventoryEntriesMetric = "telegram_collector_message_entries_loaded"
	collectorInventoryValidMetric   = "telegram_collector_inventory_snapshot_valid"
	// Six normal account-runtime refresh intervals. No timer or background work
	// is created: freshness is checked only when the SDK collects metrics.
	collectorInventoryMaxAge = 90 * time.Second
)

type collectorInventorySnapshot struct {
	ctx     context.Context
	at      time.Time
	sources int64
	entries int64
}

type collectorInventoryObserver struct {
	mu       sync.RWMutex
	snapshot collectorInventorySnapshot
	now      func() time.Time
}

var (
	collectorInventoryOnce     sync.Once
	collectorInventoryInstance *collectorInventoryObserver
)

// ObserveAccountInventory replaces the local provider-list snapshot. loaded is
// false on provider absence or load failure, invalidating the previous snapshot.
//
// Coverage is instance_loaded, never a global enabled total or leased/online
// connection count. Entries are distinct runtime AccountIDs with at least one
// valid collection source; listener-only and task-only bindings do not count.
// Sources are distinct (TenantID, SourceID) pairs in those bindings. AccountID
// inside AccountRuntimeSource is the business account, NOT the runtime AccountID.
//
// Bot inventory is deliberately absent: per-Bot/message routing lookups cannot
// supply a complete Bot configuration inventory. No IDs are metric attributes.
// Each resource/instance is a separate observation; neither sum nor max across
// replicas yields a global enabled inventory.
func ObserveAccountInventory(ctx context.Context, bindings []*sysin.AccountRuntimeBinding, loaded bool) {
	collectorInventoryOnce.Do(func() {
		observer, err := newCollectorInventoryObserver(otel.Meter("hotgo/addons/telegram_collector/inventory"), time.Now)
		if err != nil {
			g.Log().Warning(ctx, "初始化采集配置快照指标失败", g.Map{"event": "collector.inventory.init_failed"})
			return
		}
		collectorInventoryInstance = observer
	})
	if collectorInventoryInstance != nil {
		collectorInventoryInstance.replace(ctx, bindings, loaded)
	}
}

func newCollectorInventoryObserver(meter metric.Meter, now func() time.Time) (*collectorInventoryObserver, error) {
	sources, sourcesErr := meter.Int64ObservableGauge(collectorInventorySourcesMetric,
		metric.WithUnit("{source}"), metric.WithDescription("Distinct collection sources in this instance's account-runtime provider list; not global enabled inventory"))
	entries, entriesErr := meter.Int64ObservableGauge(collectorInventoryEntriesMetric,
		metric.WithUnit("{entry}"), metric.WithDescription("Distinct protocol accounts with collection sources in this instance's provider list; not online or lease ownership"))
	valid, validErr := meter.Int64ObservableGauge(collectorInventoryValidMetric,
		metric.WithUnit("1"), metric.WithDescription("1 only while the loaded inventory snapshot is fresh and its runtime context is live; gate inventory queries with this metric"))
	if err := errors.Join(sourcesErr, entriesErr, validErr); err != nil {
		return nil, err
	}
	observer := &collectorInventoryObserver{now: now}
	options := metric.WithAttributes(
		attribute.String("coverage", "instance_loaded"),
		attribute.String("loader", "account_runtime_provider"),
		attribute.String("entry_type", "protocol_account"),
	)
	_, err := meter.RegisterCallback(func(_ context.Context, output metric.Observer) error {
		observer.mu.RLock()
		snapshot := observer.snapshot
		observer.mu.RUnlock()
		age := observer.now().Sub(snapshot.at)
		if snapshot.ctx == nil || snapshot.ctx.Err() != nil || snapshot.at.IsZero() || age < 0 || age >= collectorInventoryMaxAge {
			output.ObserveInt64(valid, 0, options)
			return nil
		}
		output.ObserveInt64(valid, 1, options)
		output.ObserveInt64(sources, snapshot.sources, options)
		output.ObserveInt64(entries, snapshot.entries, options)
		return nil
	}, sources, entries, valid)
	if err != nil {
		return nil, err
	}
	return observer, nil
}

func (o *collectorInventoryObserver) replace(ctx context.Context, bindings []*sysin.AccountRuntimeBinding, loaded bool) {
	var snapshot collectorInventorySnapshot
	if loaded && ctx != nil && ctx.Err() == nil {
		sourceIDs := make(map[[2]int64]struct{})
		entryIDs := make(map[int64]struct{})
		for _, binding := range bindings {
			if binding == nil || binding.AccountID <= 0 {
				continue
			}
			for _, source := range binding.Sources {
				if source.TenantID <= 0 || source.SourceID <= 0 {
					continue
				}
				sourceIDs[[2]int64{source.TenantID, source.SourceID}] = struct{}{}
				entryIDs[binding.AccountID] = struct{}{}
			}
		}
		snapshot = collectorInventorySnapshot{
			ctx: ctx, at: o.now(), sources: int64(len(sourceIDs)), entries: int64(len(entryIDs)),
		}
	}
	o.mu.Lock()
	o.snapshot = snapshot
	o.mu.Unlock()
}
