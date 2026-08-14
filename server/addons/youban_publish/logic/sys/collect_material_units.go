package sys

import (
	"fmt"
	"sort"
	"strings"
	"time"

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
	RawText string
	Media   []collectMediaItem
}

type collectMaterialPair struct {
	DisplayIndex int
	VerifyIndex  int
}

func pairCollectMaterialMessages(messages []collectMaterialMessageView) []collectMaterialPair {
	pairs := make([]collectMaterialPair, 0)
	lastDisplayIndex := -1
	for index, message := range messages {
		switch classifyProfileMessage(message.RawText, message.Media).Kind {
		case profileMessageKindDisplay:
			// A repeated profile text may be posted between a media group and its
			// verification video. Keep the media-bearing group as the pairing
			// candidate; a text-only display can start a pair only when no display
			// is currently waiting.
			if lastDisplayIndex < 0 || len(message.Media) > 0 {
				lastDisplayIndex = index
			}
		case profileMessageKindVerify:
			if lastDisplayIndex >= 0 {
				pairs = append(pairs, collectMaterialPair{DisplayIndex: lastDisplayIndex, VerifyIndex: index})
				lastDisplayIndex = -1
			}
		}
	}
	return pairs
}

func collectMaterialEventViews(rows []gdb.Record, mediaByEvent map[int64][]collectMediaItem) []collectMaterialMessageView {
	views := make([]collectMaterialMessageView, 0, len(rows))
	for _, row := range rows {
		views = append(views, collectMaterialMessageView{
			RawText: row["raw_text"].String(),
			Media:   mediaByEvent[row["id"].Int64()],
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
		views = append(views, collectMaterialMessageView{RawText: unit.RawText, Media: unit.Media})
	}
	verifyByDisplay := make(map[int]int)
	for _, pair := range pairCollectMaterialMessages(views) {
		verifyByDisplay[pair.DisplayIndex] = pair.VerifyIndex
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
		if verifyIndex, ok := verifyByDisplay[index]; ok && verifyIndex < len(units) && units[verifyIndex] != nil {
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
