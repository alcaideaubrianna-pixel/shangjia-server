package sys

import "testing"

func TestCollectMediaTypesEquivalent(t *testing.T) {
	tests := []struct {
		source string
		target string
		want   bool
	}{
		{source: "photo", target: "image", want: true},
		{source: "image", target: "photo", want: true},
		{source: "video", target: "video", want: true},
		{source: "document", target: "video", want: false},
	}
	for _, test := range tests {
		if got := collectMediaTypesEquivalent(test.source, test.target); got != test.want {
			t.Fatalf("collectMediaTypesEquivalent(%q, %q) = %t, want %t", test.source, test.target, got, test.want)
		}
	}
}
