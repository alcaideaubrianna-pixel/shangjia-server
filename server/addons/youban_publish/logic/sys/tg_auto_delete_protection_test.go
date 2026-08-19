package sys

import "testing"

func TestIsProtectedTelegramPublishPurpose(t *testing.T) {
	tests := []struct {
		name    string
		purpose string
		want    bool
	}{
		{name: "display media", purpose: "display", want: true},
		{name: "verify media", purpose: "verify", want: true},
		{name: "collect notice", purpose: "", want: false},
		{name: "unknown purpose", purpose: "notice", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProtectedTelegramPublishPurpose(tt.purpose); got != tt.want {
				t.Fatalf("purpose %q protected=%v, want %v", tt.purpose, got, tt.want)
			}
		})
	}
}
