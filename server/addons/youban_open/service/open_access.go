package service

import (
	"context"
	"sync"

	"hotgo/addons/youban_open/model/input/sysin"
)

type IOpenAccess interface {
	AppList(ctx context.Context, in *sysin.CmsAppListInp) ([]*sysin.CmsAppModel, error)
	AppSave(ctx context.Context, in *sysin.CmsAppSaveInp) (*sysin.CmsAppCredentialModel, error)
	AppResetSecret(ctx context.Context, in *sysin.CmsAppResetSecretInp) (*sysin.CmsAppCredentialModel, error)
	RegisterInstance(ctx context.Context, in *sysin.CmsInstanceRegisterInp, sourceIP string) (*sysin.CmsInstanceRegisterModel, error)
	HeartbeatInstance(ctx context.Context, in *sysin.CmsInstanceHeartbeatInp, sourceIP string) (*sysin.CmsInstanceRegisterModel, error)
	AppSecret(ctx context.Context, appId string) (string, error)
	AllowedTenantIds(ctx context.Context, appId string) ([]int64, error)
	SaveBindingCode(ctx context.Context, appId string, in *sysin.CmsBindingCodeSaveInp) (*sysin.CmsBindingCodeModel, error)
	ClaimBinding(ctx context.Context, tenantId int64, in *sysin.CmsBindingClaimInp) (*sysin.CmsBindingModel, error)
	RevokeTenantBinding(ctx context.Context, tenantId int64, in *sysin.CmsBindingRevokeInp) (*sysin.CmsBindingModel, error)
	SaveAppSettings(ctx context.Context, appId string, in *sysin.CmsAppSettingsInp) (*sysin.CmsAppSettingsModel, error)
	LookupBindingCode(ctx context.Context, code string) (*sysin.CmsBindingLookupModel, error)
	TenantBinding(ctx context.Context, tenantId int64) ([]*sysin.CmsBindingModel, error)
	Bindings(ctx context.Context, appId string, in *sysin.CmsBindingListInp) ([]*sysin.CmsBindingModel, error)
	UpdateBinding(ctx context.Context, appId string, in *sysin.CmsBindingStatusInp) (*sysin.CmsBindingModel, error)
	AdminBindings(ctx context.Context, in *sysin.CmsBindingListInp) ([]*sysin.CmsBindingModel, error)
	RecordInteraction(ctx context.Context, appId string, in *sysin.ProfileInteractionInp) (bool, error)
	RankedProfileIds(ctx context.Context, appId, actorId, feed string, limit int) ([]int64, error)
}

var (
	openAccess     IOpenAccess
	openAccessOnce sync.Once
)

func RegisterOpenAccess(service IOpenAccess) {
	openAccessOnce.Do(func() { openAccess = service })
}

func OpenAccess() IOpenAccess {
	if openAccess == nil {
		panic("youban publish open access service is not registered")
	}
	return openAccess
}
