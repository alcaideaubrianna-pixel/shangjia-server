package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type collectContentResult struct {
	Id             int64
	RawText        string
	NormalizedText string
	MediaCount     int
	Media          []collectMediaItem
	TextHash       string
	DedupeKey      string
	PreviousSeenAt *gtime.Time
}

// preferCollectContentText prevents a duplicate Telegram account/event with a
// shortened caption from overwriting the richer canonical caption already in
// the content pool. This is especially important for profile fields such as
// age, height and weight, which may be omitted by one collector account while
// still present in another account's copy of the same message.
func preferCollectContentText(existing, incoming string) bool {
	existing = strings.TrimSpace(existing)
	incoming = strings.TrimSpace(incoming)
	if incoming == "" {
		return false
	}
	if existing == "" {
		return true
	}
	fieldScore := func(text string) int {
		score := 0
		for _, field := range []string{"年龄", "身高", "体重", "罩杯", "职业", "省份", "城市", "介绍费", "能否", "是否", "大姨妈", "自我加分"} {
			score += strings.Count(text, field)
		}
		return score
	}
	if incomingScore, existingScore := fieldScore(incoming), fieldScore(existing); incomingScore != existingScore {
		return incomingScore > existingScore
	}
	return len([]rune(incoming)) > len([]rune(existing))
}

func (s *sSysPublish) CollectContentList(ctx context.Context, in *sysin.CollectContentListInp) (list []*sysin.CollectContentModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.CollectContentListInp{}
	}
	contentDao := pdao.YoubanPublishCollectContent
	contentCols := contentDao.Columns()
	mod := contentDao.Ctx(ctx).
		Where(contentCols.TenantId, account.TenantId).
		Where(contentCols.AccountId, account.Id)
	if status := strings.TrimSpace(in.Status); status != "" {
		mod = mod.Where(contentCols.Status, status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where(
			"("+contentCols.RawText+" LIKE ? OR "+contentCols.NormalizedText+" LIKE ? OR "+contentCols.DedupeKey+" LIKE ? OR "+contentCols.TextHash+" LIKE ?)",
			like,
			like,
			like,
			like,
		)
	}
	if in.Duplicated == 1 {
		mod = mod.WhereGT(contentCols.DuplicateTotal, 0)
	} else if in.Duplicated == 2 {
		mod = mod.Where(contentCols.DuplicateTotal, 0)
	}
	if in.MinMediaCount > 0 {
		mod = mod.WhereGTE(contentCols.MediaCount, in.MinMediaCount)
	}
	if totalCount, err = mod.Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "统计采集内容池失败")
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc(contentCols.LastSeenAt).OrderDesc(contentCols.Id).Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取采集内容池失败")
	}
	return
}

func (s *sSysPublish) CollectContentView(ctx context.Context, in *sysin.CollectContentViewInp) (res *sysin.CollectContentModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("内容ID不能为空")
	}
	contentDao := pdao.YoubanPublishCollectContent
	contentCols := contentDao.Columns()
	if err = contentDao.Ctx(ctx).
		Where(contentCols.Id, in.Id).
		Where(contentCols.TenantId, account.TenantId).
		Where(contentCols.AccountId, account.Id).
		Scan(&res); err != nil {
		return nil, gerror.Wrap(err, "读取采集内容池详情失败")
	}
	if res == nil || res.Id <= 0 {
		return nil, gerror.New("采集内容不存在")
	}
	rows, err := s.collectEventMediaRows(ctx, res.LastEventId)
	if err != nil {
		return nil, err
	}
	res.MediaList = collectContentMediaModels(collectMediaRowsToItems(rows, ""), account.TenantId, account.Id, in.Id)
	return res, nil
}

