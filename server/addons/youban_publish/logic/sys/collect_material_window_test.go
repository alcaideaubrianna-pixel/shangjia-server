package sys

import (
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectMaterialMutableStatusesExcludeTerminalEvents(t *testing.T) {
	statuses := make(map[string]struct{})
	for _, status := range collectMaterialMutableStatuses() {
		statuses[status] = struct{}{}
	}
	for _, terminal := range []string{sysin.CollectEventStatusProcessed, sysin.CollectEventStatusDispatched, sysin.CollectEventStatusIgnored} {
		if _, ok := statuses[terminal]; ok {
			t.Fatalf("terminal status %q must not be mutable", terminal)
		}
	}
	for _, active := range []string{sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusMediaReady} {
		if _, ok := statuses[active]; !ok {
			t.Fatalf("active status %q must remain mutable", active)
		}
	}
}

func TestFindCollectVerifyEventsUsesFollowingVideoGroups(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-4 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(1), "source_message_id": gvar.New(100), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A100"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(2), "source_message_id": gvar.New(101), "material_role": gvar.New("pending"), "raw_text": gvar.New("✅提交成功！"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(3), "source_message_id": gvar.New(102), "material_role": gvar.New("pending"), "raw_text": gvar.New(""), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(4), "source_message_id": gvar.New(103), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A101"), "received_at": gvar.New(receivedAt)},
	}
	if got := (&sSysPublish{}).findCollectVerifyEvents(rows, collectMaterialEventViews(rows, collectTestMediaByEvent(rows)), 0); len(got) != 1 || got[0] != 2 {
		t.Fatalf("verify event indexes=%v, want [2]", got)
	}
}

func TestTargetedRequeuePairsFollowingVerifyContext(t *testing.T) {
	displayAt := time.Now().Add(-time.Hour)
	rows := []gdb.Record{
		{
			"id": gvar.New(4871529), "source_message_id": gvar.New(21459),
			"material_role": gvar.New(collectMaterialRolePending), "raw_text": gvar.New("昵称：A1"),
			"received_at": gvar.New(displayAt),
		},
		{
			"id": gvar.New(4871536), "source_message_id": gvar.New(21464),
			"material_role": gvar.New(collectMaterialRolePending), "raw_text": gvar.New(""),
			"received_at": gvar.New(displayAt.Add(7 * time.Second)),
		},
	}
	mediaByEvent := map[int64][]collectMediaItem{
		4871529: {{Type: "photo", FileId: "display-photo"}},
		4871536: {{Type: "video", FileId: "verify-video"}},
	}
	views := collectMaterialEventViews(rows, mediaByEvent)
	if got := NewSysPublish().findCollectVerifyEvents(rows, views, 0); len(got) != 1 || got[0] != 1 {
		t.Fatalf("targeted requeue verify indexes=%v, want [1]", got)
	}
	processEventIDs := map[int64]struct{}{4871529: {}}
	if _, ok := processEventIDs[4871536]; ok {
		t.Fatal("verification context must not become an independent processing target")
	}
}

func TestFindCollectVerifyEventDoesNotCrossNextDisplayGroup(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-4 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(1), "source_message_id": gvar.New(100), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A100"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(2), "source_message_id": gvar.New(101), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A101"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(3), "source_message_id": gvar.New(102), "material_role": gvar.New("pending"), "raw_text": gvar.New(""), "received_at": gvar.New(receivedAt)},
	}
	if got := (&sSysPublish{}).findCollectVerifyEvents(rows, collectMaterialEventViews(rows, collectTestMediaByEvent(rows)), 0); len(got) != 0 {
		t.Fatalf("verify event indexes=%v, want no cross-group match", got)
	}
}

