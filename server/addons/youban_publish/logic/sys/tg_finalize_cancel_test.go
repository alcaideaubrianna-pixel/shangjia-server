package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestTelegramFinalizeSendCancellationRequiresReconciliation(t *testing.T) {
	for _, cause := range []error{
		context.Canceled,
		context.DeadlineExceeded,
		fmt.Errorf("SDK sendMediaGroup: %w", context.Canceled),
		fmt.Errorf("SDK sendVideo: %w", context.DeadlineExceeded),
		errors.New("SDK sendMediaGroup: context canceled"),
		errors.New("SDK sendVideo: context deadline exceeded"),
	} {
		if !isTelegramAmbiguousDeliveryError(cause) {
			t.Errorf("send cancellation must await reconciliation: %v", cause)
		}
	}
}

func TestTelegramFinalizePreparationCancellationRemainsDefiniteFailure(t *testing.T) {
	for _, cause := range []error{
		telegramPreparationError(context.Canceled),
		telegramPreparationError(context.DeadlineExceeded),
		fmt.Errorf("prepare media: %w", telegramPreparationError(context.Canceled)),
		fmt.Errorf("下载远程媒体失败: %w", context.Canceled),
		errors.New("保存媒体缓存临时文件失败: context canceled"),
	} {
		if isTelegramAmbiguousDeliveryError(cause) {
			t.Errorf("pre-send preparation failure must remain safe to retry: %v", cause)
		}
	}
}
