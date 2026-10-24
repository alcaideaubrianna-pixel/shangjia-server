package sys

import (
	"strings"
	"testing"
)

func TestAccountReportSplitsWholeChannelLines(t *testing.T) {
	lines := []string{"<b>日报</b>"}
	for i := 0; i < 100; i++ {
		lines = append(lines, "• ["+accountReportTitle(strings.Repeat("<&来源", 80))+"] 采集 12组")
	}
	text := strings.Join(lines, "\n")
	parts := accountReportMessageParts(text)
	if len(parts) < 2 {
		t.Fatal("large report not split")
	}
	if strings.Join(parts, "\n") != text {
		t.Fatal("report lines lost or changed")
	}
	for _, part := range parts {
		if len([]rune(part)) > 3000 {
			t.Fatalf("message too long: %d", len([]rune(part)))
		}
	}
	if strings.Contains(accountReportTitle("<>&"), "<") {
		t.Fatal("unsafe title")
	}
}