func TestFindCollectDisplayEventRepairsAlreadyPairedVerify(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-4 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(10), "source_message_id": gvar.New(200), "material_role": gvar.New("display"), "material_parent_event_id": gvar.New(0), "raw_text": gvar.New("昵称：A200"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(11), "source_message_id": gvar.New(201), "material_role": gvar.New("verify"), "material_parent_event_id": gvar.New(10), "status": gvar.New("ignored"), "error_message": gvar.New(collectMaterialVerifyUnmatchedMessage), "raw_text": gvar.New(""), "received_at": gvar.New(receivedAt)},
	}
	if got := (&sSysPublish{}).findCollectDisplayEvent(rows, collectMaterialEventViews(rows, collectTestMediaByEvent(rows)), 1); got != 0 {
		t.Fatalf("display event index=%d, want 0", got)
	}
	if !collectMaterialEventNeedsPairRepair(rows[1]) {
		t.Fatal("already ignored unmatched verify event should be repairable")
	}
}

func TestFindCollectDisplayEventPrefersNearestDisplayOverStaleParent(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-4 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(10), "source_message_id": gvar.New(200), "material_role": gvar.New("display"), "raw_text": gvar.New("昵称：旧资料"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(11), "source_message_id": gvar.New(210), "material_role": gvar.New("display"), "raw_text": gvar.New("昵称：相邻资料"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(12), "source_message_id": gvar.New(220), "material_role": gvar.New("verify"), "material_parent_event_id": gvar.New(10), "raw_text": gvar.New(""), "received_at": gvar.New(receivedAt)},
	}
	mediaByEvent := map[int64][]collectMediaItem{
		10: {{Type: "photo", FileId: "old-display"}},
		11: {{Type: "photo", FileId: "nearest-display"}},
		12: {{Type: "video", FileId: "verify"}},
	}
	got := (&sSysPublish{}).findCollectDisplayEvent(rows, collectMaterialEventViews(rows, mediaByEvent), 2)
	if got != 1 {
		t.Fatalf("display event index=%d, want nearest display index 1", got)
	}
}

func TestFindCollectVerifyEventAcceptsUnmatchedVerifyRetry(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-6 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(20), "source_message_id": gvar.New(300), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A300"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(21), "source_message_id": gvar.New(301), "material_role": gvar.New("verify"), "status": gvar.New("ignored"), "error_message": gvar.New(collectMaterialVerifyUnmatchedMessage), "raw_text": gvar.New(""), "received_at": gvar.New(receivedAt)},
	}
	if got := (&sSysPublish{}).findCollectVerifyEvents(rows, collectMaterialEventViews(rows, collectTestMediaByEvent(rows)), 0); len(got) != 1 || got[0] != 1 {
		t.Fatalf("verify retry event indexes=%v, want [1]", got)
	}
}

func TestUnmatchedVerificationRetryRequiresPersistedVerifyRole(t *testing.T) {
	pending := gdb.Record{
		"material_role": gvar.New(collectMaterialRolePending),
		"status":        gvar.New(sysin.CollectEventStatusIgnored),
		"error_message": gvar.New(collectMaterialVerifyUnmatchedMessage),
	}
	if collectMaterialEventNeedsPairRepair(pending) {
		t.Fatal("pending ignored event must not silently enter verify recovery")
	}
	pending["material_role"] = gvar.New(collectMaterialRoleVerify)
	if !collectMaterialEventNeedsPairRepair(pending) {
		t.Fatal("persisted verify role must keep an unmatched video recoverable")
	}
}

func collectTestMediaByEvent(rows []gdb.Record) map[int64][]collectMediaItem {
	result := make(map[int64][]collectMediaItem, len(rows))
	for _, row := range rows {
		text := row["raw_text"].String()
		messageId := row["source_message_id"].Int64()
		switch {
		case text == "" && (messageId == 102 || messageId == 201 || messageId == 301):
			result[row["id"].Int64()] = []collectMediaItem{{Type: "video", FileId: "video"}}
		case strings.HasPrefix(text, "昵称："):
			result[row["id"].Int64()] = []collectMediaItem{{Type: "photo", FileId: "photo"}}
		}
	}
	return result
}

