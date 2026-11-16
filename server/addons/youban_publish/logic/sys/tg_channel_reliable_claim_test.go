package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

// 永远自行创建隔离 Redis；不接受外部地址或生产 DSN。
func reliableChannelTestRedis(t *testing.T, real bool) {
	t.Helper()
	address := ""
	if real {
		binary := "/tmp/redis-7.4.2/src/redis-server"
		if _, err := os.Stat(binary); err != nil {
			t.Skip("本地 Redis 7.4.2 未安装")
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		dir := t.TempDir()
		log, err := os.Create(filepath.Join(dir, "redis.log"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "--bind", "127.0.0.1", "--port", fmt.Sprint(port), "--save", "", "--appendonly", "no", "--dir", dir)
		cmd.Stdout = log
		cmd.Stderr = log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); _ = log.Close() })
		address = fmt.Sprintf("127.0.0.1:%d", port)
		deadline := time.Now().Add(3 * time.Second)
		for {
			conn, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("隔离Redis启动超时")
			}
			time.Sleep(10 * time.Millisecond)
		}
	} else {
		address = miniredis.RunT(t).Addr()
	}
	adapter, err := gcfg.NewAdapterContent(fmt.Sprintf(`{"cache":{"adapter":"redis"},"redis":{"default":{"address":%q}}}`, address))
	if err != nil {
		t.Fatal(err)
	}
	previousConfig := g.Cfg().GetAdapter()
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	client, err := gredis.New(&gredis.Config{Address: address})
	if err != nil {
		t.Fatal(err)
	}
	shared := g.Redis()
	previous := shared.GetAdapter()
	shared.SetAdapter(client.GetAdapter())
	t.Cleanup(func() { shared.SetAdapter(previous); _ = client.Close(context.Background()) })
}

func TestTelegramChannelReliableClaim(t *testing.T) {
	reliableChannelTestRedis(t, false)
	testTelegramChannelReliableClaim(t)
}

func TestTelegramChannelReliableClaimRealRedis(t *testing.T) {
	if os.Getenv("YOUBAN_CHANNEL_RELIABLE_REAL_REDIS") != "1" {
		t.Skip("设置 YOUBAN_CHANNEL_RELIABLE_REAL_REDIS=1 启动隔离真实 Redis")
	}
	reliableChannelTestRedis(t, true)
	testTelegramChannelReliableClaim(t)
}

