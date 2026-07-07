package sys

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
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

func (s *sSysPublish) previewAccountProfileNo(ctx context.Context, tenantId int64, accountId int64, source string) (string, error) {
	if source == "random" {
		return randomProfileNo()
	}
	count, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereGT("profile_id", 0).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return "", gerror.Wrap(err, "生成账号资料编号预览失败")
	}
	return fmt.Sprintf("%03d", count+1), nil
}

func accountSettingPreviewMark(setting *sysin.AccountSettingModel, accountName string, profileNo string) string {
	if setting == nil || setting.EnableTitleMark != 1 || profileNo == "" {
		return ""
	}
	if setting.NumberSource == "random" {
		return profileNo
	}
	prefix := setting.CustomMarkText
	if setting.MarkMode != "custom" || prefix == "" {
		prefix = accountName
	}
	if prefix == "" {
		return profileNo
	}
	return prefix + profileNo
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
	for i := count + 1; i <= count+1000; i++ {
		code := fmt.Sprintf("%03d", i)
		exists, existsErr := s.accountProfileNoExists(ctx, tx, tenantId, accountId, code)
		if existsErr != nil {
			return "", existsErr
		}
		if !exists {
			return code, nil
		}
	}
	return "", gerror.New("生成资料编号失败，请重试")
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
		Unscoped().
		Where("p."+profileColumns.ProfileNo, profileNo).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查资料编号失败")
	}
	return count > 0, nil
}
