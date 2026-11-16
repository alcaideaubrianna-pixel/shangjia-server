package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
	"hotgo/internal/model"
)

// The fixture deliberately omits tags, channels, collection and media tables.
// A task status read must succeed independently of those detail dependencies.
func TestProfileTaskStatusLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_TASK_STATUS_TEST_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:55497, user test, database postgres")
	}
	beforeDB, beforeConfig := gdb.GetConfig("default"), g.Cfg().GetAdapter()
	t.Cleanup(func() {
		gdb.SetConfigGroup("default", beforeDB)
		g.Cfg().SetAdapter(beforeConfig)
	})
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55497", User: "test", Name: "postgres"}})
	adapter, err := gcfg.NewAdapterContent(`{"cache":{"adapter":"memory"}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cache.SetAdapter(ctx)
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_content_profile(id bigint PRIMARY KEY,source_note_uuid text,profile_no text,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,publish_task_status text,publish_operation_no text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint PRIMARY KEY,tenant_id bigint,parent_id bigint,account_type text,status int,last_active_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tenant(id bigint PRIMARY KEY,name text,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_tenant VALUES(7,'test',NULL),(8,'other',NULL)`,
			`INSERT INTO hg_youban_publish_account(id,tenant_id,parent_id,account_type,status) VALUES(10,7,0,'admin',1),(11,7,10,'uploader',1),(12,7,99,'uploader',1),(13,8,10,'uploader',1),(14,7,10,'uploader',1)`,
			`UPDATE hg_youban_publish_account SET deleted_at=NOW() WHERE id=14`,
			`INSERT INTO hg_content_profile VALUES(1,'uuid-one','A001',2,NULL),(2,'uuid-two','A002',1,NULL),(3,'uuid-three','A003',2,NULL),(4,'uuid-four','A004',2,NULL),(5,'uuid-five','A005',2,NOW()),(6,'uuid-six','A006',2,NULL),(7,'','A007',2,NULL),(8,'uuid-eight','A008',2,NULL),(9,'uuid-nine','A009',2,NULL)`,
			`INSERT INTO hg_youban_publish_profile_state VALUES(1,7,11,'unpublishing','unpublish:1',NULL),(2,7,10,'pending','cycle_batch:2',NULL),(3,7,12,'unpublishing','unpublish:3',NULL),(4,8,13,'unpublishing','unpublish:4',NULL),(5,7,11,'','',NULL),(6,7,11,'','',NOW()),(7,7,11,NULL,NULL,NULL),(8,7,14,'','',NULL)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		service := &sSysPublish{}
		adminCtx := context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{User: &model.Identity{Id: 10}})
		for _, tc := range []struct {
			name string
			in   sysin.ProfileViewInp
			want sysin.ProfileTaskStatusModel
		}{
			{"id", sysin.ProfileViewInp{Id: 1}, sysin.ProfileTaskStatusModel{Id: 1, Uuid: "uuid-one", Status: 2, TaskStatus: "unpublishing"}},
			{"uuid", sysin.ProfileViewInp{Uuid: " uuid-one "}, sysin.ProfileTaskStatusModel{Id: 1, Uuid: "uuid-one", Status: 2, TaskStatus: "unpublishing"}},
			{"number", sysin.ProfileViewInp{ProfileNo: " a001 "}, sysin.ProfileTaskStatusModel{Id: 1, Uuid: "uuid-one", Status: 2, TaskStatus: "unpublishing"}},
			{"id_precedence", sysin.ProfileViewInp{Id: 1, Uuid: "uuid-two"}, sysin.ProfileTaskStatusModel{Id: 1, Uuid: "uuid-one", Status: 2, TaskStatus: "unpublishing"}},
			{"uuid_precedence", sysin.ProfileViewInp{Uuid: "uuid-one", ProfileNo: "A002"}, sysin.ProfileTaskStatusModel{Id: 1, Uuid: "uuid-one", Status: 2, TaskStatus: "unpublishing"}},
			{"own_profile_cycle", sysin.ProfileViewInp{Id: 2}, sysin.ProfileTaskStatusModel{Id: 2, Uuid: "uuid-two", Status: 1, TaskStatus: "cycling"}},
			{"legacy_uuid_null_state", sysin.ProfileViewInp{Id: 7}, sysin.ProfileTaskStatusModel{Id: 7, Uuid: "id:7", Status: 2, TaskStatus: ""}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := service.AdminProfileTaskStatus(adminCtx, &tc.in)
				if err != nil || got == nil || *got != tc.want {
					t.Fatalf("got %+v err=%v, want %+v", got, err, tc.want)
				}
			})
		}
		// Unmanaged account, cross-tenant child, soft deletions and absent state.
		for _, id := range []int64{3, 4, 5, 6, 8, 9, 999} {
			if got, err := service.AdminProfileTaskStatus(adminCtx, &sysin.ProfileViewInp{Id: id}); err == nil || got != nil {
				t.Fatalf("inaccessible profile %d returned %+v, err=%v", id, got, err)
			}
		}
		uploaderCtx := context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{User: &model.Identity{Id: 11}})
		for _, deniedCtx := range []context.Context{ctx, uploaderCtx} {
			if got, err := service.AdminProfileTaskStatus(deniedCtx, &sysin.ProfileViewInp{Id: 1}); err == nil || got != nil {
				t.Fatalf("unauthorized caller returned %+v, err=%v", got, err)
			}
		}
		for _, tc := range []struct{ task, operation, want string }{
			{"pending", "profile:1", "pending"},
			{"publishing", "PROFILE_CYCLE:1", "cycling"},
			{"failed", "cycle_batch:1", "failed"},
			{"unpublish_failed", "unpublish:1", "unpublish_failed"},
			{"", "unpublish:1", ""},
		} {
			if _, err := tx.Exec(`UPDATE hg_youban_publish_profile_state SET publish_task_status=?,publish_operation_no=? WHERE profile_id=1`, tc.task, tc.operation); err != nil {
				return err
			}
			got, err := service.AdminProfileTaskStatus(adminCtx, &sysin.ProfileViewInp{Id: 1})
			if err != nil || got == nil || got.TaskStatus != tc.want {
				t.Fatalf("status %s/%s: got %+v err=%v, want %s", tc.task, tc.operation, got, err, tc.want)
			}
		}
		if _, err := tx.Exec(`UPDATE hg_youban_publish_account SET parent_id=99 WHERE id=11`); err != nil {
			return err
		}
		if got, err := service.AdminProfileTaskStatus(adminCtx, &sysin.ProfileViewInp{Id: 1}); err == nil || got != nil {
			t.Fatalf("permission revocation returned %+v, err=%v", got, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
