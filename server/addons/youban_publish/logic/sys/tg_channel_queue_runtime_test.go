package sys

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTelegramChannelQueueDispatcherSlowChannelDoesNotBlockFastChannel(t *testing.T) {
	dispatcher := newTelegramChannelQueueDispatcher(2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	fastDone := make(chan struct{})
	var calls atomic.Int32

	if !dispatcher.submit(ctx, 1, func(int64) {
		close(slowStarted)
		<-releaseSlow
	}) {
		t.Fatal("slow channel was not scheduled")
	}
	<-slowStarted
	if !dispatcher.submit(ctx, 2, func(int64) {
		calls.Add(1)
		close(fastDone)
	}) {
		t.Fatal("fast channel was not scheduled")
	}

	select {
	case <-fastDone:
	case <-time.After(time.Second):
		t.Fatal("fast channel was blocked by slow channel")
	}
	// Completion notification precedes dispatcher cleanup; retry submission
	// until the freed slot becomes visible without releasing the slow channel.
	deadline := time.After(time.Second)
	second := make(chan struct{})
	for !dispatcher.submit(ctx, 2, func(int64) { calls.Add(1); close(second) }) {
		select {
		case <-deadline:
			t.Fatal("fast channel could not consume again while slow channel was running")
		default:
		}
		time.Sleep(time.Millisecond)
	}
	<-second
	if dispatcher.submit(ctx, 1, func(int64) { t.Error("slow channel started twice") }) {
		t.Error("running channel was accepted twice")
	}
	close(releaseSlow)
	dispatcher.wait()
	if got := calls.Load(); got != 2 {
		t.Fatalf("fast channel calls = %d, want 2", got)
	}
}

func TestTelegramChannelQueueDispatcherDeduplicatesAndBoundsConcurrency(t *testing.T) {
	dispatcher := newTelegramChannelQueueDispatcher(2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var active atomic.Int32
	var maxActive atomic.Int32
	var calls atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for i := int64(1); i <= 6; i++ {
		wg.Add(1)
		go func(channelID int64) {
			defer wg.Done()
			dispatcher.submit(ctx, channelID, func(int64) {
				current := active.Add(1)
				for {
					old := maxActive.Load()
					if current <= old || maxActive.CompareAndSwap(old, current) {
						break
					}
				}
				calls.Add(1)
				started <- struct{}{}
				<-release
				active.Add(-1)
			})
		}(i)
	}
	wg.Wait()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("running calls = %d, want 2", got)
	}
	if got := maxActive.Load(); got > 2 {
		t.Fatalf("max active = %d, want <= 2", got)
	}
	close(release)
	dispatcher.wait()

	if dispatcher.submit(ctx, 1, func(int64) { calls.Add(1) }) {
		dispatcher.wait()
	} else {
		t.Fatal("completed channel could not be scheduled again")
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls after reschedule = %d, want 3", got)
	}
}

func TestTelegramChannelQueueDispatcherWaitsForCancellationAwareWorker(t *testing.T) {
	dispatcher := newTelegramChannelQueueDispatcher(1)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	finished := make(chan struct{})
	if !dispatcher.submit(ctx, 9, func(int64) {
		close(started)
		<-ctx.Done()
		close(finished)
	}) {
		t.Fatal("channel was not scheduled")
	}
	<-started
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("worker did not observe context cancellation")
	}
	dispatcher.wait()
}

func TestTelegramChannelQueueScheduleRecoveryDoesNotBlockPolling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	recoveryTicks := make(chan time.Time)
	recovered := make(chan struct{}, 4)
	polled := make(chan struct{}, 4)
	done := make(chan struct{})
	var calls atomic.Int32
	go func() {
		defer close(done)
		runTelegramChannelQueueSchedule(ctx, ticks, recoveryTicks,
			func(context.Context) { polled <- struct{}{} },
			func(ctx context.Context) { calls.Add(1); recovered <- struct{}{}; <-ctx.Done() })
	}()
	send := func(ch chan time.Time) {
		t.Helper()
		select {
		case ch <- time.Now():
		case <-time.After(time.Second):
			t.Fatal("scheduler blocked")
		}
	}
	send(recoveryTicks)
	select {
	case <-recovered:
	case <-time.After(time.Second):
		t.Fatal("recovery did not start")
	}
	for i := 0; i < 3; i++ {
		send(recoveryTicks)
		send(ticks)
		select {
		case <-polled:
		case <-time.After(time.Second):
			t.Fatal("slow recovery blocked polling")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("recovery overlapped")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not join recovery on cancellation")
	}
}

func TestTelegramChannelQueueDispatcherRotatesAndRejectsCancelled(t *testing.T) {
	d := newTelegramChannelQueueDispatcher(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	channels := []int64{1, 2, 3}
	for _, want := range channels {
		started := make(chan int64, 1)
		release := make(chan struct{})
		d.dispatch(ctx, channels, func(id int64) { started <- id; <-release })
		select {
		case got := <-started:
			if got != want {
				t.Errorf("scheduled %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("worker did not start")
		}
		close(release)
		d.wait()
	}
	cancel()
	if d.submit(ctx, 1, func(int64) { t.Error("cancelled callback started") }) {
		t.Fatal("cancelled context accepted")
	}
	d.wait()
}
