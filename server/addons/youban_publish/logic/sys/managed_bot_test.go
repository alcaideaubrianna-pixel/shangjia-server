package sys

import (
	"errors"
	"testing"
)

func TestNormalizeManagedBotUsername(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "valid", value: "@youban_notice_bot", want: "youban_notice_bot"},
		{name: "invalid suffix", value: "youban_notice", wantErr: true},
		{name: "invalid character", value: "youban-notice_bot", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeManagedBotUsername(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeManagedBotUsername() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("normalizeManagedBotUsername() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestManagedBotErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "occupied", err: errors.New("USERNAME_OCCUPIED"), want: "该 Bot 用户名已被占用"},
		{name: "create blocked", err: errors.New("CREATE_BOT_BLOCKED"), want: "当前TG账号已被Telegram限制创建机器人，请切换其他TG账号后重试"},
		{name: "create limit", err: errors.New("BOT_CREATE_LIMIT_EXCEEDED"), want: "当前TG账号创建的机器人数量已达上限，请切换其他TG账号后重试"},
		{name: "manager permission", err: errors.New("MANAGER_PERMISSION_MISSING"), want: "官方Bot尚未开启Bot Management Mode"},
		{name: "account busy", err: errors.New("TG账号连接正在使用，等待账号连接释放"), want: "TG账号正在执行其他操作，请稍后重试"},
		{name: "runtime unavailable", err: errors.New("TG账号常驻客户端尚未就绪，请稍后重试"), want: "TG账号正在执行其他操作，请稍后重试"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := managedBotErrorMessage(tt.err); got != tt.want {
				t.Fatalf("managedBotErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
