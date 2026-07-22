package sys

import "testing"

func TestMaterialImportMergeCrossPageLeadingUnits(t *testing.T) {
	newerPage := []*materialImportMessageUnit{
		{
			GroupedId: "14277572021261444",
			MessageId: 96,
			Messages:  []int{96},
			Media:     []collectMediaItem{{Type: "photo"}},
		},
		{
			MessageId: 97,
			Messages:  []int{97},
			Media:     []collectMediaItem{{Type: "video"}},
		},
	}

	processable, pending := materialImportSplitLeadingUnits(newerPage)
	if len(processable) != 0 {
		t.Fatalf("expected no processable units from newer page, got %d", len(processable))
	}
	if len(pending) != 2 {
		t.Fatalf("expected two pending units from newer page, got %d", len(pending))
	}

	olderPage := []*materialImportMessageUnit{
		{
			GroupedId: "14277572021261444",
			RawText:   "昵称：八方来财324",
			MessageId: 93,
			Messages:  []int{93, 94, 95},
			Media: []collectMediaItem{
				{Type: "photo"},
				{Type: "photo"},
				{Type: "photo"},
			},
		},
	}
	processable, carry := materialImportSplitLeadingUnits(olderPage)
	if len(carry) != 0 {
		t.Fatalf("expected no carry units from older page, got %d", len(carry))
	}

	merged := materialImportMergeAdjacentUnits(append(processable, pending...))
	if len(merged) != 2 {
		t.Fatalf("expected display unit and verify unit, got %d", len(merged))
	}
	if got := len(merged[0].Messages); got != 4 {
		t.Fatalf("expected display messages 93-96, got %d", got)
	}
	if merged[0].Messages[0] != 93 || merged[0].Messages[3] != 96 {
		t.Fatalf("unexpected display message ids: %#v", merged[0].Messages)
	}
	if merged[1].MessageId != 97 || len(merged[1].Media) != 1 || merged[1].Media[0].Type != "video" {
		t.Fatalf("unexpected verify unit: %#v", merged[1])
	}
}
