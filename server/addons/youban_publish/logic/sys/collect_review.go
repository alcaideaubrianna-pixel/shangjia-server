package sys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/hgrds/lock"
)

type collectReviewCursor struct {
	Id int64 `json:"id"`
}

func (s *sSysPublish) CollectReviewList(ctx context.Context, in *sysin.CollectReviewListInp) (res *sysin.CollectReviewPageModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		in = &sysin.CollectReviewListInp{}
	}
	mod := pdao.YoubanPublishCollectReview.DB().Model(pdao.YoubanPublishCollectReview.Table()+" r").Safe().Ctx(ctx).
		LeftJoin(pdao.YoubanPublishCollectSource.Table()+" s", "s.id=r.source_id").
		LeftJoin(pdao.YoubanPublishCollectRule.Table()+" rule", "rule.id=r.rule_id").
		LeftJoin(publishTgAccountTable+" ta", "ta.id=s.tg_account_id AND ta.deleted_at IS NULL").
		LeftJoin(publishBotTable+" b", "b.id=s.bot_id AND b.deleted_at IS NULL").
		LeftJoin(publishAccountTable+" fa", "fa.id=s.follow_account_id AND fa.deleted_at IS NULL").
		Where("r.tenant_id", account.TenantId).
		Where("r.account_id", account.Id)
	if in.Status != "" {
		mod = mod.Where("r.status", strings.TrimSpace(in.Status))
	}
	if in.SourceId > 0 {
		mod = mod.Where("r.source_id", in.SourceId)
	}
	if in.RuleId > 0 {
		mod = mod.Where("r.rule_id", in.RuleId)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		mod = mod.WhereLike("r.raw_text", "%"+keyword+"%")
	}
	if cursor, cursorErr := decodeCollectReviewCursor(in.Cursor); cursorErr != nil {
		return nil, cursorErr
	} else if cursor != nil {
		mod = mod.Where("r.id < ?", cursor.Id)
	}
	perPage := in.PerPage
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	fields := "r.*,s.title AS source_title,s.source_type,s.source_username,rule.name AS rule_name," +
		"CASE s.source_type WHEN 'account' THEN COALESCE(NULLIF(ta.display_name,''),NULLIF(ta.telegram_username,''),NULLIF(s.source_username,''),s.source_chat_id) WHEN 'bot' THEN COALESCE(NULLIF(b.bot_name,''),NULLIF(b.bot_username,''),NULLIF(s.source_username,''),s.source_chat_id) WHEN 'follow' THEN COALESCE(NULLIF(fa.nickname,''),NULLIF(fa.username,''),NULLIF(s.source_username,''),s.source_chat_id) ELSE COALESCE(NULLIF(s.title,''),s.source_chat_id) END AS source_display_name"
	list := make([]*sysin.CollectReviewModel, 0, perPage+1)
	if err = mod.Fields(fields).OrderDesc("r.id").Limit(perPage + 1).Scan(&list); err != nil {
		return nil, gerror.Wrap(err, "获取采集审核失败")
	}
	hasMore := len(list) > perPage
	if hasMore {
		list = list[:perPage]
	}
	if err = s.fillCollectReviewTargetChannelNames(ctx, list, account.TenantId); err != nil {
		return nil, err
	}
	fillCollectReviewMedia(list)
	nextCursor := ""
	if hasMore && len(list) > 0 {
		nextCursor = encodeCollectReviewCursor(list[len(list)-1].Id)
	}
	return &sysin.CollectReviewPageModel{List: list, HasMore: hasMore, NextCursor: nextCursor}, nil
}

func encodeCollectReviewCursor(id int64) string {
	if id <= 0 {
		return ""
	}
	payload, err := json.Marshal(collectReviewCursor{Id: id})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCollectReviewCursor(raw string) (*collectReviewCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, gerror.New("采集审核游标不合法")
	}
	var cursor collectReviewCursor
	if err = json.Unmarshal(payload, &cursor); err != nil || cursor.Id <= 0 {
		return nil, gerror.New("采集审核游标不合法")
	}
	return &cursor, nil
}

