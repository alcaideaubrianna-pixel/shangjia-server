package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestHasProfileViewSelectorAcceptsProfileNo(t *testing.T) {
	if !hasProfileViewSelector(&sysin.ProfileViewInp{ProfileNo: " K99559 "}) {
		t.Fatal("profile number must be accepted as a view selector")
	}
	if hasProfileViewSelector(&sysin.ProfileViewInp{}) {
		t.Fatal("empty profile view selector must be rejected")
	}
}
