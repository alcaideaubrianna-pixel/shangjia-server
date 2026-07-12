package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_bot/model/input/sysin"
)

const (
	superNotifyRegister = "register"
	superNotifyError    = "error"
	superNotifyBind     = "bind"
)

func (s *sSysBot) notifySuperAdmins(ctx context.Context, botId int64, scene string, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if !s.superNotifyEnabled(ctx, scene) {
		return nil
	}
	telegramUserIds := s.superNotifyTelegramUserIds(ctx)
	if len(telegramUserIds) == 0 {
		return nil
	}
	var row *sysin.BotModel
	var err error
	if botId > 0 {
		row, err = s.botById(ctx, botId)
		if err != nil {
			return err
		}
	} else {
		row, err = s.officialBot(ctx)
		if err != nil {
			return err
		}
		botId = row.Id
	}
	var users []*struct {
		ChatId string `json:"chat_id"`
	}
	err = g.DB().Model(userTable+" u").Safe().Ctx(ctx).
		Fields("u.chat_id").
		Where("u.bot_id", botId).
		WhereIn("u.telegram_user_id", telegramUserIds).
		Where("u.status", 1).
		Where("EXISTS (SELECT 1 FROM "+accountBindTbl+" ab WHERE ab.telegram_user_id=u.telegram_user_id AND ab.app=? AND ab.status=1 AND ab.deleted_at IS NULL)", sysin.BotAppAdmin).
		Scan(&users)
	if err != nil {
		return gerror.Wrap(err, "读取Bot超级通知管理员失败")
	}
	for _, user := range users {
		if user == nil || strings.TrimSpace(user.ChatId) == "" {
			continue
		}
		if _, sendErr := s.sendMessage(ctx, row.BotToken, user.ChatId, text, "HTML", false); sendErr != nil {
			g.Log().Warningf(ctx, "推送Bot超级通知失败 botId:%d chatId:%s err:%+v", botId, user.ChatId, sendErr)
		}
	}
	return nil
}

func (s *sSysBot) NotifySuperAdmins(ctx context.Context, botId int64, scene string, text string) error {
	return s.notifySuperAdmins(ctx, botId, scene, text)
}

func (s *sSysBot) superNotifyEnabled(ctx context.Context, scene string) bool {
	values := s.superNotifyConfig(ctx)
	switch scene {
	case superNotifyRegister:
		return configBool(values["enableRegister"])
	case superNotifyError:
		return configBool(values["enableError"])
	case superNotifyBind:
		return configBool(values["enableBind"])
	default:
		return false
	}
}

func (s *sSysBot) superNotifyTelegramUserIds(ctx context.Context) []string {
	values := s.superNotifyConfig(ctx)
	return configStringList(values["adminTelegramUserIds"])
}

func (s *sSysBot) superNotifyConfig(ctx context.Context) map[string]interface{} {
	row, _ := s.featureConfig(ctx, superNotifyFeature{}.Key())
	if row == nil {
		return map[string]interface{}{}
	}
	return featureConfigValues(row.ConfigJson)
}

func configBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v == 1
	case int64:
		return v == 1
	case float64:
		return int(v) == 1
	case string:
		vv := strings.TrimSpace(strings.ToLower(v))
		return vv == "1" || vv == "true" || vv == "yes"
	default:
		return false
	}
}

func configStringList(value interface{}) []string {
	list := make([]string, 0)
	appendValue := func(v interface{}) {
		s := strings.TrimSpace(fmt.Sprintf("%v", v))
		if s != "" && s != "<nil>" {
			list = append(list, s)
		}
	}
	switch v := value.(type) {
	case []interface{}:
		for _, item := range v {
			appendValue(item)
		}
	case []string:
		for _, item := range v {
			appendValue(item)
		}
	case string:
		if strings.HasPrefix(strings.TrimSpace(v), "[") {
			return list
		}
		for _, item := range strings.Split(v, ",") {
			appendValue(item)
		}
	case float64:
		appendValue(strconv.FormatInt(int64(v), 10))
	default:
		appendValue(v)
	}
	return list
}

func botBindNotifyText(app string, accountId int64, telegramUsername string) string {
	name := strings.TrimSpace(telegramUsername)
	if name != "" {
		name = "@" + strings.TrimPrefix(name, "@")
	} else {
		name = "未知TG用户"
	}
	return fmt.Sprintf("用户绑定通知\n应用：%s\n账号ID：%d\nTelegram：%s", accountLabel(app), accountId, name)
}
