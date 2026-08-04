package sys

import (
	"context"
	"testing"
)

func TestIsReusableRegisterInvite(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{source: registerInviteSourceWeb, want: true},
		{source: registerInviteSourceBot, want: true},
		{source: registerInviteSourceSelf, want: false},
		{source: "unknown", want: false},
	}
	for _, test := range tests {
		if got := isReusableRegisterInvite(test.source); got != test.want {
			t.Fatalf("isReusableRegisterInvite(%q) = %t, want %t", test.source, got, test.want)
		}
	}
}

func TestWebInviteExpireDays(t *testing.T) {
	if got := (&sSysPublish{}).webInviteExpireDays(context.Background()); got != 7 {
		t.Fatalf("webInviteExpireDays() = %d, want 7", got)
	}
}
