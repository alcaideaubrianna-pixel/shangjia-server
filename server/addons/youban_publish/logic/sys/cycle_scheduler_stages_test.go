package sys

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRunChannelCycleSchedulerStagesContinuesAfterFailure(t *testing.T) {
	called := make([]string, 0, 3)
	stage := func(name string, err error) channelCycleSchedulerStage {
		return channelCycleSchedulerStage{name: name, run: func(context.Context) error {
			called = append(called, name)
			return err
		}}
	}

	err := runChannelCycleSchedulerStages(t.Context(), []channelCycleSchedulerStage{
		stage("batch", errors.New("missing column")),
		stage("reschedule", nil),
		stage("time", nil),
	})
	if !reflect.DeepEqual(called, []string{"batch", "reschedule", "time"}) {
		t.Fatalf("scheduler stages = %v", called)
	}
	if err == nil || !strings.Contains(err.Error(), "batch: missing column") {
		t.Fatalf("scheduler error = %v", err)
	}
}

func TestRunChannelCycleSchedulerStagesReturnsNilOnSuccess(t *testing.T) {
	err := runChannelCycleSchedulerStages(t.Context(), []channelCycleSchedulerStage{
		{name: "time", run: func(context.Context) error { return nil }},
	})
	if err != nil {
		t.Fatalf("scheduler error = %v", err)
	}
}
