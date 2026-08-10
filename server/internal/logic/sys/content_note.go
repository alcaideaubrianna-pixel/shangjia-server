package sys

import (
	"context"
	"fmt"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type sSysContentNote struct{}

func NewSysContentNote() *sSysContentNote {
	return &sSysContentNote{}
}

func init() {
	service.RegisterSysContentNote(NewSysContentNote())
}

// List 获取后台内容笔记列表。
func (s *sSysContentNote) List(ctx context.Context, in *sysin.ContentNoteListInp) (list []*sysin.ContentNoteListModel, totalCount int, err error) {
	mod := s.listModel(ctx)
	mod = s.applyListWhere(mod, in)

	totalCount, err = mod.Count()
	if err != nil {
		err = gerror.Wrap(err, "统计内容笔记失败")
		return
	}
	if totalCount == 0 {
		list = []*sysin.ContentNoteListModel{}
		return
	}

	if err = s.listFields(mod).
		Page(in.Page, in.PerPage).
		OrderDesc(noteAliasField("p", dao.ContentProfile.Columns().Id)).
		Scan(&list); err != nil {
		err = gerror.Wrap(err, "获取内容笔记列表失败，请稍后重试")
		return
	}
	if err = s.fillCollectionSources(ctx, list); err != nil {
		return nil, 0, err
	}
	err = s.fillListMedia(ctx, list)
	return
}

// View 获取后台内容笔记详情。
func (s *sSysContentNote) View(ctx context.Context, in *sysin.ContentNoteViewInp) (res *sysin.ContentNoteViewModel, err error) {
	profileColumns := dao.ContentProfile.Columns()
	var row *sysin.ContentNoteViewModel
	if err = s.detailFields(s.listModel(ctx)).
		Where(noteAliasField("p", profileColumns.Id)+"=?", in.Id).
		Scan(&row); err != nil {
		err = gerror.Wrap(err, "获取内容笔记详情失败，请稍后重试")
		return
	}
	if row == nil {
		err = gerror.New("内容笔记不存在")
		return
	}
	res = row
	res.Source, err = s.getSource(ctx, in.Id)
	if err != nil {
		return
	}
	if res.Source != nil {
		res.SourceChannelTitle = res.Source.ChannelTitle
		res.SourceChannelUsername = res.Source.ChannelUsername
		res.SourceUrl = res.Source.SourceUrl
		if res.SourceMessageId <= 0 {
			res.SourceMessageId = res.Source.SourceMessageId
		}
		if res.SourceChannelId <= 0 {
			res.SourceChannelId = res.Source.SourceChannelId
		}
	}
	res.Media, err = s.getMedia(ctx, in.Id)
	return
}

// Edit 修改后台内容笔记。
func (s *sSysContentNote) Edit(ctx context.Context, in *sysin.ContentNoteEditInp) (err error) {
	if err = in.Filter(ctx); err != nil {
		return
	}
	if _, err = dao.ContentProfile.Ctx(ctx).
		Fields(sysin.ContentNoteUpdateFields{}).
		WherePri(in.Id).
		Data(in).
		Update(); err != nil {
		err = gerror.Wrap(err, "修改内容笔记失败，请稍后重试")
		return
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return
}

// MediaEdit 修改后台内容笔记媒体。
func (s *sSysContentNote) MediaEdit(ctx context.Context, in *sysin.ContentNoteMediaEditInp) (err error) {
	if err = in.Filter(ctx); err != nil {
		return
	}

	mediaColumns := dao.ContentMedia.Columns()
	profileColumns := dao.ContentProfile.Columns()
	var media struct {
		ProfileId int64 `json:"profileId"`
	}
	if err = dao.ContentMedia.Ctx(ctx).
		Fields(mediaColumns.ProfileId).
		WherePri(in.Id).
		Scan(&media); err != nil {
		err = gerror.Wrap(err, "获取内容媒体失败")
		return
	}
	if media.ProfileId <= 0 {
		err = gerror.New("内容媒体不存在")
		return
	}

	if _, err = dao.ContentMedia.Ctx(ctx).
		Fields(sysin.ContentNoteMediaUpdateFields{}).
		WherePri(in.Id).
		Data(in).
		Update(); err != nil {
		err = gerror.Wrap(err, "修改内容媒体失败，请稍后重试")
		return
	}

	imageCount, err := dao.ContentMedia.Ctx(ctx).
		Where(mediaColumns.ProfileId, media.ProfileId).
		Where(mediaColumns.MediaType, "image").
		Where(mediaColumns.Status, 1).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计图片数量失败")
		return
	}
	videoCount, err := dao.ContentMedia.Ctx(ctx).
		Where(mediaColumns.ProfileId, media.ProfileId).
		Where(mediaColumns.MediaType, "video").
		Where(mediaColumns.Status, 1).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计视频数量失败")
		return
	}
	_, err = dao.ContentProfile.Ctx(ctx).
		WherePri(media.ProfileId).
		Data(map[string]interface{}{
			profileColumns.ImageCount: imageCount,
			profileColumns.VideoCount: videoCount,
		}).
		Update()
	if err != nil {
		err = gerror.Wrap(err, "更新笔记媒体数量失败")
		return
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return
}

func (s *sSysContentNote) BatchDelete(ctx context.Context, in *sysin.ContentNoteBatchDeleteInp) (err error) {
	if len(in.Ids) == 0 {
		return gerror.New("请选择要删除的笔记")
	}
	profileColumns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).
		WhereIn(profileColumns.Id, in.Ids).
		Data(map[string]interface{}{
			profileColumns.DeletedAt: gtime.Now(),
		}).
		Unscoped().
		Update(); err != nil {
		err = gerror.Wrap(err, "批量删除内容笔记失败")
		return
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return
}

func (s *sSysContentNote) BatchReview(ctx context.Context, in *sysin.ContentNoteBatchReviewInp) (err error) {
	if len(in.Ids) == 0 {
		return gerror.New("请选择要审核的笔记")
	}
	if in.ReviewStatus != consts.ContentReviewApproved && in.ReviewStatus != consts.ContentReviewRejected && in.ReviewStatus != consts.ContentReviewPending {
		return gerror.New("审核状态不合法")
	}
	profileColumns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).
		WhereIn(profileColumns.Id, in.Ids).
		Data(map[string]interface{}{
			profileColumns.ReviewStatus: in.ReviewStatus,
			profileColumns.UpdatedAt:    gtime.Now(),
		}).
		Update(); err != nil {
		err = gerror.Wrap(err, "批量审核内容笔记失败")
		return
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return
}

func (s *sSysContentNote) BatchStatus(ctx context.Context, in *sysin.ContentNoteBatchStatusInp) (err error) {
	if len(in.Ids) == 0 {
		return gerror.New("请选择要处理的笔记")
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("状态值不合法")
	}
	profileColumns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).
		WhereIn(profileColumns.Id, in.Ids).
		Data(map[string]interface{}{
			profileColumns.Status:    in.Status,
			profileColumns.UpdatedAt: gtime.Now(),
		}).
		Update(); err != nil {
		err = gerror.Wrap(err, "批量更新内容笔记状态失败")
		return
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return
}

func (s *sSysContentNote) BatchRemark(ctx context.Context, in *sysin.ContentNoteBatchRemarkInp) (err error) {
	if len(in.Ids) == 0 {
		return gerror.New("请选择要备注的笔记")
	}
	profileColumns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).
		WhereIn(profileColumns.Id, in.Ids).
		Data(map[string]interface{}{
			profileColumns.AdminRemark: in.AdminRemark,
			profileColumns.UpdatedAt:   gtime.Now(),
		}).
		Update(); err != nil {
		err = gerror.Wrap(err, "批量备注内容笔记失败")
	}
	return
}

