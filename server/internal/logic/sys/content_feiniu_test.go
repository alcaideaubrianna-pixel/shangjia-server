package sys

import (
	"testing"
)

func TestIsFeiNiuMediaReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cosPath     string
		displayPath string
		want        bool
	}{
		{
			name:        "image with origin url and no cos path",
			displayPath: "https://img.yuebanby.com/telegram/resource/6047470543440645937?kind=origin",
			want:        true,
		},
		{
			name:        "video with origin url and no cos path",
			displayPath: "https://img.yuebanby.com/telegram/resource/6047470543440645937?kind=origin",
			want:        true,
		},
		{
			name:    "missing display path",
			cosPath: "telegram/content/1.jpg",
			want:    false,
		},
		{
			name:        "display path whitespace only",
			cosPath:     "telegram/content/1.jpg",
			displayPath: "   ",
			want:        false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isFeiNiuMediaReady(tt.displayPath); got != tt.want {
				t.Fatalf("isFeiNiuMediaReady() = %v, want %v", got, tt.want)
			}
		})
	}
}
