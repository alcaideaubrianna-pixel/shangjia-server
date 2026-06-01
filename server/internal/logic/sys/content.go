package sys

import (
	"context"
	"hotgo/internal/consts"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	contentSourceFeiNiu = "feiniu"

	contentTableProfile    = "hg_content_profile"
	contentTableMedia      = "hg_content_media"
	contentTableChannel    = "hg_content_channel"
	contentTableSourceMap  = "hg_content_source_map"
	contentTableCheckpoint = "hg_content_import_checkpoint"
	contentTableImportRun  = "hg_content_import_run"
)

type sSysContent struct{}

func NewSysContent() *sSysContent {
	return &sSysContent{}
}

func init() {
	service.RegisterSysContent(NewSysContent())
}

// ListProfiles 获取前台资料列表。
func (s *sSysContent) ListProfiles(ctx context.Context, in *sysin.ContentProfileListInp) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	mod := g.DB().Model(contentTableProfile + " p").Safe().Ctx(ctx)
	mod = s.publicProfileWhere(mod)

	if in.Keyword != "" {
		mod = mod.WhereLike("p.title", "%"+in.Keyword+"%").WhereOrLike("p.summary", "%"+in.Keyword+"%").WhereOrLike("p.profile_no", "%"+in.Keyword+"%")
	}
	if in.Province != "" {
		mod = mod.Where("p.province", in.Province)
	}
	if in.City != "" {
		mod = mod.Where("p.city", in.City)
	}

	totalCount, err = mod.Count()
	if err != nil {
		err = gerror.Wrap(err, "获取资料数据行失败")
		return
	}
	if totalCount == 0 {
		list = []*sysin.ContentProfileListModel{}
		return
	}

	mod = mod.Fields(
		"p.id,p.profile_no,p.title,p.summary,p.province,p.city,p.age,p.height,p.weight,p.cup_size,p.has_verification_video,p.member_only_video,p.published_at",
	)
	mod = mod.Page(in.Page, in.PerPage)
	switch in.Sort {
	case "oldest":
		mod = mod.OrderAsc("p.published_at").OrderAsc("p.id")
	default:
		mod = mod.OrderDesc("p.published_at").OrderDesc("p.id")
	}

	var rows []contentProfileRow
	if err = mod.Scan(&rows); err != nil {
		err = gerror.Wrap(err, "获取资料列表失败，请稍后重试")
		return
	}

	list = make([]*sysin.ContentProfileListModel, 0, len(rows))
	for _, row := range rows {
		item := row.toListModel()
		item.CoverUrl, _ = s.getProfileCoverUrl(ctx, row.Id)
		item.Avatar = item.CoverUrl
		list = append(list, item)
	}
	return
}

// ViewProfile 获取前台资料详情。
func (s *sSysContent) ViewProfile(ctx context.Context, in *sysin.ContentProfileViewInp) (res *sysin.ContentProfileViewModel, err error) {
	var row *contentProfileRow
	if err = s.publicProfileWhere(g.DB().Model(contentTableProfile+" p").Safe().Ctx(ctx)).
		Fields("p.*").
		Where("p.id", in.Id).
		Scan(&row); err != nil {
		err = gerror.Wrap(err, "获取资料详情失败，请稍后重试")
		return
	}
	if row == nil {
		err = gerror.New("资料不存在或暂未公开")
		return
	}

	res = &sysin.ContentProfileViewModel{
		ContentProfileListModel: *row.toListModel(),
		Intro:                   row.Summary,
		PlainText:               row.PlainText,
		ImageCount:              row.ImageCount,
		VideoCount:              row.VideoCount,
		MemberOnly:              row.Visibility == consts.ContentVisibilityMemberOnly,
	}
	res.Media, err = s.listProfileMedia(ctx, row.Id, false)
	if err != nil {
		return
	}
	res.Photos = make([]string, 0, len(res.Media))
	for _, item := range res.Media {
		if item.Type == consts.ContentMediaTypeImage && item.DisplayUrl != "" {
			res.Photos = append(res.Photos, item.DisplayUrl)
		}
	}
	if len(res.Photos) > 0 {
		res.CoverUrl = res.Photos[0]
		res.Avatar = res.Photos[0]
	}
	return
}

