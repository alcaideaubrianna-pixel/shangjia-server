package publishmedia

import (
	"context"
	"sync"
)

type Item struct {
	ID         int64
	ProfileID  int64
	Type       string
	URL        string
	PreviewURL string
}

type Provider func(ctx context.Context, profileIDs []int64) (map[int64][]Item, error)

var (
	provider Provider
	mu       sync.RWMutex
)

func Register(value Provider) {
	mu.Lock()
	provider = value
	mu.Unlock()
}

func Resolve(ctx context.Context, profileIDs []int64) (map[int64][]Item, error) {
	mu.RLock()
	resolver := provider
	mu.RUnlock()
	if resolver == nil {
		return map[int64][]Item{}, nil
	}
	return resolver(ctx, profileIDs)
}
