package sys

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	hglock "hotgo/internal/library/hgrds/lock"
)

func TestIsInvalidTelegramMediaReference(t *testing.T) {
	if !isInvalidTelegramMediaReference(errors.New("bad request: wrong file identifier/HTTP URL specified")) {
		t.Fatal("expected invalid Telegram media reference to be detected")
	}
	if !isInvalidTelegramMediaReference(errors.New(`Bad Request: failed to send message #5 with the error message "WEBPAGE_CURL_FAILED"`)) {
		t.Fatal("expected Telegram URL rejection to be detected")
	}
	if isInvalidTelegramMediaReference(errors.New("request timeout")) {
		t.Fatal("unexpectedly classified timeout as invalid media reference")
	}
}

func TestProfileMediaGroupIdleWaitUsesLastMessageTime(t *testing.T) {
	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.Local)
	pending := profilePendingMediaGroup{CreatedAt: now.Add(-10 * time.Minute).Unix(), UpdatedAt: now.Add(-time.Second).UnixNano()}
	if wait := profileMediaGroupIdleWait(pending, now); wait != time.Second {
		t.Fatalf("unexpected idle wait: got %s, want %s", wait, time.Second)
	}
}

func TestProfileMediaGroupIdleWaitReturnsReadyAfterDebounce(t *testing.T) {
	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.Local)
	pending := profilePendingMediaGroup{UpdatedAt: now.Add(-profileMediaGroupDebounce).UnixNano()}
	if wait := profileMediaGroupIdleWait(pending, now); wait != 0 {
		t.Fatalf("expected media group to be ready, got wait %s", wait)
	}
}

func TestNextProfileRecoveryCursorRotatesAndWraps(t *testing.T) {
	if got := nextProfileRecoveryCursor([]profileSessionRow{{Id: 21}, {Id: 34}}); got != 34 {
		t.Fatalf("nextProfileRecoveryCursor() = %d, want last batch ID 34", got)
	}
	if got := nextProfileRecoveryCursor(nil); got != 0 {
		t.Fatalf("empty recovery batch must wrap cursor to 0, got %d", got)
	}
}

func TestProfileDraftRecoveryResumesSubmittedVerificationOnly(t *testing.T) {
	if profileDraftHasSubmittedVerification(&profileCreateDraft{DisplayMedia: make([]*publishsysin.MessageTemplateMediaInp, 1)}) {
		t.Fatal("display-only draft must not be treated as submitted verification")
	}
	if !profileDraftHasSubmittedVerification(&profileCreateDraft{VerifyMedia: make([]*publishsysin.MessageTemplateMediaInp, 1)}) {
		t.Fatal("persisted verification media must be recoverable")
	}
	if profileDraftHasSubmittedVerification(&profileCreateDraft{VerifyMedia: make([]*publishsysin.MessageTemplateMediaInp, 1), VerificationRecoveryBlocked: true}) {
		t.Fatal("an ambiguous create failure must not be retried automatically")
	}
	if !profileDraftHasSubmittedVerification(&profileCreateDraft{VerificationSubmitted: true}) {
		t.Fatal("explicitly submitted skip/empty verification must remain recoverable")
	}
}

