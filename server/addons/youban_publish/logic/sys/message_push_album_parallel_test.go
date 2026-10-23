package sys

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelOrderedMapBoundsConcurrencyAndPreservesOrder(t *testing.T) {
	input := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	var active atomic.Int32
	var maximum atomic.Int32
	started := time.Now()
	result, err := parallelOrderedMap(context.Background(), input, 2, func(_ context.Context, index, value int) (int, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		return value + index, nil
	})
	if err != nil {
		t.Fatalf("parallel map: %v", err)
	}
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrency = %d, want 2", maximum.Load())
	}
	if elapsed := time.Since(started); elapsed >= 350*time.Millisecond {
		t.Fatalf("ten media uploads remained effectively sequential: %s", elapsed)
	}
	for index, value := range result {
		if value != index*2 {
			t.Fatalf("result[%d] = %d, want %d", index, value, index*2)
		}
	}
}

func TestParallelOrderedMapCancelsSiblingsAfterError(t *testing.T) {
	cause := errors.New("upload failed")
	started := make(chan struct{})
	var once sync.Once
	_, err := parallelOrderedMap(context.Background(), []int{0, 1, 2}, 2, func(ctx context.Context, _ int, value int) (int, error) {
		if value == 0 {
			once.Do(func() { close(started) })
			return 0, cause
		}
		<-started
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Second):
			return value, nil
		}
	})
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want %v", err, cause)
	}
}
