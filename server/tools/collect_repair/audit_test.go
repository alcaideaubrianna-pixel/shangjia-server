package main

import (
	"context"
	publishsys "hotgo/addons/youban_publish/logic/sys"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestBuildReplayLanesPreservesAccountOrder(t *testing.T) {
	pairs := []publishsys.CollectRepairCandidate{
		{TenantID: 1, AccountID: 10, DisplayID: 101},
		{TenantID: 1, AccountID: 20, DisplayID: 201},
		{TenantID: 1, AccountID: 10, DisplayID: 102},
		{TenantID: 2, AccountID: 10, DisplayID: 301},
	}

	lanes := buildReplayLanes(pairs)
	if len(lanes) != 3 {
		t.Fatalf("lane count: got %d want 3", len(lanes))
	}
	if !reflect.DeepEqual(lanes[0].indexes, []int{0, 2}) {
		t.Fatalf("first lane indexes: got %v want [0 2]", lanes[0].indexes)
	}
	if lanes[0].key != "1:10" || lanes[1].key != "1:20" || lanes[2].key != "2:10" {
		t.Fatalf("unexpected lane order: %+v", lanes)
	}
}

func TestAuditMediaRejectsPending(t *testing.T) {
	got := auditMedia(context.Background(), publishsys.CollectRepairMedia{ID: 1, Status: "pending"})
	if got.Readable || got.Reason != "media_not_cached" {
		t.Fatalf("pending media accepted: %+v", got)
	}
}

func TestAuditMediaRejectsUnavailableAndEmpty(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusOK} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		got := auditMedia(context.Background(), publishsys.CollectRepairMedia{ID: 1, Status: "ready", URL: server.URL})
		server.Close()
		if got.Readable {
			t.Fatalf("HTTP %d empty media accepted", code)
		}
	}
}

// A stalled download must stop with its parent deadline and remain unreadable.
func TestAuditMediaHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := auditMedia(ctx, publishsys.CollectRepairMedia{ID: 1, Status: "ready", URL: server.URL})
	if got.Readable || got.Reason != "media_download_failed" {
		t.Fatalf("cancelled media accepted: %+v", got)
	}
}
