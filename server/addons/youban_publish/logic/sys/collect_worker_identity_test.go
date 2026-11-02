package sys

import (
	"context"
	"testing"

	"hotgo/utility/runrole"
)

func TestCurrentCollectWorkerIdentity(t *testing.T) {
	runrole.Set(runrole.CollectorWorker)
	t.Cleanup(func() { runrole.Set() })
	t.Setenv("HOSTNAME", "collector-pod-1")
	t.Setenv("APP_REVISION", "abc1234")
	t.Setenv("APP_IMAGE_DIGEST", "sha256:digest")

	got := currentCollectWorkerIdentity(context.Background())
	if got.Role != runrole.CollectorWorker || got.InstanceID != "collector-pod-1" || got.Revision != "abc1234" || got.ImageDigest != "sha256:digest" {
		t.Fatalf("unexpected worker identity: %+v", got)
	}
}
