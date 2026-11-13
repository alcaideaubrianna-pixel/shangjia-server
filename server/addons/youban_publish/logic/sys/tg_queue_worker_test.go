package sys

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/gogf/gf/v2/errors/gerror"
)

func TestNormalizeTelegramAutoDeleteConcurrency(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "configured", in: 32, want: 32},
		{name: "zero", in: 0, want: 1},
		{name: "negative", in: -1, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeTelegramAutoDeleteConcurrency(test.in); got != test.want {
				t.Fatalf("normalizeTelegramAutoDeleteConcurrency(%d) = %d, want %d", test.in, got, test.want)
			}
		})
	}
}

func TestIsTelegramMissingJobError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "direct sql no rows", err: sql.ErrNoRows, want: true},
		{name: "wrapped sql no rows", err: fmt.Errorf("读取TG任务失败: %w", sql.ErrNoRows), want: true},
		{name: "gerror wrapped sql no rows", err: gerror.Wrap(sql.ErrNoRows, "读取TG任务失败"), want: true},
		{name: "missing job message", err: gerror.New("TG任务不存在"), want: true},
		{name: "database unavailable", err: fmt.Errorf("读取TG任务失败: connection refused"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isTelegramMissingJobError(test.err); got != test.want {
				t.Fatalf("isTelegramMissingJobError(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}