// ImportFeiNiu 从 FeiNiu_bot 增量导入资料。
func (s *sSysContent) ImportFeiNiu(ctx context.Context, in *sysin.ContentImportFeiNiuInp) (res *sysin.ContentImportFeiNiuModel, err error) {
	res = new(sysin.ContentImportFeiNiuModel)
	sourceGroup := g.Cfg().MustGet(ctx, "contentImport.feiniu.dbGroup", "feiniu").String()
	batchSize := in.BatchSize
	if batchSize <= 0 {
		batchSize = g.Cfg().MustGet(ctx, "contentImport.feiniu.batchSize", 200).Int()
	}
	if batchSize <= 0 || batchSize > 1000 {
		batchSize = 200
	}
	triggerType := in.TriggerType
	if triggerType == "" {
		triggerType = "manual"
	}
	startedAt := gtime.Now()
	runId, err := s.createImportRun(ctx, contentSourceFeiNiu, triggerType, batchSize, startedAt)
	if err != nil {
		return
	}
	defer func() {
		status := "success"
		errorMessage := ""
		if err != nil {
			status = "failed"
			errorMessage = err.Error()
		}
		_ = s.finishImportRun(ctx, runId, status, errorMessage, startedAt, res)
	}()

	lastNoteId, err := s.getCheckpoint(ctx, contentSourceFeiNiu)
	if err != nil {
		return
	}

	sourceDB := g.DB(sourceGroup)
	rows, err := sourceDB.GetAll(ctx, `
SELECT
  note_id,note_uuid,note_code,title,summary,plain_text,source_key,province,city,age,height,weight,cup_size,
  has_verification_video,cover_asset_id,image_count,video_count,duplicate_note_id,ingest_status,status,create_time,update_time
FROM tg_content_note
WHERE note_id > ? AND status = '0'
ORDER BY note_id ASC
LIMIT ?`, lastNoteId, batchSize)
	if err != nil {
		err = gerror.Wrap(err, "读取 FeiNiu 资料失败，请检查 contentImport.feiniu.dbGroup 配置")
		_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
		return
	}

	for _, source := range rows {
		res.Scanned++
		sourceNoteId := source["note_id"].Int64()
		res.LastSourceNote = sourceNoteId
		imported, profileId, importErr := s.importFeiNiuProfile(ctx, sourceDB, source)
		if importErr != nil {
			err = importErr
			_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
			return
		}
		if imported {
			res.Imported++
		} else {
			res.Duplicate++
		}
		mediaCount, importErr := s.importFeiNiuMedia(ctx, sourceDB, profileId, sourceNoteId)
		if importErr != nil {
			err = importErr
			_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
			return
		}
		res.MediaImported += mediaCount
	}

	if res.LastSourceNote > lastNoteId {
		if err = s.saveCheckpoint(ctx, contentSourceFeiNiu, res.LastSourceNote); err != nil {
			return
		}
	}
	return
}