func TestCollectMaterialEventOlderThanUsesDatabaseWallClock(t *testing.T) {
	databaseTime := time.Now().Add(-4 * time.Minute).UTC()
	event := gdb.Record{"created_at": gvar.New(databaseTime)}
	if !collectMaterialEventOlderThan(event, 3*time.Minute) {
		t.Fatalf("database wall-clock event should be considered older than grouping window")
	}
}

func TestCollectMaterialEventOlderThanWaitsFromHistoricalIngestion(t *testing.T) {
	event := gdb.Record{
		"received_at": gvar.New(time.Now().Add(-24 * time.Hour)),
		"created_at":  gvar.New(time.Now().Add(-time.Minute)),
	}
	if collectMaterialEventOlderThan(event, 3*time.Minute) {
		t.Fatal("historical media group should wait from local ingestion time")
	}
}

func TestCollectMaterialEventIngestOlderThanUsesCreatedAt(t *testing.T) {
	event := gdb.Record{
		"received_at": gvar.New(time.Now().Add(-24 * time.Hour)),
		"created_at":  gvar.New(time.Now().Add(-time.Minute)),
	}
	if collectMaterialEventIngestOlderThan(event, 3*time.Minute) {
		t.Fatal("historical event should wait from ingestion time before finalizing an unmatched pair")
	}
}

func TestPairCollectMaterialMessagesCases(t *testing.T) {
	tests := []struct {
		name  string
		views []collectMaterialMessageView
		want  []collectMaterialPair
	}{
		{
			name: "display then verify",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}},
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 0, VerifyIndex: 1}},
		},
		{
			name: "notice between display and verify",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}},
				{RawText: "✅提交成功！"},
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 0, VerifyIndex: 2}},
		},
		{
			name: "new profile context replaces previous display",
			views: []collectMaterialMessageView{
				{RawText: "昵称：SKS457", Media: []collectMediaItem{{Type: "photo", FileId: "p"}, {Type: "video", FileId: "display-v"}}},
				{RawText: "昵称：SKS457\n年龄：20"},
				{Media: []collectMediaItem{{Type: "video", FileId: "verify-v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 1, VerifyIndex: 2}},
		},
		{
			name: "text only display can pair when no media display is waiting",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A\n年龄：20"},
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 0, VerifyIndex: 1}},
		},
		{
			name: "next display closes previous",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p1"}}},
				{RawText: "昵称：B", Media: []collectMediaItem{{Type: "photo", FileId: "p2"}}},
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 1, VerifyIndex: 2}},
		},
		{
			name: "verify before display is isolated",
			views: []collectMaterialMessageView{
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}},
			},
		},
		{
			name: "mixed media without profile text is verify",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}},
				{Media: []collectMediaItem{{Type: "photo", FileId: "p2"}, {Type: "video", FileId: "v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 0, VerifyIndex: 1}},
		},
		{
			name: "unknown message blocks later video",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}},
				{RawText: "普通业务消息"},
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := pairCollectMaterialMessages(test.views)
			if len(got) != len(test.want) {
				t.Fatalf("pairs=%+v want=%+v", got, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("pairs=%+v want=%+v", got, test.want)
				}
			}
		})
	}
}

func TestPairCollectMaterialMessagesRejectsVerifyOutsideTwentyMinutes(t *testing.T) {
	displayAt := time.Now().Add(-30 * time.Minute)
	views := []collectMaterialMessageView{
		{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}, MessageAt: displayAt},
		{Media: []collectMediaItem{{Type: "video", FileId: "v"}}, MessageAt: displayAt.Add(20*time.Minute + time.Second)},
	}
	if pairs := pairCollectMaterialMessages(views); len(pairs) != 0 {
		t.Fatalf("verification outside twenty-minute window must not pair: %+v", pairs)
	}
}

func TestPairCollectMaterialMessagesAcceptsVerifyAtTwentyMinuteBoundary(t *testing.T) {
	displayAt := time.Now().Add(-30 * time.Minute)
	views := []collectMaterialMessageView{
		{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}, MessageAt: displayAt},
		{Media: []collectMediaItem{{Type: "video", FileId: "v"}}, MessageAt: displayAt.Add(20 * time.Minute)},
	}
	if pairs := pairCollectMaterialMessages(views); len(pairs) != 1 {
		t.Fatalf("verification at twenty-minute boundary must pair: %+v", pairs)
	}
}

