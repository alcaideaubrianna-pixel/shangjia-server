package sys

import (
	"context"
	"errors"
	"testing"
)

func TestScanTelegramErrorType(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, "none"},
		{context.DeadlineExceeded, "timeout"},
		{errors.New("dial tcp: lookup service: no such host"), "dns"},
		{errors.New("Bad Request: wrong file identifier"), "bad_request"},
		{errors.New("unknown failure"), "other"},
	}
	for _, item := range tests {
		if got := scanTelegramErrorType(item.err); got != item.want {
			t.Fatalf("scanTelegramErrorType(%v)=%q want=%q", item.err, got, item.want)
		}
	}
}