func testTelegramChannelReliableClaim(t *testing.T) {
	ctx := context.Background()
	enqueue := func(channel, job int64) {
		t.Helper()
		if err := enqueueTelegramChannelJob(ctx, channel, job, tgJobPriorityDefault, time.Now().Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	claim := func(channel int64) telegramChannelQueueClaim {
		t.Helper()
		c, err := claimTelegramChannelJob(ctx, channel)
		if err != nil || c.JobID == 0 {
			t.Fatalf("领取失败 %+v %v", c, err)
		}
		return c
	}
	expire := func(c telegramChannelQueueClaim) {
		t.Helper()
		if _, err := telegramChannelRedisDo(ctx, "HSET", telegramChannelProcessingKey(c.ChannelID), "deadline", 0); err != nil {
			t.Fatal(err)
		}
		if _, err := telegramChannelRedisDo(ctx, "ZADD", telegramChannelProcessingDueKey, 0, c.ChannelID); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("exclusive_channels_and_ack", func(t *testing.T) {
		enqueue(9101, 1000)
		enqueue(9101, 2000)
		enqueue(9102, 3000)
		first := claim(9101)
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c, err := claimTelegramChannelJob(ctx, 9101)
				if err != nil || c.JobID != 0 {
					t.Errorf("并发重复领取 %+v %v", c, err)
				}
			}()
		}
		wg.Wait()
		other := claim(9102)
		if ok, err := settleTelegramChannelClaim(ctx, first, false, 0, time.Time{}); err != nil || !ok {
			t.Fatalf("ack %v %v", ok, err)
		}
		second := claim(9101)
		if second.JobID != 2000 {
			t.Fatal(second)
		}
		for _, c := range []telegramChannelQueueClaim{other, second} {
			if ok, err := settleTelegramChannelClaim(ctx, c, false, 0, time.Time{}); err != nil || !ok {
				t.Fatal(err)
			}
		}
	})
	t.Run("crash_expiry_and_late_owner", func(t *testing.T) {
		enqueue(9103, 4000)
		old := claim(9103)
		expire(old)
		if ok, err := settleTelegramChannelClaim(ctx, old, false, 0, time.Time{}); err != nil || ok {
			t.Fatalf("过期ACK生效 %v %v", ok, err)
		}
		if ok, err := renewTelegramChannelClaim(ctx, old); err != nil || ok {
			t.Fatalf("过期续期生效 %v %v", ok, err)
		}
		if ok, err := recoverExpiredTelegramChannelClaim(ctx, 9103); err != nil || !ok {
			t.Fatalf("过期恢复 %v %v", ok, err)
		}
		current := claim(9103)
		if current.JobID != old.JobID || current.Token == old.Token {
			t.Fatal(current)
		}
		if ok, err := settleTelegramChannelClaim(ctx, old, true, tgJobPriorityHistory, time.Now()); err != nil || ok {
			t.Fatalf("迟到requeue覆盖 %v %v", ok, err)
		}
		if ok, err := settleTelegramChannelClaim(ctx, old, false, 0, time.Time{}); err != nil || ok {
			t.Fatalf("迟到ack覆盖 %v %v", ok, err)
		}
		if ok, err := renewTelegramChannelClaim(ctx, current); err != nil || !ok {
			t.Fatalf("新租约续期 %v %v", ok, err)
		}
		_, _ = settleTelegramChannelClaim(ctx, current, false, 0, time.Time{})
	})
	t.Run("producer_wakeup_survives_ack_and_requeue", func(t *testing.T) {
		for i := int64(0); i < 2; i++ {
			channel := int64(9110) + i
			enqueue(channel, 5000)
			c := claim(channel)
			if err := enqueueTelegramChannelJob(ctx, channel, 5000, tgJobPriorityInteractive, time.Now().Add(-time.Minute)); err != nil {
				t.Fatal(err)
			}
			if ok, err := settleTelegramChannelClaim(ctx, c, i == 1, tgJobPriorityHistory, time.Now().Add(time.Hour)); err != nil || !ok {
				t.Fatalf("settle %v %v", ok, err)
			}
			next := claim(channel)
			if !next.Urgent || next.JobID != 5000 {
				t.Fatalf("新唤醒被覆盖 %+v", next)
			}
			_, _ = settleTelegramChannelClaim(ctx, next, false, 0, time.Time{})
		}
	})
	t.Run("active_remove_race", func(t *testing.T) {
		for i := int64(0); i < 40; i++ {
			channel := int64(9200) + i
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); enqueue(channel, 6000) }()
			go func() {
				defer wg.Done()
				if err := removeActiveTelegramChannel(ctx, channel); err != nil {
					t.Error(err)
				}
			}()
			wg.Wait()
			v, err := telegramChannelRedisDo(ctx, "SISMEMBER", telegramChannelActiveSetKey, channel)
			if err != nil || v.Int() != 1 {
				t.Fatalf("入队丢失active channel:%d %v", channel, err)
			}
		}
	})
	t.Run("redis_rehydrate_legacy_lanes_and_processing", func(t *testing.T) {
		channels := []int64{9301, 9302, 9303, 9304}
		for i, id := range channels[:3] {
			priority := []int{tgJobPriorityInteractive, tgJobPriorityDefault, tgJobPriorityHistory}[i]
			if err := enqueueTelegramChannelJob(ctx, id, 7000, priority, time.Now().Add(-time.Minute)); err != nil {
				t.Fatal(err)
			}
		}
		enqueue(9304, 8000)
		orphan := claim(9304)
		expire(orphan)
		// 即使摘要索引丢失，也应由公平 SCAN 找回持久凭证。
		_, _ = telegramChannelRedisDo(ctx, "ZREM", telegramChannelProcessingDueKey, 9304)
		for _, id := range channels {
			_, _ = telegramChannelRedisDo(ctx, "SREM", telegramChannelActiveSetKey, id)
		}
		service := &sSysPublish{}
		for pass := 0; pass < 100; pass++ {
			if err := service.rehydrateMissingTelegramChannelQueue(ctx, 2); err != nil {
				t.Fatal(err)
			}
			all := true
			for _, id := range channels {
				v, err := telegramChannelRedisDo(ctx, "SISMEMBER", telegramChannelActiveSetKey, id)
				if err != nil {
					t.Fatal(err)
				}
				all = all && v.Int() == 1
			}
			if all {
				return
			}
		}
		t.Fatal("Redis游标未恢复全部lane/processing")
	})
	t.Run("cursor_cas_and_oversized_page_budget", func(t *testing.T) {
		if _, err := telegramChannelRedisDo(ctx, "SET", telegramChannelRehydrateLockKey, "new-owner", "PX", 8000); err != nil {
			t.Fatal(err)
		}
		if ok, err := commitTelegramChannelRehydrateCursor(ctx, "old-owner", "bad"); err != nil || ok {
			t.Fatalf("迟到cursor覆盖 %v %v", ok, err)
		}
		_, _ = telegramChannelRedisDo(ctx, "DEL", telegramChannelRehydrateLockKey)
		keys := []string{}
		for i := int64(0); i < 5; i++ {
			id := int64(9500) + i
			enqueue(id, 10000)
			_, _ = telegramChannelRedisDo(ctx, "SREM", telegramChannelActiveSetKey, id)
			keys = append(keys, telegramChannelQueueKey(id))
		}
		state, _ := json.Marshal(telegramChannelRehydratePage{Cursor: "0", Pending: keys})
		_, _ = telegramChannelRedisDo(ctx, "SET", telegramChannelRehydrateCursorKey, string(state))
		service := &sSysPublish{}
		if err := service.rehydrateMissingTelegramChannelQueue(ctx, 2); err != nil {
			t.Fatal(err)
		}
		raw, err := telegramChannelRedisDo(ctx, "GET", telegramChannelRehydrateCursorKey)
		if err != nil {
			t.Fatal(err)
		}
		var next telegramChannelRehydratePage
		if err := json.Unmarshal(raw.Bytes(), &next); err != nil {
			t.Fatal(err)
		}
		if len(next.Pending) != 3 {
			t.Fatalf("超额页丢失或超出预算 %+v", next)
		}
		for i := int64(0); i < 5; i++ {
			v, err := telegramChannelRedisDo(ctx, "SISMEMBER", telegramChannelActiveSetKey, 9500+i)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if i < 2 {
				want = 1
			}
			if v.Int() != want {
				t.Fatalf("第%d个键超预算处理=%d", i, v.Int())
			}
		}
	})
	t.Run("restore_bad_key_does_not_block_cursor_or_next_channel", func(t *testing.T) {
		badKey := telegramChannelQueueKey(9701)
		if _, err := telegramChannelRedisDo(ctx, "SET", badKey, "wrong-type"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = telegramChannelRedisDo(ctx, "DEL", badKey) })
		enqueue(9702, 12000)
		enqueue(9703, 13000)
		orphan := claim(9703)
		expire(orphan)
		if _, err := telegramChannelRedisDo(ctx, "ZREM", telegramChannelProcessingDueKey, 9703); err != nil {
			t.Fatal(err)
		}
		for _, id := range []int64{9702, 9703} {
			if _, err := telegramChannelRedisDo(ctx, "SREM", telegramChannelActiveSetKey, id); err != nil {
				t.Fatal(err)
			}
		}
		state, err := json.Marshal(telegramChannelRehydratePage{Cursor: "0", Pending: []string{
			badKey, telegramChannelQueueKey(9702), telegramChannelProcessingKey(9703),
		}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := telegramChannelRedisDo(ctx, "SET", telegramChannelRehydrateCursorKey, string(state)); err != nil {
			t.Fatal(err)
		}
		service := &sSysPublish{}
		if err := service.rehydrateMissingTelegramChannelQueue(ctx, 2); err != nil {
			t.Fatalf("坏频道阻塞了当前恢复页：%v", err)
		}
		raw, err := telegramChannelRedisDo(ctx, "GET", telegramChannelRehydrateCursorKey)
		if err != nil {
			t.Fatal(err)
		}
		var next telegramChannelRehydratePage
		if err := json.Unmarshal(raw.Bytes(), &next); err != nil {
			t.Fatal(err)
		}
		if next.Cursor != "0" || len(next.Pending) != 1 || next.Pending[0] != telegramChannelProcessingKey(9703) {
			t.Fatalf("坏频道导致游标未提交或页尾丢失：%+v", next)
		}
		v, err := telegramChannelRedisDo(ctx, "SISMEMBER", telegramChannelActiveSetKey, 9702)
		if err != nil || v.Int() != 1 {
			t.Fatalf("同页后续频道未恢复：%v", err)
		}
		if err := service.rehydrateMissingTelegramChannelQueue(ctx, 2); err != nil {
			t.Fatalf("坏频道阻塞了下一恢复页：%v", err)
		}
		v, err = telegramChannelRedisDo(ctx, "SISMEMBER", telegramChannelActiveSetKey, 9703)
		if err != nil || v.Int() != 1 {
			t.Fatalf("下一页频道未恢复：%v", err)
		}
		recovered := claim(9703)
		if recovered.JobID != orphan.JobID || recovered.Token == orphan.Token {
			t.Fatalf("后续正常 processing 未完成过期恢复：%+v", recovered)
		}
		if ok, err := settleTelegramChannelClaim(ctx, recovered, false, 0, time.Time{}); err != nil || !ok {
			t.Fatalf("恢复后任务无法 ACK：%v", err)
		}
	})
	t.Run("expired_recovery_keeps_new_schedule", func(t *testing.T) {
		enqueue(9401, 9000)
		c := claim(9401)
		if err := enqueueTelegramChannelJob(ctx, 9401, 9000, tgJobPriorityHistory, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		expire(c)
		if ok, err := recoverExpiredTelegramChannelClaim(ctx, 9401); err != nil || !ok {
			t.Fatal(err)
		}
		v, err := telegramChannelRedisDo(ctx, "ZSCORE", telegramChannelHistoryQueueKey(9401), 9000)
		if err != nil || v.IsNil() {
			t.Fatal("新计划丢失", err)
		}
		if c, err := claimTelegramChannelJob(ctx, 9401); err != nil || c.JobID != 0 {
			t.Fatalf("旧时间覆盖新计划 %+v %v", c, err)
		}
	})
}

func TestTelegramChannelReliableRenewalCancelsExecution(t *testing.T) {
	reliableChannelTestRedis(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := enqueueTelegramChannelJob(ctx, 9600, 11000, tgJobPriorityDefault, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	claim, err := claimTelegramChannelJob(ctx, 9600)
	if err != nil || claim.JobID == 0 {
		t.Fatal(err)
	}
	stop := keepTelegramChannelClaim(ctx, cancel, claim)
	defer func() { cancel(); stop() }()
	if _, err := telegramChannelRedisDo(ctx, "HSET", telegramChannelProcessingKey(9600), "token", "new-owner"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(12 * time.Second):
		t.Fatal("丢失租约后没有取消发送context")
	}
}