func fillCollectReviewMedia(list []*sysin.CollectReviewModel) {
	for _, review := range list {
		if review == nil {
			continue
		}
		items := collectMediaRowsToItemsFromJSON(review.MediaJson)
		review.Media = make([]*sysin.CollectReviewMediaModel, 0, len(items))
		for _, item := range items {
			review.Media = append(review.Media, &sysin.CollectReviewMediaModel{
				Type: item.Type, Purpose: item.Purpose, FileId: item.FileId,
				FileUrl: item.FileUrl, StoragePath: item.StoragePath,
				PosterUrl: item.PosterUrl, MetaJson: item.MetaJson,
			})
		}
		review.MediaCount = len(items)
	}
}

func (s *sSysPublish) fillCollectReviewTargetChannelNames(ctx context.Context, list []*sysin.CollectReviewModel, tenantId int64) error {
	ids := make([]int64, 0)
	for _, review := range list {
		if review == nil {
			continue
		}
		var channelIds []int64
		if err := json.Unmarshal([]byte(review.TargetChannelIdJson), &channelIds); err != nil {
			continue
		}
		ids = append(ids, channelIds...)
	}
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return nil
	}
	var rows []struct {
		Id       int64  `json:"id"`
		Title    string `json:"title"`
		Username string `json:"username"`
		Target   string `json:"target"`
	}
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,channel_title AS title,channel_username AS username,target_chat_id AS target").
		Where("tenant_id", tenantId).WhereIn("id", ids).WhereNull("deleted_at").Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取审核目标频道失败")
	}
	names := make(map[int64]string, len(rows))
	for _, row := range rows {
		names[row.Id] = firstNonEmpty(row.Title, row.Username, row.Target, fmt.Sprintf("频道 %d", row.Id))
	}
	for _, review := range list {
		if review == nil {
			continue
		}
		var channelIds []int64
		_ = json.Unmarshal([]byte(review.TargetChannelIdJson), &channelIds)
		for _, id := range channelIds {
			if name := strings.TrimSpace(names[id]); name != "" {
				review.TargetChannelNames = append(review.TargetChannelNames, name)
			}
		}
	}
	return nil
}

func (s *sSysPublish) CollectReviewEdit(ctx context.Context, in *sysin.CollectReviewEditInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("审核参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	result, err := pdao.YoubanPublishCollectReview.Ctx(ctx).
		Where("id", in.Id).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		Where("status", sysin.CollectReviewStatusPending).
		Update(g.Map{"raw_text": in.RawText, "updated_at": gtime.Now()})
	if err != nil {
		return gerror.Wrap(err, "更新审核文案失败")
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "读取审核更新结果失败")
	}
	if rowsAffected == 0 {
		return gerror.New("审核不存在、已处理或无权编辑")
	}
	return nil
}

