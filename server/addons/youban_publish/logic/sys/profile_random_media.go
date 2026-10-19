package sys

import (
	"context"
	"encoding/json"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func saveProfileRandomMediaTx(ctx context.Context, tx gdb.TX, profileID int64, policy *sysin.ProfileRandomMediaPolicy) error {
	if policy == nil {
		return nil
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		return gerror.Wrap(err, "编码随机选图配置失败")
	}
	_, err = tx.Model(publishProfileStateTable).Ctx(ctx).Where("profile_id", profileID).Data(g.Map{"random_media_config": string(payload)}).Update()
	return gerror.Wrap(err, "保存随机选图配置失败")
}

func decodeProfileRandomMedia(payload string) (*sysin.ProfileRandomMediaPolicy, error) {
	if payload == "" {
		return nil, nil
	}
	var policy sysin.ProfileRandomMediaPolicy
	if err := json.Unmarshal([]byte(payload), &policy); err != nil {
		return nil, gerror.Wrap(err, "读取随机选图配置失败")
	}
	return &policy, policy.Filter()
}

func (s *sSysPublish) loadProfileRandomMedia(ctx context.Context, profile *sysin.ProfileModel) error {
	state, err := s.profileState(ctx, profile.Id, profile.TenantId, profile.AccountId)
	if err != nil {
		return err
	}
	if state["random_media_config"] == nil {
		return gerror.New("随机选图数据库字段尚未迁移")
	}
	profile.RandomMedia, err = decodeProfileRandomMedia(state["random_media_config"].String())
	return err
}
