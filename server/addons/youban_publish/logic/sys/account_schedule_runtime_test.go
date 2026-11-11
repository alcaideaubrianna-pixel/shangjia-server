package sys

import (
	"fmt"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// TestLocalAccountBusinessRuntime 在固定本地依赖提交真实业务操作，由Docker角色实际发送并核验；不会请求真实TG。
func TestLocalAccountBusinessRuntime(t *testing.T) {
	ctx := localAccountRuntimeContext(t)
	s := NewSysPublish()
	tag := fmt.Sprintf("runtime-business:%d", time.Now().UnixNano())
	profiles := localRuntimeProfiles(t, ctx, 0, 6)
	channels := []int64{1, 2, 3}
	// 历史积压与新手工操作必须分属不同任务；原操作的优先级不能泄漏到手工操作。
	historyOp := "collect:" + tag
	history, err := s.prepareProfilePublishJobs(ctx, profiles[0].ID, 999, 999, historyOp, channels, false, telegramProfilePublishMeta{InitialPriority: tgJobPriorityHistory})
	if err != nil {
		t.Fatal(err)
	}
	manualOp := "profile:" + tag
	manual, err := s.prepareProfilePublishJobs(ctx, profiles[0].ID, 999, 999, manualOp, channels, false, telegramProfilePublishMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 || len(manual) != 3 {
		t.Fatal(history, manual)
	}
	priorityCount, err := g.DB().Model(publishTgJobTable).Ctx(ctx).WhereIn("id", manual).Where("priority", tgJobPriorityInteractive).Count()
	if err != nil || priorityCount != 3 {
		t.Fatal("manual operation lost interactive priority", priorityCount, err)
	}
	for i := range manual {
		if manual[i] == history[i] {
			t.Fatal("manual reused historical operation")
		}
	}
	repeated, err := s.prepareProfilePublishJobs(ctx, profiles[0].ID, 999, 999, manualOp, channels, false, telegramProfilePublishMeta{})
	if err != nil || fmt.Sprint(repeated) != fmt.Sprint(manual) {
		t.Fatal("same operation was not idempotent", repeated, err)
	}
	s.enqueuePreparedProfileJobs(ctx, manual, 0)
	// 编辑也经过同一业务入口，而不是直接构造执行队列消息。
	editOp := "edit:" + tag
	if err := s.submitProfilePublish(ctx, profiles[1].ID, 999, 999, 999, editOp, channels, false); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{manualOp, editOp} {
		for {
			count, err := g.DB().Model(publishTgJobTable).Ctx(ctx).Where("operation_no", op).Where("account_id", 999).Where("status", "sent").Count()
			if err != nil {
				t.Fatal(err)
			}
			if count == 3 {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("business operation did not complete", op, ctx.Err())
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	// 通过已部署scheduler读取真实publish_at：到期前不能生成任务，到期后自动发布。
	source, err := s.profilePublishSource(ctx, profiles[2].ID, 999, 999, false)
	if err != nil {
		t.Fatal(err)
	}
	expectedChannels, err := s.telegramJobChannels(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	at := gtime.Now().Add(8 * time.Second)
	if _, err := g.DB().Model(publishProfileStateTable).Ctx(ctx).Where("tenant_id", 999).Where("account_id", 999).Where("profile_id", profiles[2].ID).Data(g.Map{"publish_at": at}).Update(); err != nil {
		t.Fatal(err)
	}
	before, err := g.DB().Model(publishTgJobTable).Ctx(ctx).Where("account_id", 999).Where("profile_id", profiles[2].ID).OrderDesc("id").Fields("id").Value()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(3 * time.Second):
	}
	early, err := g.DB().Model(publishTgJobTable).Ctx(ctx).Where("account_id", 999).Where("profile_id", profiles[2].ID).WhereGT("id", before.Int64()).Count()
	if err != nil || early != 0 {
		t.Fatal("scheduled publication ran early", early, err)
	}
	for {
		count, err := g.DB().Model(publishTgJobTable).Ctx(ctx).Where("account_id", 999).Where("profile_id", profiles[2].ID).WhereGT("id", before.Int64()).Where("status", "sent").Count()
		if err != nil {
			t.Fatal(err)
		}
		if count == len(expectedChannels) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("scheduled publication did not run", ctx.Err())
		case <-time.After(time.Second):
		}
	}
	t.Logf("real business ingress passed: %s manual=%v edit=%s scheduledProfile=%d", tag, manual, editOp, profiles[2].ID)
}
