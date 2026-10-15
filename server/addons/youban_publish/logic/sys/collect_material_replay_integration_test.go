package sys

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

type collectReplayRow struct {
	TenantID      int64    `json:"tenant_id"`
	AccountID     int64    `json:"account_id"`
	TGAccountID   int64    `json:"tg_account_id"`
	SourceID      int64    `json:"source_id"`
	SourceChatID  string   `json:"source_chat_id"`
	EventID       int64    `json:"event_id"`
	MessageID     int64    `json:"message_id"`
	RawText       string   `json:"raw_text"`
	ReceivedAt    string   `json:"received_at"`
	MediaTypes    []string `json:"media_types"`
	MaterialRole  string   `json:"material_role"`
	ParentEventID int64    `json:"parent_event_id"`
}

type collectReplayStats struct {
	Channels             int
	Events               int
	Displays             int
	Verifies             int
	Pairs                int
	MultiVerifyDisplays  int
	ExistingBindings     int
	ExistingBindingDiffs int
	ExistingNowUnmatched int
	ExistingParentDiffs  int
	SampledDisplays      int
	SampledWithVerify    int
	SampledWithoutVerify int
	NewlyMatchedDisplays int
}

func TestCollectMaterialReplayProductionMessages(t *testing.T) {
	var reader io.Reader = os.Stdin
	if path := strings.TrimSpace(os.Getenv("YOUBAN_COLLECT_REPLAY_FILE")); path != "" {
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open production replay file: %v", err)
		}
		defer file.Close()
		reader = file
	} else if os.Getenv("YOUBAN_COLLECT_REPLAY_STDIN") != "1" {
		t.Skip("set YOUBAN_COLLECT_REPLAY_FILE or YOUBAN_COLLECT_REPLAY_STDIN=1")
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	stats := collectReplayStats{}
	currentKey := ""
	rows := make([]collectReplayRow, 0, 4096)
	flush := func() {
		if len(rows) == 0 {
			return
		}
		stats.Channels++
		replayCollectMaterialChannel(t, rows, &stats)
		rows = rows[:0]
	}
	for scanner.Scan() {
		var row collectReplayRow
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatalf("decode production replay row: %v", err)
		}
		key := fmt.Sprintf("%d:%d:%d:%d:%s", row.TenantID, row.AccountID, row.TGAccountID, row.SourceID, row.SourceChatID)
		if currentKey != "" && key != currentKey {
			flush()
		}
		currentKey = key
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read production replay stream: %v", err)
	}
	flush()
	if stats.Channels != 50 {
		t.Fatalf("replayed channels=%d, want 50", stats.Channels)
	}
	if stats.Events < 1000 || stats.Displays == 0 || stats.Verifies == 0 || stats.Pairs == 0 {
		t.Fatalf("production sample is not representative: %+v", stats)
	}
	t.Logf("production replay: channels=%d events=%d displays=%d verifies=%d pairs=%d multiVerifyDisplays=%d existingBindings=%d bindingDiffs=%d existingNowUnmatched=%d existingParentDiffs=%d newlyMatchedDisplays=%d sampledDisplays=%d sampledWithVerify=%d sampledWithoutVerify=%d",
		stats.Channels, stats.Events, stats.Displays, stats.Verifies, stats.Pairs, stats.MultiVerifyDisplays, stats.ExistingBindings, stats.ExistingBindingDiffs, stats.ExistingNowUnmatched, stats.ExistingParentDiffs, stats.NewlyMatchedDisplays,
		stats.SampledDisplays, stats.SampledWithVerify, stats.SampledWithoutVerify)
}

func TestPairCollectMaterialMessagesHundredThousandWaitingEvents(t *testing.T) {
	const eventCount = 100000
	base := time.Now().Add(-time.Hour)
	views := make([]collectMaterialMessageView, 0, eventCount)
	for index := 0; index < eventCount; index++ {
		views = append(views, collectMaterialMessageView{
			RawText:   fmt.Sprintf("昵称：压力资料%d", index),
			Media:     []collectMediaItem{{Type: "photo"}},
			MessageAt: base.Add(time.Duration(index) * time.Millisecond),
		})
	}
	startedAt := time.Now()
	pairs := pairCollectMaterialMessages(views)
	if len(pairs) != 0 {
		t.Fatalf("waiting displays unexpectedly paired: %d", len(pairs))
	}
	if elapsed := time.Since(startedAt); elapsed > 10*time.Second {
		t.Fatalf("100k waiting-event replay too slow: %s", elapsed)
	}
}

func TestHundredThousandWaitingEventsHaveIndependentSchedules(t *testing.T) {
	const eventCount = 100000
	keys := make(map[string]struct{}, eventCount)
	startedAt := time.Now()
	for eventID := int64(1); eventID <= eventCount; eventID++ {
		payload := collectProcessQueuePayload{TenantId: 1, AccountId: 2, SourceId: 3, SourceChatId: "-100123", EventId: eventID}
		key := collectProcessScheduleKey(payload)
		if _, exists := keys[key]; exists {
			t.Fatalf("duplicate event schedule key: %s", key)
		}
		keys[key] = struct{}{}
		body, err := collectProcessTaskBody(payload)
		if err != nil || len(body) == 0 {
			t.Fatalf("encode event %d schedule: %v", eventID, err)
		}
	}
	if elapsed := time.Since(startedAt); elapsed > 10*time.Second {
		t.Fatalf("100k independent schedules too slow: %s", elapsed)
	}
}

