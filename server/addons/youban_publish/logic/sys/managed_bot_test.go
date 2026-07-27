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
		{name: "manager permission", err: errors.New("MANAGER_PERMISSION_MISSING"), want: "官方Bot尚未开启Bot Management Mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := managedBotErrorMessage(tt.err); got != tt.want {
				t.Fatalf("managedBotErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