func TestPairCollectMaterialMessagesMultipleAdjacentGroups(t *testing.T) {
	base := time.Now().Add(-time.Minute)
	views := []collectMaterialMessageView{
		{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p1"}}, MessageAt: base},
		{Media: []collectMediaItem{{Type: "video", FileId: "v1"}}, MessageAt: base.Add(time.Second)},
		{RawText: "昵称：B", Media: []collectMediaItem{{Type: "photo", FileId: "p2"}}, MessageAt: base.Add(2 * time.Second)},
		{Media: []collectMediaItem{{Type: "video", FileId: "v2"}}, MessageAt: base.Add(3 * time.Second)},
	}
	want := []collectMaterialPair{{DisplayIndex: 0, VerifyIndex: 1}, {DisplayIndex: 2, VerifyIndex: 3}}
	got := pairCollectMaterialMessages(views)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("pairs=%+v want=%+v", got, want)
	}
}

func TestPairCollectMaterialMessagesAttachesAllContinuousVerifyGroups(t *testing.T) {
	base := time.Now().Add(-time.Minute)
	views := []collectMaterialMessageView{
		{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo"}}, MessageAt: base},
		{Media: []collectMediaItem{{Type: "video", FileId: "verify-1"}}, MessageAt: base.Add(time.Second)},
		{RawText: "✅提交成功！", MessageAt: base.Add(2 * time.Second)},
		{Media: []collectMediaItem{{Type: "video", FileId: "verify-2a"}, {Type: "video", FileId: "verify-2b"}}, MessageAt: base.Add(3 * time.Second)},
	}
	want := []collectMaterialPair{{DisplayIndex: 0, VerifyIndex: 1}, {DisplayIndex: 0, VerifyIndex: 3}}
	got := pairCollectMaterialMessages(views)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("pairs=%+v want=%+v", got, want)
	}
	if collectMaterialContextClosed(views, 0) {
		t.Fatal("continuous verification groups must keep the context open")
	}
}

