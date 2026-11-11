package sys

import (
	"testing"
	"time"
)

func TestProfileSessionDuration(t *testing.T) {
	for _, scene := range []string{"edit_title", "edit_text", "edit_verify", "replace"} {
		if got := profileSessionDuration(scene); got != 6*time.Minute {
			t.Fatalf("scene %q duration=%s, want 6m", scene, got)
		}
	}
	if got := profileSessionDuration("create"); got != 30*time.Minute {
		t.Fatalf("create duration=%s, want 30m", got)
	}
}
