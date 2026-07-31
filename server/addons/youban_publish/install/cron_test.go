package install

import "testing"

func TestCycleSchedulerCronPattern(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{mode: "develop", want: cycleSchedulerDevelopCronPattern},
		{mode: "testing", want: cycleSchedulerDevelopCronPattern},
		{mode: "", want: cycleSchedulerDevelopCronPattern},
		{mode: "product", want: cycleSchedulerProductCronPattern},
		{mode: "staging", want: cycleSchedulerProductCronPattern},
	}
	for _, test := range tests {
		if got := cycleSchedulerCronPattern(test.mode); got != test.want {
			t.Fatalf("cycleSchedulerCronPattern(%q) = %q, want %q", test.mode, got, test.want)
		}
	}
}