// ImportOverview 获取内容导入概览。
func (s *sSysContent) ImportOverview(ctx context.Context, in *sysin.ContentImportOverviewInp) (res *sysin.ContentImportOverviewModel, err error) {
	sourceName := in.SourceName
	if sourceName == "" {
		sourceName = contentSourceFeiNiu
	}
	res = &sysin.ContentImportOverviewModel{SourceName: sourceName}

	res.ProfileTotal, err = g.DB().Model(contentTableProfile).Safe().Ctx(ctx).Where("source_type", sourceName).Count()
	if err != nil {
		err = gerror.Wrap(err, "统计资料总数失败")
		return
	}
	res.PublicTotal, err = g.DB().Model(contentTableProfile).Safe().Ctx(ctx).
		Where("source_type", sourceName).
		Where("review_status", consts.ContentReviewApproved).
		WhereIn("visibility", []string{consts.ContentVisibilityPublic, consts.ContentVisibilityMemberOnly}).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计公开资料数失败")
		return
	}
	res.PendingTotal, err = g.DB().Model(contentTableProfile).Safe().Ctx(ctx).
		Where("source_type", sourceName).
		Where("review_status", consts.ContentReviewPending).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计待审核资料数失败")
		return
	}
	res.DuplicateTotal, err = g.DB().Model(contentTableProfile).Safe().Ctx(ctx).
		Where("source_type", sourceName).
		Where("duplicate_of_id>0").
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计重复资料数失败")
		return
	}
	res.MediaTotal, err = g.DB().Model(contentTableMedia+" m").Safe().Ctx(ctx).
		LeftJoin(contentTableProfile+" p", "p.id=m.profile_id").
		Where("p.source_type", sourceName).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计媒体总数失败")
		return
	}
	res.DuplicateMedia, err = g.DB().Model(contentTableMedia+" m").Safe().Ctx(ctx).
		LeftJoin(contentTableProfile+" p", "p.id=m.profile_id").
		Where("p.source_type", sourceName).
		Where("m.duplicate_of_media_id>0").
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计重复媒体数失败")
		return
	}

	checkpoint, err := g.DB().Model(contentTableCheckpoint).Safe().Ctx(ctx).
		Fields("last_source_note_id,last_success_at,last_error").
		Where("source_name", sourceName).
		One()
	if err != nil {
		err = gerror.Wrap(err, "读取导入游标失败")
		return
	}
	if checkpoint != nil {
		res.LastSourceNoteId = checkpoint["last_source_note_id"].Int64()
		res.LastSuccessAt = checkpoint["last_success_at"].GTime()
		res.LastError = checkpoint["last_error"].String()
	}

	lastRun, err := g.DB().Model(contentTableImportRun).Safe().Ctx(ctx).
		Fields("status,cost_ms").
		Where("source_name", sourceName).
		OrderDesc("id").
		One()
	if err != nil {
		err = gerror.Wrap(err, "读取最近导入运行记录失败")
		return
	}
	if lastRun != nil {
		res.LastRunStatus = lastRun["status"].String()
		res.LastRunCostMs = lastRun["cost_ms"].Int()
	}
	return
}

// ImportRunList 获取内容导入运行记录。
func (s *sSysContent) ImportRunList(ctx context.Context, in *sysin.ContentImportRunListInp) (list []*sysin.ContentImportRunListModel, totalCount int, err error) {
	mod := g.DB().Model(contentTableImportRun).Safe().Ctx(ctx)
	if in.SourceName != "" {
		mod = mod.Where("source_name", in.SourceName)
	}
	if in.Status != "" {
		mod = mod.Where("status", in.Status)
	}
	totalCount, err = mod.Count()
	if err != nil {
		err = gerror.Wrap(err, "统计导入运行记录失败")
		return
	}
	if totalCount == 0 {
		list = []*sysin.ContentImportRunListModel{}
		return
	}
	err = mod.Fields("id,source_name,trigger_type,batch_size,scanned,imported,duplicate,media_imported,last_source_note_id,status,error_message,started_at,finished_at,cost_ms").
		Page(in.Page, in.PerPage).
		OrderDesc("id").
		Scan(&list)
	if err != nil {
		err = gerror.Wrap(err, "获取导入运行记录失败")
	}
	return
}

func (s *sSysContent) publicProfileWhere(mod *gdb.Model) *gdb.Model {
	return mod.
		Where("p.status", 1).
		Where("p.review_status", consts.ContentReviewApproved).
		WhereIn("p.visibility", []string{consts.ContentVisibilityPublic, consts.ContentVisibilityMemberOnly})
}

func (s *sSysContent) getProfileCoverUrl(ctx context.Context, profileId int64) (url string, err error) {
	one, err := g.DB().Model(contentTableMedia).Safe().Ctx(ctx).
		Fields("display_storage_path").
		Where("profile_id", profileId).
		Where("media_type", consts.ContentMediaTypeImage).
		Where("status", 1).
		OrderAsc("sort_index").
		One()
	if err != nil || one == nil {
		return "", err
	}
	return one["display_storage_path"].String(), nil
}

