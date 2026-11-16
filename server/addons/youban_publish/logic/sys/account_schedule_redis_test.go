package sys

import (
	"context"
	"errors"
	"fmt"
	"hotgo/addons/youban_publish/library/accountschedule"
	"testing"
)

func TestAccountScheduleResultRealErrorPrecedesMore(t *testing.T) {
	db := errors.New("database unavailable")
	for _, tt := range []struct {
		name        string
		err, budget error
		want        accountschedule.Outcome
	}{
		{"success", nil, nil, accountschedule.Success},
		{"more", errAccountScheduleMore, nil, accountschedule.More},
		{"joined more db", errors.Join(errAccountScheduleMore, db), nil, accountschedule.Failure},
		{"wrapped joined more db", fmt.Errorf("stage: %w", errors.Join(errAccountScheduleMore, db)), nil, accountschedule.Failure},
		{"db during deadline", errors.Join(errAccountScheduleMore, db), context.DeadlineExceeded, accountschedule.Failure},
		{"budget exhausted", context.DeadlineExceeded, context.DeadlineExceeded, accountschedule.Deferred},
		{"stage database deadline", context.DeadlineExceeded, nil, accountschedule.Failure},
		{"busy", errAccountScheduleBusy, nil, accountschedule.Deferred},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := accountScheduleResult(tt.err, tt.budget); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}
