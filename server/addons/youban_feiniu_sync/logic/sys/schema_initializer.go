package sys

import (
	"context"
	"sync"
)

// schemaInitializer suppresses repeated DDL, without caching failed attempts.
type schemaInitializer struct {
	mu    sync.Mutex
	ready bool
}

func (s *schemaInitializer) ensure(ctx context.Context, initialize func(context.Context) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.ready {
		return nil
	}
	if err := initialize(ctx); err != nil {
		return err
	}
	s.ready = true
	return nil
}
