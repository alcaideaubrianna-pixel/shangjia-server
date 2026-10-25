package sys

import (
	"testing"

	"github.com/gogf/gf/v2/frame/g"
)

func TestCloneConfigValuesDoesNotMutateCachedSecrets(t *testing.T) {
	cached := g.Map{
		"uploadDrive":        "oss",
		"uploadCosSecretKey": "cos-secret",
		"uploadOssSecretKey": "oss-secret",
		"uploadR2SecretKey":  "r2-secret",
	}

	response := cloneConfigValues(cached)
	response["uploadOssSecretKey"] = "masked"

	if got := cached["uploadOssSecretKey"]; got != "oss-secret" {
		t.Fatalf("cached secret was mutated: %v", got)
	}
	for _, key := range []string{"uploadDrive", "uploadCosSecretKey", "uploadOssSecretKey", "uploadR2SecretKey"} {
		if _, ok := response[key]; !ok {
			t.Fatalf("cloned config lost %s", key)
		}
	}
}

func TestCloneConfigValuesNil(t *testing.T) {
	if got := cloneConfigValues(nil); got != nil {
		t.Fatalf("expected nil clone, got %#v", got)
	}
}
