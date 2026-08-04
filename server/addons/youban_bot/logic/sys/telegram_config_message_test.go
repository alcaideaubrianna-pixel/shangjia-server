package sys

import "testing"

func TestTelegramHTMLTextLength(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{name: "plain", text: "欢迎使用", want: 4},
		{name: "html", text: "<b>欢迎</b>使用", want: 4},
		{name: "emoji", text: "<i>🚀立即注册</i>", want: 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramHTMLTextLength(test.text); got != test.want {
				t.Fatalf("telegramHTMLTextLength(%q)=%d, want %d", test.text, got, test.want)
			}
		})
	}
}
