package sys

import "testing"

func TestNormalizeCollectMediaConcurrency(t *testing.T) {
	tests := []struct {
		name       string
		global     int
		wantGlobal int
	}{
		{name: "defaults", global: collectMediaDefaultGlobalConcurrency, wantGlobal: 64},
		{name: "minimum", global: 0, wantGlobal: 1},
		{name: "maximum", global: 1000, wantGlobal: collectMediaMaxGlobalConcurrency},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			globalLimit := normalizeCollectMediaConcurrency(test.global)
			if globalLimit != test.wantGlobal {
				t.Fatalf("normalizeCollectMediaConcurrency(%d)=%d want=%d", test.global, globalLimit, test.wantGlobal)
			}
		})
	}
}
