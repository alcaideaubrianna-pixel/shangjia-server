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

func TestPublishRecordFilterPagination(t *testing.T) {
	for _, tc := range []struct{ page, size, wantPage, wantSize int }{
		{0, 0, 1, 10}, {-1, -1, 1, 10}, {2, 10, 2, 10}, {3, 50, 3, 50},
	} {
		in := &PublishRecordListInp{}
		in.Page, in.PerPage = tc.page, tc.size
		if err := in.Filter(context.Background()); err != nil {
			t.Fatal(err)
		}
		if in.Page != tc.wantPage || in.PerPage != tc.wantSize {
			t.Fatalf("page=%d size=%d want page=%d size=%d", in.Page, in.PerPage, tc.wantPage, tc.wantSize)
		}
	}
}
