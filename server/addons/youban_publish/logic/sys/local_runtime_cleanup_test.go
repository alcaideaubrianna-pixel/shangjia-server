package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

// TestLocalRuntimeCleanupLoad 调用真实业务入队入口，由正在运行的 Docker Worker 删除本地夹具的消息。
// 只允许固定本地 PG 和专用 Redis；不启动测试替身 Worker，也不修改任务为伪成功。
func TestLocalRuntimeCleanupLoad(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_RUNTIME_CLEANUP") != "1" {
		t.Skip("requires running local Docker workers using disposable Redis 16382")
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	before := gdb.GetConfig("default")
	oldConfig := g.Cfg().GetAdapter()
	t.Cleanup(func() { gdb.SetConfigGroup("default", before); g.Cfg().SetAdapter(oldConfig) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "5432", User: "postgres", Pass: "postgres", Name: "youban_hotgo"}})
	adapter, err := gcfg.NewAdapterContent(`{"redis":{"default":{"address":"127.0.0.1:16382","db":15}}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	rows, err := g.DB().GetAll(ctx, `SELECT DISTINCT j.id FROM hg_youban_publish_tg_job j
 JOIN hg_content_profile p ON p.id=j.profile_id
 JOIN hg_youban_publish_tg_message m ON m.job_id=j.id AND m.status='sent'
 WHERE j.account_id=999 AND j.tenant_id=999 AND p.source_type='local_regression'
 AND j.status='sent' AND j.operation_no LIKE '%:local:%' ORDER BY j.id LIMIT 200`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 200 {
		t.Fatalf("need 200 successfully published fixture jobs, got %d", len(rows))
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row["id"].Int64())
	}
	initial, err := g.DB().Model(publishTgMessageTable).Ctx(ctx).WhereIn("job_id", ids).Where("status", "sent").Count()
	if err != nil {
		t.Fatal(err)
	}
	publish := NewSysPublish()
	client, err := publish.telegramQueueClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	started := time.Now()
	for _, id := range ids {
		if err := publish.enqueueTelegramCleanupJob(ctx, id, 0); err != nil {
			t.Fatal(err)
		}
	}
	for {
		remaining, err := g.DB().Model(publishTgMessageTable).Ctx(ctx).WhereIn("job_id", ids).WhereNot("status", "deleted").Count()
		if err != nil {
			t.Fatal(err)
		}
		if remaining == 0 {
			t.Logf("real cleanup completed jobs=%d messages=%d elapsed=%s", len(ids), initial, time.Since(started))
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("cleanup incomplete: remaining=%d err=%v", remaining, ctx.Err())
		case <-time.After(time.Second):
		}
	}
}
