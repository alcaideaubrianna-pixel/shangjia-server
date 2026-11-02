package sys

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestDeliveryRecoveryPriorityLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_tg_collector_event(id bigint,source_type text,raw_update jsonb,created_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_tg_collector_delivery(id bigint,event_id bigint) ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_event VALUES(1,'account','{"history":true}',now()),(2,'account','{"history":false}',now()),(3,'account','{"receivedAt":"2020-01-01T00:00:00Z"}',now())`,
			`INSERT INTO hg_tg_collector_delivery VALUES(11,1),(12,2),(13,3)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		for id, want := range map[int64]int{11: sysin.EventPriorityNormal, 12: sysin.EventPriorityRealtime, 13: sysin.EventPriorityNormal} {
			got, err := NewCollector().persistedDeliveryPriority(ctx, id)
			if err != nil {
				return err
			}
			if got != want {
				t.Fatalf("delivery %d recovery priority=%d want=%d", id, got, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeliveryHistoryAndRealtimeClassification(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name, raw string
		want      string
	}{
		{"explicit history even recent", `{"history":true}`, consts.QueueDeliveryHistory},
		{"explicit realtime even old message", `{"history":false,"receivedAt":"2020-01-01T00:00:00Z"}`, consts.QueueDeliveryRealtime},
		{"legacy history", `{"receivedAt":"2020-01-01T00:00:00Z"}`, consts.QueueDeliveryHistory},
		{"legacy realtime", `{"receivedAt":"` + now.Format(time.RFC3339) + `"}`, consts.QueueDeliveryRealtime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := &entity.TgCollectorEvent{SourceType: sysin.SourceTypeAccount, CreatedAt: gtime.New(now)}
			delivery, err := collectorDeliveryFromEvent(event, []byte(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got := deliveryQueueName(deliveryPriority(&delivery)); got != tc.want {
				t.Fatalf("queue=%s want=%s", got, tc.want)
			}
			// Recovery must not classify against the wall clock after a long backlog.
			again, err := collectorDeliveryFromEvent(event, []byte(tc.raw))
			if err != nil || deliveryPriority(&again) != deliveryPriority(&delivery) {
				t.Fatal("recovery changed priority", err)
			}
		})
	}
	for _, history := range []bool{false, true} {
		message := sysin.AccountMessageEvent{History: history, ReceivedAt: now}
		raw, _ := json.Marshal(message)
		delivery, err := collectorDeliveryFromEvent(&entity.TgCollectorEvent{SourceType: sysin.SourceTypeAccount, CreatedAt: gtime.New(now)}, raw)
		if err != nil || accountMessagePriority(&message) != deliveryPriority(&delivery) {
			t.Fatal("ingress and delivery disagree", err)
		}
	}
}

func TestDeliveryRedisQueueIsolationAndDuplicateEnqueue(t *testing.T) {
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	collector := &sCollector{queueClient: client}
	ctx := context.Background()
	for i := int64(1); i <= 20; i++ {
		if err := collector.enqueueDeliveryTask(ctx, i, sysin.EventPriorityNormal); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []int{sysin.EventPriorityRealtime, sysin.EventPriorityUrgent} {
		for i := 0; i < 3; i++ {
			if err := collector.enqueueDeliveryTask(ctx, int64(p), p); err != nil {
				t.Fatal(err)
			}
		}
	}
	for queue, want := range map[string]int{consts.QueueDeliveryHistory: 20, consts.QueueDeliveryRealtime: 1, consts.QueueDeliveryUrgent: 1} {
		tasks, err := inspector.ListPendingTasks(queue)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != want {
			t.Fatalf("queue %s pending=%d want=%d", queue, len(tasks), want)
		}
	}
}
