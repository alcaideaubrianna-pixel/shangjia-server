package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
)

func TestMarkProfileOnlineForEditRestoresPublishState(t *testing.T) {
	in := &sysin.ProfileSaveInp{Status: 2, Visibility: consts.ContentVisibilityPrivate}
	markProfileOnlineForEdit(in)
	if in.Status != 1 || in.Visibility != consts.ContentVisibilityPublic {
		t.Fatalf("edit republish state = status:%d visibility:%q", in.Status, in.Visibility)
	}
}

func TestProfileEditClaimBlocksOnlyActiveOrDifferentEdit(t *testing.T) {
	tests := []struct {
		name, status, current, operation string
		want                             bool
	}{
		{name: "idle", status: "", want: false},
		{name: "failed can retry", status: "failed", want: false},
		{name: "pending", status: "pending", want: true},
		{name: "publishing", status: "publishing", want: true},
		{name: "same edit retry", status: "editing", current: "edit:1", operation: "edit:1", want: false},
		{name: "different edit", status: "editing", current: "edit:1", operation: "edit:2", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := profileEditClaimBlocks(test.status, test.current, test.operation); got != test.want {
				t.Fatalf("profileEditClaimBlocks() = %t, want %t", got, test.want)
			}
		})
	}
}
