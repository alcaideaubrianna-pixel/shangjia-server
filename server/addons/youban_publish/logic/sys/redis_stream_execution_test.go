package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
	"hotgo/internal/library/cache"
)

// TestMissingQueueTaskRecoveryLocalPostgres 验证真实Redis中缺失的任务可恢复，仍存在的延迟任务不会被重复投递。
func TestMissingQueueTaskRecoveryLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_STREAM_MEDIA_PG") != "1" {
		t.Skip("requires local PostgreSQL and Redis16382")
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	previousConfig := g.Cfg().GetAdapter()
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	adapter, err := gcfg.NewAdapterContent(`{"redis":{"default":{"address":"127.0.0.1:16382","db":15}}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	previous := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previous) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "5432", User: "postgres", Pass: "postgres", Name: "youban_hotgo"}})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	client := asynq.NewClient(telegramQueueRedisOpt(ctx))
	defer client.Close()
	inspector := asynq.NewInspector(telegramQueueRedisOpt(ctx))
	defer inspector.Close()
	rollback := errors.New("rollback missing queue fixture")
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var ids []int64
		for index := 0; index < 3; index++ {
			id, err := tx.GetValue("INSERT INTO hg_youban_publish_tg_job(tenant_id,account_id,operation_no,status,dispatch_status,queue_name,asynq_task_id,updated_at) VALUES(999,999,?,'pending','queued',?,'missing-local-task',now()-interval '1 minute') RETURNING id", fmt.Sprintf("profile:local:queue-recovery:%d", index), channelDispatchQueueSend)
			if err != nil {
				return err
			}
			ids = append(ids, id.Int64())
			if index == 2 {
				if _, err := tx.Exec("UPDATE hg_youban_publish_tg_job SET dispatch_status='idle',asynq_task_id='',target_chat_id='-100999999956' WHERE id=?", id.Int64()); err != nil {
					return err
				}
			}
			if _, err := tx.Model(publishTgOutboxTable).Data(g.Map{"job_id": id.Int64(), "event_type": telegramOutboxEventPublish, "dedupe_key": fmt.Sprintf("local-queue-recovery:%d", id.Int64()), "status": telegramOutboxDelivered, "available_at": gtime.Now()}).Insert(); err != nil {
				return err
			}
		}
		taskID := fmt.Sprintf("local-queue-recovery:%d", ids[1])
		if _, err := client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypePublish, []byte(fmt.Sprintf(`{"jobId":%d}`, ids[1]))), asynq.Queue(channelDispatchQueueSend), asynq.TaskID(taskID), asynq.ProcessIn(3*time.Minute)); err != nil {
			return err
		}
		defer inspector.DeleteTask(channelDispatchQueueSend, taskID)
		if _, err := tx.Exec("UPDATE hg_youban_publish_tg_job SET asynq_task_id=? WHERE id=?", taskID, ids[1]); err != nil {
			return err
		}
		publish := NewSysPublish()
		cursor, err := publish.recoverMissingTelegramStreamTasks(ctx, ids[0]-1, 1)
		if err != nil {
			return err
		}
		if cursor != ids[0] {
			return fmt.Errorf("recovery cursor=%d want=%d", cursor, ids[0])
		}
		for cursor != 0 {
			cursor, err = publish.recoverMissingTelegramStreamTasks(ctx, cursor, 1)
			if err != nil {
				return err
			}
		}
		for index, id := range ids {
			row, err := tx.Model(publishTgJobTable).Where("id", id).One()
			if err != nil {
				return err
			}
			outbox, err := tx.Model(publishTgOutboxTable).Where("job_id", id).Fields("status").Value()
			if err != nil {
				return err
			}
			if index != 1 {
				if row["dispatch_status"].String() != tgDispatchStatusIdle || outbox.String() != telegramOutboxPending {
					return fmt.Errorf("missing task not restored: job=%d status=%s outbox=%s", id, row["dispatch_status"].String(), outbox.String())
				}
			} else if row["dispatch_status"].String() != tgDispatchStatusQueued || outbox.String() != telegramOutboxDelivered {
				return fmt.Errorf("existing task was duplicated: job=%d status=%s outbox=%s", id, row["dispatch_status"].String(), outbox.String())
			}
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}

// TestUnknownSendOnlyBlocksOwnChannelLocalPostgres 验证未知外部结果必须挡住本频道后续发送，但不会挡住其他频道。
func TestUnknownSendOnlyBlocksOwnChannelLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_STREAM_MEDIA_PG") != "1" {
		t.Skip("requires local PostgreSQL")
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	previous := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previous) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "5432", User: "postgres", Pass: "postgres", Name: "youban_hotgo"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rollback := errors.New("rollback unknown channel fixture")
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// 前序任务故意使用不同账户，验证同一 tenant + target_chat_id
		// 仍然共享频道顺序，不会被 account_id 条件错误过滤。
		if _, err := tx.Exec("INSERT INTO hg_youban_publish_tg_job(tenant_id,account_id,channel_id,target_chat_id,operation_no,status,dispatch_status,priority,send_phase,created_at) VALUES(999,998,0,'-100999999981','collect:local:unknown','unknown','processing',10,'display_sending',now())"); err != nil {
			return err
		}
		id, err := tx.GetValue("INSERT INTO hg_youban_publish_tg_job(tenant_id,account_id,channel_id,target_chat_id,operation_no,status,dispatch_status,priority,created_at) VALUES(999,999,0,'-100999999981','profile:local:unknown','pending','idle',1,now()) RETURNING id")
		if err != nil {
			return err
		}
		job := telegramJobRecord{Id: id.Int64(), TenantId: 999, AccountId: 999, TargetChatId: "-100999999981", OperationNo: "profile:local:unknown", Priority: 1, CreatedAt: gtime.Now()}
		publish := NewSysPublish()
		blocked, err := publish.telegramChannelHasEarlierActiveJob(ctx, job)
		if err != nil {
			return err
		}
		if !blocked {
			t.Fatal("unknown send allowed own channel to overtake")
		}
		job.TargetChatId = "-100999999982"
		blocked, err = publish.telegramChannelHasEarlierActiveJob(ctx, job)
		if err != nil {
			return err
		}
		if blocked {
			t.Fatal("unknown send blocked another channel")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}

// TestPublishScheduleNeverRunsEarly 验证按Asynq秒级分数取整后不会早于实际发送间隔。
func TestPublishScheduleNeverRunsEarly(t *testing.T) {
	base := time.Unix(1700000000, 0)
	for _, delay := range []time.Duration{time.Millisecond, 200 * time.Millisecond, 999 * time.Millisecond, time.Second, 1500 * time.Millisecond} {
		for _, offset := range []time.Duration{0, 500 * time.Millisecond, 999 * time.Millisecond} {
			now := base.Add(offset)
			due := now.Add(delay)
			scheduled := telegramPublishScheduleTime(now, delay)
			if scheduled.Before(due) || scheduled.Sub(due) >= time.Second || scheduled.Nanosecond() != 0 {
				t.Fatalf("invalid scheduling now=%s due=%s scheduled=%s", now, due, scheduled)
			}
		}
	}
}

// TestManualBypassesQueuedBulkLocalPostgres 验证未发送历史任务不能挡住手工任务，已发送部分仍保持频道顺序。
func TestManualBypassesQueuedBulkLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_STREAM_MEDIA_PG") != "1" {
		t.Skip("requires local PostgreSQL")
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	previous := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previous) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "5432", User: "postgres", Pass: "postgres", Name: "youban_hotgo"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rollback := errors.New("rollback manual dispatch fixture")
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		id, err := tx.GetValue("INSERT INTO hg_youban_publish_tg_job(tenant_id,account_id,channel_id,target_chat_id,operation_no,status,dispatch_status,priority) VALUES(999,999,0,'-100999999983','collect:local:capacity','pending','queued',95) RETURNING id")
		if err != nil {
			return err
		}
		job := telegramJobRecord{Id: -1, TenantId: 999, AccountId: 999, TargetChatId: "-100999999983", OperationNo: "profile:local:capacity", Priority: 1}
		publish := NewSysPublish()
		for _, test := range []struct {
			status, phase string
			want          bool
		}{{"pending", "", true}, {"sending", "", false}, {"failed_retry", telegramSendPhaseDisplayConfirmed, false}} {
			if _, err := tx.Model(publishTgJobTable).Where("id", id.Int64()).Data(g.Map{"status": test.status, "send_phase": test.phase}).Update(); err != nil {
				return err
			}
			available, err := publish.telegramChannelHasDispatchCapacity(ctx, job)
			if err != nil {
				return err
			}
			if available != test.want {
				t.Fatalf("status=%s phase=%s capacity=%v want=%v", test.status, test.phase, available, test.want)
			}
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}

// TestStreamPrioritySQLLocalPostgres 验证数据库队首排序与Go入队分类一致，历史采集不挤占实时采集。
func TestStreamPrioritySQLLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_STREAM_MEDIA_PG") != "1" {
		t.Skip("requires local PostgreSQL")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "5432", User: "postgres", Pass: "postgres", Name: "youban_hotgo"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, job := range []telegramJobRecord{
		{OperationNo: "profile:local:1", Priority: tgJobPriorityBulk},
		{OperationNo: "collect:local:2", Priority: tgJobPriorityUrgent},
		{OperationNo: "collect:local:3", Priority: tgJobPriorityHistory},
		{OperationNo: "collect:local:4", Priority: tgJobPriorityDefault},
		{OperationNo: "profile_cycle:local:5", Priority: tgJobPriorityDefault},
		{OperationNo: "cycle_batch:local:6", Priority: tgJobPriorityDefault},
	} {
		value, err := db.GetValue(ctx, "SELECT "+telegramJobEffectivePrioritySQL("job")+" FROM (SELECT ?::text AS operation_no, ?::integer AS priority, ?::text AS status) job", job.OperationNo, job.Priority, job.Status)
		if err != nil {
			t.Fatal(err)
		}
		if value.Int() != telegramJobPriorityValue(job) {
			t.Fatalf("SQL/Go priority mismatch job=%+v SQL=%d Go=%d", job, value.Int(), telegramJobPriorityValue(job))
		}
	}
}

// TestStreamCheckpointRecoveryLocalPostgres 验证活跃租约不会被接管、失去执行者的完整展示证据可直接收口且不创建替代任务。
func TestStreamCheckpointRecoveryLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_STREAM_MEDIA_PG") != "1" {
		t.Skip("requires disposable local PG and Redis16382")
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	previousConfig := g.Cfg().GetAdapter()
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	adapter, err := gcfg.NewAdapterContent(`{"redis":{"default":{"address":"127.0.0.1:16382","db":15}}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	previousDB := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "5432", User: "postgres", Pass: "postgres", Name: "youban_hotgo"}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cache.SetAdapter(ctx)
	rollback := errors.New("rollback checkpoint recovery fixture")
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var job telegramJobRecord
		if err := tx.Ctx(ctx).Model(publishTgJobTable+" j").Fields("j.*").
			Where("j.account_id", 999).Where("j.operation_no LIKE '%:local:%'").Where("j.status", telegramJobStatusSent).
			Where("j.send_phase", telegramSendPhaseCompletedNoVerify).
			Where("EXISTS (SELECT 1 FROM " + publishTgMessageTable + " m WHERE m.job_id=j.id AND m.status='sent' AND m.deleted_at IS NULL)").
			OrderDesc("j.id").Limit(1).Scan(&job); err != nil {
			return err
		}
		if job.Id <= 0 {
			t.Fatal("missing completed local fixture")
		}
		if _, err := tx.Ctx(ctx).Model(publishTgJobTable).Where("id", job.Id).Data(g.Map{"status": telegramJobStatusSending, "send_phase": telegramSendPhaseDisplayConfirmed, "dispatch_status": tgDispatchStatusProcessing, "sent_at": nil, "updated_at": gtime.Now().Add(-time.Minute)}).Update(); err != nil {
			return err
		}
		job.Status = telegramJobStatusSending
		job.SendPhase = telegramSendPhaseDisplayConfirmed
		lease := newTelegramStreamExecutionLease(job)
		if ok, err := lease.TryAcquire(ctx); err != nil || !ok {
			t.Fatal("fixture channel busy", ok, err)
		}
		defer lease.Unlock(context.Background())
		publish := NewSysPublish()
		if err := publish.recoverTelegramStreamExecution(ctx, job); err != nil {
			return err
		}
		status, err := tx.Ctx(ctx).Model(publishTgJobTable).Where("id", job.Id).Fields("status").Value()
		if err != nil {
			return err
		}
		if status.String() != telegramJobStatusSending {
			t.Fatal("live execution was stolen", status.String())
		}
		if err := lease.Unlock(ctx); err != nil {
			return err
		}
		before, err := tx.Ctx(ctx).Model(publishTgAttemptTable).Where("job_id", job.Id).Count()
		if err != nil {
			return err
		}
		if err := publish.recoverTelegramStreamExecution(ctx, job); err != nil {
			return err
		}
		status, err = tx.Ctx(ctx).Model(publishTgJobTable).Where("id", job.Id).Fields("status").Value()
		if err != nil {
			return err
		}
		if status.String() != telegramJobStatusSent {
			t.Fatalf("checkpoint did not complete: %s", status.String())
		}
		after, err := tx.Ctx(ctx).Model(publishTgAttemptTable).Where("job_id", job.Id).Count()
		if err != nil {
			return err
		}
		if after != before {
			t.Fatal("checkpoint recovery created another send attempt", before, after)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
