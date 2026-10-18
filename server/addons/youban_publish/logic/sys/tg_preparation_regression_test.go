package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestTelegramPreparationTimeoutIsRetryable(t *testing.T) {
	err := fmt.Errorf("准备视频失败: %w", telegramPreparationError(context.DeadlineExceeded))
	if isTelegramAmbiguousDeliveryError(err) {
		t.Fatal("local preparation timeout must not enter unknown")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("preparation error must preserve its cause")
	}
	if !isTelegramAmbiguousDeliveryError(context.DeadlineExceeded) {
		t.Fatal("actual request timeout must remain ambiguous")
	}
	if !isTelegramAmbiguousDeliveryError(telegramDeliveryUncertainError(err)) {
		t.Fatal("explicit delivery uncertainty must take precedence")
	}
}

func TestTelegramPreviewBudgetExpiryDoesNotCancelSend(t *testing.T) {
	budget, cancel := context.WithCancel(context.Background())
	cancel()
	file, closer, err := telegramVideoPreviewWithBudget(context.Background(), budget, nil, "thumbnail")
	if err != nil || file != nil || closer != nil {
		t.Fatalf("expired optional budget must fall back without preview: %v", err)
	}
	_, _, err = telegramVideoPreviewWithBudget(budget, budget, nil, "thumbnail")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("parent cancellation must stop preparation: %v", err)
	}
}
