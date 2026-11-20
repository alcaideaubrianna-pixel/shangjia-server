package sys

import (
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestCollectEventFailureRecoveryRulesStayConsistent(t *testing.T) {
	for _, message := range collectEventTerminalFailureMessages {
		row := gdb.Record{
			"status":        gvar.New("failed"),
			"error_message": gvar.New("媒体处理失败: " + message),
		}
		if shouldRecoverCollectEvent(row) {
			t.Fatalf("terminal failure %q must not be recovered", message)
		}
	}
	for _, message := range collectEventRecoverableFailureMessages {
		row := gdb.Record{
			"status":        gvar.New("failed"),
			"error_message": gvar.New("处理失败: " + strings.ToUpper(message)),
		}
		if !shouldRecoverCollectEvent(row) {
			t.Fatalf("transient failure %q must remain recoverable", message)
		}
	}
}

func TestCollectEventMediaRecordsCanResume(t *testing.T) {
	now := gtime.Now()
	row := func(status string, values map[string]interface{}) gdb.Record {
		result := gdb.Record{"cache_status": gvar.New(status)}
		for key, value := range values {
			result[key] = gvar.New(value)
		}
		return result
	}
	tests := []struct {
		name string
		rows gdb.Result
		want bool
	}{
		{name: "no media", rows: nil, want: false},
		{name: "durable storage", rows: gdb.Result{row(collectMediaCacheReady, map[string]interface{}{"storage_path": "profiles/a.mp4"})}, want: true},
		{name: "durable url", rows: gdb.Result{row(collectMediaCacheReady, map[string]interface{}{"file_url": "https://example.test/a.jpg"})}, want: true},
		{name: "durable backup", rows: gdb.Result{row(collectMediaCacheReady, map[string]interface{}{"backup_chat_id": "-1001", "backup_message_id": int64(2)})}, want: true},
		{name: "historical fake ready is repairable", rows: gdb.Result{row(collectMediaCacheReady, nil)}, want: true},
		{name: "pending retry due", rows: gdb.Result{row(collectMediaCachePending, map[string]interface{}{"next_retry_at": now.Add(-time.Second)})}, want: true},
		{name: "pending retry unscheduled", rows: gdb.Result{row(collectMediaCachePending, nil)}, want: true},
		{name: "pending retry in future", rows: gdb.Result{row(collectMediaCachePending, map[string]interface{}{"next_retry_at": now.Add(time.Minute)})}, want: false},
		{name: "download in progress", rows: gdb.Result{row(collectMediaCacheDownloading, nil)}, want: false},
		{name: "mixed rows wait for future retry", rows: gdb.Result{
			row(collectMediaCacheReady, map[string]interface{}{"storage_path": "profiles/a.mp4"}),
			row(collectMediaCachePending, map[string]interface{}{"next_retry_at": now.Add(time.Minute)}),
		}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectEventMediaRecordsCanResume(test.rows, now); got != test.want {
				t.Fatalf("collectEventMediaRecordsCanResume()=%t, want %t", got, test.want)
			}
		})
	}
}

func TestCollectMediaRecoveryFollowsItemTimeout(t *testing.T) {
	if collectMediaRecoverAfter <= 15*time.Minute {
		t.Fatalf("media recovery %s must leave commit grace after item timeout", collectMediaRecoverAfter)
	}
	if collectMediaRecoverAfter > 20*time.Minute {
		t.Fatalf("media recovery is too slow: %s", collectMediaRecoverAfter)
	}
}

func TestCollectMediaValidationStopsPollingUntilReplay(t *testing.T) {
	event := gdb.Record{"status": gvar.New("failed"), "error_message": gvar.New("采集规则338分发失败: 展示媒体实际1个: 资料媒体校验不合格")}
	if shouldRecoverCollectEvent(event) || !collectProcessEventFinished(event) || !collectEventTerminalProcessingFailure(event) {
		t.Fatal("validation failure must remain terminal for automatic recovery")
	}
	fields := collectReplayUpdateFields(event, &CollectMessage{}, gtime.Now())
	if fields["status"] == "failed" || fields["processed_at"] != nil || fields["error_message"] != "" {
		t.Fatalf("media replay must reopen failed event: %+v", fields)
	}
	event["status"] = gvar.New(fields["status"])
	event["error_message"] = gvar.New(fields["error_message"])
	if collectEventTerminalProcessingFailure(event) || collectProcessEventFinished(event) {
		t.Fatal("updated material must be processable")
	}
}

func TestCollectTerminalInputFailureRecovery(t *testing.T) {
	for _, message := range []string{"采集规则1分发失败: 采集资料标题为空", "采集规则1分发失败: pq: value too long for type character varying(255)"} {
		row := gdb.Record{"status": gvar.New("failed"), "error_message": gvar.New(message)}
		if shouldRecoverCollectEvent(row) || !collectProcessEventFinished(row) || !collectEventTerminalProcessingFailure(row) {
			t.Fatalf("persisted terminal input error reopened: %s", message)
		}
		row["status"] = gvar.New("pending")
		row["error_message"] = gvar.New("")
		if !shouldRecoverCollectEvent(row) || collectProcessEventFinished(row) || collectEventTerminalProcessingFailure(row) {
			t.Fatal("explicitly reopened event must resume")
		}
	}
}
