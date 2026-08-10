package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

const adminNoteCountCacheTTL = 5 * time.Second

func (s *sSysPublish) profileList(ctx context.Context, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	if err = s.ensureLegacyProfileNosOnce(ctx); err != nil {
		return nil, 0, err
	}
	base, err := s.profileBaseModel(ctx, in.TenantId, in.AccountId)
	if err != nil {
		return nil, 0, err
	}
	return s.profileListByModel(ctx, base, in)
}

func (s *sSysPublish) profileListByAccountIds(ctx context.Context, in *sysin.ProfileListInp, tenantId int64, accountIds []int64) (list []*sysin.ProfileModel, totalCount int, err error) {
	if err = s.ensureLegacyProfileNosOnce(ctx); err != nil {
		return nil, 0, err
	}
	accountIds = uniqueIds(accountIds)
	if len(accountIds) == 0 {
		return []*sysin.ProfileModel{}, 0, nil
	}
	base, err := s.profileBaseModel(ctx, tenantId, 0)
	if err != nil {
		return nil, 0, err
	}
	base = base.WhereIn("ps.account_id", accountIds)
	return s.profileListByModel(ctx, base, in)
}

func (s *sSysPublish) profileListByModel(ctx context.Context, base *gdb.Model, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	list, totalCount, err = s.searchProfilePage(ctx, base, in, profileListFields(), "统计资料失败", "获取资料列表失败")
	if err != nil {
		return nil, 0, err
	}
	if err = s.ensureProfileListUUID(ctx, list); err != nil {
		return nil, 0, err
	}
	if err = s.applyProfileOwnerNames(ctx, list); err != nil {
		return nil, 0, err
	}
	if err = s.applyProfileTagNames(ctx, list); err != nil {
		return nil, 0, err
	}
	if err = s.applyProfileCollectionMetadata(ctx, list); err != nil {
		return nil, 0, err
	}
	return
}

func (s *sSysPublish) profileView(ctx context.Context, profileId int64, tenantId int64, accountId int64) (res *sysin.ProfileModel, err error) {
	if profileId <= 0 {
		return nil, gerror.New("资料ID不能为空")
	}
	base, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if err = base.Where("p.id", profileId).Fields(profileListFields()).Scan(&res); err != nil {
		return nil, gerror.Wrap(err, "获取资料详情失败")
	}
	if res == nil || res.Id <= 0 {
		return nil, gerror.New("资料不存在或无权操作")
	}
	if err = s.ensureProfileModelUUID(ctx, res); err != nil {
		return nil, err
	}
	if err = s.applyProfileTagNames(ctx, []*sysin.ProfileModel{res}); err != nil {
		return nil, err
	}
	if err = s.applyProfileCollectionMetadata(ctx, []*sysin.ProfileModel{res}); err != nil {
		return nil, err
	}
	if res.ChannelIds, err = s.profileChannelIdsOrDefaults(ctx, res.TenantId, res.AccountId, res.Id); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *sSysPublish) profileViewBySelector(ctx context.Context, in *sysin.ProfileViewInp, tenantId int64, accountId int64) (res *sysin.ProfileModel, err error) {
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return nil, gerror.New("资料UUID不能为空")
	}
	base, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if in.Id > 0 {
		base = base.Where("p.id", in.Id)
	} else {
		base = base.Where("p.source_note_uuid", normalizeProfileUUID(in.Uuid))
	}
	if err = base.Fields(profileListFields()).Scan(&res); err != nil {
		return nil, gerror.Wrap(err, "获取资料详情失败")
	}
	if res == nil || res.Id <= 0 {
		return nil, gerror.New("资料不存在或无权操作")
	}
	if err = s.ensureProfileModelUUID(ctx, res); err != nil {
		return nil, err
	}
	if err = s.applyProfileTagNames(ctx, []*sysin.ProfileModel{res}); err != nil {
		return nil, err
	}
	if res.ChannelIds, err = s.profileChannelIdsOrDefaults(ctx, res.TenantId, res.AccountId, res.Id); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *sSysPublish) noteList(ctx context.Context, in *sysin.NoteListInp) (list []*sysin.NoteModel, totalCount int, err error) {
	profiles, totalCount, err := s.profileList(ctx, &in.ProfileListInp)
	if err != nil {
		return nil, 0, err
	}
	list = make([]*sysin.NoteModel, 0, len(profiles))
	for _, item := range profiles {
		note := &sysin.NoteModel{ProfileModel: *item}
		note.Media, err = s.mediaListByProfile(ctx, item.Id, item.TenantId, item.AccountId)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, note)
	}
	return
}