func (s *sSysContentNote) listModel(ctx context.Context) *gdb.Model {
	profileColumns := dao.ContentProfile.Columns()
	channelColumns := dao.ContentChannel.Columns()
	sourceColumns := dao.ContentSourceMap.Columns()
	return dao.ContentProfile.Ctx(ctx).As("p").
		LeftJoin(dao.ContentChannel.Table()+" c", noteAliasField("c", channelColumns.Id)+"="+noteAliasField("p", profileColumns.ChannelId)).
		LeftJoin(dao.ContentSourceMap.Table()+" sm", noteAliasField("sm", sourceColumns.ProfileId)+"="+noteAliasField("p", profileColumns.Id))
}

func (s *sSysContentNote) listFields(mod *gdb.Model) *gdb.Model {
	profileColumns := dao.ContentProfile.Columns()
	channelColumns := dao.ContentChannel.Columns()
	sourceColumns := dao.ContentSourceMap.Columns()
	fields := []string{
		noteAliasFields("p",
			profileColumns.Id,
			profileColumns.ProfileNo,
			profileColumns.SourceType,
			profileColumns.SourceNoteId,
			profileColumns.SourceKey,
			profileColumns.SourceTextHash,
			profileColumns.ChannelId,
			profileColumns.Title,
			profileColumns.Summary,
			profileColumns.PlainText,
			profileColumns.HtmlText,
			profileColumns.SourceCategoryCode,
			profileColumns.Province,
			profileColumns.City,
			profileColumns.Age,
			profileColumns.Height,
			profileColumns.Weight,
			profileColumns.CupSize,
			profileColumns.DaysWithEscort,
			profileColumns.ExpectedLivingCost,
			profileColumns.CanFlyToProvince,
			profileColumns.CanGoAbroad,
			profileColumns.CanOvernight,
			profileColumns.CanCohabitate,
			profileColumns.HasHealthCheck,
			profileColumns.IsFullMonth,
			profileColumns.IsVirgin,
			profileColumns.AcceptSm,
			profileColumns.NoCondomAfterCheck,
			profileColumns.AllowCreampie,
			profileColumns.HasTattoo,
			profileColumns.IsFavorite,
			profileColumns.SourceEditedAt,
			profileColumns.GroupParams,
			profileColumns.TagParams,
			profileColumns.TextBlockCount,
			profileColumns.StoragePolicy,
			profileColumns.SourceRemark,
			profileColumns.SourceCreateBy,
			profileColumns.SourceUpdateBy,
			profileColumns.SourceCreatedAt,
			profileColumns.SourceUpdatedAt,
			profileColumns.ImageCount,
			profileColumns.VideoCount,
			profileColumns.HasVerificationVideo,
			profileColumns.MemberOnlyVideo,
			profileColumns.DuplicateOfId,
			profileColumns.Visibility,
			profileColumns.ReviewStatus,
			profileColumns.ImportStatus,
			profileColumns.AdminRemark,
			contentProfileHomeRecommend,
			contentProfileHomeSort,
			profileColumns.Status,
			profileColumns.PublishedAt,
			profileColumns.CreatedAt,
			profileColumns.UpdatedAt,
		),
		noteAliasField("c", channelColumns.SourceChannelId),
		noteAliasFieldAs("c", channelColumns.Title, "channel_title"),
		noteAliasFieldAs("c", channelColumns.Username, "channel_username"),
		noteAliasField("c", channelColumns.TgChatId),
		noteAliasField("sm", sourceColumns.SourceMessageId),
	}
	return mod.Fields(strings.Join(fields, ","))
}

