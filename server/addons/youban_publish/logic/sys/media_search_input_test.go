package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestNormalizeProfileImageSearchInputDefaultsToTelegramRecompressionThreshold(t *testing.T) {
	in := &sysin.ProfileImageSearchInp{}
	normalizeProfileImageSearchInput(in)
	if in.Threshold != 12 {
		t.Fatalf("default image search threshold = %d, want 12", in.Threshold)
	}
}
