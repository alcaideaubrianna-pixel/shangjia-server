package sys

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSchemaInitializerRetriesFailureAndSerializesSuccess(t *testing.T) {
	var initializer schemaInitializer
	want := errors.New("lock timeout")
	if err := initializer.ensure(context.Background(), func(context.Context) error { return want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
	var calls atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := initializer.ensure(context.Background(), func(context.Context) error { calls.Add(1); return nil }); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("successful DDL executed %d times", calls.Load())
	}
}

func TestSchemaInitializerCanceledContextDoesNotInitialize(t *testing.T) {
	var initializer schemaInitializer
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := initializer.ensure(ctx, func(context.Context) error { t.Fatal("DDL executed after cancellation"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if initializer.ready {
		t.Fatal("canceled initialization cached")
	}
}