func (s *sSysContentNote) detailFields(mod *gdb.Model) *gdb.Model {
	profileColumns := dao.ContentProfile.Columns()
	return s.listFields(mod).Fields(
		noteAliasField("p", profileColumns.PlainText),
	)
}

func (s *sSysContentNote) applyListWhere(mod *gdb.Model, in *sysin.ContentNoteListInp) *gdb.Model {
	profileColumns := dao.ContentProfile.Columns()
	channelColumns := dao.ContentChannel.Columns()

	if in.Id > 0 {
		mod = mod.Where(noteAliasField("p", profileColumns.Id)+"=?", in.Id)
	}
	if in.ProfileNo != "" {
		mod = mod.WhereLike(noteAliasField("p", profileColumns.ProfileNo), "%"+in.ProfileNo+"%")
	}
	if in.Keyword != "" {
		like := "%" + in.Keyword + "%"
		mod = mod.Where(
			"("+noteAliasField("p", profileColumns.ProfileNo)+" LIKE ? OR "+noteAliasField("p", profileColumns.Title)+" LIKE ? OR "+noteAliasField("p", profileColumns.Summary)+" LIKE ? OR "+noteAliasField("p", profileColumns.PlainText)+" LIKE ? OR "+noteAliasField("p", profileColumns.Province)+" LIKE ? OR "+noteAliasField("p", profileColumns.City)+" LIKE ? OR "+noteAliasField("p", profileColumns.CupSize)+" LIKE ?)",
			like,
			like,
			like,
			like,
			like,
			like,
			like,
		)
	}
	if in.SourceNoteId > 0 {
		mod = mod.Where(noteAliasField("p", profileColumns.SourceNoteId)+"=?", in.SourceNoteId)
	}
	if in.SourceChannelId > 0 {
		mod = mod.Where(noteAliasField("c", channelColumns.SourceChannelId)+"=?", in.SourceChannelId)
	}
	if in.ChannelKeyword != "" {
		like := "%" + in.ChannelKeyword + "%"
		mod = mod.Where(
			"("+noteAliasField("c", channelColumns.Title)+" LIKE ? OR "+noteAliasField("c", channelColumns.Username)+" LIKE ? OR "+noteAliasField("c", channelColumns.TgChatId)+" LIKE ?)",
			like,
			like,
			like,
		)
	}
	if in.Province != "" {
		mod = mod.Where(noteAliasField("p", profileColumns.Province)+"=?", in.Province)
	}
	if in.City != "" {
		mod = mod.Where(noteAliasField("p", profileColumns.City)+"=?", in.City)
	}
	if in.Visibility != "" {
		mod = mod.Where(noteAliasField("p", profileColumns.Visibility)+"=?", in.Visibility)
	}
	if in.ReviewStatus != "" {
		mod = mod.Where(noteAliasField("p", profileColumns.ReviewStatus)+"=?", in.ReviewStatus)
	}
	if in.ImportStatus != "" {
		mod = mod.Where(noteAliasField("p", profileColumns.ImportStatus)+"=?", in.ImportStatus)
	}
	if in.CupSize != "" {
		mod = mod.WhereLike(noteAliasField("p", profileColumns.CupSize), "%"+in.CupSize+"%")
	}
	if in.AgeMin > 0 {
		mod = mod.WhereGTE(noteAliasField("p", profileColumns.Age), in.AgeMin)
	}
	if in.AgeMax > 0 {
		mod = mod.WhereLTE(noteAliasField("p", profileColumns.Age), in.AgeMax)
	}
	if in.HeightMin > 0 {
		mod = mod.WhereGTE(noteAliasField("p", profileColumns.Height), in.HeightMin)
	}
	if in.HeightMax > 0 {
		mod = mod.WhereLTE(noteAliasField("p", profileColumns.Height), in.HeightMax)
	}
	if in.WeightMin > 0 {
		mod = mod.WhereGTE(noteAliasField("p", profileColumns.Weight), in.WeightMin)
	}
	if in.WeightMax > 0 {
		mod = mod.WhereLTE(noteAliasField("p", profileColumns.Weight), in.WeightMax)
	}
	if in.DaysMin > 0 {
		mod = mod.WhereGTE(noteAliasField("p", profileColumns.DaysWithEscort), in.DaysMin)
	}
	if in.DaysMax > 0 {
		mod = mod.WhereLTE(noteAliasField("p", profileColumns.DaysWithEscort), in.DaysMax)
	}
	if in.CostMin > 0 {
		mod = mod.WhereGTE(noteAliasField("p", profileColumns.ExpectedLivingCost), in.CostMin)
	}
	if in.CostMax > 0 {
		mod = mod.WhereLTE(noteAliasField("p", profileColumns.ExpectedLivingCost), in.CostMax)
	}
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.CanFlyToProvince), in.CanFly)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.CanGoAbroad), in.CanGoAbroad)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.CanOvernight), in.CanOvernight)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.CanCohabitate), in.CanCohabitate)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.HasHealthCheck), in.HasHealthCheck)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.IsFullMonth), in.IsFullMonth)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.IsVirgin), in.IsVirgin)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.AcceptSm), in.AcceptSm)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.NoCondomAfterCheck), in.NoCondom)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.AllowCreampie), in.AllowCreampie)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.HasTattoo), in.HasTattoo)
	mod = applyBoolIntWhere(mod, noteAliasField("p", profileColumns.IsFavorite), in.IsFavorite)
	mod = applyBoolIntWhere(mod, noteAliasField("p", contentProfileHomeRecommend), in.HomeRecommend)
	if in.Status > 0 {
		mod = mod.Where(noteAliasField("p", profileColumns.Status)+"=?", in.Status)
	}
	if in.HasVerification == 1 {
		mod = mod.Where(noteAliasField("p", profileColumns.HasVerificationVideo)+"=?", 1)
	}
	if in.HasVerification == 2 {
		mod = mod.Where(noteAliasField("p", profileColumns.HasVerificationVideo)+"=?", 0)
	}
	if in.MemberOnlyVideo == 1 {
		mod = mod.Where(noteAliasField("p", profileColumns.MemberOnlyVideo)+"=?", 1)
	}
	if in.MemberOnlyVideo == 2 {
		mod = mod.Where(noteAliasField("p", profileColumns.MemberOnlyVideo)+"=?", 0)
	}
	if in.HasVideo == 1 {
		mod = mod.WhereGT(noteAliasField("p", profileColumns.VideoCount), 0)
	}
	if in.HasVideo == 2 {
		mod = mod.Where(noteAliasField("p", profileColumns.VideoCount)+"=?", 0)
	}
	if in.HasDuplicate == 1 {
		mod = mod.WhereGT(noteAliasField("p", profileColumns.DuplicateOfId), 0)
	}
	if in.HasDuplicate == 2 {
		mod = mod.Where("(" + noteAliasField("p", profileColumns.DuplicateOfId) + " IS NULL OR " + noteAliasField("p", profileColumns.DuplicateOfId) + "=0)")
	}
	if len(in.CreatedAt) == 2 {
		mod = mod.WhereBetween(noteAliasField("p", profileColumns.CreatedAt), in.CreatedAt[0], in.CreatedAt[1])
	}
	return mod
}

