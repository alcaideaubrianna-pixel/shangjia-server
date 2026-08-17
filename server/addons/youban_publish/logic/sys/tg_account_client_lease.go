package sys

import (
	"errors"
	"fmt"
)

// telegramAccountBusyError remains for retry-policy compatibility with tasks
// persisted by older versions. New account operations use AccountRuntime.
type telegramAccountBusyError struct {
	tgAccountId int64
	err         error
}

func (e *telegramAccountBusyError) Error() string {
	if e == nil || e.err == nil {
		return fmt.Sprintf("TG账号连接正在使用，等待账号连接释放 tgAccountId:%d", e.tgAccountId)
	}
	return fmt.Sprintf("TG账号连接正在使用，等待账号连接释放 tgAccountId:%d: %v", e.tgAccountId, e.err)
}

func (e *telegramAccountBusyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func isTelegramAccountBusyError(err error) bool {
	var busyErr *telegramAccountBusyError
	return errors.As(err, &busyErr)
}