func (s *sSysPublish) CollectReviewAction(ctx context.Context, in *sysin.CollectReviewActionInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("审核参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	ids := uniqueIds(in.Ids)
	if in.Status == sysin.CollectReviewStatusApproved {
		return s.approveCollectReviews(ctx, ids, account.TenantId, account.Id, in.Reason)
	}
	if in.Status == sysin.CollectReviewStatusRejected {
		now := gtime.Now()
		if _, err = pdao.YoubanPublishCollectReview.Ctx(ctx).
			WhereIn("id", ids).
			Where("tenant_id", account.TenantId).
			Where("account_id", account.Id).
			Where("status", sysin.CollectReviewStatusPending).
			Data(g.Map{
				"status": in.Status, "review_reason": in.Reason,
				"reviewed_by": account.Id, "reviewed_at": now, "updated_at": now,
			}).Update(); err != nil {
			return gerror.Wrap(err, "更新采集审核失败")
		}
		if err = s.rejectCollectReviews(ctx, ids, account.TenantId, account.Id, in.Reason); err != nil {
			return err
		}
		return nil
	}
	return gerror.New("不支持的审核状态")
}

func (s *sSysPublish) approveCollectReviews(ctx context.Context, reviewIds []int64, tenantId int64, accountId int64, reason string) error {
	if len(reviewIds) == 0 {
		return nil
	}
	startedAt := time.Now()
	const concurrency = 4
	semaphore := make(chan struct{}, concurrency)
	results := make([]error, len(reviewIds))
	var waitGroup sync.WaitGroup

	g.Log().Infof(ctx, "开始批量通过采集审核 count:%d concurrency:%d tenant_id:%d account_id:%d", len(reviewIds), concurrency, tenantId, accountId)
	for index, reviewId := range reviewIds {
		waitGroup.Add(1)
		go func(index int, reviewId int64) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			itemStartedAt := time.Now()
			results[index] = s.approveCollectReview(ctx, reviewId, tenantId, accountId, reason)
			if results[index] != nil {
				g.Log().Errorf(ctx, "批量通过采集审核失败 review_id:%d duration_ms:%d err:%+v", reviewId, time.Since(itemStartedAt).Milliseconds(), results[index])
				return
			}
			g.Log().Debugf(ctx, "批量通过采集审核完成 review_id:%d duration_ms:%d", reviewId, time.Since(itemStartedAt).Milliseconds())
		}(index, reviewId)
	}
	waitGroup.Wait()

	failedIds := make([]int64, 0)
	failedErrors := make([]error, 0)
	failureMessages := make([]string, 0)
	for index, itemErr := range results {
		if itemErr == nil {
			continue
		}
		failedIds = append(failedIds, reviewIds[index])
		failedErrors = append(failedErrors, itemErr)
		failureMessages = append(failureMessages, fmt.Sprintf("review_id:%d %s", reviewIds[index], itemErr.Error()))
	}
	duration := time.Since(startedAt).Milliseconds()
	if len(failedErrors) > 0 {
		g.Log().Errorf(ctx, "批量通过采集审核完成但有失败 count:%d success:%d failed:%d duration_ms:%d failed_ids:%v", len(reviewIds), len(reviewIds)-len(failedIds), len(failedIds), duration, failedIds)
		return gerror.Wrap(errors.Join(failedErrors...), fmt.Sprintf("批量通过采集审核失败 %d 条：%s", len(failedIds), strings.Join(failureMessages, "；")))
	}
	g.Log().Infof(ctx, "批量通过采集审核完成 count:%d duration_ms:%d", len(reviewIds), duration)
	return nil
}

func (s *sSysPublish) CollectReviewDelete(ctx context.Context, in *sysin.IdsInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择审核资料")
	}
	ids := uniqueIds(in.Ids)
	var dispatchIds []int64
	if err = pdao.YoubanPublishCollectReview.Ctx(ctx).
		Fields("dispatch_id").
		WhereIn("id", ids).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		Where("status", sysin.CollectReviewStatusPending).
		WhereGT("dispatch_id", 0).
		Scan(&dispatchIds); err != nil {
		return gerror.Wrap(err, "读取待删除审核分发失败")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if len(dispatchIds) > 0 {
			if _, err = tx.Model(pdao.YoubanPublishCollectDispatch.Table()).
				WhereIn("id", uniqueIds(dispatchIds)).
				Where("tenant_id", account.TenantId).
				Where("account_id", account.Id).
				Where("status", sysin.CollectDispatchStatusReviewing).
				Data(g.Map{
					"status":        sysin.CollectDispatchStatusSkipped,
					"error_message": "审核资料已删除",
					"finished_at":   gtime.Now(),
					"updated_at":    gtime.Now(),
				}).Update(); err != nil {
				return gerror.Wrap(err, "更新审核分发状态失败")
			}
		}
		if _, err = tx.Model(pdao.YoubanPublishCollectReview.Table()).
			WhereIn("id", ids).
			Where("tenant_id", account.TenantId).
			Where("account_id", account.Id).
			Delete(); err != nil {
			return gerror.Wrap(err, "删除采集审核失败")
		}
		return nil
	})
}

