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
