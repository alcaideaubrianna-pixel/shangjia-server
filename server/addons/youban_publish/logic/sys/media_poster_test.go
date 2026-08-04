package sys

import (
	"errors"
	"strings"
	"testing"
)

func TestVideoPosterCommandErrorUsesOutputTail(t *testing.T) {
	output := []byte(strings.Repeat("banner", 100) + "actual ffmpeg error")
	detail := videoPosterCommandError(output, errors.New("exit status 1"))
	if !strings.HasSuffix(detail, "actual ffmpeg error") {
		t.Fatalf("detail=%q", detail)
	}
	if len([]rune(detail)) > 503 {
		t.Fatalf("detail too long: %d", len([]rune(detail)))
	}
}

func TestVideoPosterCommandErrorFallsBackToRunError(t *testing.T) {
	detail := videoPosterCommandError(nil, errors.New("signal: killed"))
	if detail != "signal: killed" {
		t.Fatalf("detail=%q", detail)
	}
}
