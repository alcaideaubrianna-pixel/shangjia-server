package sys

import (
	"context"

	"hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/errors/gerror"
)

// RelayOwnedBots excludes tenant-shared and global Bots: the account must have
// created this Bot itself. Lock is used only inside the caller's transaction.
func (s *sSysPublish) RelayOwnedBots(ctx context.Context, tenantID, accountID, botID int64, lock bool) ([]*sysin.RelayOwnedBotModel, error) {
	if tenantID <= 0 || accountID <= 0 {
		return nil, gerror.New("租户账户无效")
	}
	c := dao.YoubanPublishBot.Columns()
	q := dao.YoubanPublishBot.Ctx(ctx).Where(c.TenantId, tenantID).Where(c.AccountId, accountID).Where(c.CreatedBy, accountID).Where(c.Status, 1).Where(c.BotType, consts.PublishBotTypeTwoWay)
	ac := dao.YoubanPublishAccount.Columns()
	q = q.Where("EXISTS (SELECT 1 FROM "+dao.YoubanPublishAccount.Table()+" owner_account WHERE owner_account."+ac.Id+"=? AND owner_account."+ac.TenantId+"=? AND owner_account."+ac.Status+"=1 AND owner_account."+ac.DeletedAt+" IS NULL)", accountID, tenantID)
	if botID > 0 {
		q = q.Where(c.Id, botID)
	}
	if lock {
		q = q.LockUpdate()
	}
	var rows []*sysin.RelayOwnedBotModel
	err := q.OrderAsc(c.Id).Limit(100).Scan(&rows)
	return rows, gerror.Wrap(err, "读取自己创建的Bot失败")
}
