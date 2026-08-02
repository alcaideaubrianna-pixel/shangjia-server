package sys

import (
	"context"
	"strings"

	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const publishTenantTable = "hg_youban_publish_tenant"

func (s *sSysBot) AdminAccountBindList(ctx context.Context, in *sysin.AccountBindListInp) (list []*sysin.AccountBindModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.AccountBindListInp{}
	}
	mod := g.DB().Model(accountBindTbl+" ab").Safe().Ctx(ctx).
		LeftJoin(botTable+" b", "b.id=ab.bot_id").
		LeftJoin(publishAccountTable+" pa", "pa.id=ab.account_id AND ab.app='"+sysin.BotAppApi+"' AND pa.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" pt", "pt.id=pa.tenant_id AND pt.deleted_at IS NULL").
		LeftJoin(dao.AdminMember.Table()+" am", "am.id=ab.account_id AND ab.app='"+sysin.BotAppAdmin+"'").
		Fields("ab.id,ab.app,ab.account_id,COALESCE(pa.username,am.username,'') AS account_username,COALESCE(pa.nickname,am.real_name,'') AS account_name,COALESCE(pa.tenant_id,0) AS tenant_id,COALESCE(pt.name,'') AS tenant_name,ab.telegram_user_id,ab.telegram_username,ab.telegram_first_name,ab.telegram_last_name,ab.bot_id,COALESCE(b.bot_username,'') AS bot_username,ab.status,ab.created_at,ab.updated_at").
		WhereNull("ab.deleted_at")
	if in.BotId > 0 {
		mod = mod.Where("ab.bot_id", in.BotId)
	}
	if app := strings.TrimSpace(strings.ToLower(in.App)); app != "" {
		if app != sysin.BotAppAdmin && app != sysin.BotAppApi {
			return nil, 0, gerror.New("绑定应用不合法")
		}
		mod = mod.Where("ab.app", app)
	}
	if in.Status > 0 {
		if in.Status != consts.StatusEnabled && in.Status != consts.StatusDisable {
			return nil, 0, gerror.New("绑定状态不合法")
		}
		mod = mod.Where("ab.status", in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(ab.telegram_user_id LIKE ? OR ab.telegram_username LIKE ? OR ab.telegram_first_name LIKE ? OR ab.telegram_last_name LIKE ? OR pa.username LIKE ? OR pa.nickname LIKE ? OR pt.name LIKE ? OR am.username LIKE ? OR am.real_name LIKE ?)", like, like, like, like, like, like, like, like, like)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("ab.id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG绑定列表失败")
	}
	return
}

func (s *sSysBot) AdminAccountBindUnbind(ctx context.Context, in *sysin.AccountBindUnbindInp) error {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要解绑的记录")
	}
	result, err := g.DB().Model(accountBindTbl).Safe().Ctx(ctx).
		WhereIn("id", in.Ids).
		Where("status", consts.StatusEnabled).
		WhereNull("deleted_at").
		Data(g.Map{
			"status":     consts.StatusDisable,
			"updated_at": gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "解绑TG账号失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return gerror.New("绑定记录不存在或已经解绑")
	}
	return nil
}
