package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// Reproduce N13669 using a disposable local database and the real media reader.
func TestTelegramDeletedMediaPostgres(t *testing.T) {
	link := os.Getenv("YOUBAN_DELETED_MEDIA_TEST_PG")
	if link == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	if link != "pgsql:test:@tcp(127.0.0.1:35439)/postgres" {
		t.Fatal("refusing non-test database")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Link: link}})
	ctx := context.Background()
	sql, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(sql), `CREATE TABLE IF NOT EXISTS "hg_youban_publish_media" (`)
	if start < 0 {
		t.Fatal("media DDL missing")
	}
	ddl := string(sql)[start:]
	ddl = ddl[:strings.Index(ddl, ");")+2]
	ddl = strings.Replace(ddl, "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1)
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(ddl); err != nil {
			return err
		}
		for _, q := range []string{
			`INSERT INTO hg_youban_publish_media(profile_id,purpose,media_type,deleted_at) VALUES(1,'display','image',now()),(1,'verify','video',now()),(2,'display','image',now()),(3,'verify','video',now())`,
			`INSERT INTO hg_youban_publish_media(profile_id,purpose,media_type,file_url) VALUES(2,'display','image','https://example.com/replacement.jpg')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		s := &sSysPublish{}
		for _, tc := range []struct {
			name    string
			profile int64
			purpose string
			wantErr bool
			count   int
		}{
			{"deleted display blocks text fallback", 1, "display", true, 0},
			{"deleted verification blocks false completion", 1, "verify", true, 0},
			{"replacement remains sendable", 2, "display", false, 1},
			{"never had verification remains optional", 2, "verify", false, 0},
			{"another profile does not poison missing purpose", 3, "display", false, 0},
			{"genuine text-only remains supported", 4, "display", false, 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				media, err := s.telegramJobMedia(ctx, telegramJobRecord{ProfileId: tc.profile}, tc.purpose)
				if (err != nil) != tc.wantErr {
					t.Fatalf("media=%d err=%v", len(media), err)
				}
				if tc.wantErr && !strings.Contains(err.Error(), "媒体已删除") {
					t.Fatalf("unexpected error: %v", err)
				}
				if !tc.wantErr && len(media) != tc.count {
					t.Fatalf("got %d want %d", len(media), tc.count)
				}
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
