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
