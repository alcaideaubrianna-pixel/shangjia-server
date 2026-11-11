package sys

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestStreamConsumerReconnectsAfterFailure 验证连接失败后会重新执行消费，关闭时立即退出。
func TestStreamConsumerReconnectsAfterFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	calls := 0
	runStreamGrayUntilCancelled(ctx, func(ctx context.Context) error {
		calls++
		if calls == 1 {
			return errors.New("local simulated Redis disconnect")
		}
		cancel()
		return ctx.Err()
	})
	if calls != 2 {
		t.Fatal("consumer did not reconnect", calls)
	}
}
