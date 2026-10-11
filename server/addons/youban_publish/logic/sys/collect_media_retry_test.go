package sys

import (
	"errors"
	"testing"
)

func TestCollectMediaRetryErrorRejectsPermanentWrongFileID(t *testing.T) {
	err := errors.New("Bad Request: wrong file_id or the file is temporarily unavailable")
	if retryErr := collectMediaRetryErrorFrom(err); retryErr != nil {
		t.Fatalf("permanent wrong file_id must not retry: %+v", retryErr)
	}
}

func TestCollectMediaRetryErrorKeepsTemporaryNetworkFailures(t *testing.T) {
	err := errors.New("temporary connection reset")
	if retryErr := collectMediaRetryErrorFrom(err); retryErr == nil {
		t.Fatal("temporary network failure must remain retryable")
	}
}

func TestCollectMediaRetryErrorKeepsDockerDNSFailures(t *testing.T) {
	err := errors.New("dial tcp: lookup telegram-bot-api on 127.0.0.11:53: no such host")
	if retryErr := collectMediaRetryErrorFrom(err); retryErr == nil {
		t.Fatal("Docker DNS failure must remain retryable")
	}
}