func (s *sSysContentNote) getSource(ctx context.Context, profileId int64) (res *sysin.ContentNoteSourceModel, err error) {
	sourceColumns := dao.ContentSourceMap.Columns()
	if err = dao.ContentSourceMap.Ctx(ctx).
		Fields(
			sourceColumns.SourceType,
			sourceColumns.SourceKey,
			sourceColumns.SourceChannelId,
			sourceColumns.SourceMessageId,
			sourceColumns.SourceGroupedId,
			sourceColumns.SourceTextHash,
			sourceColumns.RawText,
			sourceColumns.RawMessageJson,
		).
		Where(sourceColumns.ProfileId, profileId).
		OrderAsc(sourceColumns.Id).
		Scan(&res); err != nil {
		err = gerror.Wrap(err, "获取内容来源失败")
	}
	rows, loadErr := s.loadCollectionSources(ctx, []int64{profileId})
	if loadErr != nil {
		return nil, loadErr
	}
	if source, ok := rows[profileId]; ok {
		if res == nil {
			return source, nil
		}
		if res.SourceType == "" {
			res.SourceType = source.SourceType
		}
		if res.SourceKey == "" {
			res.SourceKey = source.SourceKey
		}
		if res.SourceMessageId <= 0 {
			res.SourceMessageId = source.SourceMessageId
		}
		if res.SourceTextHash == "" {
			res.SourceTextHash = source.SourceTextHash
		}
		if res.RawText == "" {
			res.RawText = source.RawText
		}
		res.ChannelTitle = source.ChannelTitle
		res.ChannelUsername = source.ChannelUsername
		res.SourceUrl = source.SourceUrl
		return res, nil
	}
	return
}

