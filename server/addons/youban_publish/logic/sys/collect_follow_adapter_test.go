package sys

import "testing"

func TestNormalizeCollectAccountUsername(t *testing.T) {
	tests := map[string]string{
		"tianmei":   "tianmei",
		"@TianMei":  "tianmei",
		" tianmei ": "tianmei",
		"other":     "other",
	}
	for input, want := range tests {
		if got := normalizeCollectAccountUsername(input); got != want {
			t.Fatalf("normalizeCollectAccountUsername(%q) = %q, want %q", input, got, want)
		}
	}
}
