package api

import (
	"testing"

	"hotgo/addons/youban_open/api/api/open"
)

func TestNormalizeOpenProfilePage(t *testing.T) {
	tests := []struct {
		name    string
		page    int
		current int
		want    int
	}{
		{name: "current compatibility", current: 2, want: 2},
		{name: "page takes precedence", page: 3, current: 2, want: 3},
		{name: "defaults remain unset", want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := &open.ListReq{Current: test.current}
			req.Page = test.page
			normalizeOpenProfilePage(req)
			if req.Page != test.want {
				t.Fatalf("page=%d, want %d", req.Page, test.want)
			}
		})
	}
}

func TestOpenProfileListInputForwardsPaginationAndExclusions(t *testing.T) {
	req := &open.ListReq{ExcludeIds: "12,34"}
	req.Page, req.PerPage = 2, 15
	in := openProfileListInput(req)
	if in.Page != 2 || in.PerPage != 15 {
		t.Fatalf("pagination=(%d,%d), want (2,15)", in.Page, in.PerPage)
	}
	if in.ExcludeProfileIds != "12,34" {
		t.Fatalf("excludeIds=%q, want 12,34", in.ExcludeProfileIds)
	}
}