func TestPairCollectMaterialMessagesAcceptsMixedVerificationAlbum(t *testing.T) {
	views := []collectMaterialMessageView{
		{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo"}}},
		{Media: []collectMediaItem{{Type: "video"}, {Type: "video"}, {Type: "photo"}}},
	}
	if got := pairCollectMaterialMessages(views); len(got) != 1 || got[0] != (collectMaterialPair{DisplayIndex: 0, VerifyIndex: 1}) {
		t.Fatalf("mixed verification album pairs=%v", got)
	}
}

func TestPairCollectMaterialMessagesSkipsEmptyDeletedPlaceholder(t *testing.T) {
	views := []collectMaterialMessageView{
		{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo"}}},
		{},
		{Media: []collectMediaItem{{Type: "video"}}},
	}
	if got := pairCollectMaterialMessages(views); len(got) != 1 {
		t.Fatalf("empty deleted placeholder blocked verification: %v", got)
	}
}

func TestPairCollectMaterialMessagesCrossesSplitDisplayPhotoGroups(t *testing.T) {
	views := []collectMaterialMessageView{
		{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo"}}},
		{Media: []collectMediaItem{{Type: "photo"}, {Type: "photo"}}},
		{Media: []collectMediaItem{{Type: "video"}}},
	}
	if got := pairCollectMaterialMessages(views); len(got) != 1 || got[0].VerifyIndex != 2 {
		t.Fatalf("split display photo group blocked verification: %v", got)
	}
}

func TestCollectMaterialContextClosesOnNextDisplay(t *testing.T) {
	views := []collectMaterialMessageView{
		{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo"}}},
		{Media: []collectMediaItem{{Type: "video"}}},
		{RawText: "昵称：B", Media: []collectMediaItem{{Type: "photo"}}},
	}
	if !collectMaterialContextClosed(views, 0) {
		t.Fatal("next display must immediately close the previous context")
	}
	if collectMaterialContextClosed(views, 2) {
		t.Fatal("latest display context must remain open")
	}
}

func TestCollectMaterialVerifyDeadline(t *testing.T) {
	t.Setenv("YOUBAN_COLLECT_VERIFY_WINDOW", "20m")
	displayAt := time.Now().Add(-10 * time.Minute)
	verifyAt := displayAt.Add(30 * time.Second)
	if collectMaterialVerifyDeadlineReached(displayAt, verifyAt, verifyAt.Add(59*time.Second)) {
		t.Fatal("verification quiet period must last one minute")
	}
	if !collectMaterialVerifyDeadlineReached(displayAt, verifyAt, verifyAt.Add(time.Minute)) {
		t.Fatal("verification must settle after one minute")
	}
	lateVerify := displayAt.Add(20 * time.Minute)
	if !collectMaterialVerifyDeadlineReached(displayAt, lateVerify, displayAt.Add(21*time.Minute)) {
		t.Fatal("paired material must not wait beyond window plus one minute")
	}
}

func TestCollectMaterialVerifyWindowFromEnvironment(t *testing.T) {
	t.Setenv("YOUBAN_COLLECT_VERIFY_WINDOW", "12m")
	if got := collectMaterialVerifyWindow(); got != 12*time.Minute {
		t.Fatalf("window=%s want=12m", got)
	}
	t.Setenv("YOUBAN_COLLECT_VERIFY_WINDOW", "900")
	if got := collectMaterialVerifyWindow(); got != 15*time.Minute {
		t.Fatalf("seconds window=%s want=15m", got)
	}
	t.Setenv("YOUBAN_COLLECT_VERIFY_WINDOW", "invalid")
	if got := collectMaterialVerifyWindow(); got != collectMaterialVerifyWindowDefault {
		t.Fatalf("invalid window=%s want=%s", got, collectMaterialVerifyWindowDefault)
	}
}

func TestPairCollectMaterialMessagesMatchesProductionContextWithoutStealingNextProfileVerify(t *testing.T) {
	base := time.Now().Add(-time.Minute)
	views := []collectMaterialMessageView{
		{RawText: "C2195 昵称: 豆豆 年龄:17", Media: []collectMediaItem{{Type: "photo"}}, MessageAt: base},
		{RawText: "L226 省份:辽宁 年龄:24", Media: []collectMediaItem{{Type: "photo"}, {Type: "video"}}, MessageAt: base.Add(95 * time.Second)},
		{Media: []collectMediaItem{{Type: "video"}}, MessageAt: base.Add(104 * time.Second)},
	}
	pairs := pairCollectMaterialMessages(views)
	if len(pairs) != 1 || pairs[0].DisplayIndex != 1 || pairs[0].VerifyIndex != 2 {
		t.Fatalf("production context paired incorrectly: %+v", pairs)
	}
}

func TestPairCollectMaterialMessagesOnlySkipsKnownCollectorNotices(t *testing.T) {
	for _, text := range []string{
		"❌ 重复投稿 检测到短时间内提交的相同内容，请稍后再试",
		"✅ 提交成功！ 投稿已自动审核通过并成功分发",
		"收录失败！ 此信息已存在，无需重复收录",
		"收录成功！ 编号：261012321392",
		"✅ 收录成功 编号：SG6316",
		"信息投稿推送成功",
		"信息已存在请勿重复提交",
		"⚠ 提交失败：图片重复",
		"❌ 上架被拒绝 去重阶段：数据库去重",
		"❌ 未收录 原因：文案不足 50 字",
		"⚠️采集失败！ 原因：检测到重复资料！",
		"⚠️ 未找到待配对的妹子资料，请先发送妹子资料",
		"✅ 内容已自动转发到发布频道，编号：MM2610140541",
		"✅ 视频已自动转发到视频频道，编号：MM2610145172",
		"验证视频",
	} {
		views := []collectMaterialMessageView{
			{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo"}}},
			{RawText: text},
			{Media: []collectMediaItem{{Type: "video"}}},
		}
		if got := pairCollectMaterialMessages(views); len(got) != 1 {
			t.Fatalf("known collector notice was not skipped: %q pairs=%v", text, got)
		}
	}
	views := []collectMaterialMessageView{
		{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo"}}},
		{RawText: "自定义删除关键词"},
		{Media: []collectMediaItem{{Type: "video"}}},
	}
	if got := pairCollectMaterialMessages(views); len(got) != 0 {
		t.Fatalf("configured deletion keyword must not influence pairing: %v", got)
	}
}

func TestPairCollectMaterialMessagesFindsVerifyAfterHeavyWindowNoise(t *testing.T) {
	base := time.Now().Add(-time.Minute)
	views := []collectMaterialMessageView{{
		RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}, MessageAt: base,
	}}
	for index := 0; index < 100; index++ {
		views = append(views, collectMaterialMessageView{RawText: "✅提交成功！", MessageAt: base.Add(time.Duration(index+1) * time.Second)})
	}
	views = append(views, collectMaterialMessageView{
		Media: []collectMediaItem{{Type: "video", FileId: "verify"}}, MessageAt: base.Add(2 * time.Minute),
	})
	got := pairCollectMaterialMessages(views)
	if len(got) != 1 || got[0].DisplayIndex != 0 || got[0].VerifyIndex != 101 {
		t.Fatalf("verification after noisy messages was not paired: %+v", got)
	}
}

func TestPairCollectMaterialMessagesRejectsOutOfOrderVerifyTimestamp(t *testing.T) {
	base := time.Now().Add(-time.Minute)
	views := []collectMaterialMessageView{
		{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}, MessageAt: base},
		{Media: []collectMediaItem{{Type: "video", FileId: "v"}}, MessageAt: base.Add(-time.Second)},
	}
	if got := pairCollectMaterialMessages(views); len(got) != 0 {
		t.Fatalf("older verification must not pair with a later display: %+v", got)
	}
}

func TestCollectMaterialShouldWaitForVerify(t *testing.T) {
	oldEvent := gdb.Record{"created_at": gvar.New(time.Now().Add(-collectMaterialVerifyWindow() - collectMaterialVerifySettleDelay - time.Minute))}
	freshEvent := gdb.Record{"created_at": gvar.New(time.Now())}
	settlingEvent := gdb.Record{"created_at": gvar.New(time.Now().Add(-collectMaterialVerifyWindow() - 30*time.Second))}
	if !collectMaterialShouldWaitForVerify(oldEvent, true) {
		t.Fatal("a truncated scan must never finalize a display without verification")
	}
	if collectMaterialShouldWaitForVerify(oldEvent, false) {
		t.Fatal("an exhausted verification window may continue without verification")
	}
	if !collectMaterialShouldWaitForVerify(freshEvent, false) {
		t.Fatal("a fresh display must wait for verification")
	}
	if !collectMaterialShouldWaitForVerify(settlingEvent, false) {
		t.Fatal("a display must wait through the one-minute settlement period")
	}
}

func TestCollectMaterialWindowHasBoundedExecution(t *testing.T) {
	if collectMaterialWindowExecutionTimeout <= 0 || collectMaterialWindowExecutionTimeout > 2*time.Minute {
		t.Fatalf("unexpected material window timeout: %s", collectMaterialWindowExecutionTimeout)
	}
	if collectMaterialVerifyWindow() != 20*time.Minute {
		t.Fatalf("unexpected verification window: %s", collectMaterialVerifyWindow())
	}
}

func TestAllCollectRepairSuspendedStatusesAreBlockedFromReplay(t *testing.T) {
	for _, status := range collectRepairSuspendedStatuses {
		if !isCollectRepairSuspended(status) {
			t.Fatalf("repair status %q must remain suspended", status)
		}
	}
}
