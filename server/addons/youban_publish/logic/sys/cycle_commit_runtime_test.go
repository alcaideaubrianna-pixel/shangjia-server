package sys

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	"testing"
	"time"
)

// TestLocalCycleCommitRuntime 真实TG确认后故意拒绝本地映射写入；验证事务回滚、其他频道推进和不重发恢复。
func TestLocalCycleCommitRuntime(t *testing.T) {
	ctx := localAccountRuntimeContext(t)
	s := NewSysPublish()
	exec := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := g.DB().Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = g.DB().Exec(context.Background(), "DROP TRIGGER IF EXISTS local_cycle_projection_fault ON "+publishChannelProfileTable)
		_, _ = g.DB().Exec(context.Background(), "DROP FUNCTION IF EXISTS local_cycle_projection_fault()")
		_, _ = g.DB().Exec(context.Background(), "UPDATE "+publishChannelTable+" SET status=0,cycle_publish_enabled=0 WHERE id=999995")
	})
	exec("INSERT INTO " + publishChannelTable + " SELECT (jsonb_populate_record(NULL::" + publishChannelTable + ",to_jsonb(c)||jsonb_build_object('id',999995,'target_chat_id','-100999999995','channel_username','local_commit_fault','is_default_selected',0,'cycle_publish_enabled',0,'cycle_active_run_id',0))).* FROM " + publishChannelTable + " c WHERE c.id=1 ON CONFLICT(id) DO UPDATE SET status=1,cycle_publish_enabled=0")
	// 重跑时先让之前中断测试遗留的已确认任务由真实恢复链路收口，不能用SQL伪造成功。
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE tenant_id=999 AND channel_id=999995 AND status IN ('sending','unknown')", 0)
	exec("CREATE OR REPLACE FUNCTION local_cycle_projection_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.channel_id=999995 THEN RAISE EXCEPTION 'local confirmed projection write failure'; END IF; RETURN NEW; END $$")
	exec("CREATE TRIGGER local_cycle_projection_fault BEFORE INSERT OR UPDATE ON " + publishChannelProfileTable + " FOR EACH ROW EXECUTE FUNCTION local_cycle_projection_fault()")
	profile := localRuntimeProfiles(t, ctx, 410, 1)[0].ID
	op := fmt.Sprintf("profile:commit-fault:%d", time.Now().UnixNano())
	if err := s.submitProfilePublish(ctx, profile, 999, 999, 999, op, []int64{1, 999995}, false); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE operation_no=? AND channel_id=999995 AND status='unknown'", 1, op)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE operation_no=? AND channel_id=1 AND status='sent'", 1, op)
	jobID, err := g.DB().GetValue(ctx, "SELECT id FROM "+publishTgJobTable+" WHERE operation_no=? AND channel_id=999995", op)
	if err != nil {
		t.Fatal(err)
	}
	attempts, err := g.DB().Model(publishTgAttemptTable).Ctx(ctx).Where("job_id", jobID.Int64()).Count()
	if err != nil || attempts < 1 {
		t.Fatal(attempts, err)
	}
	messages, err := g.DB().Model(publishTgMessageTable).Ctx(ctx).Where("job_id", jobID.Int64()).Count()
	if err != nil || messages < 1 {
		t.Fatal(messages, err)
	}
	// 解除故障后只恢复本地状态，不再次调用发送接口。
	exec("DROP TRIGGER local_cycle_projection_fault ON " + publishChannelProfileTable)
	exec("DROP FUNCTION local_cycle_projection_fault()")
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE id=? AND status='sent'", 1, jobID.Int64())
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishChannelProfileTable+" WHERE tenant_id=999 AND channel_id=999995 AND profile_id=? AND last_job_id=?", 1, profile, jobID.Int64())
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgAttemptTable+" WHERE job_id=?", attempts, jobID.Int64())
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgMessageTable+" WHERE job_id=?", messages, jobID.Int64())
	t.Logf("confirmed local-write failure recovered without resending: job=%d attempts=%d messages=%d; other channel completed during fault", jobID.Int64(), attempts, messages)
}
