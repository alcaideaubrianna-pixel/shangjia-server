package sys

import (
	"context"
	"fmt"
	"html"
	"strings"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysBot) relayAdminCommand(ctx context.Context, cfg *entity.YoubanBotRelayConfig, cmd relayCommand) (string, error) {
	if cmd.Name == "添加管理员" {
		if !sysin.RelayIdentityValid(cmd.Args) || strings.HasPrefix(cmd.Args, "-") {
			return "", gerror.New("格式：添加管理员 用户ID或@用户名")
		}
		admins, err := s.resolveRelayAdmins(ctx, cfg.BotId, []string{cmd.Args})
		if err != nil {
			return "", err
		}
		c := dao.YoubanBotRelayAdmin.Columns()
		_, err = dao.YoubanBotRelayAdmin.Ctx(ctx).Data(admins[0]).OnConflict(c.BotId, c.Username).InsertIgnore()
		if err != nil {
			return "", gerror.Wrap(err, "添加管理员失败")
		}
		return "管理员配置已添加。若使用 @用户名，对方需到此双向Bot私聊发送 /start 确认身份。所有管理员需先私聊此Bot才能接收首次使用通知。", nil
	}
	var admins []*entity.YoubanBotRelayAdmin
	if err := dao.YoubanBotRelayAdmin.Ctx(ctx).Where(do.YoubanBotRelayAdmin{BotId: cfg.BotId}).OrderAsc(dao.YoubanBotRelayAdmin.Columns().Id).Scan(&admins); err != nil {
		return "", err
	}
	lines := []string{"<b>附加管理员</b>", "默认管理员：首次保存配置的后台账号（需绑定TG账号）。"}
	for i, a := range admins {
		if i >= 50 {
			lines = append(lines, "其余管理员请在后台查看。")
			break
		}
		state := "待确认：请到此Bot私聊发送 /start"
		if a.UserId != "" {
			state = "身份已确认 · ID " + a.UserId
		}
		lines = append(lines, html.EscapeString(a.Username)+" — "+html.EscapeString(state))
	}
	if len(admins) == 0 {
		lines = append(lines, "暂无附加管理员。")
	}
	lines = append(lines, fmt.Sprintf("添加：<code>添加管理员 123456789</code>\n当前Bot ID：%d", cfg.BotId))
	return strings.Join(lines, "\n"), nil
}
