package sys

import (
	"context"
	"encoding/json"
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
	MediaJSON      string
	TextHash       string
	DedupeKey      string
	PreviousSeenAt *gtime.Time
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
	mediaDao := pdao.YoubanPublishCollectContentMedia
	mediaCols := mediaDao.Columns()
	if err = mediaDao.Ctx(ctx).
		Where(mediaCols.TenantId, account.TenantId).
		Where(mediaCols.AccountId, account.Id).
		Where(mediaCols.ContentId, in.Id).
		OrderAsc(mediaCols.SortIndex).
		OrderAsc(mediaCols.Id).
		Scan(&res.MediaList); err != nil {
		return nil, gerror.Wrap(err, "读取采集内容媒体失败")
	}
	if res.MediaList == nil {
		res.MediaList = []*sysin.CollectContentMediaModel{}
	}
	return res, nil
}

func (s *sSysPublish) ensureCollectContent(ctx context.Context, event gdb.Record) (*collectContentResult, error) {
	result := collectContentFromEvent(event)
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
			contentCols.MediaSignature: collectMediaSignature(result.MediaJSON),
			contentCols.MediaJson:      result.MediaJSON,
			contentCols.TextHash:       result.TextHash,
			contentCols.DedupeKey:      result.DedupeKey,
			contentCols.DuplicateTotal: 0,
			contentCols.Status:         "active",
			contentCols.FirstSeenAt:    seenAt,
			contentCols.LastSeenAt:     seenAt,
			contentCols.CreatedAt:      now,
			contentCols.UpdatedAt:      now,
		}).InsertAndGetId()
		if err != nil {
			return nil, gerror.Wrap(err, "创建采集内容池失败")
		}
		result.Id = contentId
		if err = s.syncCollectContentMedia(ctx, event, contentId, result.MediaJSON); err != nil {
			return nil, err
		}
		return result, nil
	}
	result.Id = content["id"].Int64()
	result.PreviousSeenAt = content["last_seen_at"].GTime()
	mediaJSON, mediaCount := mergeCollectMediaJSON(content["media_json"].String(), result.MediaJSON)
	if mediaCount > result.MediaCount {
		result.MediaCount = mediaCount
		result.MediaJSON = mediaJSON
	}
	_, err = contentDao.Ctx(ctx).
		Where(contentCols.Id, result.Id).
		Data(g.Map{
			contentCols.LastEventId:    event["id"].Int64(),
			contentCols.RawText:        result.RawText,
			contentCols.NormalizedText: result.NormalizedText,
			contentCols.MediaCount:     result.MediaCount,
			contentCols.MediaSignature: collectMediaSignature(result.MediaJSON),
			contentCols.MediaJson:      result.MediaJSON,
			contentCols.TextHash:       result.TextHash,
			contentCols.PreviousSeenAt: result.PreviousSeenAt,
			contentCols.LastSeenAt:     seenAt,
			contentCols.DuplicateTotal: gdb.Raw(contentCols.DuplicateTotal + "+1"),
			contentCols.UpdatedAt:      now,
		}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "更新采集内容池失败")
	}
	if err = s.syncCollectContentMedia(ctx, event, result.Id, result.MediaJSON); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *sSysPublish) collectContentSnapshot(ctx context.Context, event gdb.Record) (*collectContentResult, error) {
	result := collectContentFromEvent(event)
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
	result.MediaCount = content["media_count"].Int()
	result.MediaJSON = content["media_json"].String()
	result.TextHash = strings.TrimSpace(content["text_hash"].String())
	result.DedupeKey = strings.TrimSpace(content["dedupe_key"].String())
	result.PreviousSeenAt = content["previous_seen_at"].GTime()
	return result, nil
}