func replayCollectMaterialChannel(t *testing.T, rows []collectReplayRow, stats *collectReplayStats) {
	t.Helper()
	views := make([]collectMaterialMessageView, 0, len(rows))
	eventIndex := make(map[int64]int, len(rows))
	for index, row := range rows {
		media := make([]collectMediaItem, 0, len(row.MediaTypes))
		for _, mediaType := range row.MediaTypes {
			media = append(media, collectMediaItem{Type: strings.TrimSpace(mediaType)})
		}
		messageAt, err := time.Parse(time.RFC3339Nano, row.ReceivedAt)
		if err != nil {
			t.Fatalf("parse event %d received_at %q: %v", row.EventID, row.ReceivedAt, err)
		}
		views = append(views, collectMaterialMessageView{RawText: row.RawText, Media: media, MessageAt: messageAt})
		eventIndex[row.EventID] = index
		stats.Events++
		switch classifyProfileMessage(row.RawText, media).Kind {
		case profileMessageKindDisplay:
			stats.Displays++
		case profileMessageKindVerify:
			stats.Verifies++
		}
	}
	pairs := pairCollectMaterialMessages(views)
	predictedParent := make(map[int]int, len(pairs))
	verifyCountByDisplay := make(map[int]int)
	existingVerifyByDisplay := make(map[int64]int)
	for _, row := range rows {
		if row.MaterialRole == collectMaterialRoleVerify && row.ParentEventID > 0 {
			existingVerifyByDisplay[row.ParentEventID]++
		}
	}
	for _, pair := range pairs {
		if pair.DisplayIndex >= pair.VerifyIndex {
			t.Fatalf("non-forward pair in channel %s: %+v", rows[0].SourceChatID, pair)
		}
		if _, exists := predictedParent[pair.VerifyIndex]; exists {
			t.Fatalf("verification event assigned twice: event=%d", rows[pair.VerifyIndex].EventID)
		}
		predictedParent[pair.VerifyIndex] = pair.DisplayIndex
		verifyCountByDisplay[pair.DisplayIndex]++
		stats.Pairs++
	}
	for _, count := range verifyCountByDisplay {
		if count > 1 {
			stats.MultiVerifyDisplays++
		}
	}
	channelDisplays := 0
	channelPairs := len(pairs)
	channelUnmatched := 0
	for index, row := range rows {
		if classifyProfileMessage(row.RawText, views[index].Media).Kind != profileMessageKindDisplay {
			continue
		}
		channelDisplays++
		predictedCount := verifyCountByDisplay[index]
		if predictedCount == 0 {
			channelUnmatched++
			assertCollectReplayDisplayContext(t, rows, views, index, predictedCount)
		} else if existingVerifyByDisplay[row.EventID] == 0 {
			stats.NewlyMatchedDisplays++
		}
		if row.EventID%10 != 0 {
			continue
		}
		stats.SampledDisplays++
		if predictedCount > 0 {
			stats.SampledWithVerify++
		} else {
			stats.SampledWithoutVerify++
		}
		assertCollectReplayDisplayContext(t, rows, views, index, predictedCount)
	}
	for verifyIndex, row := range rows {
		if row.MaterialRole != collectMaterialRoleVerify || row.ParentEventID <= 0 {
			continue
		}
		stats.ExistingBindings++
		parentIndex, parentExists := eventIndex[row.ParentEventID]
		predictedIndex, predicted := predictedParent[verifyIndex]
		if !predicted {
			stats.ExistingNowUnmatched++
			if parentExists {
				delta := views[verifyIndex].MessageAt.Sub(views[parentIndex].MessageAt)
				t.Logf("unmatched existing binding channel=%s display=%d verify=%d delta=%s", row.SourceChatID, row.ParentEventID, row.EventID, delta)
			}
		} else if !parentExists || predictedIndex != parentIndex {
			stats.ExistingParentDiffs++
		}
		if !parentExists || !predicted || predictedIndex != parentIndex {
			stats.ExistingBindingDiffs++
		}
	}
	t.Logf("channel=%s source=%d events=%d displays=%d pairs=%d unmatchedDisplays=%d", rows[0].SourceChatID, rows[0].SourceID, len(rows), channelDisplays, channelPairs, channelUnmatched)
}

func assertCollectReplayDisplayContext(t *testing.T, rows []collectReplayRow, views []collectMaterialMessageView, displayIndex, predictedCount int) {
	t.Helper()
	displayAt := views[displayIndex].MessageAt
	expected := 0
	for index := displayIndex + 1; index < len(views); index++ {
		view := views[index]
		if !view.MessageAt.IsZero() && view.MessageAt.Sub(displayAt) > collectMaterialVerifyWindow() {
			break
		}
		if collectMaterialSkippableMessage(view) {
			continue
		}
		switch classifyProfileMessage(view.RawText, view.Media).Kind {
		case profileMessageKindVerify:
			expected++
		default:
			index = len(views)
		}
	}
	if predictedCount != expected {
		t.Fatalf("10%% context audit failed channel=%s displayEvent=%d predicted=%d expected=%d", rows[displayIndex].SourceChatID, rows[displayIndex].EventID, predictedCount, expected)
	}
}
