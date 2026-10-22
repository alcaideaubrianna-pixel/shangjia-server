package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// BotMediaSearchKnownItems is a read-only preflight before Telegram getFile.
// It returns no profile information and uses the same visibility as search.
func (s *sSysPublish) BotMediaSearchKnownItems(ctx context.Context, in *sysin.BotMediaSearchInp) ([]bool, error) {
	if in == nil || in.TenantId <= 0 || in.AccountId <= 0 {
		return nil, gerror.New("上架账号信息不完整")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.EnsureBotMediaSearchAccess(ctx, in.TenantId); err != nil {
		return nil, err
	}
	ids, err := s.botMediaSearchAccountIds(ctx, in)
	if err != nil {
		return nil, err
	}
	scope, err := s.mediaSearchScopeByAccountIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	matches, err := botMediaIdentityCandidates(ctx, in.Items, scope)
	if err != nil {
		return nil, err
	}
	known := make([]bool, len(in.Items))
	for i := range known {
		known[i] = len(matches[i]) > 0
	}
	return known, nil
}

func botMediaItemUniqueIDs(item *sysin.BotMediaSearchItem) []string {
	if item == nil {
		return nil
	}
	ids := make([]string, 0, len(item.FileUniqueIds)+1)
	seen := make(map[string]bool)
	for _, raw := range append([]string{item.FileUniqueId}, item.FileUniqueIds...) {
		if id := strings.TrimSpace(raw); id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids
}

// Keep indexed identity lookups separate; an OR across file IDs and source
// references can force a scan of the entire send ledger.
func botMediaIdentityCandidates(ctx context.Context, items []*sysin.BotMediaSearchItem, scope *publishmodel.MediaSearchScope) ([][]*botMediaSearchCandidate, error) {
	result := make([][]*botMediaSearchCandidate, len(items))
	if scope == nil || len(scope.Partitions) == 0 {
		return result, nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, botMediaItemUniqueIDs(item)...)
	}
	direct, err := botMediaSentFileCandidates(ctx, ids, scope)
	if err != nil {
		return nil, err
	}
	for i, item := range items {
		if item == nil {
			continue
		}
		seen := make(map[int64]bool)
		add := func(id int64) {
			if id > 0 && !seen[id] {
				seen[id] = true
				result[i] = append(result[i], &botMediaSearchCandidate{ProfileId: id, MatchCount: 1})
			}
		}
		for _, id := range botMediaItemUniqueIDs(item) {
			for _, candidate := range direct[id] {
				add(candidate.ProfileId)
			}
		}
		if len(result[i]) > 0 {
			continue
		}
		scopeSQL, args := mediaPHashBucketScopeSQL("m", scope.Partitions)
		if scopeSQL == "" {
			continue
		}
		mod := g.DB().Model(publishTgMessageTable+" m").Safe().Ctx(ctx).Fields("m.profile_id").
			Where("m.status", "sent").WhereNull("m.deleted_at").Where("("+scopeSQL+")", args...).
			Where("EXISTS (SELECT 1 FROM hg_content_profile p WHERE p.id=m.profile_id AND p.deleted_at IS NULL)").
			Where("EXISTS (SELECT 1 FROM hg_youban_publish_profile_state ps WHERE ps.profile_id=m.profile_id AND ps.tenant_id=m.tenant_id AND ps.account_id=m.account_id AND ps.deleted_at IS NULL)")
		if item.SourceChatId != "" && item.SourceMessageId > 0 {
			mod = mod.Where("m.target_chat_id", item.SourceChatId).Where("m.tg_message_id", item.SourceMessageId)
		} else if item.BotId > 0 && strings.TrimSpace(item.FileId) != "" && len(botMediaItemUniqueIDs(item)) == 0 {
			// Telegram supplies a stable ID with normal media. Do not repeat
			// a failed stable-ID lookup using the longer Bot-scoped ID.
			mod = mod.Where("m.bot_id", item.BotId).Where("m.tg_file_id", item.FileId).Where("m.tg_file_id <> ''")
		} else {
			continue
		}
		rows, err := mod.Group("m.profile_id").Limit(botMediaSearchMaxResults).All()
		if err != nil {
			return nil, gerror.Wrap(err, "查询已推送媒体来源失败")
		}
		for _, row := range rows {
			add(row["profile_id"].Int64())
		}
	}
	return result, nil
}
