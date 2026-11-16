package ownedlease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/errors/gerror"
)

func TestRedactErrorPreservesSentinelWithoutExposingCredential(t *testing.T) {
	token := `{"owner":"worker","nonce":"secret-credential"}`
	escaped, _ := json.Marshal(token)
	raw := fmt.Errorf("EVAL %s %s: %w", token, escaped, context.DeadlineExceeded)
	safe := RedactError(raw, token)
	if !errors.Is(safe, context.DeadlineExceeded) {
		t.Fatal("lost deadline error identity")
	}
	for _, rendered := range []string{safe.Error(), fmt.Sprintf("%+v", safe), fmt.Sprintf("%#v", safe), fmt.Sprintf("%+v", gerror.Wrap(safe, "租约失败")), gerror.Stack(safe)} {
		if strings.Contains(rendered, "secret-credential") {
			t.Fatalf("credential leaked through upper error formatter: %s", rendered)
		}
	}
	if errors.Unwrap(safe) != nil || RedactError(nil, token) != nil {
		t.Fatal("redacted error exposed underlying command or changed nil")
	}
}
