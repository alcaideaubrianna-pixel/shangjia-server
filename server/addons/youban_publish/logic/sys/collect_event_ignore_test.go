package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectEventIgnoreType(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		message string
		want    string
	}{
		{
			name:    "dedupe",
			status:  sysin.CollectEventStatusIgnored,
			message: "图文重复",
			want:    sysin.CollectEventIgnoreTypeDedupe,
		},
		{
			name:    "match",
			status:  sysin.CollectEventStatusIgnored,
			message: "消息不是资料组或验证组",
			want:    sysin.CollectEventIgnoreTypeMatch,
		},
		{
			name:    "non ignored event",
			status:  sysin.CollectEventStatusProcessed,
			message: "图文重复",
			want:    "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectEventIgnoreType(test.status, test.message); got != test.want {
				t.Fatalf("collectEventIgnoreType() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCollectEventTerminalStatus(t *testing.T) {
	for _, status := range []string{sysin.CollectEventStatusIgnored, sysin.CollectEventStatusFailed} {
		if !collectEventTerminalStatus(status) {
			t.Fatalf("status %q should be terminal", status)
		}
	}
	if collectEventTerminalStatus(sysin.CollectEventStatusGroupCollect) {
		t.Fatal("group_collecting should remain processable")
	}
}
