package sys

import (
	"errors"
	"testing"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/hibiken/asynq"
)

func TestProfilePublishPermissionPreparationResult(t *testing.T) {
	for _, test := range []struct {
		name     string
		channels int
		jobs     []int64
		paused   bool
	}{
		{"all paused", 2, nil, true},
		{"one usable channel", 2, []int64{15}, false},
		{"all usable", 2, []int64{15, 16}, false},
		{"no resolved channels", 0, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := profilePublishPreparationError(test.channels, test.jobs)
			if errors.Is(err, errTelegramPublishPermissionPaused) != test.paused {
				t.Fatalf("permission skip classification: %v", err)
			}
		})
	}
	wrapped := gerror.Wrap(errTelegramPublishPermissionPaused, "提交资料上架失败")
	if !errors.Is(wrapped, errTelegramPublishPermissionPaused) {
		t.Fatal("callers must recognize a wrapped permission pause")
	}
	terminal := errors.Join(wrapped, asynq.SkipRetry)
	if !errors.Is(terminal, asynq.SkipRetry) || !errors.Is(terminal, errTelegramPublishPermissionPaused) {
		t.Fatal("background save must preserve the cause without retrying the upload")
	}
}
