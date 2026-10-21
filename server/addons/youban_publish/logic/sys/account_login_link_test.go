package sys

import (
	"context"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	_ "hotgo/internal/logic/admin"
	"hotgo/internal/model"
	internalService "hotgo/internal/service"
)

func TestAccountLoginLinkRejectsNonSystemIdentities(t *testing.T) {
	for _, user := range []*model.Identity{nil, {Id: 1, App: consts.AppApi, DeptType: sysin.PublishAccountTypeAdmin}, {Id: 2, App: consts.AppApi, DeptType: sysin.PublishAccountTypeUploader}} {
		ctx := context.WithValue(context.Background(), consts.ContextHTTPKey, &model.Context{User: user})
		result, err := NewSysPublish().ServerAccountLoginLink(ctx, &sysin.AccountLoginLinkInp{Id: 10})
		if err == nil || result != nil {
			t.Fatalf("non-system identity generated a login credential: %v", user)
		}
	}
}

func TestAccountLoginLinkInput(t *testing.T) {
	for _, in := range []*sysin.AccountLoginLinkInp{nil, {}, {Id: -1}} {
		if err := in.Filter(context.Background()); err == nil {
			t.Fatal("invalid target accepted")
		}
	}
	if err := (&sysin.AccountLoginLinkInp{Id: 1}).Filter(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type loginLinkAdminPermissionStub struct {
	internalService.IAdminMember
	allowed bool
	called  bool
}

func (s *loginLinkAdminPermissionStub) VerifySuperId(ctx context.Context, id int64) bool {
	s.called = true
	return s.allowed
}

func TestAccountLoginLinkChecksSystemSuperAdmin(t *testing.T) {
	original := internalService.AdminMember()
	defer internalService.RegisterAdminMember(original)
	checker := &loginLinkAdminPermissionStub{IAdminMember: original}
	internalService.RegisterAdminMember(checker)
	ctx := context.WithValue(context.Background(), consts.ContextHTTPKey, &model.Context{
		User: &model.Identity{Id: 10, App: consts.AppAdmin},
	})
	result, err := NewSysPublish().ServerAccountLoginLink(ctx, &sysin.AccountLoginLinkInp{Id: 20})
	if !checker.called || err == nil || result != nil {
		t.Fatal("ordinary system admin bypassed super-admin check")
	}
	checker.allowed = true
	// An authorized caller reaches input validation, without accessing a DB.
	result, err = NewSysPublish().ServerAccountLoginLink(ctx, nil)
	if err == nil || err.Error() != "请选择目标账号" || result != nil {
		t.Fatalf("super admin did not reach target validation: %v", err)
	}
}
