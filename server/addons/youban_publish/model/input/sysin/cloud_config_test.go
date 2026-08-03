package sysin

import (
	"context"
	"testing"

	"hotgo/internal/model/input/form"
)

func TestCloudResourceUsageListFilter(t *testing.T) {
	in := &CloudResourceUsageListInp{
		CloudResourceUsageQueryInp: CloudResourceUsageQueryInp{
			StartDate: "2026-08-01",
			EndDate:   "2026-08-03",
		},
		PageReq: form.PageReq{PerPage: 500},
	}
	if err := in.Filter(context.Background()); err != nil {
		t.Fatalf("filter returned error: %v", err)
	}
	if in.Page != 1 || in.PerPage != 100 || !in.Pagination {
		t.Fatalf("unexpected pagination: page=%d pageSize=%d pagination=%v", in.Page, in.PerPage, in.Pagination)
	}
}

func TestCloudResourceUsageListFilterRejectsLongRange(t *testing.T) {
	in := &CloudResourceUsageListInp{CloudResourceUsageQueryInp: CloudResourceUsageQueryInp{
		StartDate: "2025-01-01",
		EndDate:   "2026-08-03",
	}}
	if err := in.Filter(context.Background()); err == nil {
		t.Fatal("expected long date range to be rejected")
	}
}
