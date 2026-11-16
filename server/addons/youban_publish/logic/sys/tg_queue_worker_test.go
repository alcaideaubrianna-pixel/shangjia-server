package sys

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/hibiken/asynq"
)

func TestTelegramDeleteMuxRoutesSharedQueueTasks(t *testing.T) {
	s := &sSysPublish{}
	tests := []struct {
		taskType string
		payload  []byte
		handler  asynq.HandlerFunc
	}{
		{tgTaskTypeCleanup, []byte(`{"jobId":1}`), s.handleTelegramCleanupTask},
		// 回归线上样本：关键词队列可能由 publish cleanup 或 background Worker 取走。
		{tgTaskTypeAutoDelete, []byte(`{"botId":84,"tenantId":90,"accountId":83,"channelId":302,"chatId":"-1002615112422","messageId":79151,"keyword":"收录成功"}`), s.handleTelegramAutoDeleteTask},
		{tgTaskTypeDown, []byte(`{"tenantId":90,"profileIds":[1]}`), s.handleProfileDownTask},
	}
	for _, test := range tests {
		t.Run(test.taskType, func(t *testing.T) {
			mux := s.newTelegramDeleteMux()
			handler, pattern := mux.Handler(asynq.NewTask(test.taskType, test.payload))
			if pattern != test.taskType {
				t.Fatalf("task %q resolved to %q, want exact handler", test.taskType, pattern)
			}
			if reflect.ValueOf(handler).Pointer() != reflect.ValueOf(test.handler).Pointer() {
				t.Fatalf("task %q resolved to the wrong business handler", test.taskType)
			}
		})
	}
}

func TestTelegramDeleteMuxKeepsUnrelatedTasksUnregistered(t *testing.T) {
	mux := (&sSysPublish{}).newTelegramDeleteMux()
	for _, taskType := range []string{tgTaskTypePublish, "unknown:task"} {
		if _, pattern := mux.Handler(asynq.NewTask(taskType, nil)); pattern != "" {
			t.Fatalf("unrelated task %q unexpectedly resolved to %q", taskType, pattern)
		}
	}
}

func TestTelegramAutoDeleteBackfillMuxUsesExactHandler(t *testing.T) {
	s := &sSysPublish{}
	mux := s.newTelegramAutoDeleteBackfillMux()
	for _, test := range []struct {
		taskType string
		handler  asynq.HandlerFunc
	}{
		{tgTaskTypeAutoDelete, s.handleTelegramAutoDeleteTask},
		{tgTaskTypeAutoDeleteBackfill, s.handleTelegramAutoDeleteBackfillTask},
	} {
		handler, pattern := mux.Handler(asynq.NewTask(test.taskType, nil))
		if pattern != test.taskType || reflect.ValueOf(handler).Pointer() != reflect.ValueOf(test.handler).Pointer() {
			t.Fatalf("task %q resolved to %q with wrong handler", test.taskType, pattern)
		}
	}
}

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
