package sys

import "testing"

func TestFeatureLabelMatchesConfiguredKeyboardLabel(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "🚀 立即注册", want: true},
		{text: "立即注册", want: true},
		{text: "/register", want: true},
		{text: "register", want: true},
		{text: "生成注册邀请码", want: false},
	}
	for _, test := range tests {
		if got := featureLabelMatches(test.text, "🚀 立即注册", "立即注册", "register"); got != test.want {
			t.Fatalf("featureLabelMatches(%q)=%t, want %t", test.text, got, test.want)
		}
	}
}