func (s *sSysContent) listProfileMedia(ctx context.Context, profileId int64, isMember bool) (list []*sysin.ContentMediaModel, err error) {
	rows, err := g.DB().Model(contentTableMedia).Safe().Ctx(ctx).
		Fields("id,media_type,display_storage_path,preview_storage_path,width,height,duration,process_status").
		Where("profile_id", profileId).
		Where("status", 1).
		OrderAsc("sort_index").
		OrderAsc("id").
		All()
	if err != nil {
		err = gerror.Wrap(err, "获取资料媒体失败，请稍后重试")
		return
	}
	list = make([]*sysin.ContentMediaModel, 0, len(rows))
	for _, row := range rows {
		mediaType := row["media_type"].String()
		locked := mediaType == consts.ContentMediaTypeVideo && !isMember
		displayUrl := row["display_storage_path"].String()
		if locked {
			displayUrl = ""
		}
		list = append(list, &sysin.ContentMediaModel{
			Id:          row["id"].Int64(),
			Type:        mediaType,
			DisplayUrl:  displayUrl,
			PreviewUrl:  row["preview_storage_path"].String(),
			Width:       row["width"].Int(),
			Height:      row["height"].Int(),
			Duration:    row["duration"].Int(),
			Locked:      locked,
			ProcessDone: row["process_status"].String() == "processed",
		})
	}
	return
}

func (s *sSysContent) getCheckpoint(ctx context.Context, sourceName string) (lastNoteId int64, err error) {
	one, err := g.DB().Model(contentTableCheckpoint).Safe().Ctx(ctx).
		Fields("last_source_note_id").
		Where("source_name", sourceName).
		One()
	if err != nil {
		err = gerror.Wrap(err, "读取内容导入游标失败")
		return
	}
	if one == nil {
		return 0, nil
	}
	return one["last_source_note_id"].Int64(), nil
}

func (s *sSysContent) saveCheckpoint(ctx context.Context, sourceName string, lastNoteId int64) (err error) {
	now := gtime.Now()
	one, err := g.DB().Model(contentTableCheckpoint).Safe().Ctx(ctx).Fields("id").Where("source_name", sourceName).One()
	if err != nil {
		return gerror.Wrap(err, "读取内容导入游标失败")
	}
	data := g.Map{
		"last_source_note_id": lastNoteId,
		"last_success_at":     now,
		"last_error":          "",
		"updated_at":          now,
	}
	if one == nil {
		data["source_name"] = sourceName
		data["created_at"] = now
		_, err = g.DB().Model(contentTableCheckpoint).Safe().Ctx(ctx).Data(data).Insert()
	} else {
		_, err = g.DB().Model(contentTableCheckpoint).Safe().Ctx(ctx).Where("id", one["id"].Int64()).Data(data).Update()
	}
	if err != nil {
		err = gerror.Wrap(err, "保存内容导入游标失败")
	}
	return
}

func (s *sSysContent) saveCheckpointError(ctx context.Context, sourceName string, sourceErr error) (err error) {
	_, err = g.DB().Model(contentTableCheckpoint).Safe().Ctx(ctx).
		Where("source_name", sourceName).
		Data(g.Map{"last_error": sourceErr.Error(), "updated_at": gtime.Now()}).
		Update()
	return
}

func (s *sSysContent) createImportRun(ctx context.Context, sourceName string, triggerType string, batchSize int, startedAt *gtime.Time) (id int64, err error) {
	id, err = g.DB().Model(contentTableImportRun).Safe().Ctx(ctx).Data(g.Map{
		"source_name":  sourceName,
		"trigger_type": triggerType,
		"batch_size":   batchSize,
		"status":       "running",
		"started_at":   startedAt,
		"created_at":   startedAt,
		"updated_at":   startedAt,
	}).InsertAndGetId()
	if err != nil {
		err = gerror.Wrap(err, "创建内容导入运行记录失败")
	}
	return
}

