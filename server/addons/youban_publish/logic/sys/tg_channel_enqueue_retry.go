package sys

import (
	"context"
	"errors"
	"math/rand/v2"
	"net"
	"strings"
	"time"
)

type telegramChannelOutboxRetryKey struct{}

// Only a caller already holding a persisted Outbox row may defer an ambiguous
// timeout to the durable dispatcher. Direct/shadow callers retain three tries.
func telegramChannelOutboxRetryContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, telegramChannelOutboxRetryKey{}, true)
}

func telegramChannelEnqueueCanRetry(ctx context.Context, err error, durable bool) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	if !durable {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return false
	}
	message := strings.ToUpper(err.Error())
	if strings.Contains(message, "TIMEOUT") || strings.Contains(message, "DEADLINE EXCEEDED") {
		return false
	}
	// These Redis responses reject execution before the enqueue script runs.
	// All other errors (OOM, WRONGTYPE, auth, ambiguous connection failures)
	// return to the existing Outbox retry path instead of adding local load.
	for _, code := range []string{"TRYAGAIN", "LOADING", "BUSY", "READONLY", "CLUSTERDOWN", "MASTERDOWN"} {
		if strings.HasPrefix(message, code+" ") || message == code {
			return true
		}
	}
	return false
}

func telegramChannelEnqueueRetryDelay(attempt int) time.Duration {
	base := 50 * time.Millisecond
	if attempt > 1 {
		base *= 2
	}
	return base + time.Duration(rand.Int64N(int64(base)+1))
}

func runTelegramChannelEnqueueRetries(ctx context.Context, durable bool, enqueue func() error) (attempts int, err error) {
	for attempts < 3 {
		if ctx.Err() != nil {
			return attempts, ctx.Err()
		}
		attempts++
		err = enqueue()
		if err != nil && ctx.Err() != nil {
			return attempts, ctx.Err()
		}
		if err == nil || attempts == 3 || !telegramChannelEnqueueCanRetry(ctx, err, durable) {
			return attempts, err
		}
		timer := time.NewTimer(telegramChannelEnqueueRetryDelay(attempts))
		select {
		case <-ctx.Done():
			timer.Stop()
			return attempts, ctx.Err()
		case <-timer.C:
		}
	}
	return attempts, err
}
