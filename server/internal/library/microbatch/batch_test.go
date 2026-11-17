package microbatch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchCoalescesWithSingleWriter(t *testing.T) {
	var active, maxActive, maxBatch atomic.Int32
	b := New(context.Background(), 8, 5*time.Millisecond, func(ctx context.Context, input []int) []Result[int] {
		n := active.Add(1)
		defer active.Add(-1)
		maxActive.Store(max(maxActive.Load(), n))
		maxBatch.Store(max(maxBatch.Load(), int32(len(input))))
		out := make([]Result[int], len(input))
		for i, n := range input {
			out[i].Value = n * 2
		}
		return out
	})
	defer b.Close()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			got, err := b.Do(context.Background(), n)
			if err != nil || got != n*2 {
				t.Errorf("result=%d err=%v", got, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if maxActive.Load() != 1 || maxBatch.Load() < 2 || maxBatch.Load() > 8 {
		t.Fatalf("writers=%d batch=%d", maxActive.Load(), maxBatch.Load())
	}
}

func TestBatchCancellationFailureAndClose(t *testing.T) {
	want := errors.New("write failed")
	b := New(context.Background(), 5, time.Millisecond, func(ctx context.Context, input []int) []Result[int] {
		out := make([]Result[int], len(input))
		for i := range out {
			out[i].Err = want
		}
		return out
	})
	if _, err := b.Do(context.Background(), 1); !errors.Is(err, want) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Do(ctx, 2); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	b.Close()
	if _, err := b.Do(context.Background(), 3); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
