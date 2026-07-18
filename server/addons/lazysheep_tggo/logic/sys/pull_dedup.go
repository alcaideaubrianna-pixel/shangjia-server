package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/internal/library/cache"
)

const pullDedupTTL = time.Hour * 24 * 7

type noteMediaFingerprint struct {
	Kind string   `json:"kind"`
	URLs []string `json:"urls"`
}

type pullDedupRecord struct {
	Fingerprint string   `json:"fingerprint"`
	SourceURLs  []string `json:"sourceUrls"`
	NoteID      int64    `json:"noteId"`
	SeenAt      string   `json:"seenAt"`
}

func noteFingerprint(note noteContent) (string, []string) {
	sourceURLs := make([]string, 0, len(note.Items))
	mediaURLs := make([]string, 0, len(note.Items))
	for _, item := range note.Items {
		if isRemoteMedia(item.Type) && strings.TrimSpace(item.Content) != "" {
			rawURL := strings.TrimSpace(item.Content)
			sourceURLs = append(sourceURLs, rawURL)
			mediaURLs = append(mediaURLs, normalizeDedupMediaURL(rawURL))
		}
	}
	mediaURLs = sortedNonEmptyStrings(mediaURLs)
	if len(mediaURLs) > 0 {
		raw, _ := json.Marshal(noteMediaFingerprint{Kind: "media", URLs: mediaURLs})
		sum := sha256.Sum256(raw)
		return hex.EncodeToString(sum[:]), sourceURLs
	}
	return "", sourceURLs
}

func normalizeDedupMediaURL(rawURL string) string {
	return strings.TrimSpace(rawURL)
}

func sortedNonEmptyStrings(items []string) []string {
	sort.Strings(items)
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func pullDedupKey(scope, fingerprint string) string {
	return fmt.Sprintf("lazysheep_tggo:pull:dedup:%s:%s", strings.TrimSpace(scope), strings.TrimSpace(fingerprint))
}

func pullDedupSeen(ctx context.Context, scope, fingerprint string) (bool, error) {
	if strings.TrimSpace(scope) == "" || strings.TrimSpace(fingerprint) == "" {
		return false, nil
	}
	cacheKey := pullDedupKey(scope, fingerprint)
	val, err := cache.Instance().Get(ctx, cacheKey)
	if err != nil {
		return false, gerror.Wrap(err, "查询重复采集记录失败")
	}
	if val.IsNil() || val.String() == "" {
		return false, nil
	}
	var record pullDedupRecord
	if err = json.Unmarshal([]byte(val.String()), &record); err != nil {
		return true, nil
	}
	if pullDedupRecordFresh(record, time.Now()) {
		return true, nil
	}
	if _, err = cache.Instance().Remove(ctx, cacheKey); err != nil {
		return false, gerror.Wrap(err, "删除过期重复采集记录失败")
	}
	return false, nil
}

func pullDedupRecordFresh(record pullDedupRecord, now time.Time) bool {
	seenAt := strings.TrimSpace(record.SeenAt)
	if seenAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, seenAt)
	if err != nil {
		return true
	}
	return now.Sub(t) < pullDedupTTL
}

func pullDedupRemember(ctx context.Context, scope, fingerprint string, noteID int64, sourceURLs []string) error {
	if strings.TrimSpace(scope) == "" || strings.TrimSpace(fingerprint) == "" {
		return nil
	}
	payload, err := json.Marshal(pullDedupRecord{
		Fingerprint: fingerprint,
		SourceURLs:  sourceURLs,
		NoteID:      noteID,
		SeenAt:      time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return gerror.Wrap(err, "编码重复采集记录失败")
	}
	if err := cache.Instance().Set(ctx, pullDedupKey(scope, fingerprint), string(payload), pullDedupTTL); err != nil {
		return gerror.Wrap(err, "保存重复采集记录失败")
	}
	return nil
}

func clearPullDedupScope(ctx context.Context, scope string) error {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return nil
	}
	keys, err := cache.Instance().Keys(ctx)
	if err != nil {
		return gerror.Wrap(err, "列出重复采集缓存失败")
	}
	prefix := pullDedupKey(scope, "")
	removeKeys := make([]interface{}, 0)
	for _, key := range keys {
		text := strings.TrimSpace(fmt.Sprint(key))
		if strings.HasPrefix(text, prefix) {
			removeKeys = append(removeKeys, key)
		}
	}
	if len(removeKeys) == 0 {
		return nil
	}
	_, err = cache.Instance().Remove(ctx, removeKeys...)
	if err != nil {
		return gerror.Wrap(err, "删除重复采集缓存失败")
	}
	return nil
}

func pullDedupScope(botKey string, binding *model.BindingRecord, chatID int64) string {
	targetChatID := chatID
	if targetChatID == 0 && binding != nil {
		if binding.AutoPush && binding.PublishChatID != 0 {
			targetChatID = binding.PublishChatID
		} else if binding.ReviewChatID != 0 {
			targetChatID = binding.ReviewChatID
		} else {
			targetChatID = binding.PublishChatID
		}
	}
	return fmt.Sprintf("%s:%d", strings.TrimSpace(botKey), targetChatID)
}

func pullCursorFromMessages(messages []json.RawMessage) (int64, string) {
	var maxContentID int64
	var latestCursor string
	for _, raw := range messages {
		var msg sourceMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		contentID := parseInt(msg.ContentId)
		if contentID > maxContentID {
			maxContentID = contentID
			latestCursor = msg.Id
		}
	}
	return maxContentID, latestCursor
}
