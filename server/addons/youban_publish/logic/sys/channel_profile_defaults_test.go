package sys

import "testing"

func TestMergeDefaultChannelIds(t *testing.T) {
	tests := []struct {
		name     string
		current  []int64
		defaults []int64
		want     []int64
	}{
		{name: "append defaults", current: []int64{2}, defaults: []int64{1, 3}, want: []int64{2, 1, 3}},
		{name: "deduplicate", current: []int64{1, 2, 1}, defaults: []int64{2, 3, 3}, want: []int64{1, 2, 3}},
		{name: "ignore invalid ids", current: []int64{0, -1, 2}, defaults: []int64{0, 3}, want: []int64{2, 3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := mergeDefaultChannelIds(test.current, test.defaults)
			if !sameInt64Slice(got, test.want) {
				t.Fatalf("mergeDefaultChannelIds() = %v, want %v", got, test.want)
			}
		})
	}
}
