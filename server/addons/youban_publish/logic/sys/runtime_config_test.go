package sys

import (
	"reflect"
	"testing"
)

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
				PushWorker: true, MediaWorker: true, BackgroundWorker: true,
			},
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
				Account: true, PushWorker: true, BackgroundWorker: true,
			},
		},
		{
			name:   "worker preset",
			values: []string{"worker"},
			want: publishRuntimeConfig{
				Roles: []string{publishRuntimeRoleWorker}, PushWorker: true, MediaWorker: true, BackgroundWorker: true,
			},
		},
		{
			name:   "all overrides other roles",
			values: []string{"web", "all"},
			want: publishRuntimeConfig{
				Roles: []string{publishRuntimeRoleAll}, Account: true, Scheduler: true,
				PushWorker: true, MediaWorker: true, BackgroundWorker: true,
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
