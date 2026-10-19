package sys

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/errors/gerror"
)

func TestBotErrorMessageHidesWrappedCredentials(t *testing.T) {
	secret := "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZ_fake_token"
	inner := errors.New(`Get "https://example.invalid/files/` + secret + `/photo.jpg": read tcp: connection reset by peer`)
	if got := botErrorMessage(gerror.Wrap(inner, "展示资料保存失败，请重新发送")); got != "展示资料保存失败，请重新发送" {
		t.Fatalf("unexpected message: %s", got)
	}
	if got := botErrorMessage(inner); strings.Contains(got, secret) || strings.Contains(got, "https://") {
		t.Fatal("credential exposed")
	}
}
