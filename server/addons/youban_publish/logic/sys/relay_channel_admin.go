package sys

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gotd/td/tg"
	"hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
	"strconv"
	"strings"
)

func (s *sSysPublish) AdminRelayChannelList(ctx context.Context, in *sysin.RelayChannelListInp) (list []*sysin.RelayChannelModel, err error) {
	list = []*sysin.RelayChannelModel{}
	err = dao.YoubanPublishRelayChannel.Ctx(ctx).Where(dao.YoubanPublishRelayChannel.Columns().TenantId, in.TenantId).Order(dao.YoubanPublishRelayChannel.Columns().Id + " ASC").Scan(&list)
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
	permissionStatus := relayBotPermissionUnknown
	if strings.TrimSpace(in.BotUsername) != "" {
		bot, resolveErr := resolveRelayBot(ctx, client.API(), in.BotUsername)
		if resolveErr != nil {
			return nil, resolveErr
		}
		inputChannel := &tg.InputChannel{ChannelID: channel.ID, AccessHash: accessHash}
		if _, err = client.API().ChannelsInviteToChannel(ctx, &tg.ChannelsInviteToChannelRequest{Channel: inputChannel, Users: []tg.InputUserClass{bot}}); err != nil && !strings.Contains(strings.ToUpper(err.Error()), "ALREADY") {
			return nil, gerror.Wrap(err, "邀请 relay Bot 失败")
		}
		if _, err = client.API().ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{Channel: inputChannel, UserID: bot, AdminRights: tg.ChatAdminRights{PostMessages: true, EditMessages: true, DeleteMessages: true, ChangeInfo: true, Other: true}, Rank: "relay"}); err != nil {
			return nil, gerror.Wrap(err, "设置 relay Bot 权限失败")
		}
		permissionStatus = relayBotPermissionReady
	}
	_, err = dao.YoubanPublishRelayChannel.Ctx(ctx).Data(do.YoubanPublishRelayChannel{TenantId: in.TenantId, TgAccountId: in.TgAccountId, ChannelId: strconv.FormatInt(channel.ID, 10), TargetChatId: target, Title: channel.Title, Status: relayChannelStatusDisabled, HealthStatus: relayChannelHealthUnknown, BotPermissionStatus: permissionStatus}).Insert()
	if err != nil {
		return nil, gerror.Wrap(err, "保存 relay 频道失败")
	}
	return &sysin.RelayChannelModel{TenantId: in.TenantId, TgAccountId: in.TgAccountId, TargetChatId: target, Title: channel.Title, Status: relayChannelStatusDisabled, BotPermissionStatus: permissionStatus}, nil
}

func resolveRelayBot(ctx context.Context, api *tg.Client, username string) (*tg.InputUser, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	result, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, gerror.Wrap(err, "解析 relay Bot 失败")
	}
	for _, item := range result.GetUsers() {
		if user, ok := item.(*tg.User); ok && user.Bot {
			hash, ok := user.GetAccessHash()
			if ok {
				return &tg.InputUser{UserID: user.ID, AccessHash: hash}, nil
			}
		}
	}
	return nil, gerror.New("relay Bot 不存在或不是 Bot")
}
func (s *sSysPublish) AdminRelayChannelSave(ctx context.Context, in *sysin.RelayChannelSaveInp) error {
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = relayChannelStatusDisabled
	}
	if status != relayChannelStatusEnabled && status != relayChannelStatusDisabled {
		return gerror.New("relay 频道状态不合法")
	}
	if status == relayChannelStatusEnabled {
		return gerror.New("启用 relay 频道必须通过状态接口完成 Bot 权限校验")
	}
	data := do.YoubanPublishRelayChannel{TenantId: in.TenantId, TgAccountId: in.TgAccountId, TargetChatId: strings.TrimSpace(in.TargetChatId), Title: in.Title, Status: status, HealthStatus: relayChannelHealthUnknown, BotPermissionStatus: relayBotPermissionUnknown}
	m := dao.YoubanPublishRelayChannel.Ctx(ctx)
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
	data := do.YoubanPublishRelayChannel{Status: in.Status}
	if in.Status == relayChannelStatusEnabled {
		var current relayChannelRecord
		if err := dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(in.Id).Scan(&current); err != nil {
			return gerror.Wrap(err, "读取 relay 频道权限失败")
		}
		if current.Id == 0 {
			return gerror.New("relay 频道不存在")
		}
		if current.BotPermissionStatus != relayBotPermissionReady {
			return gerror.New("relay Bot 管理员权限未就绪，不能启用频道")
		}
		data.HealthStatus = relayChannelHealthReady
	}
	_, err := dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(in.Id).Data(data).Update()
	return err
}