func (s *sSysContent) finishImportRun(ctx context.Context, runId int64, status string, errorMessage string, startedAt *gtime.Time, res *sysin.ContentImportFeiNiuModel) (err error) {
	if runId <= 0 {
		return nil
	}
	finishedAt := gtime.Now()
	costMs := int(finishedAt.Sub(startedAt).Milliseconds())
	data := g.Map{
		"status":              status,
		"error_message":       errorMessage,
		"finished_at":         finishedAt,
		"cost_ms":             costMs,
		"updated_at":          finishedAt,
		"scanned":             res.Scanned,
		"imported":            res.Imported,
		"duplicate":           res.Duplicate,
		"media_imported":      res.MediaImported,
		"last_source_note_id": res.LastSourceNote,
	}
	_, err = g.DB().Model(contentTableImportRun).Safe().Ctx(ctx).Where("id", runId).Data(data).Update()
	if err != nil {
		err = gerror.Wrap(err, "更新内容导入运行记录失败")
	}
	return
}

func (s *sSysContent) importFeiNiuProfile(ctx context.Context, sourceDB gdb.DB, source gdb.Record) (imported bool, profileId int64, err error) {
	profileNo := source["note_code"].String()
	if profileNo == "" {
		profileNo = "FN" + source["note_id"].String()
	}
	one, err := g.DB().Model(contentTableProfile).Safe().Ctx(ctx).Fields("id").Where("profile_no", profileNo).One()
	if err != nil {
		err = gerror.Wrap(err, "检查资料是否存在失败")
		return
	}

	sourceInfo, err := s.getFeiNiuSourceInfo(ctx, sourceDB, source["note_id"].Int64())
	if err != nil {
		return
	}
	channelId, err := s.upsertFeiNiuChannel(ctx, sourceInfo)
	if err != nil {
		return
	}
	duplicateOfId, err := s.findDuplicateProfileId(ctx, source["note_id"].Int64(), source["duplicate_note_id"].Int64(), sourceInfo)
	if err != nil {
		return
	}

	now := gtime.Now()
	data := g.Map{
		"profile_no":             profileNo,
		"source_type":            contentSourceFeiNiu,
		"source_note_id":         source["note_id"].Int64(),
		"source_note_uuid":       source["note_uuid"].String(),
		"source_key":             source["source_key"].String(),
		"source_text_hash":       recordString(sourceInfo, "source_text_hash"),
		"channel_id":             channelId,
		"duplicate_of_id":        duplicateOfId,
		"title":                  source["title"].String(),
		"summary":                source["summary"].String(),
		"plain_text":             source["plain_text"].String(),
		"province":               source["province"].String(),
		"city":                   source["city"].String(),
		"age":                    source["age"].Int(),
		"height":                 source["height"].Int(),
		"weight":                 source["weight"].Int(),
		"cup_size":               source["cup_size"].String(),
		"has_verification_video": flagToInt(source["has_verification_video"].String()),
		"image_count":            source["image_count"].Int(),
		"video_count":            source["video_count"].Int(),
		"visibility":             consts.ContentVisibilityPrivate,
		"review_status":          consts.ContentReviewPending,
		"import_status":          "imported",
		"updated_at":             now,
	}
	if duplicateOfId > 0 {
		data["import_status"] = "duplicate"
	}
	if one == nil {
		data["created_at"] = now
		id, insertErr := g.DB().Model(contentTableProfile).Safe().Ctx(ctx).Data(data).InsertAndGetId()
		if insertErr != nil {
			err = gerror.Wrap(insertErr, "导入资料失败")
			return
		}
		if err = s.upsertFeiNiuSourceMap(ctx, id, sourceInfo); err != nil {
			return
		}
		return true, id, nil
	}
	profileId = one["id"].Int64()
	if _, err = g.DB().Model(contentTableProfile).Safe().Ctx(ctx).Where("id", profileId).Data(data).Update(); err != nil {
		err = gerror.Wrap(err, "更新资料失败")
		return
	}
	if err = s.upsertFeiNiuSourceMap(ctx, profileId, sourceInfo); err != nil {
		return
	}
	return false, profileId, nil
}