func (s *sSysPublish) adminNoteList(ctx context.Context, in *sysin.NoteListInp, tenantId int64, tenantIds []int64, accountIds []int64, viewer *sysin.AccountModel) (*sysin.AdminNotePageModel, error) {
	startedAt := time.Now()
	profiles, hasMore, nextCursor, err := s.adminNoteIndexList(ctx, in, tenantId, tenantIds, accountIds)
	if err != nil {
		return nil, err
	}
	logSlowAdminNoteListStage(ctx, "profile_query", startedAt, len(profiles), boolToInt(hasMore))
	if err = s.ensureProfileListUUID(ctx, profiles); err != nil {
		return nil, err
	}
	if err = s.applyProfileTagNames(ctx, profiles); err != nil {
		return nil, err
	}
	if err = s.applyProfileCollectionMetadata(ctx, profiles); err != nil {
		return nil, err
	}
	stageStartedAt := time.Now()
	mediaBuckets, err := s.adminNoteCoverMediaByProfiles(ctx, profiles)
	if err != nil {
		return nil, err
	}
	logSlowAdminNoteListStage(ctx, "cover_media", stageStartedAt, len(profiles), len(mediaBuckets))
	list := make([]*sysin.AdminNoteListModel, 0, len(profiles))
	for _, item := range profiles {
		if item == nil {
			continue
		}
		permission := sysin.ProfilePermissionAdmin
		if len(accountIds) > 0 {
			permission = profilePermissionForViewer(viewer, item)
		}
		markProfilePermission(item, permission)
		list = append(list, adminNoteListFromProfile(item, mediaBuckets[item.Id]))
	}
	return &sysin.AdminNotePageModel{List: list, HasMore: hasMore, NextCursor: nextCursor}, nil
}

func adminNoteListFromProfile(profile *sysin.ProfileModel, media []*sysin.AdminNoteMediaModel) *sysin.AdminNoteListModel {
	if profile == nil {
		return nil
	}
	if media == nil {
		media = []*sysin.AdminNoteMediaModel{}
	}
	return &sysin.AdminNoteListModel{
		Id:                    profile.Id,
		Uuid:                  profile.Uuid,
		AccountId:             profile.AccountId,
		SourceType:            profile.SourceType,
		IsCollected:           profile.IsCollected,
		CollectSourceId:       profile.CollectSourceId,
		CollectSourceName:     profile.CollectSourceName,
		CollectSourceUsername: profile.CollectSourceUsername,
		AccountName:           profile.AccountName,
		Nickname:              profile.Nickname,
		Username:              profile.Username,
		ProfileNo:             profile.ProfileNo,
		Title:                 profile.Title,
		Province:              profile.Province,
		City:                  profile.City,
		Tag:                   profile.Tag,
		Status:                profile.Status,
		TaskStatus:            profile.TaskStatus,
		CanEdit:               profile.CanEdit,
		Permission:            profile.Permission,
		CreatedAt:             profile.CreatedAt,
		UpdatedAt:             profile.UpdatedAt,
		Media:                 media,
	}
}

func (s *sSysPublish) adminNoteCoverMediaByProfiles(ctx context.Context, profiles []*sysin.ProfileModel) (map[int64][]*sysin.AdminNoteMediaModel, error) {
	buckets := make(map[int64][]*sysin.AdminNoteMediaModel, len(profiles))
	profileIds := make([]int64, 0, len(profiles))
	for _, profile := range profiles {
		if profile == nil || profile.Id <= 0 {
			continue
		}
		profileIds = append(profileIds, profile.Id)
		buckets[profile.Id] = []*sysin.AdminNoteMediaModel{}
	}
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 {
		return buckets, nil
	}
	media, err := firstProfileCoverMedia(ctx, profileIds)
	if err != nil {
		return nil, gerror.Wrap(err, "获取笔记封面失败")
	}
	for _, item := range media {
		applyProfileCoverAsset(item)
	}
	normalizeMediaListFileURL(media)
	for _, item := range media {
		if item == nil || item.ProfileId <= 0 || len(buckets[item.ProfileId]) > 0 {
			continue
		}
		buckets[item.ProfileId] = append(buckets[item.ProfileId], &sysin.AdminNoteMediaModel{
			Id:        item.Id,
			ProfileId: item.ProfileId,
			MediaType: item.MediaType,
			FileUrl:   item.FileUrl,
			PosterUrl: item.PosterUrl,
			SortIndex: item.SortIndex,
		})
	}
	return buckets, nil
}