func TestProfileShouldSendVerifyPrompt(t *testing.T) {
	tests := []struct {
		name   string
		step   string
		status string
		draft  *profileCreateDraft
		want   bool
	}{
		{name: "waiting for verify", step: "waiting_verify", status: profileSessionStatusActive, draft: &profileCreateDraft{}, want: true},
		{name: "display group pending", step: "waiting_verify", status: profileSessionStatusActive, draft: &profileCreateDraft{PendingGroup: &profilePendingMediaGroup{}}, want: false},
		{name: "verify already received", step: "waiting_verify", status: profileSessionStatusActive, draft: &profileCreateDraft{PendingVerify: &profilePendingMediaGroup{}}, want: false},
		{name: "verification already submitted", step: "waiting_verify", status: profileSessionStatusActive, draft: &profileCreateDraft{VerificationSubmitted: true, VerifyMedia: []*publishsysin.MessageTemplateMediaInp{{MediaType: "video"}}}, want: false},
		{name: "still collecting display", step: "waiting_display", status: profileSessionStatusActive, draft: &profileCreateDraft{}, want: false},
		{name: "session completed", step: "waiting_verify", status: profileSessionStatusCompleted, draft: &profileCreateDraft{}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := profileShouldSendVerifyPrompt(test.step, test.status, test.draft); got != test.want {
				t.Fatalf("profileShouldSendVerifyPrompt() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestProfileGroupMediaKeepsTelegramOrderAndDropsDuplicates(t *testing.T) {
	first := &publishsysin.MessageTemplateMediaInp{MediaType: "image", TgFileId: "first"}
	second := &publishsysin.MessageTemplateMediaInp{MediaType: "video", TgFileId: "second"}
	media := appendProfileGroupMedia(nil, 102, []*publishsysin.MessageTemplateMediaInp{second})
	media = appendProfileGroupMedia(media, 101, []*publishsysin.MessageTemplateMediaInp{first})
	media = appendProfileGroupMedia(media, 101, []*publishsysin.MessageTemplateMediaInp{first})
	media = orderedProfileGroupMedia(media)
	if len(media) != 2 {
		t.Fatalf("unexpected media count: got %d, want 2", len(media))
	}
	if media[0].TgFileId != "first" || media[0].SortIndex != 1 {
		t.Fatalf("unexpected first media: %#v", media[0])
	}
	if media[1].TgFileId != "second" || media[1].SortIndex != 2 {
		t.Fatalf("unexpected second media: %#v", media[1])
	}
}

func TestProfileMediaGroupPurposeUsesFirstAndSecondGroup(t *testing.T) {
	draft := &profileCreateDraft{
		PendingGroupId: "display-group",
		PendingGroup:   &profilePendingMediaGroup{GroupId: "display-group", Purpose: "display", Text: "正文"},
	}
	if got := profileMediaGroupPurpose("waiting_display", draft, "display-group"); got != "display" {
		t.Fatalf("first group purpose = %q, want display", got)
	}
	if got := profileMediaGroupPurpose("waiting_display", draft, "verify-group"); got != "verify_pending" {
		t.Fatalf("second group purpose = %q, want verify_pending", got)
	}
	if got := profileMediaGroupPurpose("waiting_verify", draft, "verify-group"); got != "verify" {
		t.Fatalf("waiting verify purpose = %q, want verify", got)
	}
}

func TestProfileMediaFinalizeLockKeyScopesSessionAndGroup(t *testing.T) {
	if got, want := profileMediaFinalizeLockKey(1646, "group-1"), "youban_bot:profile_media_finalize:1646:group-1"; got != want {
		t.Fatalf("profileMediaFinalizeLockKey() = %q, want %q", got, want)
	}
	if profileMediaFinalizeLockKey(1646, "group-1") == profileMediaFinalizeLockKey(1647, "group-1") {
		t.Fatal("different sessions must not share a finalize lock")
	}
	if profileMediaFinalizeLockKey(1646, "group-1") == profileMediaFinalizeLockKey(1646, "group-2") {
		t.Fatal("different media groups must not share a finalize lock")
	}
}

func TestProfileMediaFinalizeLockIsExclusiveAcrossInstances(t *testing.T) {
	redis := miniredis.RunT(t)
	config, err := gcfg.NewAdapterContent(fmt.Sprintf(`{"redis":{"default":{"address":%q}}}`, redis.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	previous := g.Cfg().GetAdapter()
	g.Cfg().SetAdapter(config)
	t.Cleanup(func() { g.Cfg().SetAdapter(previous) })

	var successes atomic.Int32
	var lockErrors atomic.Int32
	var wait sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			lease, err := tryProfileMediaFinalizeLock(1646, "album-1")
			if err != nil {
				if gerror.Is(err, hglock.ErrLockFailed) {
					lockErrors.Add(1)
					return
				}
				t.Errorf("acquire finalize lock: %v", err)
				return
			}
			successes.Add(1)
			defer func() { _ = lease.Unlock(context.Background()) }()
			time.Sleep(20 * time.Millisecond)
		}()
	}
	close(start)
	wait.Wait()
	if successes.Load() != 1 || lockErrors.Load() != 1 {
		t.Fatalf("concurrent finalize lock results: acquired=%d rejected=%d, want 1 each", successes.Load(), lockErrors.Load())
	}

	// A crashed finalizer does not leave a permanent lock; the fixed lease expires.
	crashedLease, err := tryProfileMediaFinalizeLock(1646, "album-2")
	if err != nil {
		t.Fatal(err)
	}
	_ = crashedLease // Deliberately do not unlock, simulating process termination.
	redis.FastForward(10 * time.Minute)
	recoveredLease, err := tryProfileMediaFinalizeLock(1646, "album-2")
	if err != nil {
		t.Fatalf("lock did not recover after lease expiry: %v", err)
	}
	if err = recoveredLease.Unlock(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestProfileShouldStageEarlyVerify(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		media []*publishsysin.MessageTemplateMediaInp
		want  bool
	}{
		{
			name:  "standalone verify video arrives before display group",
			media: []*publishsysin.MessageTemplateMediaInp{{MediaType: "video", TgFileId: "verify"}},
			want:  true,
		},
		{
			name:  "captioned video can be display content",
			text:  "资料正文",
			media: []*publishsysin.MessageTemplateMediaInp{{MediaType: "video", TgFileId: "display"}},
			want:  false,
		},
		{
			name:  "standalone image is not verify media",
			media: []*publishsysin.MessageTemplateMediaInp{{MediaType: "image", TgFileId: "image"}},
			want:  false,
		},
		{
			name: "mixed media group is not standalone verify media",
			media: []*publishsysin.MessageTemplateMediaInp{
				{MediaType: "video", TgFileId: "video"},
				{MediaType: "image", TgFileId: "image"},
			},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := profileShouldStageEarlyVerify(test.text, test.media); got != test.want {
				t.Fatalf("profileShouldStageEarlyVerify() = %t, want %t", got, test.want)
			}
		})
	}
}
