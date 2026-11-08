package sys

import "testing"

func TestRedisStreamGrayRequiresPushWorker(t *testing.T) {
	config := parsePublishRuntimeRoles([]string{publishRuntimeRoleScheduler})
	if config.PushWorker || config.RedisStreamGray {
		t.Fatalf("scheduler must not enable stream gray: %+v", config)
	}
	config = parsePublishRuntimeRoles([]string{publishRuntimeRolePushWorker})
	if !config.PushWorker || config.RedisStreamGray {
		t.Fatalf("gray must default off: %+v", config)
	}
}
