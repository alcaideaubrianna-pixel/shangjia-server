package sys

import (
	"strings"
	"testing"
)

func TestSafeProfileRegionValueClearsOversizedValue(t *testing.T) {
	if got := safeProfileRegionValue(" 北京 "); got != "北京" {
		t.Fatalf("trimmed region = %q, want %q", got, "北京")
	}
	if got := safeProfileRegionValue("北京"); got != "北京" {
		t.Fatalf("valid region = %q, want %q", got, "北京")
	}
	longValue := strings.Repeat("北", profileRegionMaxRunes+1)
	if got := safeProfileRegionValue(longValue); got != "" {
		t.Fatalf("oversized region should be cleared, got length %d", len([]rune(got)))
	}
}
