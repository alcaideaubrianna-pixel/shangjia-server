package sys

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

var profileNoFormatRegexp = regexp.MustCompile(`^[A-Z][0-9]{5}$`)

func (s *sSysPublish) nextAccountProfileNo(ctx context.Context, tx gdb.TX, tenantId int64, accountId int64) (string, error) {
	// 资料编号统一采用“随机大写字母 + 五位数字”，例如 A00001。
	// 保留 tenantId/accountId 参数用于兼容旧调用链，唯一性检查仍保持全局唯一。
	return s.nextRandomProfileNo(ctx, tx, tenantId, accountId)
}

func (s *sSysPublish) previewAccountProfileNo(ctx context.Context, tenantId int64, accountId int64, source string) (string, error) {
	if source != "random" {
		return "001", nil
	}
	return randomProfileNo()
}

func accountSettingPreviewMark(setting *sysin.AccountSettingModel, accountName string, profileNo string) string {
	if setting == nil || setting.EnableTitleMark != 1 || profileNo == "" {
		return ""
	}
	if setting.NumberSource == "random" {
		return profileNo
	}
	if sequence := strings.TrimSpace(profileNo); sequence != "" {
		return markPrefix(setting, accountName) + sequence
	}
	return ""
}

func markPrefix(setting *sysin.AccountSettingModel, accountName string) string {
	prefix := setting.CustomMarkText
	if setting.MarkMode != "custom" || prefix == "" {
		prefix = accountName
	}
	return strings.TrimSpace(prefix)
}

func (s *sSysPublish) nextSequenceProfileNo(ctx context.Context, tx gdb.TX, tenantId int64, accountId int64) (string, error) {
	_ = tenantId
	_ = accountId
	maxNumber := 26 * 99999
	for i := 0; i < 1000; i++ {
		count, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Unscoped().Count()
		if err != nil {
			return "", gerror.Wrap(err, "生成资料编号失败")
		}
		seq := count + i + 1
		if seq > maxNumber {
			return "", gerror.New("资料编号已用尽")
		}
		code := formatProfileNo(seq)
		exists, existsErr := s.accountProfileNoExists(ctx, tx, 0, 0, code)
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
	letter, err := cryptorand.Int(cryptorand.Reader, big.NewInt(26))
	if err != nil {
		return "", gerror.Wrap(err, "生成随机资料编号失败")
	}
	number, err := cryptorand.Int(cryptorand.Reader, big.NewInt(99999))
	if err != nil {
		return "", gerror.Wrap(err, "生成随机资料编号失败")
	}
	return fmt.Sprintf("%c%05d", byte('A'+letter.Int64()), number.Int64()+1), nil
}

func formatProfileNo(seq int) string {
	if seq <= 0 {
		seq = 1
	}
	letterIndex := (seq - 1) / 99999
	number := (seq-1)%99999 + 1
	if letterIndex > 25 {
		letterIndex = 25
	}
	return fmt.Sprintf("%c%05d", byte('A'+letterIndex), number)
}

func previewGlobalProfileNo(ctx context.Context) (string, error) {
	count, err := g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).Unscoped().Count()
	if err != nil {
		return "", gerror.Wrap(err, "生成资料编号预览失败")
	}
	return formatProfileNo(count + 1), nil
}

func (s *sSysPublish) ensureLegacyProfileNos(ctx context.Context, tx gdb.TX) error {
	profileColumns := dao.ContentProfile.Columns()
	rows, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).
		Unscoped().
		Fields(profileColumns.Id, profileColumns.ProfileNo).
		OrderAsc(profileColumns.Id).
		Limit(500).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取旧资料编号失败")
	}
	for _, row := range rows {
		id := row[profileColumns.Id].Int64()
		current := row[profileColumns.ProfileNo].String()
		if id <= 0 || profileNoFormatRegexp.MatchString(current) {
			continue
		}
		var next string
		for i := 0; i < 20; i++ {
			code, genErr := randomProfileNo()
			if genErr != nil {
				return genErr
			}
			exists, existsErr := s.accountProfileNoExists(ctx, tx, 0, 0, code)
			if existsErr != nil {
				return existsErr
			}
			if !exists {
				next = code
				break
			}
		}
		if next == "" {
			return gerror.New("迁移旧资料编号失败，请重试")
		}
		if _, err = tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(profileColumns.Id, id).Data(g.Map{profileColumns.ProfileNo: next}).Update(); err != nil {
			return gerror.Wrap(err, "迁移旧资料编号失败")
		}
	}
	return nil
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