func (s *sSysContent) importFeiNiuMedia(ctx context.Context, sourceDB gdb.DB, profileId int64, sourceNoteId int64) (count int, err error) {
	rows, err := sourceDB.GetAll(ctx, `
SELECT
  b.asset_id,b.sort_index,a.asset_type,a.binary_md5,a.perceptual_hash,a.width,a.height,a.duration,
  a.origin_uri,a.preview_uri,c.cos_path,c.status AS cos_status
FROM tg_content_block b
JOIN tg_content_asset a ON a.asset_id = b.asset_id
LEFT JOIN tg_content_asset_cos c ON c.asset_id = a.asset_id
WHERE b.note_id = ? AND b.asset_id IS NOT NULL
ORDER BY b.sort_index ASC,b.block_id ASC`, sourceNoteId)
	if err != nil {
		err = gerror.Wrap(err, "读取 FeiNiu 媒体失败")
		return
	}

	for _, row := range rows {
		assetId := row["asset_id"].Int64()
		if assetId <= 0 {
			continue
		}
		exists, checkErr := g.DB().Model(contentTableMedia).Safe().Ctx(ctx).
			Fields("id").
			Where("profile_id", profileId).
			Where("source_asset_id", assetId).
			One()
		if checkErr != nil {
			err = gerror.Wrap(checkErr, "检查媒体是否存在失败")
			return
		}
		if exists != nil {
			continue
		}
		mediaType := row["asset_type"].String()
		if mediaType == "" {
			mediaType = consts.ContentMediaTypeImage
		}
		cosPath := row["cos_path"].String()
		previewPath := row["preview_uri"].String()
		displayPath := previewPath
		if mediaType == consts.ContentMediaTypeVideo && cosPath != "" {
			previewPath = cosPath + ".poster.jpg"
			displayPath = ""
		}
		duplicateMedia, checkDuplicateErr := s.getDuplicateMediaByMD5(ctx, mediaType, row["binary_md5"].String())
		if checkDuplicateErr != nil {
			err = checkDuplicateErr
			return
		}
		duplicateMediaId := int64(0)
		if duplicateMedia != nil {
			duplicateMediaId = duplicateMedia["id"].Int64()
			displayPath = duplicateMedia["display_storage_path"].String()
			previewPath = duplicateMedia["preview_storage_path"].String()
		}
		_, insertErr := g.DB().Model(contentTableMedia).Safe().Ctx(ctx).Data(g.Map{
			"profile_id":            profileId,
			"source_asset_id":       assetId,
			"media_type":            mediaType,
			"sort_index":            row["sort_index"].Int(),
			"original_storage_path": originalStoragePath(cosPath, duplicateMediaId),
			"display_storage_path":  displayPath,
			"preview_storage_path":  previewPath,
			"duplicate_of_media_id": duplicateMediaId,
			"binary_md5":            row["binary_md5"].String(),
			"perceptual_hash":       row["perceptual_hash"].String(),
			"width":                 row["width"].Int(),
			"height":                row["height"].Int(),
			"duration":              row["duration"].Int(),
			"process_status":        "raw",
			"encrypt_status":        "none",
			"status":                1,
			"created_at":            gtime.Now(),
			"updated_at":            gtime.Now(),
		}).Insert()
		if insertErr != nil {
			err = gerror.Wrap(insertErr, "导入媒体失败")
			return
		}
		count++
	}
	return
}

func (s *sSysContent) getFeiNiuSourceInfo(ctx context.Context, sourceDB gdb.DB, sourceNoteId int64) (row gdb.Record, err error) {
	row, err = sourceDB.GetOne(ctx, `
SELECT
  s.source_id,s.note_id,s.source_type,s.source_key,s.channel_id,s.tg_chat_id,s.tg_message_id,s.tg_grouped_id,
  s.raw_text,s.source_text_hash,s.raw_message_json,
  c.title AS channel_title,c.username AS channel_username,c.invite_link AS channel_invite_link
FROM tg_content_source s
LEFT JOIN tg_channel c ON c.channel_id = s.channel_id
WHERE s.note_id = ?
ORDER BY
  CASE WHEN s.source_type = 'telegram_group' THEN 0 ELSE 1 END,
  s.source_id ASC
LIMIT 1`, sourceNoteId)
	if err != nil {
		err = gerror.Wrap(err, "读取 FeiNiu 来源映射失败")
		return
	}
	if row == nil {
		row = gdb.Record{}
	}
	return
}