const contentNoteSourceCacheTTL = 24 * time.Hour

type contentNoteCollectionSourceRow struct {
	ProfileId       int64  `orm:"profile_id"`
	SourceType      string `orm:"source_type"`
	SourceKey       string `orm:"source_key"`
	SourceChannelId int64  `orm:"source_channel_id"`
	SourceChatId    string `orm:"source_chat_id"`
	SourceMessageId int64  `orm:"source_message_id"`
	SourceGroupedId string `orm:"source_grouped_id"`
	SourceTextHash  string `orm:"source_text_hash"`
	RawText         string `orm:"raw_text"`
	RawMessageJson  string `orm:"raw_message_json"`
	SourceName      string `orm:"source_name"`
	SourceUsername  string `orm:"source_username"`
	BotId           int64  `orm:"bot_id"`
	TgAccountId     int64  `orm:"tg_account_id"`
	TenantId        int64  `orm:"tenant_id"`
}

type contentNoteCollectionChannelDisplay struct {
	Title    string `json:"title"`
	Username string `json:"username"`
}

func (s *sSysContentNote) fillCollectionSources(ctx context.Context, list []*sysin.ContentNoteListModel) error {
	ids := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil && item.Id > 0 {
			ids = append(ids, item.Id)
		}
	}
	rows, err := s.loadCollectionSources(ctx, ids)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		source, ok := rows[item.Id]
		if !ok {
			continue
		}
		item.SourceType = firstNonEmptyString(item.SourceType, source.SourceType)
		item.SourceKey = firstNonEmptyString(item.SourceKey, source.SourceKey)
		item.SourceChannelId = firstNonZeroInt64(item.SourceChannelId, source.SourceChannelId)
		item.SourceMessageId = firstNonZeroInt64(item.SourceMessageId, source.SourceMessageId)
		item.SourceChannelTitle = source.ChannelTitle
		item.SourceChannelUsername = source.ChannelUsername
		item.SourceUrl = source.SourceUrl
	}
	return nil
}

