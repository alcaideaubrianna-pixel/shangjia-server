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

func (s *sSysPublish) tagList(ctx context.Context, in *sysin.TagListInp, operatorId int64, isAdmin bool) (list []*sysin.TagModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.TagListInp{}
	}
	mod := g.DB().Model(publishTagTable+" t").Safe().Ctx(ctx).
		LeftJoin(publishAccountTable+" a", "a.id=t.created_by AND a.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" tenant", "tenant.id=a.tenant_id AND tenant.deleted_at IS NULL").
		WhereNull("t.deleted_at")
	if !isAdmin {
		mod = mod.Where("(t.review_status = ? OR t.created_by = ?)", sysin.PublishTagReviewApproved, operatorId)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		mod = mod.WhereLike("t.name", "%"+keyword+"%")
	}
	if in.ReviewStatus != "" {
		mod = mod.Where("t.review_status", in.ReviewStatus)
	}
	if in.Status > 0 {
		mod = mod.Where("t.status", in.Status)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计标签失败")
	}
	if totalCount == 0 {
		return []*sysin.TagModel{}, 0, nil
	}
	fields := "t.id,t.name,t.review_status,t.status,t.use_count,t.created_by,t.created_at,t.updated_at,a.username AS creator_username,a.tenant_id AS creator_tenant_id,tenant.name AS creator_tenant_name"
	if err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("t.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取标签列表失败")
	}
	return
}

func (s *sSysPublish) saveTag(ctx context.Context, in *sysin.TagSaveInp, operatorId int64, isAdmin bool) (err error) {
	if in == nil {
		return gerror.New("标签信息不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	if !isAdmin {
		in.ReviewStatus = sysin.PublishTagReviewPending
		in.Status = 1
	}
	now := gtime.Now()
	names := splitTagNames(in.Name)
	if len(names) == 0 {
		return gerror.New("标签名称不能为空")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, name := range names {
			if err := s.saveOneTag(ctx, tx, name, in.ReviewStatus, in.Status, operatorId, isAdmin, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *sSysPublish) saveOneTag(ctx context.Context, tx gdb.TX, name string, reviewStatus string, status int, operatorId int64, isAdmin bool, now *gtime.Time) error {
	existing, err := tx.Model(publishTagTable).Safe().Ctx(ctx).Where("name", name).WhereNull("deleted_at").Fields("id,created_by,review_status").One()
	if err != nil {
		return gerror.Wrap(err, "检查标签失败")
	}
	if !existing.IsEmpty() && !isAdmin && existing["created_by"].Int64() != operatorId {
		return gerror.New("标签已存在，请等待审核或选择已有标签")
	}
	data := g.Map{
		"name":          name,
		"review_status": reviewStatus,
		"status":        status,
		"updated_by":    operatorId,
		"updated_at":    now,
	}
	if !existing.IsEmpty() {
		_, err = tx.Model(publishTagTable).Safe().Ctx(ctx).Where("id", existing["id"].Int64()).Data(data).Update()
	} else {
		data["created_by"] = operatorId
		data["created_at"] = now
		_, err = tx.Model(publishTagTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存标签失败")
	}
	s.clearProfileTagNameCache(ctx)
	return nil
}

func splitTagNames(name string) []string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == ',' || r == '，'
	})
	list := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		list = append(list, item)
	}
	return list
}

func (s *sSysPublish) deleteTags(ctx context.Context, in *sysin.TagDeleteInp, operatorId int64, isAdmin bool) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的标签")
	}
	mod := g.DB().Model(publishTagTable).Safe().Ctx(ctx).WhereIn("id", in.Ids).WhereNull("deleted_at")
	if !isAdmin {
		mod = mod.Where("created_by", operatorId)
	}
	result, err := mod.Data(g.Map{
		"deleted_by": operatorId,
		"deleted_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除标签失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认标签删除结果失败")
	}
	if affected == 0 {
		return gerror.New("标签不存在或无权删除")
	}
	s.clearProfileTagNameCache(ctx)
	return nil
}