func firstProfileCoverMedia(ctx context.Context, profileIds []int64) ([]*sysin.MediaModel, error) {
	ids := uniqueIds(profileIds)
	if len(ids) == 0 {
		return []*sysin.MediaModel{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	var media []*sysin.MediaModel
	sql := fmt.Sprintf(`
SELECT id, profile_id, media_type, file_url, storage_path, poster_url, poster_storage_path, sort_index
FROM (
    SELECT id, profile_id, media_type, file_url, storage_path, poster_url, poster_storage_path, sort_index,
           ROW_NUMBER() OVER (PARTITION BY profile_id ORDER BY sort_index ASC, id ASC) AS row_number
    FROM %s
    WHERE profile_id IN (%s)
      AND deleted_at IS NULL
      AND status = 1
      AND (purpose IS NULL OR purpose = '' OR purpose = 'display')
) AS profile_cover
WHERE row_number = 1
ORDER BY profile_id ASC`, publishMediaTable, placeholders)
	if err := g.DB().Raw(sql, args...).Scan(&media); err != nil {
		return nil, gerror.Wrap(err, "查询资料封面失败")
	}
	return media, nil
}

func applyProfileCoverAsset(media *sysin.MediaModel) {
	if media == nil || !strings.EqualFold(strings.TrimSpace(media.MediaType), "video") {
		return
	}
	posterURL := strings.TrimSpace(media.PosterUrl)
	posterStoragePath := strings.TrimSpace(media.PosterStoragePath)
	if posterURL == "" && posterStoragePath == "" {
		return
	}
	media.MediaType = "image"
	media.FileUrl = posterURL
	media.StoragePath = posterStoragePath
}

func (s *sSysPublish) profileBaseModel(ctx context.Context, tenantId int64, accountId int64) (*gdb.Model, error) {
	mod := dao.ContentProfile.Ctx(ctx).As("p").
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" tenant", "tenant.id=ps.tenant_id").
		LeftJoin(publishAccountTable+" a", "a.id=ps.account_id AND a.deleted_at IS NULL").
		WhereNull("p.deleted_at")
	if tenantId > 0 {
		mod = mod.Where("ps.tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("ps.account_id", accountId)
	}
	return mod, nil
}

func profileListFields() string {
	return "p.id,p.source_note_uuid AS uuid,p.source_type,p.profile_no,p.title,p.summary,p.plain_text,p.province,p.city," + profileTagFieldExpr() + " AS tag,p.visibility,p.review_status,p.status,p.image_count,p.video_count,COALESCE(ps.customer_remark,p.admin_remark) AS customer_remark,p.published_at,p.created_at,p.updated_at,ps.tenant_id,ps.account_id,tenant.name AS tenant_name,a.nickname AS account_name,a.nickname,a.username,COALESCE(ps.anti_scan_enabled,0) AS anti_scan_enabled,COALESCE(ps.publish_task_status,'') AS task_status,'' AS tg_status,CASE WHEN EXISTS (SELECT 1 FROM " + publishProfileChannelTable + " pc WHERE pc.tenant_id=ps.tenant_id AND pc.profile_id=ps.profile_id AND pc.is_manual=1 AND pc.deleted_at IS NULL) OR EXISTS (SELECT 1 FROM " + publishChannelTable + " dc WHERE dc.tenant_id=ps.tenant_id AND dc.publish_direction='up' AND dc.status=1 AND dc.is_default_selected=1 AND dc.deleted_at IS NULL) THEN 1 ELSE 0 END AS tg_push_enabled"
}

func (s *sSysPublish) applyProfileFilters(ctx context.Context, mod *gdb.Model, in *sysin.ProfileListInp) *gdb.Model {
	mod = s.applyProfileNonKeywordFilters(ctx, mod, in)
	if in == nil {
		return mod
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		if profileNo, ok := normalizeProfileNoSearchKeyword(keyword); ok {
			mod = mod.Where("p.profile_no", profileNo)
		} else {
			terms := splitProfileSearchTerms(keyword)
			if len(terms) > 0 {
				condition, args := segmentedLikeCondition([]string{"p.title", "p.plain_text"}, terms)
				mod = mod.Where(condition, args...)
			}
		}
	}
	return mod
}

func (s *sSysPublish) applyProfileNonKeywordFilters(ctx context.Context, mod *gdb.Model, in *sysin.ProfileListInp) *gdb.Model {
	_ = ctx
	if in == nil {
		return mod
	}
	if in.Province != "" {
		mod = mod.Where("p.province", strings.TrimSpace(in.Province))
	}
	if in.City != "" {
		mod = mod.Where("p.city", strings.TrimSpace(in.City))
	}
	if in.Tag != "" {
		tags := splitProfileTagValues(in.Tag)
		if len(tags) == 1 {
			tag := strings.TrimSpace(tags[0])
			tagField := profileTagFieldExpr()
			mod = mod.Where("("+tagField+" = ? OR "+tagField+" LIKE ? OR "+tagField+" LIKE ? OR "+tagField+" LIKE ?)", tag, tag+",%", "%,"+tag, "%,"+tag+",%")
		} else if len(tags) > 1 {
			conditions := make([]string, 0, len(tags)*4)
			args := make([]interface{}, 0, len(tags)*4)
			for _, tag := range tags {
				tag = strings.TrimSpace(tag)
				if tag == "" {
					continue
				}
				tagField := profileTagFieldExpr()
				conditions = append(conditions, "("+tagField+" = ? OR "+tagField+" LIKE ? OR "+tagField+" LIKE ? OR "+tagField+" LIKE ?)")
				args = append(args, tag, tag+",%", "%,"+tag, "%,"+tag+",%")
			}
			if len(conditions) > 0 {
				mod = mod.Where("("+strings.Join(conditions, " OR ")+")", args...)
			}
		}
	}
	if in.ReviewStatus != "" {
		mod = mod.Where("p.review_status", strings.TrimSpace(in.ReviewStatus))
	}
	if in.Visibility != "" {
		mod = mod.Where("p.visibility", strings.TrimSpace(in.Visibility))
	}
	if in.Status > 0 {
		mod = mod.Where("p.status", in.Status)
	}
	if in.CollectSourceId > 0 {
		mod = mod.Where("EXISTS (SELECT 1 FROM "+publishCollectDispatchTable+" d WHERE d.profile_id=p.id AND d.source_id=?)", in.CollectSourceId)
	}
	return mod
}

type profileCollectionMetadataRow struct {
	TenantId        int64  `orm:"tenant_id"`
	ProfileId       int64  `orm:"profile_id"`
	SourceId        int64  `orm:"source_id"`
	SourceType      string `orm:"source_type"`
	SourceName      string `orm:"source_name"`
	SourceUsername  string `orm:"source_username"`
	SourceChatId    string `orm:"source_chat_id"`
	SourceMessageId int64  `orm:"source_message_id"`
	BotId           int64  `orm:"bot_id"`
	TgAccountId     int64  `orm:"tg_account_id"`
}

func (s *sSysPublish) applyProfileCollectionMetadata(ctx context.Context, list []*sysin.ProfileModel) error {
	profileIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil && item.Id > 0 {
			profileIds = append(profileIds, item.Id)
		}
	}
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 {
		return nil
	}
	var rows []profileCollectionMetadataRow
	err := g.DB().Model(publishCollectDispatchTable+" d").Safe().Ctx(ctx).
		InnerJoin(publishCollectSourceTable+" s", "s.id=d.source_id AND s.deleted_at IS NULL").
		LeftJoin(publishCollectEventTable+" e", "e.id=d.event_id").
		WhereIn("d.profile_id", profileIds).
		Fields("s.tenant_id,d.profile_id,d.source_id,s.source_type,s.title AS source_name,s.source_username,s.bot_id,s.tg_account_id," +
			"COALESCE(NULLIF(e.source_chat_id,''),s.source_chat_id) AS source_chat_id," +
			"COALESCE(NULLIF(e.source_message_id,0),0) AS source_message_id").
		OrderAsc("d.profile_id").OrderDesc("d.id").Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取资料采集来源失败")
	}
	metadata := make(map[int64]profileCollectionMetadataRow, len(rows))
	for _, row := range rows {
		if _, ok := metadata[row.ProfileId]; !ok {
			metadata[row.ProfileId] = row
		}
	}
	botChats := make(map[int64][]string)
	tgChats := make(map[int64][]string)
	for _, row := range metadata {
		if strings.EqualFold(strings.TrimSpace(row.SourceType), sysin.CollectSourceTypeBot) && row.BotId > 0 {
			botChats[row.BotId] = append(botChats[row.BotId], row.SourceChatId)
		} else if row.TgAccountId > 0 {
			tgChats[row.TgAccountId] = append(tgChats[row.TgAccountId], row.SourceChatId)
		}
	}
	botDisplays := make(map[int64]map[int64]map[string]telegramChannelDisplay, len(botChats))
	for botId, chatIds := range botChats {
		tenantId := collectionMetadataTenantId(metadata, botId, true)
		displays, displayErr := s.resolveBotChannelDisplays(ctx, tenantId, botId, chatIds)
		if displayErr != nil {
			return gerror.Wrap(displayErr, "读取Bot采集来源频道失败")
		}
		if botDisplays[tenantId] == nil {
			botDisplays[tenantId] = make(map[int64]map[string]telegramChannelDisplay)
		}
		botDisplays[tenantId][botId] = displays
	}
	tgDisplays := make(map[int64]map[int64]map[string]telegramChannelDisplay, len(tgChats))
	for tgAccountId, chatIds := range tgChats {
		tenantId := collectionMetadataTenantId(metadata, tgAccountId, false)
		displays, displayErr := s.resolveTelegramChannelDisplays(ctx, tenantId, tgAccountId, chatIds)
		if displayErr != nil {
			return gerror.Wrap(displayErr, "读取TG采集来源频道失败")
		}
		if tgDisplays[tenantId] == nil {
			tgDisplays[tenantId] = make(map[int64]map[string]telegramChannelDisplay)
		}
		tgDisplays[tenantId][tgAccountId] = displays
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		row, ok := metadata[item.Id]
		item.IsCollected = item.IsCollected || item.SourceType == collectProfileSourceType || ok
		if !ok {
			continue
		}
		item.CollectSourceId = row.SourceId
		item.CollectSourceName = strings.TrimSpace(row.SourceName)
		item.CollectSourceUsername = strings.TrimSpace(row.SourceUsername)
		item.CollectSourceChatId = strings.TrimSpace(row.SourceChatId)
		item.CollectSourceMessageId = row.SourceMessageId
		display := telegramChannelDisplay{}
		if strings.EqualFold(strings.TrimSpace(row.SourceType), sysin.CollectSourceTypeBot) {
			display = botDisplays[row.TenantId][row.BotId][normalizeTelegramChannelChatID(row.SourceChatId)]
		} else {
			display = tgDisplays[row.TenantId][row.TgAccountId][normalizeTelegramChannelChatID(row.SourceChatId)]
		}
		if !display.Empty() {
			item.CollectSourceName = display.Title
			item.CollectSourceUsername = display.Username
		}
		if item.CollectSourceMessageId > 0 {
			item.CollectSourceUrl = collectedTelegramMessageURL(item.CollectSourceChatId, item.CollectSourceUsername, item.CollectSourceMessageId)
		}
	}
	return nil
}