func (s *sSysContent) upsertFeiNiuChannel(ctx context.Context, sourceInfo gdb.Record) (channelId int64, err error) {
	sourceChannelId := recordInt64(sourceInfo, "channel_id")
	if sourceChannelId <= 0 {
		return 0, nil
	}

	now := gtime.Now()
	one, err := g.DB().Model(contentTableChannel).Safe().Ctx(ctx).
		Fields("id").
		Where("source_type", contentSourceFeiNiu).
		Where("source_channel_id", sourceChannelId).
		One()
	if err != nil {
		err = gerror.Wrap(err, "读取本地来源频道失败")
		return
	}

	data := g.Map{
		"source_channel_id": sourceChannelId,
		"tg_chat_id":        recordString(sourceInfo, "tg_chat_id"),
		"title":             recordString(sourceInfo, "channel_title"),
		"username":          recordString(sourceInfo, "channel_username"),
		"invite_link":       recordString(sourceInfo, "channel_invite_link"),
		"source_type":       contentSourceFeiNiu,
		"public_status":     "hidden",
		"auth_status":       "none",
		"status":            1,
		"updated_at":        now,
	}
	if one == nil {
		data["created_at"] = now
		channelId, err = g.DB().Model(contentTableChannel).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	} else {
		channelId = one["id"].Int64()
		_, err = g.DB().Model(contentTableChannel).Safe().Ctx(ctx).Where("id", channelId).Data(data).Update()
	}
	if err != nil {
		err = gerror.Wrap(err, "保存本地来源频道失败")
	}
	return
}

func (s *sSysContent) upsertFeiNiuSourceMap(ctx context.Context, profileId int64, sourceInfo gdb.Record) (err error) {
	sourceKey := recordString(sourceInfo, "source_key")
	if sourceKey == "" {
		return nil
	}

	now := gtime.Now()
	one, err := g.DB().Model(contentTableSourceMap).Safe().Ctx(ctx).Fields("id").Where("source_key", sourceKey).One()
	if err != nil {
		return gerror.Wrap(err, "读取内容来源映射失败")
	}
	data := g.Map{
		"profile_id":        profileId,
		"source_type":       recordString(sourceInfo, "source_type"),
		"source_key":        sourceKey,
		"source_channel_id": recordInt64(sourceInfo, "channel_id"),
		"source_message_id": recordInt64(sourceInfo, "tg_message_id"),
		"source_grouped_id": recordInt64(sourceInfo, "tg_grouped_id"),
		"source_text_hash":  recordString(sourceInfo, "source_text_hash"),
		"raw_text":          recordString(sourceInfo, "raw_text"),
		"raw_message_json":  recordString(sourceInfo, "raw_message_json"),
	}
	if data["source_type"] == "" {
		data["source_type"] = contentSourceFeiNiu
	}
	if one == nil {
		data["created_at"] = now
		_, err = g.DB().Model(contentTableSourceMap).Safe().Ctx(ctx).Data(data).Insert()
	} else {
		_, err = g.DB().Model(contentTableSourceMap).Safe().Ctx(ctx).Where("id", one["id"].Int64()).Data(data).Update()
	}
	if err != nil {
		err = gerror.Wrap(err, "保存内容来源映射失败")
	}
	return
}

