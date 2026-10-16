package sys

import "testing"

func TestCollectorDeliveryIsEdit(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      bool
	}{
		{"historical album edit", `{"edited_channel_post":{"message_id":11268,"date":1791554577}}`, true},
		{"ordinary channel post", `{"channel_post":{"message_id":11272}}`, false},
		{"null edited field", `{"edited_channel_post":null,"channel_post":{"message_id":1}}`, false},
		{"private edit", `{"edited_message":{"message_id":1}}`, true},
		{"invalid", `{`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := collectorDeliveryIsEdit([]byte(tc.raw)); got != tc.want {
				t.Fatalf("isEdit=%v want=%v", got, tc.want)
			}
		})
	}
}