func (s *sSysPublish) ensureCollectContent(ctx context.Context, event gdb.Record) (*collectContentResult, error) {
	result, err := s.collectContentFromEvent(ctx, event)
	if err != nil {
		return nil, err
	}
	s.enrichCollectContentMediaMetadata(ctx, result)
	now := gtime.Now()
	seenAt := event["received_at"].GTime()
	if seenAt == nil {
		seenAt = now
	}
	contentDao := pdao.YoubanPublishCollectContent
	contentCols := contentDao.Columns()
	content, err := contentDao.Ctx(ctx).
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		Where("dedupe_key", result.DedupeKey).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集内容池失败")
	}
	if content.IsEmpty() {
		contentId, err := contentDao.Ctx(ctx).Data(g.Map{
			contentCols.TenantId:       event["tenant_id"].Int64(),
			contentCols.AccountId:      event["account_id"].Int64(),
			contentCols.FirstEventId:   event["id"].Int64(),
			contentCols.LastEventId:    event["id"].Int64(),
			contentCols.SourceType:     event["source_type"].String(),
			contentCols.RawText:        result.RawText,
			contentCols.NormalizedText: result.NormalizedText,
			contentCols.MediaCount:     result.MediaCount,
			contentCols.TextHash:       result.TextHash,
			contentCols.DedupeKey:      result.DedupeKey,
			contentCols.DuplicateTotal: 0,
			contentCols.Status:         "active",
			contentCols.FirstSeenAt:    seenAt,
			contentCols.LastSeenAt:     seenAt,
			contentCols.CreatedAt:      now,
			contentCols.UpdatedAt:      now,
		}).InsertAndGetId()
		if err == nil {
			result.Id = contentId
			return result, nil
		}
		if !isUniqueConstraintError(err) {
			return nil, gerror.Wrap(err, "创建采集内容池失败")
		}
		// Another worker may create the same content after the initial lookup.
		// Re-read the winner and continue through the normal duplicate update so
		// recovery remains idempotent instead of failing the collect event.
		content, err = contentDao.Ctx(ctx).
			Where(contentCols.TenantId, event["tenant_id"].Int64()).
			Where(contentCols.AccountId, event["account_id"].Int64()).
			Where(contentCols.DedupeKey, result.DedupeKey).
			One()
		if err != nil {
			return nil, gerror.Wrap(err, "并发创建采集内容池后读取失败")
		}
		if content.IsEmpty() {
			return nil, gerror.New("并发创建采集内容池后记录不存在")
		}
	}
	result.Id = content["id"].Int64()
	result.PreviousSeenAt = content["last_seen_at"].GTime()
	// Keep the richest caption seen for this dedupe key. Telegram collector
	// accounts can return different caption completeness for the same message.
	storedRawText := content[contentCols.RawText].String()
	storedNormalizedText := content[contentCols.NormalizedText].String()
	if !preferCollectContentText(storedRawText, result.RawText) {
		result.RawText = storedRawText
	}
	if !preferCollectContentText(storedNormalizedText, result.NormalizedText) {
		result.NormalizedText = storedNormalizedText
	}
	_, err = contentDao.Ctx(ctx).
		Where(contentCols.Id, result.Id).
		Data(g.Map{
			contentCols.LastEventId:    event["id"].Int64(),
			contentCols.RawText:        result.RawText,
			contentCols.NormalizedText: result.NormalizedText,
			contentCols.MediaCount:     result.MediaCount,
			contentCols.TextHash:       result.TextHash,
			contentCols.PreviousSeenAt: result.PreviousSeenAt,
			contentCols.LastSeenAt:     seenAt,
			contentCols.DuplicateTotal: gdb.Raw(contentCols.DuplicateTotal + "+1"),
			contentCols.UpdatedAt:      now,
		}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "更新采集内容池失败")
	}
	return result, nil
}

func (s *sSysPublish) collectContentSnapshot(ctx context.Context, event gdb.Record) (*collectContentResult, error) {
	result, err := s.collectContentFromEvent(ctx, event)
	if err != nil {
		return nil, err
	}
	s.enrichCollectContentMediaMetadata(ctx, result)
	contentDao := pdao.YoubanPublishCollectContent
	content, err := contentDao.Ctx(ctx).
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		Where("dedupe_key", result.DedupeKey).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集内容池失败")
	}
	if content.IsEmpty() {
		return s.ensureCollectContent(ctx, event)
	}
	result.Id = content["id"].Int64()
	result.RawText = strings.TrimSpace(content["raw_text"].String())
	result.NormalizedText = strings.TrimSpace(content["normalized_text"].String())
	result.TextHash = strings.TrimSpace(content["text_hash"].String())
	result.DedupeKey = strings.TrimSpace(content["dedupe_key"].String())
	result.PreviousSeenAt = content["previous_seen_at"].GTime()
	s.enrichCollectContentMediaMetadata(ctx, result)
	return result, nil
}

