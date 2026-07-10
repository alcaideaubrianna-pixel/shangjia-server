package sys

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) refreshAdminTgAccountSession(ctx context.Context, id int64, tenantId int64, operatorId int64) (status string, message string) {
	item, err := s.adminTgAccountById(ctx, id, tenantId)
	if err != nil {
		return sysin.PublishTgAccountStatusFailed, err.Error()
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return s.failTgAccountRefresh(ctx, id, tenantId, operatorId, err.Error())
	}
	storage, err := s.telegramSessionStorage(item.SessionKey)
	if err != nil {
		return s.failTgAccountRefresh(ctx, id, tenantId, operatorId, err.Error())
	}
	options := telegram.Options{SessionStorage: storage}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		return s.failTgAccountRefresh(ctx, id, tenantId, operatorId, err.Error())
	} else if resolver != nil {
		options.Resolver = resolver
	}
	user, err := s.readTelegramSelf(ctx, conf.AppId, conf.AppHash, options)
	if err != nil {
		if isTelegramAuthKeyUnregistered(err) {
			return s.expireTgAccountSession(context.Background(), id, tenantId, operatorId, tgAccountSessionExpiredMessage)
		}
		return s.failTgAccountRefresh(context.Background(), id, tenantId, operatorId, err.Error())
	}
	username := ""
	displayName := ""
	if user != nil {
		username = user.Username
		displayName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	}
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusAuthorized, "", user, username, displayName)
	return sysin.PublishTgAccountStatusAuthorized, ""
}

func (s *sSysPublish) failTgAccountRefresh(ctx context.Context, id int64, tenantId int64, operatorId int64, message string) (string, string) {
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusFailed, message, nil, "", "")
	return sysin.PublishTgAccountStatusFailed, message
}

const tgAccountSessionExpiredMessage = "TG账号登录态已失效，请重新扫码登录"

func (s *sSysPublish) expireTgAccountSession(ctx context.Context, id int64, tenantId int64, operatorId int64, message string) (string, string) {
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusExpired, message, nil, "", "")
	return sysin.PublishTgAccountStatusExpired, message
}

func isTelegramAuthKeyUnregistered(err error) bool {
	return err != nil && strings.Contains(err.Error(), "AUTH_KEY_UNREGISTERED")
}

func (s *sSysPublish) readTelegramSelf(ctx context.Context, appId int, appHash string, options telegram.Options) (*tg.User, error) {
	client := telegram.NewClient(appId, appHash, options)
	var self *tg.User
	runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	err := client.Run(runCtx, func(ctx context.Context) error {
		user, err := client.Self(ctx)
		if err != nil {
			return err
		}
		self = user
		return nil
	})
	return self, err
}

func (s *sSysPublish) updateTgAccountRefreshResult(ctx context.Context, id int64, tenantId int64, operatorId int64, status string, message string, user *tg.User, username string, displayName string) {
	data := g.Map{
		"status":        status,
		"error_message": message,
		"updated_by":    operatorId,
		"updated_at":    gtime.Now(),
	}
	if status == sysin.PublishTgAccountStatusAuthorized {
		data["last_login_at"] = gtime.Now()
		if user != nil {
			data["telegram_user_id"] = strconv.FormatInt(user.ID, 10)
			data["telegram_username"] = username
			data["telegram_first_name"] = user.FirstName
			data["telegram_last_name"] = user.LastName
			data["telegram_phone"] = user.Phone
			data["telegram_is_bot"] = boolToInt(user.Bot)
			if displayName != "" {
				data["display_name"] = displayName
			}
		}
	}
	_, _ = g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Data(data).
		Update()
}

func (s *sSysPublish) telegramSessionPathByKey(sessionKey string) (string, error) {
	parts := strings.Split(strings.TrimSpace(sessionKey), "/")
	if len(parts) != 3 || !strings.HasPrefix(parts[0], "tenant_") || !strings.HasPrefix(parts[1], "account_") {
		return "", gerror.New("TG账号会话路径无效，请重新登录")
	}
	tenantPart := strings.TrimPrefix(parts[0], "tenant_")
	accountPart := strings.TrimPrefix(parts[1], "account_")
	token := strings.TrimSuffix(parts[2], ".json")
	if tenantPart == "" || accountPart == "" || token == "" {
		return "", gerror.New("TG账号会话路径无效，请重新登录")
	}
	return filepath.Join(gfile.Pwd(), "runtime", "youban_publish", "telegram_sessions", "tenant_"+tenantPart, fmt.Sprintf("account_%s_%s.json", accountPart, token)), nil
}

func (s *sSysPublish) ensureTgAccountsBelongTenant(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return gerror.New("请选择TG账号")
	}
	count, err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG账号权限失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作的TG账号")
	}
	return nil
}

func uniqueIds(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(ids))
	list := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		list = append(list, id)
	}
	return list
}
