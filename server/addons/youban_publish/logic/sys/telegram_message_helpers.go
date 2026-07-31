package sys

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
)

var collectMediaFloodWaitPattern = regexp.MustCompile(`(?i)FLOOD_WAIT[_ ]?\(?([0-9]+)\)?`)

func collectMediaFloodWaitDelay(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	message := err.Error()
	matches := collectMediaFloodWaitPattern.FindStringSubmatch(message)
	if len(matches) < 2 {
		if !strings.Contains(strings.ToLower(message), "too many requests") {
			return 0, false
		}
		return time.Minute, true
	}
	seconds, scanErr := strconv.Atoi(matches[1])
	if scanErr != nil || seconds <= 0 {
		return time.Minute, true
	}
	return time.Duration(seconds+2) * time.Second, true
}

func positiveUniqueInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	list := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		list = append(list, value)
	}
	sort.Ints(list)
	return list
}

func collectInputPeerChannel(channel *sysin.ChannelCacheModel) (*tg.InputPeerChannel, error) {
	if channel == nil {
		return nil, gerror.New("频道缓存为空")
	}
	channelID, err := strconv.ParseInt(strings.TrimSpace(channel.ChannelId), 10, 64)
	if err != nil || channelID <= 0 {
		return nil, gerror.New("频道ID无效")
	}
	accessHash, err := strconv.ParseInt(strings.TrimSpace(channel.AccessHash), 10, 64)
	if err != nil {
		return nil, gerror.New("频道AccessHash无效")
	}
	return &tg.InputPeerChannel{ChannelID: channelID, AccessHash: accessHash}, nil
}

func parseGotdCollectFileId(fileID string) (string, int, bool) {
	fileID = strings.TrimSpace(fileID)
	if !strings.HasPrefix(fileID, "gotd:") {
		return "", 0, false
	}
	raw := strings.TrimPrefix(fileID, "gotd:")
	index := strings.LastIndex(raw, ":")
	if index <= 0 || index >= len(raw)-1 {
		return "", 0, false
	}
	messageID, err := strconv.Atoi(raw[index+1:])
	if err != nil || messageID <= 0 {
		return "", 0, false
	}
	return raw[:index], messageID, true
}

func collectForwardRandomIds(ids []int) []int64 {
	now := time.Now().UnixNano()
	randomIDs := make([]int64, 0, len(ids))
	for index, id := range ids {
		randomIDs = append(randomIDs, now+int64(id)+int64(index+1)*1000)
	}
	return randomIDs
}

func collectUpdatesList(updates tg.UpdatesClass) []tg.UpdateClass {
	switch data := updates.(type) {
	case *tg.Updates:
		return data.GetUpdates()
	case *tg.UpdatesCombined:
		return data.GetUpdates()
	default:
		return nil
	}
}
