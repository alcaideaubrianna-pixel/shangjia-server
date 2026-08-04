package sys

import "testing"

func TestDownTelegramOperationNo(t *testing.T) {
	for _, operationNo := range []string{"down:12:345", " DOWN:12:345 "} {
		if !isDownTelegramOperationNo(operationNo) {
			t.Fatalf("expected down operation: %q", operationNo)
		}
	}
	if isDownTelegramOperationNo("publish:12:345") {
		t.Fatal("publish operation must not be treated as down")
	}
}

func TestDownTelegramRateLimitUsesRetryPolicy(t *testing.T) {
	policy := telegramJobErrorRetryPolicy(assertError("too many requests, Too Many Requests: retry after 26: retry_after 26"), 1)
	if policy.Permanent {
		t.Fatal("rate limit must remain retryable")
	}
	if policy.RetryDelay <= 0 {
		t.Fatalf("retry delay = %s", policy.RetryDelay)
	}
}
