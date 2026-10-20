package adminsecurity

import (
	"context"
	"strings"
	"time"

	"github.com/mojocn/base64Captcha"
	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
	"hotgo/utility/ownedlease"
)

// The legacy captcha store is process-local. Management login must work when
// captcha generation and verification reach different Web replicas.
type captchaStore struct{ ctx context.Context }

func (s captchaStore) Set(id, value string) error {
	return cache.Instance().Set(s.ctx, consts.CacheAdminCaptcha+id, value, 5*time.Minute)
}

func (s captchaStore) Get(id string, clear bool) string {
	if id == "" || len(id) > 128 {
		return ""
	}
	if clear {
		lease := ownedlease.New(consts.CacheAdminLoginLock+"captcha:"+id, "admin-captcha", 5*time.Second)
		ok, err := lease.TryAcquire(s.ctx)
		if err != nil || !ok {
			return ""
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = lease.Unlock(ctx)
		}()
	}
	value, err := cache.Instance().Get(s.ctx, consts.CacheAdminCaptcha+id)
	if err != nil || value == nil || value.IsNil() {
		return ""
	}
	if clear {
		if _, err := cache.Instance().Remove(s.ctx, consts.CacheAdminCaptcha+id); err != nil {
			return ""
		}
	}
	return value.String()
}

func (s captchaStore) Verify(id, answer string, clear bool) bool {
	value := s.Get(id, clear)
	return value != "" && strings.EqualFold(value, answer)
}

func GenerateCaptcha(ctx context.Context, kind int) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var driver base64Captcha.Driver = base64Captcha.NewDriverString(42, 100, 0, 0, 4,
		"abcdefghjkmnpqrstuvwxyz23456789", nil, nil, []string{"chromohv.ttf"})
	if kind == consts.CaptchaTypeMath {
		driver = base64Captcha.NewDriverMath(42, 100, 0, 0, nil, nil, []string{"chromohv.ttf"})
	}
	id, image, _, err := base64Captcha.NewCaptcha(driver, captchaStore{ctx}).Generate()
	return id, image, err
}

func VerifyCaptcha(ctx context.Context, id, answer string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return captchaStore{ctx}.Verify(id, answer, true)
}
