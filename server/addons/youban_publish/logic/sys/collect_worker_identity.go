package sys

import (
	"context"
	"os"
	"strings"

	"hotgo/utility/runrole"
)

type collectWorkerIdentity struct {
	Role        string
	InstanceID  string
	Revision    string
	ImageDigest string
}

func currentCollectWorkerIdentity(ctx context.Context) collectWorkerIdentity {
	return collectWorkerIdentity{
		Role:        strings.Join(runrole.Roles(ctx), ","),
		InstanceID:  firstNonEmptyEnv("HOSTNAME", "POD_NAME", "SAE_INSTANCE_ID"),
		Revision:    firstNonEmptyEnv("APP_REVISION", "GIT_REVISION"),
		ImageDigest: firstNonEmptyEnv("APP_IMAGE_DIGEST", "SAE_IMAGE_DIGEST", "IMAGE_DIGEST"),
	}
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
