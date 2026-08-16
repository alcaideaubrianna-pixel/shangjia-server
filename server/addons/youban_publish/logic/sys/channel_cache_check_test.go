package sys

import (
	"errors"
	"testing"
)

func TestChannelBotMemberErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "member list inaccessible",
			raw:  "bad request: Bad Request: member list is inaccessible",
			want: "暂时无法读取频道中的 Bot 状态。请确认 Bot 已加入频道并已设置为管理员，同时开启“发布消息”和“删除消息”权限，然后重新检测",
		},
		{
			name: "chat not found",
			raw:  "Bad Request: chat not found",
			want: "无法找到该频道。请确认频道仍然存在，并检查频道配置是否正确，然后重新检测",
		},
		{
			name: "unknown error",
			raw:  "request failed",
			want: "暂时无法读取频道中的 Bot 状态，请确认 Bot 已加入频道并拥有发布、删除消息权限，然后重新检测",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := channelBotMemberErrorMessage(test.raw); got != test.want {
				t.Fatalf("message = %q, want %q", got, test.want)
			}
		})
	}
}

func TestChannelCheckTelegramErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "busy", err: errors.New("TG账号连接正在使用，等待账号连接释放"), want: "TG账号正在执行其他操作，请稍后刷新后重试"},
		{name: "runtime unavailable", err: errors.New("TG账号常驻客户端尚未就绪，请稍后重试"), want: "TG账号正在执行其他操作，请稍后刷新后重试"},
		{name: "deadline", err: errors.New("context deadline exceeded"), want: "TG账号正在执行其他操作，请稍后刷新后重试"},
		{name: "other", err: errors.New("解析Bot用户名失败"), want: "解析Bot用户名失败"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := channelCheckTelegramErrorMessage(test.err); got != test.want {
				t.Fatalf("message = %q, want %q", got, test.want)
			}
		})
	}
}
