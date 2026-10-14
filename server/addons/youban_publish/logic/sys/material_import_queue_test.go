package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

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
