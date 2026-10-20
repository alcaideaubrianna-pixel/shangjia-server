package common

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	api "hotgo/api/admin/common"
	"hotgo/internal/model"
	"hotgo/internal/model/input/adminin"
	"hotgo/internal/service"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gcfg"
)

type loginSecurityConfig struct {
	service.ISysConfig
	login *model.LoginConfig
}

func (s loginSecurityConfig) GetLogin(context.Context) (*model.LoginConfig, error) {
	return s.login, nil
}

type loginSecuritySite struct {
	service.IAdminSite
	calls int
}

func (s *loginSecuritySite) AccountLogin(context.Context, *adminin.AccountLoginInp) (*adminin.LoginModel, error) {
	s.calls++
	return &adminin.LoginModel{Username: "admin"}, nil
}

func TestManagementCaptchaCannotBeBypassedWithIsLock(t *testing.T) {
	var oldConfig service.ISysConfig
	var oldSite service.IAdminSite
	func() { defer func() { _ = recover() }(); oldConfig = service.SysConfig() }()
	func() { defer func() { _ = recover() }(); oldSite = service.AdminSite() }()
	oldAdapter := g.Cfg().GetAdapter()
	t.Cleanup(func() {
		service.RegisterSysConfig(oldConfig)
		service.RegisterAdminSite(oldSite)
		g.Cfg().SetAdapter(oldAdapter)
	})
	login := &model.LoginConfig{}
	account := &loginSecuritySite{}
	service.RegisterSysConfig(loginSecurityConfig{login: login})
	service.RegisterAdminSite(account)
	adapter, err := gcfg.NewAdapterContent(`{"security":{"admin":{"entryPath":"/safe_abcdefghijklmnop"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	r := &ghttp.Request{Request: httptest.NewRequest("POST", "https://example.test/admin/site/accountLogin", nil)}
	for _, isLock := range []bool{false, true} {
		_, err := Site.AccountLogin(r.Context(), &api.AccountLoginReq{AccountLoginInp: adminin.AccountLoginInp{Username: "admin", Password: "wrong", IsLock: isLock}})
		if err == nil || !strings.Contains(err.Error(), "验证码") {
			t.Fatalf("anonymous isLock=%v bypassed captcha: %v", isLock, err)
		}
	}
	if account.calls != 0 {
		t.Fatal("password query reached before captcha")
	}
	config, err := Site.LoginConfig(r.Context(), nil)
	if err != nil || config.CaptchaSwitch != 1 || login.CaptchaSwitch != 0 {
		t.Fatal("secure config not isolated", config, err)
	}
	// Disabled entry preserves the configured legacy login behavior.
	adapter, err = gcfg.NewAdapterContent(`{"security":{"admin":{"entryPath":""}}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	if _, err := Site.AccountLogin(r.Context(), &api.AccountLoginReq{AccountLoginInp: adminin.AccountLoginInp{Username: "admin", Password: "fake"}}); err != nil || account.calls != 1 {
		t.Fatal("disabled mode changed login", err, account.calls)
	}
}
