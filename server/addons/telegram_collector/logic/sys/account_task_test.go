package sys

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountTaskCanReviveMessageDeleteFallback(t *testing.T) {
	if !accountTaskCanRevive(sysin.AccountTaskTypeMessageDeleteFallback) {
		t.Fatal("message delete fallback task must revive after account runtime recovers")
	}
}

func TestAccountTaskPersistenceIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_TELEGRAM_COLLECTOR_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_TELEGRAM_COLLECTOR_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	service := &sAccountTasks{}
	seed := time.Now().UnixNano()
	submit := &sysin.AccountTaskSubmit{
		TenantID: seed, AccountID: seed, TaskType: sysin.AccountTaskTypeHistoryPage,
		TaskKey: fmt.Sprintf("integration:%d", seed), Priority: 10, HistoryTaskID: seed, MaxAttempts: 2,
	}
	taskID, err := service.Submit(ctx, submit)
	if err != nil {
		t.Fatalf("submit account task: %v", err)
	}
	duplicateID, err := service.Submit(ctx, submit)
	if err != nil || duplicateID != taskID {
		t.Fatalf("duplicate task id=%d want=%d err=%v", duplicateID, taskID, err)
	}
	loaded, err := service.Get(ctx, taskID)
	if err != nil || loaded == nil || loaded.ID != taskID || loaded.Status != sysin.AccountTaskStatusPending {
		t.Fatalf("get task=%+v err=%v", loaded, err)
	}
	lease := &sysin.AccountLease{AccountID: seed, InstanceID: "integration-owner", Epoch: seed, ExpiresAt: time.Now().Add(time.Minute)}
	tasks, err := service.Claim(ctx, lease, 1, time.Minute)
	if err != nil || len(tasks) != 1 || tasks[0].ID != taskID {
		t.Fatalf("claim tasks=%+v err=%v", tasks, err)
	}
	wrongLease := &sysin.AccountLease{AccountID: seed, InstanceID: "wrong-owner", Epoch: seed + 1}
	if err = service.Complete(ctx, taskID, wrongLease, nil); err == nil {
		t.Fatal("stale lease must not complete account task")
	}
	if err = service.Complete(ctx, taskID, lease, nil); err != nil {
		t.Fatalf("complete account task: %v", err)
	}
	columns := dao.TgCollectorAccountTask.Columns()
	row, err := dao.TgCollectorAccountTask.Ctx(ctx).WherePri(taskID).One()
	if err != nil || row[columns.Status].String() != sysin.AccountTaskStatusCompleted {
		t.Fatalf("completed status=%s err=%v", row[columns.Status].String(), err)
	}

	media := sysin.CollectorMediaItem{
		Type: "video", Purpose: "verify", FileID: fmt.Sprintf("gotd:%d", seed),
		SourceKind: "document", SourceMediaID: seed, SourceAccessHash: seed + 1,
		SourceFileReference: []byte{0, 1, 2, 127, 255}, SourceMimeType: "video/mp4",
		SourceDCID: 5, SourceSize: 4096, DebugMetaJSON: `{"duration":12}`,
	}
	mediaSubmit := &sysin.AccountTaskSubmit{
		TenantID: seed, AccountID: seed, TaskType: sysin.AccountTaskTypeMediaDownload,
		TaskKey: fmt.Sprintf("integration-media:%d", seed), Priority: 100,
		MediaOwnerAccountID: seed, Media: &media, MaxAttempts: 2,
	}
	mediaTaskID, err := service.Submit(ctx, mediaSubmit)
	if err != nil {
		t.Fatalf("submit media account task: %v", err)
	}
	mediaTasks, err := service.Claim(ctx, lease, 1, time.Minute)
	if err != nil || len(mediaTasks) != 1 || mediaTasks[0].ID != mediaTaskID {
		t.Fatalf("claim media tasks=%+v err=%v", mediaTasks, err)
	}
	claimedMedia := mediaTasks[0].Media
	if claimedMedia.FileID != media.FileID || string(claimedMedia.SourceFileReference) != string(media.SourceFileReference) {
		t.Fatalf("claimed media=%+v want=%+v", claimedMedia, media)
	}
	mediaResult := &sysin.AccountMediaDownloadResult{
		AttachmentID: seed, FileURL: "https://example.test/media.mp4",
		StoragePath: "attachment/integration/media.mp4", Media: claimedMedia,
	}
	if err = service.Complete(ctx, mediaTaskID, lease, mediaResult); err != nil {
		t.Fatalf("complete media account task: %v", err)
	}
	completedMediaTask, err := service.Get(ctx, mediaTaskID)
	if err != nil || completedMediaTask.MediaResult.AttachmentID != mediaResult.AttachmentID ||
		completedMediaTask.MediaResult.StoragePath != mediaResult.StoragePath ||
		string(completedMediaTask.Media.SourceFileReference) != string(media.SourceFileReference) {
		t.Fatalf("completed media task=%+v err=%v", completedMediaTask, err)
	}
	reusedMediaTask, err := service.SubmitAndWait(ctx, mediaSubmit, time.Millisecond)
	if err != nil || reusedMediaTask.Status != sysin.AccountTaskStatusCompleted ||
		reusedMediaTask.MediaResult.AttachmentID != mediaResult.AttachmentID {
		t.Fatalf("reused completed media task=%+v err=%v", reusedMediaTask, err)
	}
	_, err = dao.TgCollectorAccountTask.Ctx(ctx).WherePri(mediaTaskID).Data(g.Map{
		columns.Status: sysin.AccountTaskStatusDead, columns.AttemptCount: 2,
		columns.LeaseOwner: "expired-owner", columns.LeaseEpoch: seed, columns.LeaseUntil: time.Now(),
		columns.ErrorMessage: "download failed", columns.ResultErrorCode: "download_failed",
	}).Update()
	if err != nil {
		t.Fatalf("prepare dead media task: %v", err)
	}
	revivedMediaTaskID, err := service.Submit(ctx, mediaSubmit)
	if err != nil || revivedMediaTaskID != mediaTaskID {
		t.Fatalf("revive media task id=%d want=%d err=%v", revivedMediaTaskID, mediaTaskID, err)
	}
	revivedMediaTask, err := service.Get(ctx, mediaTaskID)
	if err != nil || revivedMediaTask.Status != sysin.AccountTaskStatusPending || revivedMediaTask.AttemptCount != 0 ||
		revivedMediaTask.LeaseOwner != "" || revivedMediaTask.LeaseEpoch != 0 || revivedMediaTask.LeaseUntil != nil ||
		revivedMediaTask.ErrorMessage != "" || revivedMediaTask.MediaResult.AttachmentID != 0 ||
		revivedMediaTask.MediaResult.ErrorCode != "" {
		t.Fatalf("revived media task=%+v err=%v", revivedMediaTask, err)
	}

	recoverySubmit := &sysin.AccountTaskSubmit{
		TenantID: seed, AccountID: seed, TaskType: sysin.AccountTaskTypeHistoryPage,
		TaskKey: fmt.Sprintf("integration-recovery:%d", seed), Priority: 1, HistoryTaskID: seed, MaxAttempts: 2,
	}
	recoveryID, err := service.Submit(ctx, recoverySubmit)
	if err != nil {
		t.Fatalf("submit recovery task: %v", err)
	}
	recoveryTasks, err := service.Claim(ctx, lease, 1, time.Millisecond)
	if err != nil || len(recoveryTasks) != 1 || recoveryTasks[0].ID != recoveryID {
		t.Fatalf("claim recovery tasks=%+v err=%v", recoveryTasks, err)
	}
	time.Sleep(5 * time.Millisecond)
	recovered, err := service.RecoverExpired(ctx, 10)
	if err != nil || recovered < 1 {
		t.Fatalf("recover expired=%d err=%v", recovered, err)
	}
	recoveryRow, err := dao.TgCollectorAccountTask.Ctx(ctx).WherePri(recoveryID).One()
	if err != nil || recoveryRow[columns.Status].String() != sysin.AccountTaskStatusFailedRetry {
		t.Fatalf("recovery status=%s err=%v", recoveryRow[columns.Status].String(), err)
	}

	crashedID, err := service.Submit(ctx, &sysin.AccountTaskSubmit{
		TenantID: seed, AccountID: seed, TaskType: sysin.AccountTaskTypeHistoryPage,
		TaskKey: fmt.Sprintf("integration-crashed-final:%d", seed), Priority: 2, HistoryTaskID: seed, MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("submit final-attempt recovery task: %v", err)
	}
	crashedTasks, err := service.Claim(ctx, lease, 1, time.Millisecond)
	if err != nil || len(crashedTasks) != 1 || crashedTasks[0].ID != crashedID {
		t.Fatalf("claim final-attempt recovery tasks=%+v err=%v", crashedTasks, err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err = service.RecoverExpired(ctx, 10); err != nil {
		t.Fatalf("recover final-attempt task: %v", err)
	}
	crashed, err := service.Get(ctx, crashedID)
	if err != nil || crashed.Status != sysin.AccountTaskStatusFinalizing {
		t.Fatalf("crashed final-attempt task=%+v err=%v", crashed, err)
	}

	oldPendingID, err := service.Submit(ctx, &sysin.AccountTaskSubmit{
		TenantID: seed, AccountID: seed, TaskType: sysin.AccountTaskTypeMessageMediaFallback,
		TaskKey: fmt.Sprintf("message-media-fallback:%d:verify", seed), Priority: 100, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("submit old pending task: %v", err)
	}
	_, err = g.DB().Exec(ctx, "UPDATE "+dao.TgCollectorAccountTask.Table()+" SET "+columns.UpdatedAt+"=? WHERE "+columns.Id+"=?", time.Now().Add(-time.Hour), oldPendingID)
	if err != nil {
		t.Fatalf("age pending task: %v", err)
	}
	expired, err := service.ExpirePending(ctx, sysin.AccountTaskTypeMessageMediaFallback, time.Now().Add(-45*time.Minute), 10)
	if err != nil || expired != 1 {
		t.Fatalf("expire pending count=%d err=%v", expired, err)
	}
	oldPending, err := service.Get(ctx, oldPendingID)
	if err != nil || oldPending.Status != sysin.AccountTaskStatusFinalizing {
		t.Fatalf("expired pending task=%+v err=%v", oldPending, err)
	}

	finalizingID, err := dao.TgCollectorAccountTask.Ctx(ctx).Data(g.Map{
		columns.TenantId: seed, columns.AccountId: seed, columns.TaskType: sysin.AccountTaskTypeMessageReconcile,
		columns.TaskKey: fmt.Sprintf("message-reconcile:%d:%d", seed, seed), columns.Status: sysin.AccountTaskStatusFinalizing,
		columns.MaxAttempts: 3, columns.ErrorMessage: "send failed",
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("insert finalizing task: %v", err)
	}
	finalizing, err := service.ListFinalizing(ctx, 10)
	if err != nil || len(finalizing) != 1 || finalizing[0].ID != finalizingID {
		t.Fatalf("list finalizing tasks=%+v err=%v", finalizing, err)
	}
	if err = service.FinalizeFailure(ctx, finalizingID); err != nil {
		t.Fatalf("finalize failed task: %v", err)
	}
	finalized, err := service.Get(ctx, finalizingID)
	if err != nil || finalized.Status != sysin.AccountTaskStatusDead {
		t.Fatalf("finalized task=%+v err=%v", finalized, err)
	}

	t.Cleanup(func() {
		_, _ = dao.TgCollectorAccountTask.Ctx(ctx).Where(columns.TenantId, seed).Delete()
		_, _ = g.DB().Exec(ctx, "SELECT 1")
	})
}
