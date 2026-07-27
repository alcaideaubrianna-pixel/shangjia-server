package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/service"
)

func (s *sSysPublish) ServerProfilePurgeDeleted(ctx context.Context, in *sysin.ProfilePurgeDeletedInp) (res *sysin.ProfilePurgeDeletedModel, err error) {
	if err = s.requireSystemSuperAdmin(ctx); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("清理参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if err = s.ensureTenant(ctx, in.TenantId); err != nil {
		return nil, err
	}
	if err = s.ensureAccountBelongsTenant(ctx, in.AccountId, in.TenantId); err != nil {
		return nil, err
	}

	profileIds, err := s.deletedProfileIdsByAccount(ctx, in.TenantId, in.AccountId)
	if err != nil {
		return nil, err
	}
	res = &sysin.ProfilePurgeDeletedModel{DeletedCount: len(profileIds)}
	if len(profileIds) == 0 {
		return res, nil
	}

	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		deleteTables := []string{
			dao.ContentMedia.Table(),
			dao.ContentSourceMap.Table(),
			publishMediaPHashBucketTable,
			publishMediaPHashLshTable,
			publishMediaTable,
			publishNoteIndexTable,
			publishProfileStateTable,
			publishChannelProfileTable,
		}
		for _, table := range deleteTables {
			if _, deleteErr := tx.Model(table).Safe().Ctx(ctx).WhereIn("profile_id", profileIds).Delete(); deleteErr != nil {
				return gerror.Wrapf(deleteErr, "清理软删除资料关联失败 table:%s", table)
			}
		}
		if _, updateErr := tx.Model(pdao.YoubanPublishMaterialImportGroup.Table()).Safe().Ctx(ctx).
			WhereIn("profile_id", profileIds).
			Data(g.Map{"profile_id": 0, "task_profile_id": 0}).
			Update(); updateErr != nil {
			return gerror.Wrap(updateErr, "清理TG导入资料关联失败")
		}
		if _, deleteErr := tx.Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
			WhereIn("id", profileIds).
			WhereNotNull("deleted_at").
			Delete(); deleteErr != nil {
			return gerror.Wrap(deleteErr, "清理软删除资料失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return res, nil
}

func (s *sSysPublish) deletedProfileIdsByAccount(ctx context.Context, tenantId int64, accountId int64) ([]int64, error) {
	profileTable := dao.ContentProfile.Table()
	rows, err := g.DB().Model(profileTable+" p").Safe().Ctx(ctx).
		Fields("p.id").
		WhereNotNull("p.deleted_at").
		Where("EXISTS (SELECT 1 FROM "+publishProfileStateTable+" ps WHERE ps.profile_id=p.id AND ps.tenant_id=? AND ps.account_id=?)", tenantId, accountId).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取账号软删除资料失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row["id"].Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	return uniqueIds(ids), nil
}
