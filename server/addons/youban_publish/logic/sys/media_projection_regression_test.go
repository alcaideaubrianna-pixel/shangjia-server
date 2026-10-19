package sys

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestBotMediaProjectionLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_MEDIA_PROJECTION_LOCAL_PG") != "1" {
		t.Skip("requires isolated local PostgreSQL")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE TEMP TABLE hg_youban_publish_media (id bigint,profile_id bigint,media_type text,purpose text,sort_index int,attachment_id bigint DEFAULT 0,original_attachment_id bigint DEFAULT 0,original_storage_path text,storage_path text,file_url text,poster_storage_path text,md5 text,perceptual_hash text,deleted_at timestamp,updated_at timestamp)",
			"CREATE TEMP TABLE hg_content_media (id bigserial PRIMARY KEY,profile_id bigint,source_asset_id bigint,media_type text,purpose text,sort_index int,original_storage_path text,display_storage_path text,preview_storage_path text,binary_md5 text,perceptual_hash text,process_status text,encrypt_status text,status int,deleted_at timestamp,created_at timestamp,updated_at timestamp)",
			"CREATE TEMP TABLE hg_content_profile (id bigint,image_count int,video_count int,has_verification_video int,cover_media_id bigint,updated_at timestamp)",
			"INSERT INTO hg_content_profile (id) VALUES (681235)",
			// Two images and a verification video all lack attachment IDs.
			"INSERT INTO hg_youban_publish_media (id,profile_id,media_type,purpose,sort_index,storage_path) VALUES (1,681235,'image','display',1,'a.jpg'),(2,681235,'image','display',2,'b.jpg'),(3,681235,'video','verify',1,'v.mov')",
			// Reproduce the historical overwritten and soft-deleted projection.
			"INSERT INTO hg_content_media (profile_id,source_asset_id,media_type,purpose,sort_index,original_storage_path,deleted_at) VALUES (681235,0,'video','',1,'v.mov',NOW())",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		s := new(sSysPublish)
		sync := func() error {
			return s.syncOwnedMediaToProfile(ctx, tx, gdb.Record{"profile_id": g.NewVar(681235)}, 681235)
		}
		if err := sync(); err != nil {
			return err
		}
		first, err := tx.GetAll("SELECT id FROM hg_content_media WHERE deleted_at IS NULL ORDER BY id")
		if err != nil {
			return err
		}
		if len(first) != 3 {
			t.Fatalf("active media=%d, want 3", len(first))
		}
		if err := sync(); err != nil {
			return err
		}
		second, err := tx.GetAll("SELECT id FROM hg_content_media WHERE deleted_at IS NULL ORDER BY id")
		if err != nil {
			return err
		}
		if len(second) != 3 {
			t.Fatalf("repeated sync media=%d", len(second))
		}
		for i := range first {
			if first[i]["id"].Int64() != second[i]["id"].Int64() {
				t.Fatal("projection IDs changed on retry")
			}
		}
		// Removing one display must retain the other display and verification.
		if _, err = tx.Exec("UPDATE hg_youban_publish_media SET deleted_at=NOW() WHERE id=2"); err != nil {
			return err
		}
		if err := sync(); err != nil {
			return err
		}
		row, err := tx.GetOne("SELECT count(*) n,count(*) FILTER (WHERE purpose='verify') v FROM hg_content_media WHERE deleted_at IS NULL")
		if err != nil {
			return err
		}
		if row["n"].Int() != 2 || row["v"].Int() != 1 {
			t.Fatalf("remaining=%v", row)
		}
		if _, err = tx.Exec("UPDATE hg_youban_publish_media SET deleted_at=NOW() WHERE deleted_at IS NULL"); err != nil {
			return err
		}
		if err := sync(); err != nil {
			return err
		}
		row, err = tx.GetOne("SELECT (SELECT count(*) FROM hg_content_media WHERE deleted_at IS NULL) n,image_count,video_count,cover_media_id FROM hg_content_profile WHERE id=681235")
		if err != nil {
			return err
		}
		if row["n"].Int() != 0 || row["image_count"].Int() != 0 || row["video_count"].Int() != 0 || !row["cover_media_id"].IsNil() {
			t.Fatalf("empty projection=%v", row)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
