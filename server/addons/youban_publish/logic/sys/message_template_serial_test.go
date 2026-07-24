package sys

import (
	"regexp"
	"testing"
)

func TestNewInlineTemplateSerial(t *testing.T) {
	pattern := regexp.MustCompile(`^XX[A-Z0-9]{6}$`)
	seen := map[string]struct{}{}
	for i := 0; i < 200; i++ {
		serial, err := newInlineTemplateSerial()
		if err != nil {
			t.Fatal(err)
		}
		if !pattern.MatchString(serial) {
			t.Fatalf("invalid serial: %s", serial)
		}
		if _, ok := seen[serial]; ok {
			t.Fatalf("duplicate serial: %s", serial)
		}
		seen[serial] = struct{}{}
	}
}

func TestNormalizeInlineTemplateSerial(t *testing.T) {
	if got := normalizeInlineTemplateSerial("  xx0m1700 "); got != "XX0M1700" {
		t.Fatalf("got %q", got)
	}
}
