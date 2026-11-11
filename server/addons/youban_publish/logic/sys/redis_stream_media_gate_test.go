package sys

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// TestStreamMediaFailureProtectsPartialSends 验证媒体失败不会终止已发送部分或待对账任务。
func TestStreamMediaFailureProtectsPartialSends(t *testing.T) {
	for _, test := range []struct {
		status string
		phase  string
		want   bool
	}{
		{telegramJobStatusPending, "", true},
		{telegramJobStatusRetryWait, "", true},
		{telegramJobStatusSending, "", false},
		{telegramJobStatusConfirming, "", false},
		{telegramJobStatusSent, "", false},
		{telegramJobStatusPending, telegramSendPhaseDisplayConfirmed, false},
		{telegramJobStatusRetryWait, telegramSendPhaseVerifySending, false},
	} {
		t.Run(test.status+"/"+test.phase, func(t *testing.T) {
			got := streamMediaFailureCanSettle(telegramJobRecord{Status: test.status, SendPhase: test.phase})
			if got != test.want {
				t.Fatalf("settle=%v want=%v", got, test.want)
			}
		})
	}
}

// TestStreamFailedMediaLocalPostgres 使用本地完整表结构验证 processing 继续等待、failed 收口且不会误标成功。
// 所有夹具写入同一事务并回滚；不启动发送器，不访问 Telegram。
func TestStreamFailedMediaLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_STREAM_MEDIA_PG") != "1" {
		t.Skip("requires local youban_hotgo PostgreSQL and local_regression fixtures")
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "5432", User: "postgres", Pass: "postgres", Name: "youban_hotgo"}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rollback := errors.New("rollback local media fixture")
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		profile, err := tx.GetValue("SELECT min(p.id) FROM hg_content_profile p WHERE p.source_type='local_regression' AND EXISTS (SELECT 1 FROM hg_youban_publish_media m WHERE m.profile_id=p.id AND m.account_id=999 AND m.deleted_at IS NULL)")
		if err != nil {
			return err
		}
		if profile.Int64() <= 0 {
			t.Fatal("missing local media fixture")
		}
		if _, err = tx.Exec("UPDATE hg_youban_publish_media SET processing_status='processing' WHERE profile_id=? AND account_id=999", profile.Int64()); err != nil {
			return err
		}
		id, err := tx.GetValue("INSERT INTO hg_youban_publish_tg_job(tenant_id,account_id,profile_id,channel_id,status,dispatch_status,send_phase) VALUES(999,999,?,0,'pending','idle','') RETURNING id", profile.Int64())
		if err != nil {
			return err
		}
		job := telegramJobRecord{Id: id.Int64(), TenantId: 999, AccountId: 999, ProfileId: profile.Int64(), Status: telegramJobStatusPending}
		publish := NewSysPublish()
		if settled, err := publish.settleStreamFailedMediaJob(ctx, job); err != nil || settled {
			t.Fatalf("processing must wait: settled=%v err=%v", settled, err)
		}
		if _, err = tx.Exec("UPDATE hg_youban_publish_media SET processing_status='failed' WHERE profile_id=? AND account_id=999", profile.Int64()); err != nil {
			return err
		}
		if settled, err := publish.settleStreamFailedMediaJob(ctx, job); err != nil || !settled {
			t.Fatalf("failed media must settle: settled=%v err=%v", settled, err)
		}
		status, err := tx.GetValue("SELECT status FROM hg_youban_publish_tg_job WHERE id=?", job.Id)
		if err != nil {
			return err
		}
		if status.String() != telegramJobStatusFailed {
			t.Fatalf("job status=%s", status.String())
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("local media transaction: %v", err)
	}
}