func (s *sSysPublish) enrichCollectContentMediaMetadata(ctx context.Context, result *collectContentResult) {
	if result == nil || len(result.Media) == 0 {
		return
	}
	changed := false
	for index := range result.Media {
		item := &result.Media[index]
		if reusedPublishedCollectMetadata(*item) != nil {
			continue
		}
		mediaType := collectPublishMediaType(item.Type)
		if mediaType == "" || strings.TrimSpace(item.FilePhash) != "" {
			continue
		}
		if mediaType != "image" {
			continue
		}
		storagePath := normalizeStoredMediaPath(item.StoragePath)
		if storagePath == "" && strings.TrimSpace(item.FileUrl) == "" {
			continue
		}
		assets, err := processMediaAssetMetadata(ctx, mediaType, storagePath, item.FileUrl, item.PosterUrl, "", item.FileMd5)
		if err != nil || assets == nil || strings.TrimSpace(assets.PerceptualHash) == "" {
			if err != nil {
				g.Log().Warningf(ctx, "采集资料媒体 PHash 计算失败 mediaType:%s storagePath:%s err:%v", mediaType, storagePath, err)
			}
			continue
		}
		if strings.TrimSpace(item.FileMd5) == "" {
			item.FileMd5 = assets.MD5
		}
		item.FilePhash = strings.TrimSpace(assets.PerceptualHash)
		changed = true
	}
	if !changed {
		return
	}
	result.DedupeKey = collectHash(result.NormalizedText + ":" + collectMediaSignature(result.Media))
}

func (s *sSysPublish) collectContentFromEvent(ctx context.Context, event gdb.Record) (*collectContentResult, error) {
	rawText := strings.TrimSpace(event["raw_text"].String())
	normalizedText := normalizeCollectText(rawText)
	rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
	if err != nil {
		return nil, err
	}
	media := collectMediaRowsToItems(rows, event["material_role"].String())
	textHash := strings.TrimSpace(event["text_hash"].String())
	if textHash == "" {
		textHash = collectHash(normalizedText)
	}
	dedupeKey := strings.TrimSpace(event["dedupe_key"].String())
	if dedupeKey == "" {
		dedupeKey = collectHash(normalizedText + ":" + collectMediaSignature(media))
	}
	return &collectContentResult{
		RawText:        rawText,
		NormalizedText: normalizedText,
		MediaCount:     len(media),
		Media:          media,
		TextHash:       textHash,
		DedupeKey:      dedupeKey,
	}, nil
}

func collectContentMediaModels(items []collectMediaItem, tenantId, accountId, contentId int64) []*sysin.CollectContentMediaModel {
	list := make([]*sysin.CollectContentMediaModel, 0, len(items))
	for index, item := range items {
		mediaType := collectPublishMediaType(item.Type)
		sourceFileId := collectMediaSourceKey(item)
		if mediaType == "" || sourceFileId == "" {
			continue
		}
		list = append(list, &sysin.CollectContentMediaModel{
			TenantId: tenantId, AccountId: accountId, ContentId: contentId,
			MediaType: mediaType, SourceFileId: sourceFileId,
			FileMd5: item.FileMd5, FilePhash: item.FilePhash,
			SortIndex: index + 1, Status: "active",
		})
	}
	return list
}

func normalizeCollectText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func collectMediaSignature(items []collectMediaItem) string {
	keys := make([]string, 0, len(items))
	for _, item := range items {
		sourceKey := collectMediaFingerprint(item)
		if sourceKey == "" {
			continue
		}
		keys = append(keys, strings.TrimSpace(item.Purpose)+":"+strings.TrimSpace(item.Type)+":"+sourceKey)
	}
	sort.Strings(keys)
	return collectHash(strings.Join(keys, "|"))
}

func collectMediaFingerprint(item collectMediaItem) string {
	if key := collectTelegramMediaFingerprint(item); key != "" {
		return "tgfp:" + key
	}
	return collectMediaSourceKey(item)
}

func collectMediaPHash(item collectMediaItem) string {
	return strings.ToLower(strings.TrimSpace(item.FilePhash))
}

func collectTelegramMediaFingerprint(item collectMediaItem) string {
	if item.SourceMediaId == 0 {
		return ""
	}
	parts := []string{
		strings.TrimSpace(item.Type),
		strings.TrimSpace(item.SourceKind),
		strings.TrimSpace(item.SourceMimeType),
		strings.TrimSpace(item.SourceThumbSize),
	}
	return strings.Join(parts, ":") + ":" + collectHash(fmt.Sprintf("%d:%d", item.SourceMediaId, item.SourceAccessHash))
}

func collectMediaSourceKey(item collectMediaItem) string {
	if source := strings.TrimSpace(item.FileId); source != "" {
		return "tg:" + source
	}
	if source := strings.TrimSpace(item.StoragePath); source != "" {
		return "path:" + source
	}
	if source := strings.TrimSpace(item.FileUrl); source != "" {
		return "url:" + source
	}
	return ""
}

func collectPublishMediaType(sourceType string) string {
	switch strings.TrimSpace(sourceType) {
	case "photo":
		return "image"
	case "video":
		return "video"
	default:
		return ""
	}
}
