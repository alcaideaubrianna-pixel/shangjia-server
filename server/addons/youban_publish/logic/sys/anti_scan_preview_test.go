package sys

import (
	"strings"
	"testing"
)

func TestAntiScanMattingPublicErrorDoesNotExposeProvider(t *testing.T) {
	errMessage := antiScanMattingPublicError().Error()
	if errMessage != antiScanMattingErrorMessage {
		t.Fatalf("unexpected public error: %s", errMessage)
	}
	for _, sensitiveText := range []string{"FAPIHub", "fapihub", "HTTP", "quota_exceeded"} {
		if strings.Contains(errMessage, sensitiveText) {
			t.Fatalf("public error exposes sensitive text %q: %s", sensitiveText, errMessage)
		}
	}
}
