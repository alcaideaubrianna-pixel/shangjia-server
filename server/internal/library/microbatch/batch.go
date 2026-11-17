// Package microbatch coalesces concurrent writes with a bounded single writer.
package microbatch

import (
	"context"
	"fmt"
	"time"
)

type Result[R any] struct {
	Value R
	Err   error
}
type request[T, R any] struct {
	ctx   context.Context
	value T
	reply chan Result[R]
}
type Batcher[T, R any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	in     chan request[T, R]
}

func New[T, R any](parent context.Context, size int, wait time.Duration, write func(context.Context, []T) []Result[R]) *Batcher[T, R] {
	if size < 1 {
		size = 1
	}
	if wait < 0 {
		wait = 0
	}
	ctx, cancel := context.WithCancel(parent)
	b := &Batcher[T, R]{ctx: ctx, cancel: cancel, done: make(chan struct{}), in: make(chan request[T, R], size*2)}
	go b.run(size, wait, write)
	return b
}

func (b *Batcher[T, R]) Do(ctx context.Context, value T) (zero R, err error) {
	r := request[T, R]{ctx: ctx, value: value, reply: make(chan Result[R], 1)}
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-b.ctx.Done():
		return zero, b.ctx.Err()
	case b.in <- r:
	}
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-b.ctx.Done():
		return zero, b.ctx.Err()
	case result := <-r.reply:
		return result.Value, result.Err
	}
}

func (b *Batcher[T, R]) Close() { b.cancel(); <-b.done }

func (b *Batcher[T, R]) run(size int, wait time.Duration, write func(context.Context, []T) []Result[R]) {
	defer close(b.done)
	for {
		var first request[T, R]
		select {
		case <-b.ctx.Done():
			return
		case first = <-b.in:
		}
		batch := []request[T, R]{first}
		timer := time.NewTimer(wait)
	collect:
		for len(batch) < size {
			select {
			case <-b.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				break collect
			case r := <-b.in:
				batch = append(batch, r)
			}
		}
		timer.Stop()
		live := batch[:0]
		values := make([]T, 0, len(batch))
		for _, r := range batch {
			if err := r.ctx.Err(); err != nil {
				r.reply <- Result[R]{Err: err}
				continue
			}
			live = append(live, r)
			values = append(values, r.value)
		}
		if len(live) == 0 {
			continue
		}
		ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
		results := invoke(ctx, values, write)
		cancel()
		for i, r := range live {
			if len(results) != len(live) {
				r.reply <- Result[R]{Err: fmt.Errorf("microbatch result count mismatch")}
				continue
			}
			r.reply <- results[i]
		}
	}
}

func invoke[T, R any](ctx context.Context, values []T, write func(context.Context, []T) []Result[R]) (out []Result[R]) {
	defer func() {
		if p := recover(); p != nil {
			out = make([]Result[R], len(values))
			for i := range out {
				out[i].Err = fmt.Errorf("microbatch writer panic: %v", p)
			}
		}
	}()
	return write(ctx, values)
}
