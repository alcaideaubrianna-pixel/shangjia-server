package sys

import (
	"context"
	"os"
	"strconv"
	"sync"
)

var telegramMediaUploadLimiter = struct {
	sync.Mutex
	gates map[int64]chan struct{}
}{gates: make(map[int64]chan struct{})}

func telegramMediaUploadConcurrency() int {
	if n, err := strconv.Atoi(os.Getenv("YOUBAN_TELEGRAM_MEDIA_UPLOAD_CONCURRENCY")); err == nil && n > 0 {
		if n > 32 {
			return 32
		}
		return n
	}
	return 2
}

func acquireTelegramMediaUpload(ctx context.Context, accountID int64) (func(), error) {
	if accountID <= 0 {
		return func() {}, nil
	}
	telegramMediaUploadLimiter.Lock()
	gate := telegramMediaUploadLimiter.gates[accountID]
	if gate == nil {
		gate = make(chan struct{}, telegramMediaUploadConcurrency())
		telegramMediaUploadLimiter.gates[accountID] = gate
	}
	telegramMediaUploadLimiter.Unlock()
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