func (s *sSysContentNote) loadCollectionSources(ctx context.Context, profileIds []int64) (map[int64]*sysin.ContentNoteSourceModel, error) {
	result := make(map[int64]*sysin.ContentNoteSourceModel)
	if len(profileIds) == 0 {
		return result, nil
	}
	var rows []contentNoteCollectionSourceRow
	err := g.DB().Model("hg_youban_publish_collect_dispatch d").Safe().Ctx(ctx).
		InnerJoin("hg_content_profile p", "p.id=d.profile_id").
		InnerJoin("hg_youban_publish_collect_source s", "s.id=d.source_id AND s.deleted_at IS NULL").
		LeftJoin("hg_youban_publish_collect_event e", "e.id=d.event_id").
		WhereIn("d.profile_id", profileIds).
		Fields("d.profile_id,s.tenant_id,COALESCE(NULLIF(p.source_type,''),s.source_type) AS source_type," +
			"COALESCE(NULLIF(p.source_key,''),e.source_unique_key) AS source_key,s.source_username,s.bot_id,s.tg_account_id,s.title AS source_name," +
			"COALESCE(NULLIF(e.source_chat_id,''),s.source_chat_id) AS source_chat_id," +
			"COALESCE(NULLIF(e.source_message_id,0),0) AS source_message_id," +
			"COALESCE(NULLIF(e.source_grouped_id,''),'') AS source_grouped_id,e.raw_text,e.text_hash AS source_text_hash").
		OrderAsc("d.profile_id").OrderDesc("d.id").Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集来源失败")
	}
	selected := make(map[int64]contentNoteCollectionSourceRow, len(rows))
	for _, row := range rows {
		if _, ok := selected[row.ProfileId]; !ok {
			selected[row.ProfileId] = row
		}
	}
	botDisplays, tgDisplays, err := s.resolveCollectionDisplays(ctx, selected)
	if err != nil {
		return nil, err
	}
	for profileId, row := range selected {
		display := contentNoteCollectionChannelDisplay{}
		if row.BotId > 0 {
			display = botDisplays[contentNoteDisplayKey(row.TenantId, row.BotId, row.SourceChatId)]
		} else {
			display = tgDisplays[contentNoteDisplayKey(row.TenantId, row.TgAccountId, row.SourceChatId)]
		}
		if display.Title == "" {
			display.Title = strings.TrimSpace(row.SourceName)
		}
		if display.Username == "" {
			display.Username = strings.TrimSpace(row.SourceUsername)
		}
		result[profileId] = &sysin.ContentNoteSourceModel{
			SourceType: row.SourceType, SourceKey: row.SourceKey, SourceChannelId: contentNoteChatIdInt64(row.SourceChatId),
			SourceMessageId: row.SourceMessageId, SourceTextHash: row.SourceTextHash,
			RawText: row.RawText, RawMessageJson: row.RawMessageJson, ChannelTitle: display.Title,
			ChannelUsername: display.Username, SourceUrl: contentNoteTelegramMessageURL(row.SourceChatId, display.Username, row.SourceMessageId),
		}
	}
	return result, nil
}

