package sys

import "testing"

func TestSameChannelIds(t *testing.T) {
	tests := []struct {
		name  string
		left  []int64
		right []int64
		want  bool
	}{
		{name: "same", left: []int64{1, 2}, right: []int64{1, 2}, want: true},
		{name: "order and duplicates", left: []int64{2, 1, 2}, right: []int64{1, 2}, want: true},
		{name: "custom subset", left: []int64{1}, right: []int64{1, 2}, want: false},
		{name: "custom channel", left: []int64{1, 3}, right: []int64{1, 2}, want: false},
		{name: "both empty", left: nil, right: []int64{}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sameChannelIds(test.left, test.right); got != test.want {
				t.Fatalf("sameChannelIds(%v, %v) = %v, want %v", test.left, test.right, got, test.want)
			}
		})
	}
}
