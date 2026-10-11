package sys

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/tg"
	"hotgo/addons/youban_publish/model/input/sysin"
	"strconv"
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

func (s *sSysPublish) AdminRelayChannelCreate(ctx context.Context, in *sysin.RelayChannelCreateInp) (*sysin.RelayChannelModel, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, gerror.New("relay 频道名称不能为空")
	}
	client, err := s.TelegramClient(ctx, in.TgAccountId)
	if err != nil {
		return nil, err
	}
	updates, err := client.API().ChannelsCreateChannel(ctx, &tg.ChannelsCreateChannelRequest{Broadcast: true, Title: in.Title, About: in.About})
	if err != nil {
		return nil, gerror.Wrap(err, "创建 relay 频道失败")
	}
	var channel *tg.Channel
	switch value := updates.(type) {
	case *tg.Updates:
		for _, item := range value.Chats {
			if c, ok := item.(*tg.Channel); ok {
				channel = c
				break
			}
		}
	case *tg.UpdatesCombined:
		for _, item := range value.Chats {
			if c, ok := item.(*tg.Channel); ok {
				channel = c
				break
			}
		}
	}
	if channel == nil {
		return nil, gerror.New("创建 relay 频道成功但未读取频道信息")
	}
	accessHash, ok := channel.GetAccessHash()
	if !ok {
		return nil, gerror.New("relay 频道缺少 access hash")
	}
	target := "-100" + strconv.FormatInt(channel.ID, 10)
	_, err = g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).Data(g.Map{"tenant_id": in.TenantId, "tg_account_id": in.TgAccountId, "channel_id": strconv.FormatInt(channel.ID, 10), "target_chat_id": target, "title": channel.Title, "status": relayChannelStatusDisabled, "health_status": relayChannelHealthUnknown, "bot_permission_status": relayBotPermissionUnknown, "updated_at": time.Now()}).Insert()
	if err != nil {
		return nil, gerror.Wrap(err, "保存 relay 频道失败")
	}
	return &sysin.RelayChannelModel{TenantId: in.TenantId, TgAccountId: in.TgAccountId, TargetChatId: target, Title: channel.Title, Status: relayChannelStatusDisabled}, nil
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
