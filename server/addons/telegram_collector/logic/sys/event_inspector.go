package sys

import (
	"context"
	"github.com/hibiken/asynq"
)

// Share the Redis connection pool; recovery must not dial once per candidate.
func (s *sCollector) eventTaskInspector(ctx context.Context) *asynq.Inspector {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	if s.queueInspector == nil {
		s.queueInspector = asynq.NewInspector(collectorRedisOption(ctx))
	}
	return s.queueInspector
}
