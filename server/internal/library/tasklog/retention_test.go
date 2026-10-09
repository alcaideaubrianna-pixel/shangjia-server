package tasklog

import (
	"testing"

	"hotgo/internal/consts"
)

func TestRetentionDaysFromEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "unset", value: "", want: consts.DefaultTaskExecutionLogRetentionDays},
		{name: "configured", value: "14", want: 14},
		{name: "trim spaces", value: " 7 ", want: 7},
		{name: "invalid", value: "many", want: consts.DefaultTaskExecutionLogRetentionDays},
		{name: "zero", value: "0", want: consts.DefaultTaskExecutionLogRetentionDays},
		{name: "negative", value: "-2", want: consts.DefaultTaskExecutionLogRetentionDays},
		{name: "too large", value: "3651", want: consts.DefaultTaskExecutionLogRetentionDays},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(consts.TaskLogRetentionDaysEnv, test.value)
			if got := RetentionDays(); got != test.want {
				t.Fatalf("RetentionDays() = %d, want %d", got, test.want)
			}
		})
	}
}
