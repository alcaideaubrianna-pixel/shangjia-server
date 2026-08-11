package sys

import (
	"errors"
	"testing"

	"github.com/hibiken/asynq"
)

func TestIsAsynqQueueNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "asynq error", err: asynq.ErrQueueNotFound, want: true},
		{name: "redis not found", err: errors.New(`NOT_FOUND: queue "example" does not exist`), want: true},
		{name: "other error", err: errors.New("connection refused"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isAsynqQueueNotFound(test.err); got != test.want {
				t.Fatalf("isAsynqQueueNotFound() = %v, want %v", got, test.want)
			}
		})
	}
}
