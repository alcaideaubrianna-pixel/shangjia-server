package sys

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCollectPublishChatLookupIDs(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"3736052878", []string{"3736052878", "-1003736052878"}},
		{"-1003736052878", []string{"3736052878", "-1003736052878"}},
		{" -1003736052878 ", []string{"3736052878", "-1003736052878"}},
		{"-123456789", []string{"-123456789"}},
		{"@example", []string{"@example"}},
		{"", nil},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := collectPublishChatLookupIDs(tc.input); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("lookup(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// Use only an explicitly enabled, disposable loopback PostgreSQL instance.
// Exercise the actual SQL guard: normalizing two strings alone did not catch
// the previous SQL comparison against the already-prefixed incoming chat ID.
func TestCollectPublishLoopGuardPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: "35432",
		User: os.Getenv("USER"), Name: "postgres",
	}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_youban_publish_channel (
			id bigint, tenant_id bigint, target_chat_id text,
			publish_direction text, status int, deleted_at timestamp
		) ON COMMIT DROP`); err != nil {
			return err
		}
		s := &sSysPublish{}
		for _, stored := range []string{"3736052878", "-1003736052878"} {
			if _, err := tx.Exec(`DELETE FROM hg_youban_publish_channel`); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO hg_youban_publish_channel
				VALUES (1,78,?,'up',1,NULL)`, stored); err != nil {
				return err
			}
			for _, incoming := range []string{"3736052878", "-1003736052878"} {
				blocked, err := s.collectMessageFromAccountPublishChannel(ctx, 78, 0, incoming)
				if err != nil {
					return err
				}
				if !blocked {
					t.Fatalf("stored=%s incoming=%s: self-loop was allowed", stored, incoming)
				}
			}
			for _, tc := range []struct {
				tenant int64
				chat   string
			}{{79, "-1003736052878"}, {78, "-1003856373273"}} {
				blocked, err := s.collectMessageFromAccountPublishChannel(ctx, tc.tenant, 0, tc.chat)
				if err != nil {
					return err
				}
				if blocked {
					t.Fatalf("unrelated tenant/channel was blocked: %+v", tc)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
