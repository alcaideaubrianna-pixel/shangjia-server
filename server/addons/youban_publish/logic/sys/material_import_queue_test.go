package sys

import (
	"testing"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestMaterialImportRecoveryUniqueTTLCoversTaskTimeout(t *testing.T) {
	if materialImportRecoveryUniqueTTL <= 6*time.Hour {
		t.Fatalf("recovery unique TTL = %s, must exceed task timeout", materialImportRecoveryUniqueTTL)
	}
}

func TestMaterialImportQueueNameByStage(t *testing.T) {
	tests := []struct {
		name  string
		stage string
		want  string
	}{
		{name: "created", stage: sysin.MaterialImportStageCreated, want: tgQueueNameMaterialImportPull},
		{name: "pulling", stage: sysin.MaterialImportStagePulling, want: tgQueueNameMaterialImportPull},
		{name: "media", stage: sysin.MaterialImportStageMedia, want: tgQueueNameMaterialImportMedia},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := materialImportQueueNameByStage(test.stage); got != test.want {
				t.Fatalf("queue = %q, want %q", got, test.want)
			}
		})
	}
}
