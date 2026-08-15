package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) AdminMessageTemplateList(ctx context.Context, in *sysin.MessageTemplateListInp) (list []*sysin.MessageTemplateModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.MessageTemplateListInp{}
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).
		Where("tenant_id", account.TenantId).
		WhereNull("deleted_at")
	if in.Keyword != "" {
		keyword := "%" + strings.TrimSpace(in.Keyword) + "%"
		mod = mod.Where("(name LIKE ? OR text LIKE ?)", keyword, keyword)
	}
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取消息模板总数失败")
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取消息模板列表失败")
	}
	if list == nil {
		list = []*sysin.MessageTemplateModel{}
	}
	if err = s.fillMessageTemplateMedia(ctx, list); err != nil {
		return nil, 0, err
	}
	return
}

func (s *sSysPublish) AdminMessageTemplateSave(ctx context.Context, in *sysin.MessageTemplateSaveInp) (res *sysin.MessageTemplateSaveModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("消息模板不能为空")
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return nil, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if err = in.NormalizeButtonConfig(); err != nil {
		return nil, err
	}
	if len(in.Media) > 1 && in.ButtonConfig != "" {
		in.ButtonConfig = ""
	}
	if in.Id > 0 {
		if err = s.ensureMessageTemplatesBelongTenant(ctx, []int64{in.Id}, account.TenantId); err != nil {
			return nil, err
		}
	}
	now := gtime.Now()
	previousSourceRecordIds := make([]int64, 0)
	serialNo := ""
	if in.Id <= 0 {
		serialNo, err = s.ensureInlineTemplateSerial(ctx)
		if err != nil {
			return nil, err
		}
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var storedTemplate *sysin.MessageTemplateModel
		var storedMedia []*sysin.MessageTemplateMediaModel
		if in.Id > 0 {
			if err = tx.Model(messageTemplateTable).Ctx(ctx).
				Fields("id,text,source_message_record_id").
				Where("id", in.Id).
				Where("tenant_id", account.TenantId).
				WhereNull("deleted_at").
				Scan(&storedTemplate); err != nil {
				return gerror.Wrap(err, "读取消息模板原内容失败")
			}
			if err = tx.Model(messageMediaTable).Ctx(ctx).
				Where("template_id", in.Id).
				Where("tenant_id", account.TenantId).
				Scan(&storedMedia); err != nil {
				return gerror.Wrap(err, "读取消息模板原媒体失败")
			}
			mergeStoredMessageTemplateMedia(in.Media, storedMedia)
			previousSourceRecordIds = messageTemplateStoredSourceRecordIds(storedTemplate, storedMedia)
		}
		sourceMessageRecordId := int64(0)
		if messageTemplateSourceContentUnchanged(storedTemplate, storedMedia, in) {
			sourceMessageRecordId = storedTemplate.SourceMessageRecordId
		} else {
			for _, item := range in.Media {
				if item != nil {
					item.SourceMessageRecordId = 0
				}
			}
		}
		data := g.Map{
			"tenant_id":                account.TenantId,
			"push_mode":                in.PushMode,
			"name":                     in.Name,
			"text":                     in.Text,
			"media_count":              len(in.Media),
			"status":                   in.Status,
			"source_message_record_id": sourceMessageRecordId,
			"button_config":            in.ButtonConfig,
			"updated_by":               account.Id,
			"updated_at":               now,
		}
		if in.Id > 0 {
			if _, err = tx.Model(messageTemplateTable).Ctx(ctx).
				Where("id", in.Id).
				Where("tenant_id", account.TenantId).
				WhereNull("deleted_at").
				Data(data).
				Update(); err != nil {
				return gerror.Wrap(err, "更新消息模板失败")
			}
		} else {
			data["serial_no"] = serialNo
			data["created_by"] = account.Id
			data["created_at"] = now
			in.Id, err = tx.Model(messageTemplateTable).Ctx(ctx).Data(data).InsertAndGetId()
			if err != nil {
				return gerror.Wrap(err, "新增消息模板失败")
			}
		}
		if _, err = tx.Model(messageMediaTable).Ctx(ctx).Where("template_id", in.Id).Delete(); err != nil {
			return gerror.Wrap(err, "清理消息模板媒体失败")
		}
		for _, item := range in.Media {
			if item == nil {
				continue
			}
			if _, err = tx.Model(messageMediaTable).Ctx(ctx).Data(g.Map{
				"template_id":              in.Id,
				"tenant_id":                account.TenantId,
				"source_message_record_id": item.SourceMessageRecordId,
				"media_type":               item.MediaType,
				"name":                     strings.TrimSpace(item.Name),
				"file_url":                 strings.TrimSpace(item.FileUrl),
				"storage_path":             strings.TrimSpace(item.StoragePath),
				"poster_url":               strings.TrimSpace(item.PosterUrl),
				"poster_storage_path":      strings.TrimSpace(item.PosterStoragePath),
				"tg_file_id":               strings.TrimSpace(item.TgFileId),
				"tg_thumb_file_id":         strings.TrimSpace(item.TgThumbFileId),
				"asset_hash":               messageMediaAssetHash(item),
				"sort_index":               item.SortIndex,
				"created_at":               now,
				"updated_at":               now,
			}).Insert(); err != nil {
				return gerror.Wrap(err, "保存消息模板媒体失败")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err = s.releaseUnusedStoredMessageRecords(ctx, previousSourceRecordIds); err != nil {
		return nil, err
	}
	return &sysin.MessageTemplateSaveModel{Id: in.Id}, nil
}

func (s *sSysPublish) AdminMessageTemplateDelete(ctx context.Context, in *sysin.MessageTemplateDeleteInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的消息模板")
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return err
	}
	in.Ids = uniqueIds(in.Ids)
	if err = s.ensureMessageTemplatesBelongTenant(ctx, in.Ids, account.TenantId); err != nil {
		return err
	}
	var templates []*sysin.MessageTemplateModel
	if err = g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).WhereIn("id", in.Ids).Where("tenant_id", account.TenantId).WhereNull("deleted_at").Scan(&templates); err != nil {
		return gerror.Wrap(err, "读取待删除消息模板来源失败")
	}
	if err = s.fillMessageTemplateMedia(ctx, templates); err != nil {
		return err
	}
	sourceRecordIds := make([]int64, 0)
	for _, template := range templates {
		if template != nil {
			sourceRecordIds = append(sourceRecordIds, messageTemplateStoredSourceRecordIds(template, template.Media)...)
		}
	}
	_, err = g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).
		WhereIn("id", in.Ids).
		Where("tenant_id", account.TenantId).
		Data(g.Map{
			"deleted_by": account.Id,
			"deleted_at": gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "删除消息模板失败")
	}
	return s.releaseUnusedStoredMessageRecords(ctx, sourceRecordIds)
}

