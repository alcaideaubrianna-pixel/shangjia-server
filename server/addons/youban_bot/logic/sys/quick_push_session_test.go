package sys

import "testing"

func TestQuickPushNavigationText(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "empty text stays in flow", text: "", want: false},
		{name: "command exits flow", text: "/start", want: true},
		{name: "menu label exits flow", text: "联系客服", want: true},
		{name: "quick push label exits flow", text: "快速推送", want: true},
		{name: "ordinary content stays in flow", text: "朴朴芙蓉B3054", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := quickPushNavigationText(test.text); got != test.want {
				t.Fatalf("quickPushNavigationText(%q) = %v, want %v", test.text, got, test.want)
			}
		})
	}
}