func collectContentFromEvent(event gdb.Record) *collectContentResult {
	rawText := strings.TrimSpace(event["raw_text"].String())
	normalizedText := normalizeCollectText(rawText)
	mediaJSON := event["media_json"].String()
	mediaCount := event["media_count"].Int()
	if mediaCount <= 0 {
		_, mediaCount = mergeCollectMediaJSON("", mediaJSON)
	}
	textHash := strings.TrimSpace(event["text_hash"].String())
	if textHash == "" {
		textHash = collectHash(normalizedText)
	}
	dedupeKey := strings.TrimSpace(event["dedupe_key"].String())
	if dedupeKey == "" {
		dedupeKey = collectHash(normalizedText + ":" + collectMediaSignature(mediaJSON))
	}
	return &collectContentResult{
		RawText:        rawText,
		NormalizedText: normalizedText,
		MediaCount:     mediaCount,
		MediaJSON:      mediaJSON,
		TextHash:       textHash,
		DedupeKey:      dedupeKey,
	}
}

func (s *sSysPublish) syncCollectContentMedia(ctx context.Context, event gdb.Record, contentId int64, mediaJSON string) error {
	if contentId <= 0 {
		return nil
	}
	var items []collectMediaItem
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		return nil
	}
	now := gtime.Now()
	mediaDao := pdao.YoubanPublishCollectContentMedia
	mediaCols := mediaDao.Columns()
	sortIndex := 1
	for _, item := range items {
		sourceKey := collectMediaSourceKey(item)
		if sourceKey == "" {
			continue
		}
		mediaType := collectPublishMediaType(item.Type)
		if mediaType == "" {
			continue
		}
		existing, err := mediaDao.Ctx(ctx).
			Where(mediaCols.ContentId, contentId).
			Where(mediaCols.SourceFileId, sourceKey).
			Fields(mediaCols.Id).
			Value()
		if err != nil {
			return gerror.Wrap(err, "读取采集内容媒体失败")
		}
		if existing.Int64() > 0 {
			sortIndex++
			continue
		}
		_, err = mediaDao.Ctx(ctx).Data(g.Map{
			mediaCols.TenantId:        event["tenant_id"].Int64(),
			mediaCols.AccountId:       event["account_id"].Int64(),
			mediaCols.ContentId:       contentId,
			mediaCols.MediaType:       mediaType,
			mediaCols.SourceFileId:    sourceKey,
			mediaCols.SourceUniqueKey: event["source_unique_key"].String(),
			mediaCols.SortIndex:       sortIndex,
			mediaCols.Status:          "active",
			mediaCols.CreatedAt:       now,
			mediaCols.UpdatedAt:       now,
		}).Insert()
		if err != nil {
			return gerror.Wrap(err, "创建采集内容媒体失败")
		}
		sortIndex++
	}
	return nil
}

func normalizeCollectText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func collectMediaSignature(mediaJSON string) string {
	var items []collectMediaItem
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		return ""
	}
	keys := make([]string, 0, len(items))
	for _, item := range items {
		sourceKey := collectMediaFingerprint(item)
		if sourceKey == "" {
			continue
		}
		keys = append(keys, strings.TrimSpace(item.Type)+":"+sourceKey)
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

func collectTelegramMediaFingerprint(item collectMediaItem) string {
	metaRaw := strings.TrimSpace(item.MetaJson)
	if metaRaw == "" {
		return ""
	}
	var meta struct {
		AccessHash int64  `json:"accessHash"`
		Id         int64  `json:"id"`
		Kind       string `json:"kind"`
		MimeType   string `json:"mimeType"`
		ThumbSize  string `json:"thumbSize"`
	}
	if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
		return ""
	}
	if meta.Id == 0 {
		return ""
	}
	parts := []string{
		strings.TrimSpace(item.Type),
		strings.TrimSpace(meta.Kind),
		strings.TrimSpace(meta.MimeType),
		strings.TrimSpace(meta.ThumbSize),
	}
	return strings.Join(parts, ":") + ":" + collectHash(fmt.Sprintf("%d:%d", meta.Id, meta.AccessHash))
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
