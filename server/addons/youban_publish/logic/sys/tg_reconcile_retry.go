package sys

import (
	"fmt"
	"time"
)

const telegramReconcileTakeoverWait = 5 * time.Minute

type telegramReconcileRetryError struct {
	delay time.Duration
}

func (e *telegramReconcileRetryError) Error() string {
	return fmt.Sprintf("协议号暂未找到Bot发送消息，将在%s后继续对账", e.delay.Round(time.Second))
}

func (e *telegramReconcileRetryError) AccountTaskRetryDelay() time.Duration {
	return e.delay
}

func telegramReconcileRetryDelay(createdAt, now time.Time) time.Duration {
	remaining := telegramReconcileTakeoverWait - now.Sub(createdAt)
	if remaining <= 0 {
		return 0
	}
	// Cross the safety boundary decisively instead of depending on sub-second
	// scheduling precision and consuming another account-task attempt.
	return remaining + time.Second
}
