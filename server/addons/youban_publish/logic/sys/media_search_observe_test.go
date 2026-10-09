package sys

import (
	"context"
	"errors"
	"testing"
)

func TestBotMediaDownloadErrorType(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, "none"},
		{context.DeadlineExceeded, "timeout"},
		{errors.New("dial tcp: lookup service: no such host"), "dns"},
		{&mediaFileCacheHTTPStatusError{statusCode: 451}, "http_451"},
		{errors.New("unknown failure"), "other"},
	}
	for _, item := range tests {
		if got := botMediaDownloadErrorType(item.err); got != item.want {
			t.Fatalf("botMediaDownloadErrorType(%v)=%q want=%q", item.err, got, item.want)
		}
	}
}
