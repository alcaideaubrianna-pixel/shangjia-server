package sensitive

import (
	"strings"
	"testing"
)

func TestRedactsTokensAndDiagnosticURLs(t *testing.T) {
	for _, separator := range []string{":", "%3A"} {
		token := "123456789" + separator + "ABCDEFGHIJKLMNOPQRSTUVWXYZ_fake_token"
		input := "bot" + token + " download https://example.invalid/files/" + token + "/photos/file.jpg reset " + token
		if got := Diagnostic(input); strings.Contains(got, "fake_token") || strings.Contains(got, "example.invalid") {
			t.Fatal("diagnostic leaked credentials")
		}
		if got := Tokens(input); strings.Contains(got, "fake_token") || !strings.Contains(got, "example.invalid") {
			t.Fatal("token-only redaction failed")
		}
	}
}
