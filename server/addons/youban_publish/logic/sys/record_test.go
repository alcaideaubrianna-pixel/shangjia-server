package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestNormalizePublishRecordPage(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		perPage    int
		totalCount int
		wantPage   int
	}{
		{name: "keeps valid page", page: 3, perPage: 20, totalCount: 49, wantPage: 3},
		{name: "resets page beyond total", page: 256, perPage: 20, totalCount: 49, wantPage: 1},
		{name: "keeps empty result page", page: 256, perPage: 20, totalCount: 0, wantPage: 256},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := &sysin.PublishRecordListInp{}
			in.Page = test.page
			in.PerPage = test.perPage
			normalizePublishRecordPage(in, test.totalCount)
			if in.Page != test.wantPage {
				t.Fatalf("unexpected page: got %d want %d", in.Page, test.wantPage)
			}
		})
	}
}