func (s *sSysContentNote) resolveCollectionDisplays(ctx context.Context, rows map[int64]contentNoteCollectionSourceRow) (map[string]contentNoteCollectionChannelDisplay, map[string]contentNoteCollectionChannelDisplay, error) {
	botDisplays := make(map[string]contentNoteCollectionChannelDisplay)
	tgDisplays := make(map[string]contentNoteCollectionChannelDisplay)
	botIds, tgIds, botChats, tgChats, tenantIds := make([]int64, 0), make([]int64, 0), make([]string, 0), make([]string, 0), make([]int64, 0)
	seen := make(map[string]struct{})
	for _, row := range rows {
		if row.TenantId <= 0 {
			continue
		}
		if _, ok := seen[fmt.Sprintf("tenant:%d", row.TenantId)]; !ok {
			seen[fmt.Sprintf("tenant:%d", row.TenantId)] = struct{}{}
			tenantIds = append(tenantIds, row.TenantId)
		}
		accountId := row.TgAccountId
		if row.BotId > 0 {
			accountId = row.BotId
		}
		cacheKey := contentNoteDisplayCacheKey(row.TenantId, row.BotId > 0, accountId, row.SourceChatId)
		cacheHit := false
		if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
			var display contentNoteCollectionChannelDisplay
			if value.Scan(&display) == nil && (display.Title != "" || display.Username != "") {
				cacheHit = true
				if row.BotId > 0 {
					botDisplays[contentNoteDisplayKey(row.TenantId, row.BotId, row.SourceChatId)] = display
				} else {
					tgDisplays[contentNoteDisplayKey(row.TenantId, row.TgAccountId, row.SourceChatId)] = display
				}
			}
		}
		if cacheHit {
			continue
		}
		if row.BotId > 0 {
			botIds = append(botIds, row.BotId)
			botChats = append(botChats, contentNoteLookupChatIds(row.SourceChatId)...)
		} else {
			tgIds = append(tgIds, row.TgAccountId)
			tgChats = append(tgChats, contentNoteLookupChatIds(row.SourceChatId)...)
		}
	}
	botIds = uniqueInt64s(botIds)
	tgIds = uniqueInt64s(tgIds)
	tenantIds = uniqueInt64s(tenantIds)
	botChats = uniqueStringsForContentNote(botChats)
	tgChats = uniqueStringsForContentNote(tgChats)
	if len(botIds) > 0 && len(botChats) > 0 {
		var cacheRows []struct {
			TenantId, BotId         int64
			ChatId, Title, Username string
		}
		if err := g.DB().Model("hg_youban_publish_bot_channel_cache").Safe().Ctx(ctx).Fields("tenant_id,bot_id,chat_id,chat_title AS title,chat_username AS username").WhereIn("tenant_id", tenantIds).WhereIn("bot_id", botIds).WhereIn("chat_id", botChats).Scan(&cacheRows); err != nil {
			return nil, nil, gerror.Wrap(err, "读取Bot来源频道缓存失败")
		}
		for _, row := range cacheRows {
			display := contentNoteCollectionChannelDisplay{Title: strings.TrimSpace(row.Title), Username: strings.TrimSpace(row.Username)}
			for _, id := range contentNoteLookupChatIds(row.ChatId) {
				key := contentNoteDisplayKey(row.TenantId, row.BotId, id)
				botDisplays[key] = display
				_ = cache.Instance().Set(ctx, contentNoteDisplayCacheKey(row.TenantId, true, row.BotId, id), display, contentNoteSourceCacheTTL)
			}
		}
	}
	if len(tgIds) > 0 && len(tgChats) > 0 {
		var cacheRows []struct {
			TenantId, TgAccountId   int64
			ChatId, Title, Username string
		}
		if err := g.DB().Model("hg_youban_publish_tg_channel").Safe().Ctx(ctx).Fields("tenant_id,tg_account_id,channel_id AS chat_id,channel_title AS title,channel_username AS username").WhereIn("tenant_id", tenantIds).WhereIn("tg_account_id", tgIds).WhereIn("channel_id", tgChats).Scan(&cacheRows); err != nil {
			return nil, nil, gerror.Wrap(err, "读取TG来源频道缓存失败")
		}
		for _, row := range cacheRows {
			display := contentNoteCollectionChannelDisplay{Title: strings.TrimSpace(row.Title), Username: strings.TrimSpace(row.Username)}
			for _, id := range contentNoteLookupChatIds(row.ChatId) {
				key := contentNoteDisplayKey(row.TenantId, row.TgAccountId, id)
				tgDisplays[key] = display
				_ = cache.Instance().Set(ctx, contentNoteDisplayCacheKey(row.TenantId, false, row.TgAccountId, id), display, contentNoteSourceCacheTTL)
			}
		}
	}
	return botDisplays, tgDisplays, nil
}

func contentNoteDisplayKey(tenantId, accountId int64, chatId string) string {
	return fmt.Sprintf("%d:%d:%s", tenantId, accountId, normalizeContentNoteChatId(chatId))
}

func contentNoteDisplayCacheKey(tenantId int64, bot bool, accountId int64, chatId string) string {
	kind := "tg"
	if bot {
		kind = "bot"
	}
	return fmt.Sprintf("youban_publish:content_note_source:%s:%d:%d:%s", kind, tenantId, accountId, normalizeContentNoteChatId(chatId))
}

func contentNoteLookupChatIds(chatId string) []string {
	chatId = normalizeContentNoteChatId(chatId)
	if chatId == "" {
		return nil
	}
	ids := []string{chatId}
	if strings.HasPrefix(chatId, "-100") {
		ids = append(ids, strings.TrimPrefix(chatId, "-100"))
	} else if !strings.HasPrefix(chatId, "-") {
		ids = append(ids, "-100"+chatId)
	}
	return uniqueStringsForContentNote(ids)
}

