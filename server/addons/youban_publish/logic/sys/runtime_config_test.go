package sys

import (
	"context"
	"reflect"
	"testing"

	"hotgo/utility/runrole"
)

func TestCollectWorkersAreNotWebRuntime(t *testing.T) {
	defer runrole.Set()
	for _, role := range []string{runrole.CollectorWorker, runrole.CollectProcessWorker} {
		runrole.Set(role)
		s := &sSysPublish{}
		if !s.RuntimeRoleEnabled(context.Background(), role) || s.RuntimeRoleEnabled(context.Background(), runrole.Web) {
			t.Fatalf("collect role misclassified: %s", role)
		}
	}
}

func TestCollectProcessOwnershipCutover(t *testing.T) {
	for _, isolated := range []string{"false", "true"} {
		t.Run(isolated, func(t *testing.T) {
			t.Setenv("YOUBAN_COLLECT_PROCESS_ISOLATED", isolated)
			if !collectProcessRuntimeEnabled(publishRuntimeConfig{CollectProcessWorker: true}) {
				t.Fatal("dedicated worker must always consume during handover")
			}
			if collectProcessRuntimeEnabled(publishRuntimeConfig{CollectorWorker: true}) != (isolated == "false") {
				t.Fatal("legacy collector ownership did not follow cutover")
			}
			if collectProcessRuntimeEnabled(publishRuntimeConfig{PushWorker: true}) {
				t.Fatal("publish worker must not consume collection business tasks")
			}
		})
	}
}

func TestParsePublishRuntimeRoles(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   publishRuntimeConfig
	}{
		{
			name:   "default all",
			values: nil,
			want: publishRuntimeConfig{
				Roles: []string{publishRuntimeRoleAll}, Account: true, Scheduler: true,
				PushWorker: true, MediaWorker: true, BackgroundWorker: true, CollectorWorker: true, BackgroundQueue: true,
			},
		},
		{
			name:   "collector only",
			values: []string{"collector-worker"},
			want: publishRuntimeConfig{
				Roles: []string{"collector-worker"}, CollectorWorker: true,
			},
		},
		{
			name:   "isolated collect process worker",
			values: []string{"collect-process-worker"},
			want:   publishRuntimeConfig{Roles: []string{"collect-process-worker"}, CollectProcessWorker: true},
		},
		{
			name:   "web only",
			values: []string{"web"},
			want:   publishRuntimeConfig{Roles: []string{publishRuntimeRoleWeb}},
		},
		{
			name:   "combined roles",
			values: []string{"account", "push-worker,background-worker"},
			want: publishRuntimeConfig{
				Roles:   []string{publishRuntimeRoleAccount, publishRuntimeRoleBackgroundWorker, publishRuntimeRolePushWorker},
				Account: true, PushWorker: true, BackgroundWorker: true, BackgroundQueue: true,
			},
		},
		{
			name:   "worker preset",
			values: []string{"worker"},
			want: publishRuntimeConfig{
				Roles: []string{publishRuntimeRoleWorker}, PushWorker: true, MediaWorker: true, BackgroundWorker: true, BackgroundQueue: true,
			},
		},
		{
			name:   "all overrides other roles",
			values: []string{"web", "all"},
			want: publishRuntimeConfig{
				Roles: []string{publishRuntimeRoleAll}, Account: true, Scheduler: true,
				PushWorker: true, MediaWorker: true, BackgroundWorker: true, CollectorWorker: true, BackgroundQueue: true,
			},
		},
		{
			name:   "invalid roles do not enable all",
			values: []string{"push-wroker"},
			want:   publishRuntimeConfig{Roles: []string{}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parsePublishRuntimeRoles(test.values)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parsePublishRuntimeRoles() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestTelegramDeliveryRuntimeRunsWithPushWorkerNotScheduler(t *testing.T) {
	if telegramDeliveryRuntimeEnabled(publishRuntimeConfig{Scheduler: true}) {
		t.Fatal("scheduler-only runtime must not own Telegram delivery recovery or Outbox dispatch")
	}
	if !telegramDeliveryRuntimeEnabled(publishRuntimeConfig{PushWorker: true}) {
		t.Fatal("push worker must own Telegram delivery recovery and Outbox dispatch")
	}
}