func (s *sSysContent) findDuplicateProfileId(ctx context.Context, sourceNoteId int64, duplicateNoteId int64, sourceInfo gdb.Record) (id int64, err error) {
	if duplicateNoteId > 0 {
		one, queryErr := g.DB().Model(contentTableProfile).Safe().Ctx(ctx).
			Fields("id").
			Where("source_type", contentSourceFeiNiu).
			Where("source_note_id", duplicateNoteId).
			One()
		if queryErr != nil {
			err = gerror.Wrap(queryErr, "检查重复资料失败")
			return
		}
		if one != nil {
			return one["id"].Int64(), nil
		}
	}

	sourceTextHash := recordString(sourceInfo, "source_text_hash")
	sourceChannelId := recordInt64(sourceInfo, "channel_id")
	if sourceTextHash == "" || sourceChannelId <= 0 {
		return 0, nil
	}
	one, err := g.DB().Model(contentTableProfile+" p").Safe().Ctx(ctx).
		Fields("p.id").
		LeftJoin(contentTableSourceMap+" s", "s.profile_id=p.id").
		Where("p.source_type", contentSourceFeiNiu).
		Where("p.source_note_id<>?", sourceNoteId).
		Where("s.source_channel_id", sourceChannelId).
		Where("s.source_text_hash", sourceTextHash).
		OrderAsc("p.id").
		One()
	if err != nil {
		err = gerror.Wrap(err, "检查文本重复资料失败")
		return
	}
	if one == nil {
		return 0, nil
	}
	return one["id"].Int64(), nil
}

func (s *sSysContent) getDuplicateMediaByMD5(ctx context.Context, mediaType string, md5 string) (row gdb.Record, err error) {
	if mediaType != consts.ContentMediaTypeImage || md5 == "" {
		return nil, nil
	}
	row, err = g.DB().Model(contentTableMedia).Safe().Ctx(ctx).
		Fields("id,display_storage_path,preview_storage_path").
		Where("media_type", mediaType).
		Where("binary_md5", md5).
		Where("status", 1).
		OrderAsc("id").
		One()
	if err != nil {
		err = gerror.Wrap(err, "检查重复媒体失败")
	}
	return
}

func originalStoragePath(cosPath string, duplicateMediaId int64) string {
	if duplicateMediaId > 0 {
		return ""
	}
	return cosPath
}

func recordString(record gdb.Record, key string) string {
	if record == nil {
		return ""
	}
	value := record[key]
	if value == nil {
		return ""
	}
	return value.String()
}

func recordInt64(record gdb.Record, key string) int64 {
	if record == nil {
		return 0
	}
	value := record[key]
	if value == nil {
		return 0
	}
	return value.Int64()
}

func flagToInt(flag string) int {
	if flag == "Y" || flag == "y" || flag == "1" {
		return 1
	}
	return 0
}

type contentProfileRow struct {
	Id                   int64       `json:"id"`
	ProfileNo            string      `json:"profileNo"`
	Title                string      `json:"title"`
	Summary              string      `json:"summary"`
	PlainText            string      `json:"plainText"`
	Province             string      `json:"province"`
	City                 string      `json:"city"`
	Age                  int         `json:"age"`
	Height               int         `json:"height"`
	Weight               int         `json:"weight"`
	CupSize              string      `json:"cupSize"`
	HasVerificationVideo int         `json:"hasVerificationVideo"`
	MemberOnlyVideo      int         `json:"memberOnlyVideo"`
	ImageCount           int         `json:"imageCount"`
	VideoCount           int         `json:"videoCount"`
	Visibility           string      `json:"visibility"`
	PublishedAt          *gtime.Time `json:"publishedAt"`
}

func (row contentProfileRow) toListModel() *sysin.ContentProfileListModel {
	name := row.Title
	if name == "" {
		name = row.ProfileNo
	}
	return &sysin.ContentProfileListModel{
		Id:          row.Id,
		ProfileNo:   row.ProfileNo,
		Name:        name,
		Title:       row.Title,
		Summary:     row.Summary,
		Province:    row.Province,
		City:        row.City,
		Age:         row.Age,
		Height:      row.Height,
		Weight:      row.Weight,
		Cup:         row.CupSize,
		HasVideo:    row.VideoCount > 0,
		VideoLocked: row.VideoCount > 0 && row.MemberOnlyVideo == 1,
		Verified:    row.HasVerificationVideo == 1,
		PublishedAt: row.PublishedAt,
	}
}
