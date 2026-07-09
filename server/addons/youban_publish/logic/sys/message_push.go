package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

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
	if in.Id > 0 {
		if err = s.ensureMessageTemplatesBelongTenant(ctx, []int64{in.Id}, account.TenantId); err != nil {
			return nil, err
		}
	}
	now := gtime.Now()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		data := g.Map{
			"tenant_id":   account.TenantId,
			"name":        in.Name,
			"text":        in.Text,
			"media_count": len(in.Media),
			"status":      in.Status,
			"updated_by":  account.Id,
			"updated_at":  now,
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
				"template_id":         in.Id,
				"tenant_id":           account.TenantId,
				"media_type":          item.MediaType,
				"name":                strings.TrimSpace(item.Name),
				"file_url":            strings.TrimSpace(item.FileUrl),
				"storage_path":        strings.TrimSpace(item.StoragePath),
				"poster_url":          strings.TrimSpace(item.PosterUrl),
				"poster_storage_path": strings.TrimSpace(item.PosterStoragePath),
				"tg_file_id":          strings.TrimSpace(item.TgFileId),
				"tg_thumb_file_id":    strings.TrimSpace(item.TgThumbFileId),
				"asset_hash":          messageMediaAssetHash(item),
				"sort_index":          item.SortIndex,
				"created_at":          now,
				"updated_at":          now,
			}).Insert(); err != nil {
				return gerror.Wrap(err, "保存消息模板媒体失败")
			}
		}
		return nil
	})
	if err != nil {
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
	return nil
}

func (s *sSysPublish) messageTemplate(ctx context.Context, id int64, tenantId int64) (*sysin.MessageTemplateModel, error) {
	var template *sysin.MessageTemplateModel
	if err := g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).
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
	count, err := g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查消息模板权限失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作的消息模板")
	}
	return nil
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
