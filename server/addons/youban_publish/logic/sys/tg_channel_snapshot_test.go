package sys

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

// Fixtures are exported through DBX. Replaying them never connects to a live
// database, starts a consumer or invokes Telegram.
func TestTelegramChannelHeadsSnapshotReplay(t *testing.T) {
	path := os.Getenv("YOUBAN_CHANNEL_HEAD_SNAPSHOT")
	if path == "" {
		t.Skip("set YOUBAN_CHANNEL_HEAD_SNAPSHOT to a DBX-exported JSON fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		JobID          string `json:"job_id"`
		ChannelID      string `json:"channel_id"`
		Status         string `json:"status"`
		DispatchStatus string `json:"dispatch_status"`
		NextRetryAt    string `json:"next_retry_at"`
		Now            string `json:"now"`
	}
	if err = json.Unmarshal(data, &rows); err != nil || len(rows) == 0 {
		t.Fatalf("invalid channel fixture: %v", err)
	}
	ready, timezoneBlocked := 0, 0
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		t.Run("channel_"+row.ChannelID+"_job_"+row.JobID, func(t *testing.T) {
			const layout = "2006-01-02 15:04:05.999999999"
			now, err := time.ParseInLocation(layout, row.Now, location)
			if err != nil {
				t.Fatal(err)
			}
			var retry *gtime.Time
			if row.NextRetryAt != "NULL" && row.NextRetryAt != "" {
				// Match lib/pq's timestamp-without-time-zone decode exactly.
				value, parseErr := time.ParseInLocation(layout, row.NextRetryAt, time.UTC)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				retry = gtime.New(value)
			}
			want := telegramOutboxDecisionComplete
			if telegramOutboxJobStillNeedsDispatch(row.Status, row.DispatchStatus) {
				want = telegramOutboxDecisionDispatch
				// Independent database calendar comparison: SQL evaluates these
				// two timestamp columns without any attached timezone.
				if row.NextRetryAt != "NULL" && row.NextRetryAt != "" && row.NextRetryAt > row.Now {
					want = telegramOutboxDecisionWait
				}
			}
			got := telegramOutboxDispatchDecision(row.Status, row.DispatchStatus, retry, gtime.New(now), false)
			if got != want {
				t.Fatalf("decision=%s database expectation=%s", got, want)
			}
			if want == telegramOutboxDecisionDispatch {
				ready++
				if retry != nil && retry.Time.After(now) {
					timezoneBlocked++
				}
				if got := telegramOutboxDispatchDecision(row.Status, row.DispatchStatus, retry, gtime.New(now), true); got != telegramOutboxDecisionWait {
					t.Fatalf("earlier active operation bypassed: %s", got)
				}
			}
		})
	}
	t.Logf("channels=%d ready=%d blocked_by_old_timezone_comparison=%d", len(rows), ready, timezoneBlocked)
}

func TestTelegramProductionAttemptRecoveryFixtures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		attempt telegramDeliveryAttempt
		audit   int
		want    string
	}{
		{"channel106_interrupted", telegramDeliveryAttempt{Id: 1400577, Status: telegramAttemptStatusRetryWait, ErrorMessage: "发送进程中断，等待Webhook或协议号对账"}, 0, telegramUnknownAttemptReconcile},
		{"channel230_rate_limit", telegramDeliveryAttempt{Id: 1410379, Status: telegramAttemptStatusRetryWait, ErrorMessage: "too many requests, Too Many Requests: retry after 16: retry_after 16"}, 0, telegramUnknownAttemptRetry},
		{"channel230_confirmed_display", telegramDeliveryAttempt{Id: 1410377, Status: telegramAttemptStatusConfirmed, ResponseReceived: 1, ConfirmedCount: 4, ExpectedCount: 4}, 4, telegramUnknownAttemptReconcile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := telegramUnknownAttemptRecoveryAction(tc.attempt, tc.audit); got != tc.want {
				t.Fatalf("action=%s want=%s", got, tc.want)
			}
		})
	}
}