func (s *sSysPublish) messageTemplate(ctx context.Context, id int64, tenantId int64) (*sysin.MessageTemplateModel, error) {
	var template *sysin.MessageTemplateModel
	if err := g.DB().Model(messageTemplateTable).Unscoped().Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Scan(&template); err != nil {
		return nil, gerror.Wrap(err, "读取消息模板失败")
	}
	if template == nil || template.Id <= 0 {
		return nil, gerror.New("消息模板不存在")
	}
	list := []*sysin.MessageTemplateModel{template}
	if err := s.fillMessageTemplateMedia(ctx, list); err != nil {
		return nil, err
	}
	return template, nil
}

func (s *sSysPublish) fillMessageTemplateMedia(ctx context.Context, list []*sysin.MessageTemplateModel) error {
	ids := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil && item.Id > 0 {
			ids = append(ids, item.Id)
		}
	}
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return nil
	}
	var media []*sysin.MessageTemplateMediaModel
	if err := g.DB().Model(messageMediaTable).Safe().Ctx(ctx).
		WhereIn("template_id", ids).
		OrderAsc("sort_index").
		OrderAsc("id").
		Scan(&media); err != nil {
		return gerror.Wrap(err, "读取消息模板媒体失败")
	}
	buckets := make(map[int64][]*sysin.MessageTemplateMediaModel, len(ids))
	for _, item := range media {
		if item == nil {
			continue
		}
		buckets[item.TemplateId] = append(buckets[item.TemplateId], item)
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		item.Media = buckets[item.Id]
		if item.Media == nil {
			item.Media = []*sysin.MessageTemplateMediaModel{}
		}
	}
	return nil
}

