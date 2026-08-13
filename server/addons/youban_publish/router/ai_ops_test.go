package router

import "testing"

func TestAIOpsTokenMatches(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		provided string
		want     bool
	}{
		{name: "match", expected: "secret-token", provided: "secret-token", want: true},
		{name: "empty", expected: "", provided: "", want: false},
		{name: "different", expected: "secret-token", provided: "other-token", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := aiOpsTokenMatches(test.expected, test.provided); got != test.want {
				t.Fatalf("aiOpsTokenMatches() = %t, want %t", got, test.want)
			}
		})
	}
}
