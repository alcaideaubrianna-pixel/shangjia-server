package crons

import (
	"context"
	"hotgo/internal/library/cron"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

func init() {
	cron.Register(ContentImportFeiNiu)
}

// ContentImportFeiNiu 每 30 分钟从 FeiNiu_bot 增量导入内容。
// 后台定时任务中配置任务名称 content_import_feiniu，cron 表达式 */30 * * * *。
var ContentImportFeiNiu = &cContentImportFeiNiu{name: "content_import_feiniu"}

type cContentImportFeiNiu struct {
	name string
}

func (c *cContentImportFeiNiu) GetName() string {
	return c.name
}

func (c *cContentImportFeiNiu) Execute(ctx context.Context, parser *cron.Parser) (err error) {
	res, err := service.SysContent().ImportFeiNiu(ctx, &sysin.ContentImportFeiNiuInp{TriggerType: "cron"})
	if err != nil {
		parser.Logger.Warningf(ctx, "content import feiniu failed:%+v", err)
		return err
	}
	parser.Logger.Infof(ctx, "content import feiniu scanned:%d imported:%d duplicate:%d media:%d last:%d",
		res.Scanned, res.Imported, res.Duplicate, res.MediaImported, res.LastSourceNote)
	return
}
