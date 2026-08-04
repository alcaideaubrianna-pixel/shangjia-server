package sys

import (
	"context"
	"testing"
)

func TestInviteExpireDays(t *testing.T) {
	if got := (&sSysBot{}).inviteExpireDays(context.Background()); got != 7 {
		t.Fatalf("inviteExpireDays() = %d, want 7", got)
	}
}