func (s *sSysPublish) approveCollectReview(ctx context.Context, reviewId int64, tenantId int64, accountId int64, reason string) error {
	reviewLock := lock.NewConfig(5*time.Minute, 50*time.Millisecond).Mutex(fmt.Sprintf("youban_publish:collect:review:%d", reviewId))
	if err := reviewLock.Lock(ctx); err != nil {
		return gerror.Wrap(err, "获取采集审核处理锁失败")
	}
	defer func() { _ = reviewLock.Unlock(context.Background()) }()

	review, err := pdao.YoubanPublishCollectReview.Ctx(ctx).
		Where("id", reviewId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取采集审核失败")
	}
	if review.IsEmpty() {
		return nil
	}
	reviewStatus := review["status"].String()
	if reviewStatus != sysin.CollectReviewStatusPending && reviewStatus != sysin.CollectReviewStatusApproved {
		return nil
	}
	dispatch, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("id", review["dispatch_id"].Int64()).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取采集审核分发失败")
	}
	if dispatch.IsEmpty() {
		return gerror.New("采集审核分发不存在")
	}
	if dispatch["status"].String() == sysin.CollectDispatchStatusSent || dispatch["status"].String() == sysin.CollectDispatchStatusSkipped {
		return s.markCollectReviewApproved(ctx, reviewId, tenantId, accountId, reason)
	}
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", review["event_id"].Int64()).One()
	if err != nil {
		return gerror.Wrap(err, "读取采集事件失败")
	}
	rule, err := pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("id", review["rule_id"].Int64()).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("status", 1).
		WhereNull("deleted_at").One()
	if err != nil {
		return gerror.Wrap(err, "读取采集规则失败")
	}
	if event.IsEmpty() || rule.IsEmpty() {
		return gerror.New("采集审核关联数据不存在或规则已停用")
	}
	text := strings.TrimSpace(review["raw_text"].String())
	content := &collectContentResult{
		RawText: text, NormalizedText: normalizeCollectText(text),
		MediaCount: review["media_count"].Int(), MediaJSON: review["media_json"].String(),
	}
	content.TextHash = collectHash(content.NormalizedText)
	content.DedupeKey = collectHash(content.NormalizedText + ":" + collectMediaSignature(content.MediaJSON))
	prepared, err := s.prepareCollectMaterialSnapshot(ctx, event, content)
	if err != nil {
		_ = s.markCollectDispatchFailed(ctx, review["dispatch_id"].Int64(), err.Error())
		return gerror.Wrap(err, "准备审核通过资料失败")
	}
	profileId := dispatch["profile_id"].Int64()
	if profileId <= 0 {
		profileId, err = s.commitCollectPreparedProfile(ctx, event, prepared, rule, text)
		if err != nil {
			_ = s.markCollectDispatchFailed(ctx, review["dispatch_id"].Int64(), err.Error())
			return err
		}
	}
	if err = s.submitCollectProfileDispatch(ctx, review["dispatch_id"].Int64(), profileId, event, rule); err != nil {
		return err
	}
	return s.markCollectReviewApproved(ctx, reviewId, tenantId, accountId, reason)
}

func (s *sSysPublish) markCollectReviewApproved(ctx context.Context, reviewId int64, tenantId int64, accountId int64, reason string) error {
	now := gtime.Now()
	_, err := pdao.YoubanPublishCollectReview.Ctx(ctx).
		Where("id", reviewId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("status", []string{sysin.CollectReviewStatusPending, sysin.CollectReviewStatusApproved}).
		Data(g.Map{
			"status": sysin.CollectReviewStatusApproved, "review_reason": reason,
			"reviewed_by": accountId, "reviewed_at": now, "updated_at": now,
		}).Update()
	return gerror.Wrap(err, "完成采集审核状态失败")
}

func (s *sSysPublish) rejectCollectReviews(ctx context.Context, reviewIds []int64, tenantId int64, accountId int64, reason string) error {
	if len(reviewIds) == 0 {
		return nil
	}
	var rows []struct {
		DispatchId int64 `json:"dispatchId"`
	}
	if err := pdao.YoubanPublishCollectReview.Ctx(ctx).
		Fields("dispatch_id").
		WhereIn("id", reviewIds).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("status", sysin.CollectReviewStatusRejected).
		WhereGT("dispatch_id", 0).
		Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取采集审核分发失败")
	}
	dispatchIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		dispatchIds = append(dispatchIds, row.DispatchId)
	}
	if len(dispatchIds) == 0 {
		return nil
	}
	message := strings.TrimSpace(reason)
	if message == "" {
		message = "审核拒绝"
	}
	_, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		WhereIn("id", uniqueIds(dispatchIds)).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusSkipped,
			"error_message": message,
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	return gerror.Wrap(err, "更新采集审核拒绝分发状态失败")
}
