package sys

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/frame/g"
)

func TestDecideProfileMediaReadiness(t *testing.T) {
	tests := []struct {
		name     string
		expected int
		actual   int
		pending  int
		failed   int
		ready    bool
	}{
		{name: "intentional text only", expected: 0, actual: 0, ready: true},
		{name: "operation not persisted yet", expected: 3, actual: 0, ready: false},
		{name: "media task pending", expected: 3, actual: 3, pending: 3, ready: false},
		{name: "media task processing", expected: 3, actual: 3, pending: 1, ready: false},
		{name: "failed media", expected: 3, actual: 3, failed: 1, ready: false},
		{name: "partial media rows", expected: 3, actual: 2, ready: false},
		{name: "all media ready", expected: 3, actual: 3, ready: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, _ := decideProfileMediaReadiness(tt.expected, tt.actual, tt.pending, tt.failed)
			if ready != tt.ready {
				t.Fatalf("ready=%t, want %t", ready, tt.ready)
			}
		})
	}
}

func TestLocalProfileMediaOperationPendingUsesRealProfileSchema(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_PUBLISH_INTEGRATION=1 to run local PostgreSQL integration test")
	}
	ctx := context.Background()
	profileID := time.Now().UnixNano()
	tenantID := profileID + 1
	accountID := profileID + 2
	_, err := g.DB().Ctx(ctx).Exec(ctx, `CREATE TEMP TABLE hg_content_profile (id bigint primary key, profile_no varchar(64), deleted_at timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.DB().Ctx(ctx).Exec(ctx, `CREATE TEMP TABLE hg_youban_publish_profile_media_operation (id bigserial primary key, tenant_id bigint, account_id bigint, profile_no varchar(64), status varchar(16), deleted_at timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.DB().Ctx(ctx).Exec(ctx, `INSERT INTO hg_content_profile(id,profile_no) VALUES($1,$2)`, profileID, "E2E123")
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.DB().Ctx(ctx).Exec(ctx, `INSERT INTO hg_youban_publish_profile_media_operation(tenant_id,account_id,profile_no,status) VALUES($1,$2,$3,$4)`, tenantID, accountID, "E2E123", "pending")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := (&sSysPublish{}).profileMediaOperationPending(ctx, tenantID, accountID, profileID)
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Fatal("pending media operation was not detected")
	}
}
