package sys

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type collectMaterialUnit struct {
	GroupedId string
	UniqueKey string
	RawText   string
	MessageAt *gtime.Time
	MessageId int
	Messages  []int
	Media     []collectMediaItem
}

type collectMaterialMessageView struct {
	// EventID/ParentEventID 保留已落库关系，但不能绕过消息邻接和单验证组约束。
	EventID       int64
	ParentEventID int64
	GroupedID     string
	RawText       string
	Media         []collectMediaItem
	MessageAt     time.Time
}

type collectMaterialPair struct {
	DisplayIndex int
	VerifyIndex  int
}

func pairCollectMaterialMessages(messages []collectMaterialMessageView) []collectMaterialPair {
	pairs := make([]collectMaterialPair, 0)
	// Persisted parents cannot override source message adjacency.
	pairedDisplay := make(map[int]string)
	lastDisplayIndex := -1
	for index, message := range messages {
		if collectMaterialSkippableMessage(message) {
			continue
		}
		kind := classifyProfileMessage(message.RawText, message.Media).Kind
		if lastDisplayIndex >= 0 && collectMaterialMixedVerification(message) {
			kind = profileMessageKindVerify
		}
		switch kind {
		case profileMessageKindDisplay:
			lastDisplayIndex = index
		case profileMessageKindVerify:
			if lastDisplayIndex >= 0 {
				// A stale stored parent neither wins over nor vetoes the
				// nearest legal source context. Canonical/transaction checks
				// remain responsible for validating the durable relationship.
				if group, exists := pairedDisplay[lastDisplayIndex]; exists {
					if group == "" || group != message.GroupedID {
						lastDisplayIndex = -1
						continue
					}
				}
				if !collectMaterialVerifyCandidate(message) {
					lastDisplayIndex = -1
					continue
				}
				displayAt := messages[lastDisplayIndex].MessageAt
				verifyAt := message.MessageAt
				withinWindow := displayAt.IsZero() || verifyAt.IsZero() ||
					(!verifyAt.Before(displayAt) && verifyAt.Sub(displayAt) <= collectMaterialVerifyWindow())
				if withinWindow {
					pairs = append(pairs, collectMaterialPair{DisplayIndex: lastDisplayIndex, VerifyIndex: index})
					pairedDisplay[lastDisplayIndex] = message.GroupedID
				}
			}
		default:
			lastDisplayIndex = -1
		}
	}
	return pairs
}

// Verification candidates retain the existing short-caption/video-only rule.
// Several events with one nonempty GroupedID form a single logical group.
func collectMaterialMixedVerification(message collectMaterialMessageView) bool {
	return collectMaterialVerifyCandidate(message)
}

func collectMaterialVerifyCandidate(message collectMaterialMessageView) bool {
	if utf8.RuneCountInString(strings.TrimSpace(message.RawText)) > 20 {
		return false
	}
	if !profileMessageHasMediaType(message.Media, "video") {
		return false
	}
	for _, media := range message.Media {
		if !strings.EqualFold(strings.TrimSpace(media.Type), "video") {
			return false
		}
	}
	return true
}

func collectMaterialSkippableMessage(message collectMaterialMessageView) bool {
	if profileMessageIgnoredNotice(message.RawText) {
		return true
	}
	if strings.TrimSpace(message.RawText) != "" {
		return false
	}
	// Telegram may split a display album into several media-only events. A
	// photo-only continuation cannot start a new profile and must not prevent
	// the following verification group from reaching the current display.
	return len(message.Media) == 0 || !profileMessageHasMediaType(message.Media, "video")
}

// collectMaterialContextClosed reports whether a later effective business
// message has ended the display's verification context. Members of the first
// verification group keep it open; a distinct second group closes it.
func collectMaterialContextClosed(messages []collectMaterialMessageView, displayIndex int) bool {
	if displayIndex < 0 || displayIndex >= len(messages) {
		return false
	}
	seenVerify := false
	verifyGroup := ""
	for index := displayIndex + 1; index < len(messages); index++ {
		message := messages[index]
		if collectMaterialSkippableMessage(message) {
			continue
		}
		if collectMaterialVerifyCandidate(message) {
			if !seenVerify {
				seenVerify, verifyGroup = true, message.GroupedID
				continue
			}
			if verifyGroup != "" && verifyGroup == message.GroupedID {
				continue
			}
		}
		return true
	}
	return false
}

func collectMaterialEventViews(rows []gdb.Record, mediaByEvent map[int64][]collectMediaItem) []collectMaterialMessageView {
	views := make([]collectMaterialMessageView, 0, len(rows))
	for _, row := range rows {
		views = append(views, collectMaterialMessageView{
			EventID:       row["id"].Int64(),
			ParentEventID: row["material_parent_event_id"].Int64(),
			GroupedID:     strings.TrimSpace(row["source_grouped_id"].String()),
			RawText:       row["raw_text"].String(),
			Media:         mediaByEvent[row["id"].Int64()],
			MessageAt:     collectMaterialEventAt(row),
		})
	}
	return views
}

