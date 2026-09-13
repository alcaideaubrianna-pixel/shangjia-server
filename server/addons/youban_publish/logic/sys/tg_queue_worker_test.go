package sys

import "testing"

func TestNormalizeTelegramAutoDeleteConcurrency(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "configured", in: 32, want: 32},
		{name: "zero", in: 0, want: 1},
		{name: "negative", in: -1, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeTelegramAutoDeleteConcurrency(test.in); got != test.want {
				t.Fatalf("normalizeTelegramAutoDeleteConcurrency(%d) = %d, want %d", test.in, got, test.want)
			}
		})
	}
}
