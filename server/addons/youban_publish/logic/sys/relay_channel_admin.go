package sys

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
	"strings"
	"time"
)

func (s *sSysPublish) AdminRelayChannelList(ctx context.Context, in *sysin.RelayChannelListInp) (list []*sysin.RelayChannelModel, err error) {
	list = []*sysin.RelayChannelModel{}
	err = g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).Where("tenant_id", in.TenantId).Order("id ASC").Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "读取 relay 频道失败")
	}
	return
}
func (s *sSysPublish) AdminRelayChannelSave(ctx context.Context, in *sysin.RelayChannelSaveInp) error {
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = relayChannelStatusDisabled
	}
	if status != relayChannelStatusEnabled && status != relayChannelStatusDisabled {
		return gerror.New("relay 频道状态不合法")
	}
	data := g.Map{"tenant_id": in.TenantId, "tg_account_id": in.TgAccountId, "target_chat_id": strings.TrimSpace(in.TargetChatId), "title": in.Title, "status": status, "health_status": relayChannelHealthUnknown, "bot_permission_status": relayBotPermissionUnknown, "updated_at": time.Now()}
	m := g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx)
	if in.Id > 0 {
		_, err := m.WherePri(in.Id).Data(data).Update()
		return err
	}
	_, err := m.Data(data).Insert()
	return err
}
func (s *sSysPublish) AdminRelayChannelStatus(ctx context.Context, in *sysin.RelayChannelStatusInp) error {
	if in.Status != relayChannelStatusEnabled && in.Status != relayChannelStatusDisabled {
		return gerror.New("relay 频道状态不合法")
	}
	_, err := g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).WherePri(in.Id).Data(g.Map{"status": in.Status, "updated_at": time.Now()}).Update()
	return err
}
