package streamgray

import (
	"context"
	"sync"
)

// RunBothStreams 同时运行实时和批量两个 Stream Consumer。
// 任一 Consumer 返回错误后会取消另一条，交由 Worker supervisor 统一重启。
func RunBothStreams(ctx context.Context, consumer *RedisGroupConsumer, handler func(context.Context, ReadyEvent) error) error {
	if consumer == nil || handler == nil {
		return ErrInvalidRuntime
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, stream := range []string{ReadyRealtimeStream, ReadyBulkStream} {
		stream := stream
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- consumer.Run(runCtx, stream, handler)
		}()
	}
	firstErr := <-errs
	cancel()
	wg.Wait()
	if firstErr == context.Canceled && ctx.Err() != nil {
		return ctx.Err()
	}
	return firstErr
}
