package sys

import "testing"

func TestValidCollectSourceWindowPayloadAllowsBotAccount(t *testing.T) {
	tests := []struct {
		name    string
		payload collectProcessQueuePayload
		want    bool
	}{
		{name: "BOT来源允许零账户", payload: collectProcessQueuePayload{TenantId: 999, AccountId: 0, SourceId: 900087}, want: true},
		{name: "协议号来源允许正账户", payload: collectProcessQueuePayload{TenantId: 999, AccountId: 70, SourceId: 900087}, want: true},
		{name: "缺少租户拒绝", payload: collectProcessQueuePayload{TenantId: 0, AccountId: 0, SourceId: 900087}, want: false},
		{name: "缺少来源拒绝", payload: collectProcessQueuePayload{TenantId: 999, AccountId: 0, SourceId: 0}, want: false},
		{name: "负账户拒绝", payload: collectProcessQueuePayload{TenantId: 999, AccountId: -1, SourceId: 900087}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validCollectSourceWindowPayload(tt.payload); got != tt.want {
				t.Fatalf("validCollectSourceWindowPayload() = %v, want %v", got, tt.want)
			}
		})
	}
}
