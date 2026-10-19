package sys

import (
	"context"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// Read scope is deliberately independent of write scope.
func canReadTenantProfiles(capability *sysin.AccountCapabilityModel) bool {
	return capability != nil && capability.TenantId > 0 && capability.AccountId > 0 &&
		(capability.SharedResourceEnabled == 1 || (capability.AccountType == sysin.PublishAccountTypeUploader && capability.ProfileDetailViewEnabled == 1))
}

func (s *sSysPublish) readableProfileAccountIds(ctx context.Context, capability *sysin.AccountCapabilityModel, selected int64) ([]int64, error) {
	if capability == nil {
		return nil, gerror.New("上架账号信息不完整")
	}
	readCapability := *capability
	if canReadTenantProfiles(capability) {
		readCapability.SharedResourceEnabled = 1
	}
	ids, err := s.sharedProfileAccountIds(ctx, &readCapability)
	if err != nil {
		return nil, err
	}
	if selected > 0 {
		if !containsInt64(ids, selected) {
			return nil, gerror.New("账号不存在或无权查看")
		}
		return []int64{selected}, nil
	}
	return ids, nil
}

// Only a selected, authorized profile permits widening the record query.
func (s *sSysPublish) profileRecordAccountScope(ctx context.Context, tenantId, accountId, profileId int64) (int64, error) {
	if profileId <= 0 {
		return accountId, nil
	}
	capability, err := s.activeAccountCapability(ctx, tenantId, accountId)
	if err != nil {
		return 0, err
	}
	owner := accountId
	if canReadTenantProfiles(capability) {
		owner = 0
	}
	if _, err = s.profileState(ctx, profileId, tenantId, owner); err != nil {
		return 0, err
	}
	return owner, nil
}

// Explicit manual mappings override tenant defaults, matching profileChannelIdsOrDefaults.
func applyProfilePushChannelFilter(mod *gdb.Model, in *sysin.ProfileListInp, tenantField, profileField string) *gdb.Model {
	if in == nil || in.PushChannelId <= 0 {
		return mod
	}
	manual := "SELECT 1 FROM " + publishProfileChannelTable + " pc WHERE pc.tenant_id=" + tenantField + " AND pc.profile_id=" + profileField + " AND pc.is_manual=1 AND pc.deleted_at IS NULL"
	return mod.Where("(EXISTS ("+manual+" AND pc.channel_id=?) OR (NOT EXISTS ("+manual+") AND EXISTS (SELECT 1 FROM "+publishChannelTable+" c WHERE c.id=? AND c.tenant_id="+tenantField+" AND c.publish_direction='up' AND c.status=1 AND c.is_default_selected=1 AND c.deleted_at IS NULL)))", in.PushChannelId, in.PushChannelId)
}