func (s *sSysPublish) ensureMessageTemplatesBelongTenant(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return gerror.New("请选择消息模板")
	}
	var validRows []struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).
		Fields("id").
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Scan(&validRows)
	if err != nil {
		return gerror.Wrap(err, "检查消息模板权限失败")
	}
	valid := make(map[int64]struct{}, len(validRows))
	for _, row := range validRows {
		valid[row.Id] = struct{}{}
	}
	invalid := make([]int64, 0)
	for _, id := range ids {
		if _, ok := valid[id]; !ok {
			invalid = append(invalid, id)
		}
	}
	if len(invalid) > 0 {
		return gerror.Newf("存在无效或无权操作的消息模板，模板ID：%v", invalid)
	}
	return nil
}

func (s *sSysPublish) filterDeletedMessageTemplates(ctx context.Context, ids []int64, tenantId int64) ([]int64, error) {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return nil, gerror.New("请选择消息模板")
	}
	var rows []struct {
		Id        int64       `json:"id"`
		DeletedAt *gtime.Time `json:"deleted_at"`
	}
	if err := g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).
		Fields("id, deleted_at").
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取消息模板状态失败")
	}
	active := make(map[int64]struct{}, len(rows))
	deleted := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		if row.DeletedAt != nil {
			deleted[row.Id] = struct{}{}
			continue
		}
		active[row.Id] = struct{}{}
	}
	filtered := make([]int64, 0, len(active))
	invalid := make([]int64, 0)
	for _, id := range ids {
		if _, ok := active[id]; ok {
			filtered = append(filtered, id)
			continue
		}
		if _, ok := deleted[id]; !ok {
			invalid = append(invalid, id)
		}
	}
	if len(invalid) > 0 {
		return nil, gerror.Newf("存在无效或无权操作的消息模板，模板ID：%v", invalid)
	}
	if len(filtered) == 0 {
		return nil, gerror.New("请选择推送模板")
	}
	return filtered, nil
}