func normalizeContentNoteChatId(chatId string) string { return strings.TrimSpace(chatId) }

func uniqueStringsForContentNote(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueInt64s(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func contentNoteTelegramMessageURL(chatId, username string, messageId int64) string {
	if messageId <= 0 {
		return ""
	}
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", username, messageId)
	}
	chatId = normalizeContentNoteChatId(chatId)
	if strings.HasPrefix(chatId, "-100") {
		return fmt.Sprintf("https://t.me/c/%s/%d", strings.TrimPrefix(chatId, "-100"), messageId)
	}
	return ""
}

func contentNoteChatIdInt64(chatId string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(chatId), 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func firstNonEmptyString(current, fallback string) string {
	if strings.TrimSpace(current) != "" {
		return current
	}
	return fallback
}

func firstNonZeroInt64(current, fallback int64) int64 {
	if current != 0 {
		return current
	}
	return fallback
}

func noteAliasField(alias string, column string) string {
	return alias + "." + column
}

func noteAliasFields(alias string, columns ...string) string {
	fields := make([]string, 0, len(columns))
	for _, column := range columns {
		fields = append(fields, noteAliasField(alias, column))
	}
	return strings.Join(fields, ",")
}

func noteAliasFieldAs(alias string, column string, as string) string {
	return noteAliasField(alias, column) + " AS " + as
}

func applyBoolIntWhere(mod *gdb.Model, field string, value int) *gdb.Model {
	switch value {
	case 1:
		return mod.Where(field+"=?", 1)
	case 2:
		return mod.Where(field+"=?", 0)
	default:
		return mod
	}
}

func (s *sSysContentNote) getMedia(ctx context.Context, profileId int64) (list []*sysin.ContentNoteMediaModel, err error) {
	mediaColumns := dao.ContentMedia.Columns()
	if err = dao.ContentMedia.Ctx(ctx).
		Fields(
			mediaColumns.Id,
			mediaColumns.ProfileId,
			mediaColumns.SourceAssetId,
			mediaColumns.DuplicateOfMediaId,
			mediaColumns.MediaType,
			mediaColumns.SortIndex,
			mediaColumns.DisplayStoragePath,
			mediaColumns.PreviewStoragePath,
			mediaColumns.BinaryMd5,
			mediaColumns.Width,
			mediaColumns.Height,
			mediaColumns.Duration,
			mediaColumns.ProcessStatus,
			mediaColumns.EncryptStatus,
			mediaColumns.Status,
		).
		Where(mediaColumns.ProfileId, profileId).
		OrderAsc(mediaColumns.SortIndex).
		OrderAsc(mediaColumns.Id).
		Scan(&list); err != nil {
		err = gerror.Wrap(err, "获取内容媒体失败")
	}
	if list == nil {
		list = []*sysin.ContentNoteMediaModel{}
	}
	return
}

func (s *sSysContentNote) fillListMedia(ctx context.Context, list []*sysin.ContentNoteListModel) (err error) {
	if len(list) == 0 {
		return
	}

	ids := make([]int64, 0, len(list))
	index := make(map[int64]*sysin.ContentNoteListModel, len(list))
	for _, item := range list {
		ids = append(ids, item.Id)
		index[item.Id] = item
	}

	mediaColumns := dao.ContentMedia.Columns()
	var mediaList []*sysin.ContentNoteMediaModel
	if err = dao.ContentMedia.Ctx(ctx).
		Fields(
			mediaColumns.Id,
			mediaColumns.ProfileId,
			mediaColumns.SourceAssetId,
			mediaColumns.DuplicateOfMediaId,
			mediaColumns.MediaType,
			mediaColumns.SortIndex,
			mediaColumns.DisplayStoragePath,
			mediaColumns.PreviewStoragePath,
			mediaColumns.BinaryMd5,
			mediaColumns.Width,
			mediaColumns.Height,
			mediaColumns.Duration,
			mediaColumns.ProcessStatus,
			mediaColumns.EncryptStatus,
			mediaColumns.Status,
		).
		WhereIn(mediaColumns.ProfileId, ids).
		Where(mediaColumns.Status, 1).
		OrderAsc(mediaColumns.SortIndex).
		OrderAsc(mediaColumns.Id).
		Scan(&mediaList); err != nil {
		err = gerror.Wrap(err, "获取内容笔记媒体失败")
		return
	}

	for _, media := range mediaList {
		if item, ok := index[media.ProfileId]; ok {
			item.Media = append(item.Media, media)
		}
	}
	return
}
