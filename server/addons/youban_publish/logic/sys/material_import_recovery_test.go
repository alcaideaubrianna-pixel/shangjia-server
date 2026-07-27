package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestMaterialImportRecoveryCandidate(t *testing.T) {
	tests := []struct {
		name   string
		status string
		stage  string
		want   bool
	}{
		{name: "running pulling", status: sysin.MaterialImportStatusRunning, stage: sysin.MaterialImportStagePulling, want: true},
		{name: "running media", status: sysin.MaterialImportStatusRunning, stage: sysin.MaterialImportStageMedia, want: true},
		{name: "waiting media", status: sysin.MaterialImportStatusWaiting, stage: sysin.MaterialImportStageMedia, want: true},
		{name: "success", status: sysin.MaterialImportStatusSuccess, stage: sysin.MaterialImportStageFinished},
		{name: "canceled", status: sysin.MaterialImportStatusCanceled, stage: sysin.MaterialImportStageCancelled},
		{name: "created", status: sysin.MaterialImportStatusPending, stage: sysin.MaterialImportStageCreated},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := materialImportRecoveryCandidate(test.status, test.stage); got != test.want {
				t.Fatalf("materialImportRecoveryCandidate(%q, %q) = %v, want %v", test.status, test.stage, got, test.want)
			}
		})
	}
}