func messageMediaAssetHash(item *sysin.MessageTemplateMediaInp) string {
	if item == nil {
		return ""
	}
	for _, value := range []string{item.AssetHash, item.StoragePath, item.FileUrl, item.TgFileId} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func mergeStoredMessageTemplateMedia(input []*sysin.MessageTemplateMediaInp, stored []*sysin.MessageTemplateMediaModel) {
	storedById := make(map[int64]*sysin.MessageTemplateMediaModel, len(stored))
	for _, item := range stored {
		if item != nil && item.Id > 0 {
			storedById[item.Id] = item
		}
	}
	for _, item := range input {
		if item == nil || item.Id <= 0 {
			continue
		}
		old := storedById[item.Id]
		if old == nil {
			continue
		}
		if strings.TrimSpace(item.FileUrl) == "" {
			item.FileUrl = old.FileUrl
		}
		if strings.TrimSpace(item.StoragePath) == "" {
			item.StoragePath = old.StoragePath
		}
		if strings.TrimSpace(item.PosterUrl) == "" {
			item.PosterUrl = old.PosterUrl
		}
		if strings.TrimSpace(item.PosterStoragePath) == "" {
			item.PosterStoragePath = old.PosterStoragePath
		}
		if strings.TrimSpace(item.TgFileId) == "" {
			item.TgFileId = old.TgFileId
		}
		if strings.TrimSpace(item.TgThumbFileId) == "" {
			item.TgThumbFileId = old.TgThumbFileId
		}
		if strings.TrimSpace(item.AssetHash) == "" {
			item.AssetHash = old.AssetHash
		}
		if item.SourceMessageRecordId <= 0 {
			item.SourceMessageRecordId = old.SourceMessageRecordId
		}
	}
}

func messageTemplateSourceContentUnchanged(storedTemplate *sysin.MessageTemplateModel, storedMedia []*sysin.MessageTemplateMediaModel, input *sysin.MessageTemplateSaveInp) bool {
	if storedTemplate == nil || storedTemplate.SourceMessageRecordId <= 0 || input == nil || strings.TrimSpace(storedTemplate.Text) != strings.TrimSpace(input.Text) || len(storedMedia) != len(input.Media) {
		return false
	}
	storedById := make(map[int64]*sysin.MessageTemplateMediaModel, len(storedMedia))
	for _, item := range storedMedia {
		if item != nil && item.Id > 0 {
			storedById[item.Id] = item
		}
	}
	for _, item := range input.Media {
		if item == nil || item.Id <= 0 {
			return false
		}
		old := storedById[item.Id]
		if old == nil ||
			strings.TrimSpace(item.MediaType) != strings.TrimSpace(old.MediaType) ||
			messageTemplateMediaSource(item.FileUrl, item.StoragePath) != messageTemplateMediaSource(old.FileUrl, old.StoragePath) ||
			(strings.TrimSpace(item.MediaType) == "video" && messageTemplateMediaSource(item.PosterUrl, item.PosterStoragePath) != messageTemplateMediaSource(old.PosterUrl, old.PosterStoragePath)) ||
			item.SortIndex != old.SortIndex {
			return false
		}
	}
	return true
}

func messageTemplateMediaSource(fileUrl string, storagePath string) string {
	if value := strings.TrimSpace(storagePath); value != "" {
		return strings.TrimPrefix(value, "/")
	}
	return strings.TrimSpace(fileUrl)
}

func messageTemplateStoredSourceRecordIds(template *sysin.MessageTemplateModel, media []*sysin.MessageTemplateMediaModel) []int64 {
	ids := make([]int64, 0, len(media)+1)
	if template != nil && template.SourceMessageRecordId > 0 {
		ids = append(ids, template.SourceMessageRecordId)
	}
	for _, item := range media {
		if item != nil && item.SourceMessageRecordId > 0 {
			ids = append(ids, item.SourceMessageRecordId)
		}
	}
	return uniqueIds(ids)
}

func (s *sSysPublish) releaseUnusedStoredMessageRecords(ctx context.Context, ids []int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return nil
	}
	referenced := make(map[int64]struct{}, len(ids))
	var templateRefs []struct {
		SourceMessageRecordId int64 `json:"sourceMessageRecordId"`
	}
	if err := g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).
		Fields("source_message_record_id").
		WhereIn("source_message_record_id", ids).
		WhereNull("deleted_at").
		Scan(&templateRefs); err != nil {
		return gerror.Wrap(err, "检查消息模板来源引用失败")
	}
	for _, item := range templateRefs {
		referenced[item.SourceMessageRecordId] = struct{}{}
	}
	var mediaRefs []struct {
		SourceMessageRecordId int64 `json:"sourceMessageRecordId"`
	}
	if err := g.DB().Model(messageMediaTable+" m").Safe().Ctx(ctx).
		InnerJoin(messageTemplateTable+" t", "t.id=m.template_id AND t.deleted_at IS NULL").
		Fields("m.source_message_record_id").
		WhereIn("m.source_message_record_id", ids).
		Scan(&mediaRefs); err != nil {
		return gerror.Wrap(err, "检查消息模板媒体来源引用失败")
	}
	for _, item := range mediaRefs {
		referenced[item.SourceMessageRecordId] = struct{}{}
	}
	unused := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := referenced[id]; !ok {
			unused = append(unused, id)
		}
	}
	return botService.SysBot().ReleaseStoredMessages(ctx, unused)
}
