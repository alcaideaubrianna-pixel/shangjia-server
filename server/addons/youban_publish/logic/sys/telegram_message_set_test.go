package sys

import (
	"errors"
	"testing"
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
