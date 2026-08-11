package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestFinalizeCollectTimelineSortsNodesAndFindsBottleneck(t *testing.T) {
	start := gtime.NewFromTime(time.Date(2026, 8, 11, 10, 0, 0, 0, time.Local))
	timeline := &sysin.CollectMaterialTimelineModel{Nodes: []*sysin.CollectMaterialTimelineNodeModel{
		{Stage: "job_sent", Label: "TG推送成功", At: start.Add(10 * time.Second)},
		{Stage: "tg_received", Label: "TG消息收到", At: start},
		{Stage: "media_ready", Label: "媒体下载与缓存完成", At: start.Add(7 * time.Second)},
	}}

	finalizeCollectTimeline(timeline)
	if timeline.Nodes[0].Stage != "tg_received" || timeline.Nodes[2].Stage != "job_sent" {
		t.Fatalf("unexpected node order: %+v", timeline.Nodes)
	}
	if timeline.TotalDurationMs != 10_000 {
		t.Fatalf("total duration=%d, want 10000", timeline.TotalDurationMs)
	}
	if timeline.BottleneckStage != "tg_received->media_ready" || timeline.BottleneckDurationMs != 7_000 {
		t.Fatalf("unexpected bottleneck: stage=%s duration=%d", timeline.BottleneckStage, timeline.BottleneckDurationMs)
	}
}
