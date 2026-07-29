package sys

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) CollectMaterialDiagnose(ctx context.Context, in *sysin.CollectMaterialDiagnoseInp) (*sysin.CollectMaterialDiagnoseModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	limit := 30
	if in != nil && in.Limit > 0 {
		limit = in.Limit
	}
	if limit > 100 {
		limit = 100
	}
	mod := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id)
	if in != nil && in.SourceId > 0 {
		mod = mod.Where("source_id", in.SourceId)
	}
	rows, err := mod.OrderDesc("id").Limit(limit).All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集诊断事件失败")
	}
	result := &sysin.CollectMaterialDiagnoseModel{Items: make([]*sysin.CollectMaterialDiagnoseItem, 0, len(rows))}
	if len(rows) == 0 {
		return result, nil
	}

	byChat := make(map[string][]gdb.Record)
	for _, row := range rows {
		chatID := strings.TrimSpace(row["source_chat_id"].String())
		byChat[chatID] = append(byChat[chatID], row)
	}
	for _, chatRows := range byChat {
		sort.SliceStable(chatRows, func(i, j int) bool {
			left := chatRows[i]["source_message_id"].Int64()
			right := chatRows[j]["source_message_id"].Int64()
			if left == right {
				return chatRows[i]["id"].Int64() < chatRows[j]["id"].Int64()
			}
			return left < right
		})
		views := make([]collectMaterialMessageView, 0, len(chatRows))
		for _, row := range chatRows {
			items, mediaErr := s.collectDiagnoseEventMedia(ctx, row)
			if mediaErr != nil {
				return nil, mediaErr
			}
			views = append(views, collectMaterialMessageView{RawText: row["raw_text"].String(), Media: items})
		}
		pairByDisplay := make(map[int]collectMaterialPair)
		verifyIndexes := make(map[int]struct{})
		for _, pair := range pairCollectMaterialMessages(views) {
			pairByDisplay[pair.DisplayIndex] = pair
			verifyIndexes[pair.VerifyIndex] = struct{}{}
		}
		for index, row := range chatRows {
			items, mediaErr := s.collectDiagnoseEventMedia(ctx, row)
			if mediaErr != nil {
				return nil, mediaErr
			}
			classification := classifyProfileMessage(row["raw_text"].String(), items)
			item := &sysin.CollectMaterialDiagnoseItem{
				EventId:         row["id"].Int64(),
				SourceId:        row["source_id"].Int64(),
				SourceChatId:    row["source_chat_id"].String(),
				SourceMessageId: row["source_message_id"].Int64(),
				SourceGroupedId: row["source_grouped_id"].String(),
				Status:          row["status"].String(),
				MaterialRole:    row["material_role"].String(),
				Classification:  string(classification.Kind),
				MediaCount:      len(items),
				ErrorMessage:    row["error_message"].String(),
			}
			for _, media := range items {
				if classification.Kind == profileMessageKindVerify || strings.EqualFold(strings.TrimSpace(media.Purpose), collectMaterialRoleVerify) {
					item.VerifyMedia++
				} else {
					item.DisplayMedia++
				}
			}
			if pair, ok := pairByDisplay[index]; ok {
				verify := chatRows[pair.VerifyIndex]
				item.VerifyEventId = verify["id"].Int64()
				item.VerifyMessageId = verify["source_message_id"].Int64()
				item.VerifyMedia = len(views[pair.VerifyIndex].Media)
				result.PairedEvents++
			}
			if _, ok := verifyIndexes[index]; ok && item.VerifyEventId == 0 {
				result.UnmatchedVerify++
			}
			switch classification.Kind {
			case profileMessageKindDisplay:
				result.DisplayEvents++
				if item.VerifyEventId == 0 {
					result.MissingVerify++
				}
			case profileMessageKindVerify:
				result.VerifyEvents++
			}
			if row["status"].String() == sysin.CollectEventStatusMediaPending {
				result.MediaPendingEvents++
			}
			if row["status"].String() == sysin.CollectEventStatusWaitingOrder || row["status"].String() == sysin.CollectEventStatusGroupCollect {
				result.WaitingEvents++
			}
			if row["status"].String() == sysin.CollectEventStatusFailed {
				result.FailedEvents++
			}
			result.Items = append(result.Items, item)
		}
	}
	result.TotalEvents = len(result.Items)
	sort.SliceStable(result.Items, func(i, j int) bool { return result.Items[i].EventId > result.Items[j].EventId })
	reviewCount, err := s.fillCollectMaterialDiagnoseReviews(ctx, account.TenantId, account.Id, result.Items)
	if err != nil {
		return nil, err
	}
	result.ReviewEvents = reviewCount
	g.Log().Infof(ctx, "采集链路诊断完成 total:%d display:%d verify:%d paired:%d missingVerify:%d unmatchedVerify:%d review:%d mediaPending:%d waiting:%d failed:%d", result.TotalEvents, result.DisplayEvents, result.VerifyEvents, result.PairedEvents, result.MissingVerify, result.UnmatchedVerify, result.ReviewEvents, result.MediaPendingEvents, result.WaitingEvents, result.FailedEvents)
	return result, nil
}

func (s *sSysPublish) collectDiagnoseEventMedia(ctx context.Context, event gdb.Record) ([]collectMediaItem, error) {
	mediaJSON := strings.TrimSpace(event["media_json"].String())
	if mediaJSON != "" {
		var items []collectMediaItem
		if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
			return nil, gerror.Wrapf(err, "解析诊断事件媒体失败 eventId:%d", event["id"].Int64())
		}
		return items, nil
	}
	if event["media_count"].Int() <= 0 {
		return nil, nil
	}
	rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
	if err != nil {
		return nil, err
	}
	return collectMediaRowsToItems(rows, event["material_role"].String()), nil
}

func (s *sSysPublish) fillCollectMaterialDiagnoseReviews(ctx context.Context, tenantId, accountId int64, items []*sysin.CollectMaterialDiagnoseItem) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	eventIds := make([]int64, 0, len(items))
	byEvent := make(map[int64]*sysin.CollectMaterialDiagnoseItem, len(items))
	for _, item := range items {
		if item == nil || item.EventId <= 0 {
			continue
		}
		eventIds = append(eventIds, item.EventId)
		byEvent[item.EventId] = item
	}
	var reviews []struct {
		Id      int64  `json:"id"`
		EventId int64  `json:"eventId"`
		Status  string `json:"status"`
	}
	if err := pdao.YoubanPublishCollectReview.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("event_id", eventIds).
		Fields("id,event_id,status").
		Scan(&reviews); err != nil {
		return 0, gerror.Wrap(err, "读取采集诊断审核记录失败")
	}
	reviewCount := 0
	for _, review := range reviews {
		item := byEvent[review.EventId]
		if item == nil {
			continue
		}
		item.ReviewId = review.Id
		item.ReviewStatus = review.Status
		if review.Id > 0 {
			reviewCount++
		}
	}
	return reviewCount, nil
}
