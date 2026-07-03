package sys

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/internal/dao"
)

func (s *sSysPublish) nextAccountProfileNo(ctx context.Context, tx gdb.TX, tenantId int64, accountId int64) (string, error) {
	setting, err := s.accountSetting(ctx, tenantId, accountId)
	if err != nil {
		return "", err
	}
	if setting.NumberSource == "random" {
		return s.nextRandomProfileNo(ctx, tx, tenantId, accountId)
	}
	return s.nextSequenceProfileNo(ctx, tx, tenantId, accountId)
}

func (s *sSysPublish) nextSequenceProfileNo(ctx context.Context, tx gdb.TX, tenantId int64, accountId int64) (string, error) {
	count, err := tx.Model(publishTaskTable).Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereGT("profile_id", 0).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return "", gerror.Wrap(err, "生成账号资料编号失败")
	}
	return fmt.Sprintf("%03d", count+1), nil
}

func (s *sSysPublish) nextRandomProfileNo(ctx context.Context, tx gdb.TX, tenantId int64, accountId int64) (string, error) {
	for i := 0; i < 10; i++ {
		code, err := randomProfileNo()
		if err != nil {
			return "", err
		}
		exists, err := s.accountProfileNoExists(ctx, tx, tenantId, accountId, code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", gerror.New("生成随机资料编号失败，请重试")
}

func randomProfileNo() (string, error) {
	letters := make([]byte, 2)
	for i := range letters {
		n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(26))
		if err != nil {
			return "", gerror.Wrap(err, "生成随机资料编号失败")
		}
		letters[i] = byte('A' + n.Int64())
	}
	number, err := cryptorand.Int(cryptorand.Reader, big.NewInt(900))
	if err != nil {
		return "", gerror.Wrap(err, "生成随机资料编号失败")
	}
	return fmt.Sprintf("%s%03d", string(letters), number.Int64()+100), nil
}

func (s *sSysPublish) accountProfileNoExists(ctx context.Context, tx gdb.TX, tenantId int64, accountId int64, profileNo string) (bool, error) {
	profileColumns := dao.ContentProfile.Columns()
	count, err := tx.Model(dao.ContentProfile.Table()+" p").Ctx(ctx).
		LeftJoin(publishTaskTable+" t", "t.profile_id=p.id AND t.deleted_at IS NULL").
		Where("t.tenant_id", tenantId).
		Where("t.account_id", accountId).
		Where("p."+profileColumns.ProfileNo, profileNo).
		WhereNull("p." + profileColumns.DeletedAt).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查资料编号失败")
	}
	return count > 0, nil
}
