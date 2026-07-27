package sys

import (
	"errors"
	"fmt"
	"testing"

	tgbot "github.com/go-telegram/bot"
)

func TestIsTelegramBotConfigMissingError(t *testing.T) {
	if !isTelegramBotConfigMissingError(errors.New("读取历史配置失败: Bot配置不存在")) {
		t.Fatal("expected missing bot configuration error to be recognized")
	}
	if isTelegramBotConfigMissingError(errors.New("Telegram请求失败")) {
		t.Fatal("unexpected error recognized as missing bot configuration")
	}
}

func TestIsTelegramMessagePermanentlyUndeletableError(t *testing.T) {
	if !isTelegramMessagePermanentlyUndeletableError(errors.New("Bad Request: message can't be deleted")) {
		t.Fatal("expected Telegram deletion deadline error to be recognized")
	}
	if isTelegramMessagePermanentlyUndeletableError(errors.New("Too Many Requests")) {
		t.Fatal("temporary Telegram error must remain retryable")
	}
}

func TestIsTelegramMessageAlreadyDeletedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "telegram bad request message missing",
			err:  fmt.Errorf("%w, Bad Request: message to delete not found", tgbot.ErrorBadRequest),
			want: true,
		},
		{
			name: "telegram not found message missing",
			err:  fmt.Errorf("%w, message not found", tgbot.ErrorNotFound),
			want: true,
		},
		{
			name: "legacy textual response",
			err:  errors.New("Bad Request: message to delete not found"),
			want: true,
		},
		{
			name: "chat missing",
			err:  fmt.Errorf("%w, chat not found", tgbot.ErrorBadRequest),
			want: false,
		},
		{
			name: "temporary failure",
			err:  errors.New("Too Many Requests"),
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isTelegramMessageAlreadyDeletedError(test.err); got != test.want {
				t.Fatalf("unexpected result: got=%v want=%v err=%v", got, test.want, test.err)
			}
		})
	}
}
