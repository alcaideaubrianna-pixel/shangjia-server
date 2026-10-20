package sys

import (
	"context"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/model/input/sysin"
	"testing"
	"time"
)

func TestCollectProgressAgeDoesNotClaimDeadlock(t *testing.T) {
	now := time.Date(2026, 10, 19, 23, 0, 0, 0, time.FixedZone("CST", 8*3600))
	for _, tc := range []struct {
		stage string
		age   time.Duration
		warn  bool
	}{
		{"get_file", 130 * time.Second, true}, {"download", 3 * time.Second, false}, {"scheduled", 10 * time.Minute, false}, {"finished", 10 * time.Minute, false}, {"claimed", 16 * time.Minute, true},
		{"dedupe", 10 * time.Minute, false}, {"ignored", 10 * time.Minute, false},
	} {
		p := &sysin.CollectEventProgressModel{Stage: tc.stage, Since: gtime.New(now.Add(-tc.age))}
		finishCollectProgress(p, now)
		if p.NoProgressWarning != tc.warn || p.ElapsedSeconds != int64(tc.age/time.Second) || p.Label == "" {
			t.Fatalf("stage %s: %+v", tc.stage, p)
		}
	}
	p := &sysin.CollectEventProgressModel{Stage: "queued", Since: gtime.New(now.Add(time.Minute))}
	finishCollectProgress(p, now)
	if p.ElapsedSeconds != 0 {
		t.Fatal("clock skew must not yield negative elapsed time")
	}
}

func TestCollectProgressPreservesIgnoredOutcome(t *testing.T) {
	now := gtime.New("2026-10-20 18:00:00").Time
	for _, tc := range []struct {
		name, reason, stage, label string
	}{
		{"production6120689", "资料库已存在相同资料 profileId:673495 channelId:228 layer:image_phash", "dedupe", "匹配重复"},
		{"textHash", "资料库已存在相同资料 profileId:673488 channelId:228 layer:text_hash", "dedupe", "匹配重复"},
		{"legacyDuplicate", "重复资料，已跳过", "dedupe", "匹配重复"},
		{"ruleMismatch", "消息不是资料组或验证组", "ignored", "匹配忽略"},
		{"emptyReason", "", "ignored", "匹配忽略"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := &sysin.CollectEventModel{Id: 6120689, Status: sysin.CollectEventStatusIgnored, ErrorMessage: tc.reason, ProcessedAt: gtime.New("2026-10-20 17:33:30")}
			latest := &sysin.CollectEventProgressModel{Stage: "commit", Message: "按匹配规则保存资料并创建分发任务", Since: gtime.New("2026-10-20 17:33:29")}
			outbox := gdb.Record{"event_id": gvar.New(item.Id), "status": gvar.New("pending"), "updated_at": gvar.New("2026-10-20 17:34:00"), "available_at": gvar.New("2026-10-20 17:34:10")}
			p := collectEventProgressView(item, latest, outbox, now)
			wantMessage := tc.reason
			if wantMessage == "" {
				wantMessage = tc.label
			}
			if p.Stage != tc.stage || p.Label != tc.label || p.Message != wantMessage || p.NoProgressWarning || p.NextRetryAt != nil || p.Since != item.ProcessedAt {
				t.Fatalf("terminal outcome hidden by progress or outbox: %+v", p)
			}
		})
	}
}
func TestCollectProgressReporterUsesCurrentContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := ""
	ctx = context.WithValue(ctx, collectProgressContextKey{}, collectProgressReporter(func(c context.Context, stage, message string) {
		got = stage + message
		if c.Err() != context.Canceled {
			t.Fatal("lost cancellation")
		}
	}))
	cancel()
	reportCollectProgress(ctx, "download", "file")
	if got != "downloadfile" {
		t.Fatal(got)
	}
	reportCollectProgress(context.Background(), "download", "no reporter is a no-op")
}

// Production snapshot: all 4 media of event 5905256 were pending with zero
// attempts. Its outbox lease says claimed, not that a download is running.
func TestCollectProgressProduction5905256(t *testing.T) {
	now := gtime.New("2026-10-19 23:00:00").Time
	item := &sysin.CollectEventModel{Id: 5905256, Status: sysin.CollectEventStatusGroupCollect, MediaCount: 4, CreatedAt: gtime.New("2026-10-19 22:43:57")}
	outbox := gdb.Record{"event_id": gvar.New(item.Id), "status": gvar.New("processing"), "updated_at": gvar.New("2026-10-19 22:57:18"), "lease_expires_at": gvar.New("2026-10-19 23:02:14")}
	p := collectEventProgressView(item, nil, outbox, now)
	if p.Stage != "claimed" || p.ElapsedSeconds != 162 {
		t.Fatalf("must not display downloading: %+v", p)
	}
	p = collectEventProgressView(item, nil, outbox, now.Add(3*time.Minute))
	if p.Stage != "lease_expired" {
		t.Fatalf("expired lease hidden: %+v", p)
	}
	latest := &sysin.CollectEventProgressModel{Stage: "waiting", Message: "等待验证资料静默期结束", Since: gtime.New("2026-10-19 22:55:00")}
	p = collectEventProgressView(item, latest, outbox, now)
	if p.Stage != "waiting" || p.Message != latest.Message {
		t.Fatal("scheduling timestamp hid wait reason")
	}
	item.Status = sysin.CollectEventStatusProcessed
	p = collectEventProgressView(item, latest, outbox, now)
	if p.Stage != "finished" || p.NoProgressWarning {
		t.Fatal("terminal event shown as stuck")
	}
}
