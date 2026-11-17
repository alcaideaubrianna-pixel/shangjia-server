package sys

import (
	"context"
	"errors"
	"sync"
)

// This registry belongs to one synchronous worker invocation. It closes only
// diagnostic stages left open on a panic/early return, never business work.
type publishStageRegistryKey struct{}

type publishStageRegistry struct {
	mu     sync.Mutex
	stages map[*telegramPublishStage]struct{}
	closed bool
}

var errPublishStageAbandoned = errors.New("publishing execution ended before stage finished")

func withPublishStageRegistry(ctx context.Context) (context.Context, *publishStageRegistry) {
	registry := &publishStageRegistry{stages: make(map[*telegramPublishStage]struct{})}
	return context.WithValue(ctx, publishStageRegistryKey{}, registry), registry
}

func registerPublishStage(ctx context.Context, stage *telegramPublishStage) {
	registry, _ := ctx.Value(publishStageRegistryKey{}).(*publishStageRegistry)
	if registry == nil {
		return
	}
	stage.registry = registry
	registry.mu.Lock()
	closed := registry.closed
	if !closed {
		registry.stages[stage] = struct{}{}
	}
	registry.mu.Unlock()
	if closed {
		stage.End(errPublishStageAbandoned)
	}
}

func (r *publishStageRegistry) remove(stage *telegramPublishStage) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.stages, stage)
	r.mu.Unlock()
}

func (r *publishStageRegistry) close(cause error) {
	r.mu.Lock()
	stages := make([]*telegramPublishStage, 0, len(r.stages))
	for stage := range r.stages {
		stages = append(stages, stage)
	}
	r.closed = true
	r.stages = nil
	r.mu.Unlock()
	// A stage still present in the registry never reached its own End/Finish;
	// preserve that fact even when the enclosing execution returned an error.
	cause = errPublishStageAbandoned
	for _, stage := range stages {
		stage.End(cause)
	}
}