func collectionMetadataTenantId(metadata map[int64]profileCollectionMetadataRow, accountId int64, bot bool) int64 {
	for _, row := range metadata {
		if (bot && row.BotId == accountId) || (!bot && row.TgAccountId == accountId) {
			return row.TenantId
		}
	}
	return 0
}

func collectedTelegramMessageURL(chatId, username string, messageId int64) string {
	if messageId <= 0 {
		return ""
	}
	if username = strings.TrimPrefix(strings.TrimSpace(username), "@"); username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", username, messageId)
	}
	chatId = normalizeTelegramChannelChatID(chatId)
	if strings.HasPrefix(chatId, "-100") {
		return fmt.Sprintf("https://t.me/c/%s/%d", strings.TrimPrefix(chatId, "-100"), messageId)
	}
	return ""
}

func splitProfileTagValues(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '|' || r == ';'
	})
	if len(parts) == 0 {
		return []string{strings.TrimSpace(value)}
	}
	return parts
}

func (s *sSysPublish) applyProfileOwnerNames(ctx context.Context, list []*sysin.ProfileModel) error {
	tenantIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil && item.TenantId > 0 {
			tenantIds = append(tenantIds, item.TenantId)
		}
	}
	names, err := s.tenantOwnerNames(ctx, tenantIds)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.TenantName) == "" {
			item.TenantName = names[item.TenantId]
		}
		if item.TenantName == "" && item.TenantId > 0 {
			item.TenantName = fmt.Sprintf("账号归属#%d", item.TenantId)
		}
	}
	return nil
}
