package sys

import (
	"strings"
	"testing"
)

func TestProfileSearchModelDoesNotReferenceRemovedProjectionAlias(t *testing.T) {
	for _, field := range profileSearchKeywordFields() {
		if strings.HasPrefix(field, "t.") {
			t.Fatalf("profile search references missing t alias: %s", field)
		}
	}
}

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

func TestNormalizeProfileSearchKeyword(t *testing.T) {
	for input, want := range map[string]string{
		"编号：m48574":   "m48574",
		"资料编号 M48574": "M48574",
		" 天空001 ":     "天空001",
	} {
		if got := normalizeProfileSearchKeyword(input); got != want {
			t.Fatalf("normalize %q = %q, want %q", input, got, want)
		}
	}
}

func TestParseProfilePublishMark(t *testing.T) {
	for input, want := range map[string][2]string{
		"001":   {"001", ""},
		"天空001": {"001", "天空"},
		"xxy6400": {"6400", "xxy"},
	} {
		sequence, prefix, ok := parseProfilePublishMark(input)
		if !ok || sequence != want[0] || prefix != want[1] {
			t.Fatalf("parse %q = (%q,%q,%v), want (%q,%q,true)", input, sequence, prefix, ok, want[0], want[1])
		}
	}
	if _, _, ok := parseProfilePublishMark("M48574"); ok {
		t.Fatal("profile number must not be parsed as publish mark")
	}
}
