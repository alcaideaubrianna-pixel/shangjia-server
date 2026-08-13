package sys

import (
	"strings"
	"testing"
)

func TestSegmentedLikeConditionSearchesEveryTermAcrossFields(t *testing.T) {
	condition, args := segmentedLikeCondition([]string{"p.profile_no", "p.title", "p.summary", "p.plain_text"}, []string{"af001", "深圳"})
	if strings.Count(condition, " AND ") != 1 {
		t.Fatalf("search terms must use AND: %s", condition)
	}
	if strings.Count(condition, " OR ") != 6 {
		t.Fatalf("fields must use OR for each term: %s", condition)
	}
	want := []interface{}{"%af001%", "%af001%", "%af001%", "%af001%", "%深圳%", "%深圳%", "%深圳%", "%深圳%"}
	if len(args) != len(want) {
		t.Fatalf("unexpected argument count: %d", len(args))
	}
	for index := range want {
		if args[index] != want[index] {
			t.Fatalf("argument %d = %v, want %v", index, args[index], want[index])
		}
	}
}

func TestSplitProfileSearchTermsRemovesDuplicates(t *testing.T) {
	terms := splitProfileSearchTerms(" af001  深圳 af001 ")
	if len(terms) != 2 || terms[0] != "af001" || terms[1] != "深圳" {
		t.Fatalf("unexpected terms: %#v", terms)
	}
}
