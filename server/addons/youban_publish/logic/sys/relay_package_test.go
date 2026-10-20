package sys

import (
	"context"
	"encoding/json"
	"errors"
	relayconsts "hotgo/addons/youban_publish/consts"
	"html"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func relayTestManifest() []relayPackageMessage {
	return []relayPackageMessage{
		{SourceID: 10, Marker: relayPackageMarker(5, 0), Message: telegramSentMessage{RelaySourceId: 10, Purpose: "display", MediaId: 7}},
		{SourceID: 12, Marker: relayPackageMarker(5, 1), Message: telegramSentMessage{RelaySourceId: 12, Purpose: "verify", MediaId: 9}},
	}
}

func TestRelayPackageDeliveryEvidence(t *testing.T) {
	manifest := relayTestManifest()
	messages, err := relayPackageCopiedMessages(manifest, []models.MessageID{{ID: 100}, {ID: 101}})
	if err != nil {
		t.Fatal(err)
	}
	if err = validateRelayDelivery(manifest, messages); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func([]*telegramSentMessage) []*telegramSentMessage
	}{
		{"partial", func(m []*telegramSentMessage) []*telegramSentMessage { return m[:1] }},
		{"duplicate target", func(m []*telegramSentMessage) []*telegramSentMessage { m[1].MessageId = m[0].MessageId; return m }},
		{"duplicate source", func(m []*telegramSentMessage) []*telegramSentMessage {
			m[1].RelaySourceId = m[0].RelaySourceId
			return m
		}},
		{"wrong purpose", func(m []*telegramSentMessage) []*telegramSentMessage { m[1].Purpose = "display"; return m }},
		{"wrong media", func(m []*telegramSentMessage) []*telegramSentMessage { m[1].MediaId = 8; return m }},
		{"nil", func(m []*telegramSentMessage) []*telegramSentMessage { m[1] = nil; return m }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := relayPackageCopiedMessages(manifest, []models.MessageID{{ID: 100}, {ID: 101}})
			if validateRelayDelivery(manifest, tt.mutate(m)) == nil {
				t.Fatal("accepted incomplete/misattributed evidence")
			}
		})
	}
	for _, ids := range [][]models.MessageID{{{ID: 100}}, {{ID: 100}, {ID: 100}}, {{ID: 0}, {ID: 101}}} {
		if _, err := relayPackageCopiedMessages(manifest, ids); err == nil {
			t.Fatal("accepted skipped/invalid copy result")
		}
	}
}

func TestRelayPackageManifestAndMarkers(t *testing.T) {
	manifest := relayTestManifest()
	body, _ := json.Marshal(manifest)
	if _, err := decodeRelayManifest(string(body)); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func([]relayPackageMessage){
		func(m []relayPackageMessage) { m[1].SourceID = 9 },
		func(m []relayPackageMessage) { m[1].Marker = m[0].Marker },
		func(m []relayPackageMessage) { m[1].Message.Purpose = "package" },
	} {
		m := relayTestManifest()
		mutate(m)
		b, _ := json.Marshal(m)
		if _, err := decodeRelayManifest(string(b)); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	for i, m := range manifest {
		sent, ok := relayManifestMatch(manifest, "text"+m.Marker, 100+int64(i))
		if !ok || sent.Purpose != m.Message.Purpose || sent.RelaySourceId != m.SourceID {
			t.Fatal("marker lost message ownership")
		}
	}
	markers := manifest[0].Marker + telegramAttemptMarker("relay-package:10:2")
	for _, caption := range []string{"<b>short</b>", "<b>" + strings.Repeat("😀&amp;中文", 400) + "</b>"} {
		result := relayCaptionWithMarkers(caption, markers)
		plain := html.UnescapeString(relayCaptionHTML.ReplaceAllString(result, ""))
		if !strings.HasSuffix(result, markers) || len(utf16.Encode([]rune(plain))) > telegramMediaCaptionLimit {
			t.Fatal("caption dropped markers or exceeded Telegram limit")
		}
	}
}

func TestRelayPackageRolloutAndSnapshot(t *testing.T) {
	t.Setenv("YOUBAN_RELAY_PACKAGE_ENABLED", "true")
	t.Setenv("YOUBAN_RELAY_PACKAGE_TENANTS", "10, 12")
	for _, tt := range []struct {
		job  telegramJobRecord
		want bool
	}{
		{telegramJobRecord{TenantId: 10}, true}, {telegramJobRecord{TenantId: 11}, false},
		{telegramJobRecord{TenantId: 10, SendPhase: telegramSendPhaseDisplaySending}, false},
		{telegramJobRecord{TenantId: 11, SendPhase: relayconsts.RelaySendPhaseReady}, true},
	} {
		if relayPackageEnabled(context.Background(), tt.job) != tt.want {
			t.Fatalf("unexpected rollout for %+v", tt.job)
		}
	}
	t.Setenv("YOUBAN_RELAY_PACKAGE_ENABLED", "false")
	if !relayPackageEnabled(context.Background(), telegramJobRecord{SendPhase: relayconsts.RelaySendPhaseSending}) {
		t.Fatal("disabled flag abandoned in-flight package")
	}
	original := relayPackagePayload{Display: []*telegramMediaItem{{CollectedFileIdEligible: true, AntiScanEnabled: true, AntiScanSeed: 123}}}
	body, _ := json.Marshal(original)
	var restored relayPackagePayload
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.Display[0].CollectedFileIdEligible || restored.Display[0].AntiScanSeed != 123 {
		t.Fatal("snapshot lost reuse/protection policy")
	}
}

func TestRelayFailureScope(t *testing.T) {
	for _, tt := range []struct {
		err   error
		scope string
		delay time.Duration
	}{
		{&tgbot.TooManyRequestsError{RetryAfter: 7}, "bot", 7 * time.Second},
		{errors.New("FLOOD_WAIT_30"), "account", 32 * time.Second},
		{errors.New("SLOWMODE_WAIT_15"), "channel", 17 * time.Second},
		{errors.New("Forbidden: bot is not a member of the channel chat"), "channel", time.Minute},
		{errors.New("temporary network failure"), "temporary", 30 * time.Second},
	} {
		scope, delay := relayFailureScope(tt.err)
		if scope != tt.scope || delay != tt.delay {
			t.Fatalf("%v: got %s %s", tt.err, scope, delay)
		}
	}
}

func TestRelayPackageAttemptTransition(t *testing.T) {
	from, sending, confirmed, err := telegramAttemptPhaseTransition("package")
	if err != nil || from != relayconsts.RelaySendPhaseReady || sending != relayconsts.RelaySendPhaseSending || confirmed != relayconsts.RelaySendPhaseConfirmed {
		t.Fatalf("invalid package state machine %s %s %s %v", from, sending, confirmed, err)
	}
	t.Setenv("YOUBAN_RELAY_PACKAGE_ENABLED", "false")
	if !relayPackageEnabled(context.Background(), telegramJobRecord{SendPhase: confirmed}) {
		t.Fatal("confirmed package must still finalize after rollout disabled")
	}
}