func buildCollectMaterialUnits(task *sysin.MaterialImportTaskModel, messages []*tg.Message) []*collectMaterialUnit {
	units := make([]*collectMaterialUnit, 0, len(messages))
	unitByKey := make(map[string]*collectMaterialUnit, len(messages))
	for _, msg := range messages {
		if msg == nil || msg.ID <= 0 {
			continue
		}
		groupedID := gotdMessageGroupedId(msg)
		key := fmt.Sprintf("msg:%d", msg.ID)
		if groupedID != "" {
			key = "group:" + groupedID
		}
		unit := unitByKey[key]
		if unit == nil {
			uniqueKey := fmt.Sprintf("task:%d:%d:%s:%d", task.Id, task.TgAccountId, task.SourceChatId, msg.ID)
			if groupedID != "" {
				uniqueKey = fmt.Sprintf("task:%d:%d:%s:group:%s", task.Id, task.TgAccountId, task.SourceChatId, groupedID)
			}
			unit = &collectMaterialUnit{
				GroupedId: groupedID,
				UniqueKey: uniqueKey,
				MessageAt: gtime.NewFromTime(time.Unix(int64(msg.Date), 0)),
				MessageId: msg.ID,
			}
			unitByKey[key] = unit
			units = append(units, unit)
		}
		if strings.TrimSpace(unit.RawText) == "" {
			unit.RawText = strings.TrimSpace(msg.Message)
		}
		unit.Messages = append(unit.Messages, msg.ID)
		unit.Media = append(unit.Media, gotdCollectMedia(msg, task.SourceChatId)...)
	}
	return units
}

func pairCollectMaterialUnits(units []*collectMaterialUnit) []*collectMaterialUnit {
	if len(units) == 0 {
		return nil
	}
	views := make([]collectMaterialMessageView, 0, len(units))
	for _, unit := range units {
		if unit == nil {
			views = append(views, collectMaterialMessageView{})
			continue
		}
		messageAt := time.Time{}
		if unit.MessageAt != nil {
			messageAt = unit.MessageAt.Time
		}
		views = append(views, collectMaterialMessageView{RawText: unit.RawText, Media: unit.Media, MessageAt: messageAt, GroupedID: unit.GroupedId})
	}
	verifyByDisplay := make(map[int][]int)
	for _, pair := range pairCollectMaterialMessages(views) {
		verifyByDisplay[pair.DisplayIndex] = append(verifyByDisplay[pair.DisplayIndex], pair.VerifyIndex)
	}
	paired := make([]*collectMaterialUnit, 0, len(units))
	for index, unit := range units {
		if unit == nil {
			continue
		}
		classification := classifyProfileMessage(unit.RawText, unit.Media)
		if classification.Kind != profileMessageKindDisplay {
			continue
		}
		unit.RawText = classification.Text
		for _, verifyIndex := range verifyByDisplay[index] {
			if verifyIndex >= len(units) || units[verifyIndex] == nil {
				continue
			}
			unit.Media = append(unit.Media, collectMediaItemsWithPurpose(units[verifyIndex].Media, collectMaterialRoleVerify)...)
			unit.Messages = append(unit.Messages, units[verifyIndex].Messages...)
		}
		paired = append(paired, unit)
	}
	return paired
}

func splitCollectMaterialUnits(units []*collectMaterialUnit) (processable []*collectMaterialUnit, pending []*collectMaterialUnit) {
	if len(units) == 0 {
		return nil, nil
	}
	firstTextIndex := -1
	for index, unit := range units {
		if unit != nil && strings.TrimSpace(unit.RawText) != "" {
			firstTextIndex = index
			break
		}
	}
	if firstTextIndex < 0 {
		return nil, units
	}
	if firstTextIndex == 0 {
		return units, nil
	}
	return units[firstTextIndex:], units[:firstTextIndex]
}

func mergeCollectMaterialUnits(units []*collectMaterialUnit) []*collectMaterialUnit {
	if len(units) == 0 {
		return nil
	}
	merged := make([]*collectMaterialUnit, 0, len(units))
	for _, unit := range units {
		if unit == nil {
			continue
		}
		if len(merged) > 0 {
			prev := merged[len(merged)-1]
			if prev != nil && prev.GroupedId != "" && prev.GroupedId == unit.GroupedId {
				if strings.TrimSpace(prev.RawText) == "" {
					prev.RawText = unit.RawText
				}
				if strings.TrimSpace(prev.UniqueKey) == "" {
					prev.UniqueKey = unit.UniqueKey
				}
				if prev.MessageAt == nil || (unit.MessageAt != nil && unit.MessageAt.Before(prev.MessageAt)) {
					prev.MessageAt = unit.MessageAt
				}
				prev.MessageId = minCollectMaterialMessageID(prev.MessageId, unit.MessageId)
				prev.Messages = append(prev.Messages, unit.Messages...)
				prev.Media = append(prev.Media, unit.Media...)
				continue
			}
		}
		merged = append(merged, unit)
	}
	return merged
}

func minCollectMaterialMessageID(left, right int) int {
	if left <= 0 {
		return right
	}
	if right <= 0 || left < right {
		return left
	}
	return right
}

func collectMediaItemsWithPurpose(items []collectMediaItem, purpose string) []collectMediaItem {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		purpose = collectMaterialRoleDisplay
	}
	result := make([]collectMediaItem, len(items))
	copy(result, items)
	for index := range result {
		if strings.TrimSpace(result[index].Purpose) == "" {
			result[index].Purpose = purpose
		}
	}
	return result
}

func collectMaterialMessageIDs(existing string, ids []int) string {
	values := make([]int, 0, len(ids)+1)
	for _, item := range strings.Split(existing, ",") {
		if value := gconv.Int(strings.TrimSpace(item)); value > 0 {
			values = append(values, value)
		}
	}
	for _, id := range ids {
		if id > 0 {
			values = append(values, id)
		}
	}
	values = positiveUniqueInts(values)
	sort.Ints(values)
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, gconv.String(value))
	}
	return strings.Join(parts, ",")
}
