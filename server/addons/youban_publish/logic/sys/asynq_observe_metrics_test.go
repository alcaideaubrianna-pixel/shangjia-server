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

func TestCollectQueueAmplificationRatio(t *testing.T) {
	tests := []struct {
		name       string
		tasks      int
		partitions int
		want       float64
	}{
		{name: "balanced", tasks: 125, partitions: 125, want: 1},
		{name: "amplified", tasks: 6500, partitions: 125, want: 52},
		{name: "empty", tasks: 0, partitions: 0, want: 0},
		{name: "orphan tasks", tasks: 4, partitions: 0, want: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectQueueAmplificationRatio(test.tasks, test.partitions); got != test.want {
				t.Fatalf("collectQueueAmplificationRatio(%d, %d) = %v, want %v", test.tasks, test.partitions, got, test.want)
			}
		})
	}
}
