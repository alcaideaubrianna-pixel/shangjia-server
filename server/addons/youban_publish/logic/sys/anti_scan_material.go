package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/storager"
	"hotgo/internal/service"
)

const antiScanMaterialTable = "hg_youban_publish_anti_scan_material"

func (s *sSysPublish) AdminAntiScanMaterialList(ctx context.Context, in *sysin.AntiScanMaterialListInp) ([]*sysin.AntiScanMaterialModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if err = ensureAntiScanMaterialTable(ctx); err != nil {
		return nil, err
	}
	var list []*sysin.AntiScanMaterialModel
	err = g.DB().Model(antiScanMaterialTable).Safe().Ctx(ctx).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		Where("type", in.Type).
		WhereNull("deleted_at").
		Fields("id,type,name,url,created_at").
		OrderDesc("id").
		Scan(&list)
	return list, err
}

func (s *sSysPublish) AdminAntiScanMaterialUpload(ctx context.Context, in *sysin.AntiScanMaterialUploadInp, file *ghttp.UploadFile) (*sysin.AntiScanMaterialModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if file == nil {
		return nil, gerror.New("请选择素材图片")
	}
	if err = ensureAntiScanMaterialTable(ctx); err != nil {
		return nil, err
	}
	attachment, err := service.CommonUpload().UploadFile(ctx, storager.KindImg, file)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = file.Filename
	}
	now := gtime.Now()
	id, err := g.DB().Model(antiScanMaterialTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":  account.TenantId,
		"account_id": account.Id,
		"type":       in.Type,
		"name":       name,
		"url":        attachment.FileUrl,
		"created_at": now,
		"updated_at": now,
	}).InsertAndGetId()
	if err != nil {
		return nil, err
	}
	return &sysin.AntiScanMaterialModel{
		Id:        id,
		Type:      in.Type,
		Name:      name,
		Url:       attachment.FileUrl,
		CreatedAt: now.String(),
	}, nil
}

func ensureAntiScanMaterialTable(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return ensureAntiScanMaterialPgsql(ctx)
	}
	return ensureAntiScanMaterialMysql(ctx)
}

func ensureAntiScanMaterialPgsql(ctx context.Context) error {
	// 本地开发可能未执行插件升级 SQL，这里兜底创建 PostgreSQL 素材表。
	if _, err := g.DB().Exec(ctx, `
CREATE TABLE IF NOT EXISTS "hg_youban_publish_anti_scan_material" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "type" varchar(32) NOT NULL DEFAULT 'sticker',
  "name" varchar(120) NOT NULL DEFAULT '',
  "url" varchar(1024) NOT NULL DEFAULT '',
  "sort" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
`); err != nil {
		return err
	}
	_, err := g.DB().Exec(ctx, `CREATE INDEX IF NOT EXISTS "idx_ybp_anti_scan_material_owner" ON "hg_youban_publish_anti_scan_material" ("tenant_id", "account_id", "type", "deleted_at")`)
	return err
}

func ensureAntiScanMaterialMysql(ctx context.Context) error {
	// 本地开发可能未执行插件升级 SQL，这里兜底创建 MySQL 素材表。
	_, err := g.DB().Exec(ctx, `
CREATE TABLE IF NOT EXISTS `+"`"+`hg_youban_publish_anti_scan_material`+"`"+` (
  `+"`"+`id`+"`"+` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `+"`"+`tenant_id`+"`"+` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `+"`"+`account_id`+"`"+` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `+"`"+`type`+"`"+` varchar(32) NOT NULL DEFAULT 'sticker' COMMENT '素材类型',
  `+"`"+`name`+"`"+` varchar(120) NOT NULL DEFAULT '' COMMENT '素材名称',
  `+"`"+`url`+"`"+` varchar(1024) NOT NULL DEFAULT '' COMMENT '素材地址',
  `+"`"+`sort`+"`"+` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `+"`"+`created_at`+"`"+` datetime DEFAULT NULL COMMENT '创建时间',
  `+"`"+`updated_at`+"`"+` datetime DEFAULT NULL COMMENT '更新时间',
  `+"`"+`deleted_at`+"`"+` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`+"`"+`id`+"`"+`),
  KEY `+"`"+`idx_ybp_anti_scan_material_owner`+"`"+` (`+"`"+`tenant_id`+"`"+`,`+"`"+`account_id`+"`"+`,`+"`"+`type`+"`"+`,`+"`"+`deleted_at`+"`"+`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴防扫图素材库';
`)
	return err
}
