package sys

import (
	"context"
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/os/gcache"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/library/accountschedule"
	"testing"
	"time"
)

func TestAccountScheduleDirectorySharedThrottle(t *testing.T) {
	ctx := context.Background()
	shared := gcache.NewAdapterMemory()
	first, second := gcache.New(), gcache.New()
	first.SetAdapter(shared)
	second.SetAdapter(shared)
	defer first.Close(ctx)
	ok, err := reserveAccountScheduleDirectory(ctx, first, 5*time.Minute)
	if err != nil || !ok {
		t.Fatalf("first entry: %v %v", ok, err)
	}
	// A different entrypoint/instance sharing the backend must not admit another batch.
	ok, err = reserveAccountScheduleDirectory(ctx, second, 5*time.Minute)
	if err != nil || ok {
		t.Fatalf("second entry bypassed throttle: %v %v", ok, err)
	}
	if _, err := first.Remove(ctx, publishconsts.AccountScheduleDirectoryNextKey); err != nil {
		t.Fatal(err)
	}
	ok, err = reserveAccountScheduleDirectory(ctx, second, 5*time.Minute)
	if err != nil || !ok {
		t.Fatalf("next interval blocked: %v %v", ok, err)
	}
}

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
		{"more with exhausted budget", errAccountScheduleMore, context.DeadlineExceeded, accountschedule.Stalled},
		{"budget exhausted", context.DeadlineExceeded, context.DeadlineExceeded, accountschedule.Stalled},
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
