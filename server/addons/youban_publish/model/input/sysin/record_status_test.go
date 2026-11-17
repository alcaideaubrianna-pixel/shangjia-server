package sysin

import (
	"context"
	"testing"

	publishmodel "hotgo/addons/youban_publish/model"
)

func TestPublishRecordFilterValidatesTypedStatus(t *testing.T) {
	for _, tc := range []struct {
		raw  publishmodel.PublishRecordStatus
		want publishmodel.PublishRecordStatus
		ok   bool
	}{
		{"", "", true},
		{"sent", publishmodel.PublishRecordStatusSuccess, true},
		{" success ", publishmodel.PublishRecordStatusSuccess, true},
		{publishmodel.PublishRecordStatusUnknown, publishmodel.PublishRecordStatusUnknown, true},
		{"processing", "", false},
		{"unexpected", "", false},
	} {
		in := &PublishRecordListInp{Status: tc.raw}
		err := in.Filter(context.Background())
		if (err == nil) != tc.ok {
			t.Fatalf("raw=%q err=%v want valid=%v", tc.raw, err, tc.ok)
		}
		if tc.ok && in.Status != tc.want {
			t.Fatalf("raw=%q got=%q want=%q", tc.raw, in.Status, tc.want)
		}
	}
}
