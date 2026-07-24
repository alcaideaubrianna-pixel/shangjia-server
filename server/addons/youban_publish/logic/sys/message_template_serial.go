package sys

import (
	"context"
	"crypto/rand"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func newInlineTemplateSerial() (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", gerror.Wrap(err, "生成Inline模板编号失败")
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return "XX" + string(buf), nil
}

func (s *sSysPublish) ensureInlineTemplateSerial(ctx context.Context) (string, error) {
	for i := 0; i < 8; i++ {
		serial, err := newInlineTemplateSerial()
		if err != nil {
			return "", err
		}
		count, err := g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).Where("serial_no", serial).Count()
		if err != nil {
			return "", gerror.Wrap(err, "检查Inline模板编号失败")
		}
		if count == 0 {
			return serial, nil
		}
	}
	return "", gerror.New("生成唯一Inline模板编号失败")
}

func normalizeInlineTemplateSerial(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
