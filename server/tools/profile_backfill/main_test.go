package main

import (
	"testing"

	"hotgo/internal/library/profileextractor"
)

func TestAuditProfileAcceptsMatchingFields(t *testing.T) {
	analysis := profileextractor.Analyze("身高：168 体重：98 罩杯：C")
	if issues := auditProfile(analysis); len(issues) != 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}
}

func TestAuditProfileRejectsMentionedButUnparsedValue(t *testing.T) {
	analysis := profileextractor.Analyze("身高：999 体重：20斤")
	issues := auditProfile(analysis)
	if len(issues) != 2 {
		t.Fatalf("unexpected issues: %v", issues)
	}
}

func TestAuditProfileAllowsMissingSourceValue(t *testing.T) {
	analysis := profileextractor.Analyze("身高：168\n体重：98\n罩杯：\n职业：学生")
	if issues := auditProfile(analysis); len(issues) != 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}
	if !analysis.CupSourceEmpty {
		t.Fatal("expected empty cup source to be reported")
	}
}
