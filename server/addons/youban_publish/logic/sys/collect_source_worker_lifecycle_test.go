package sys

import (
	"context"
	"testing"
	"time"
)

func TestCollectSourceRuntimeStopCancelsAndJoinsWithoutParentCancel(t *testing.T) {
	parent := context.Background()
	execution, cancel := context.WithCancel(parent)
	s := &sSysPublish{}
	control := &collectSourceRuntimeControl{cancel: cancel, done: make(chan struct{})}
	collectSourceRuntimes.Store(s, control)
	t.Cleanup(func() { cancel(); collectSourceRuntimes.Delete(s) })
	exited := make(chan struct{})
	go func() {
		<-execution.Done()
		close(exited)
		collectSourceRuntimes.CompareAndDelete(s, control)
		close(control.done)
	}()
	s.stopCollectSourceRuntime()
	select {
	case <-exited:
	default:
		t.Fatal("stop returned before workers exited")
	}
	if parent.Err() != nil {
		t.Fatal("stop required cancelling the parent")
	}
	if _, ok := collectSourceRuntimes.Load(s); ok {
		t.Fatal("runtime registry retained stopped worker")
	}
}

func TestCollectSourceRuntimeCancelledStartupCanRestart(t *testing.T) {
	s := &sSysPublish{}
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s.startCollectSourceRuntime(ctx)
		s.stopCollectSourceRuntime()
		deadline := time.Now().Add(time.Second)
		for {
			if _, ok := collectSourceRuntimes.Load(s); !ok {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("runtime startup leaked")
			}
			time.Sleep(time.Millisecond)
		}
		if len(s.collectSourceQueueServers) != 0 {
			t.Fatal("allocated a source-specific server")
		}
	}
}
