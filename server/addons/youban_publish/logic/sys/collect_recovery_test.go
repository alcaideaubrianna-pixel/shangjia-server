package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
)

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
