package sys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"golang.org/x/net/proxy"
)

const publishTgSessionTable = "hg_youban_publish_tg_session"

type setupGroupResult struct {
	AccessHash   int64
	InviteLink   string
	SupergroupId string
	Title        string
}

type telegramDBSessionStorage struct {
	key          string
	fallbackPath string
	mu           sync.Mutex
}

type publishChannelCache struct {
	AccessHash     string `json:"access_hash"`
	CanAddAdmins   int    `json:"can_add_admins"`
	CanInviteUsers int    `json:"can_invite_users"`
	ChannelId      string `json:"channel_id"`
	ChannelTitle   string `json:"channel_title"`
	IsMegagroup    int    `json:"is_megagroup"`
}

type selectedPublishGroup struct {
	AccessHash   int64
	ChannelId    int64
	ChannelTitle string
}

func (s *sSysTwoWayBot) createManagementGroup(ctx context.Context, account *publishAccount, tgAccount *publishTgAccount, botUsername string) (*setupGroupResult, error) {
	conf, err := publishTelegramConfig(ctx)
	if err != nil {
		return nil, err
	}
	if conf == nil || conf.AppId <= 0 || strings.TrimSpace(conf.AppHash) == "" {
		return nil, gerror.New("请先配置Telegram App ID和App Hash")
	}
	title := defaultManagementGroupTitle(account)
	storage, err := telegramSessionStorage(tgAccount.SessionKey)
	if err != nil {
		return nil, err
	}
	options := telegram.Options{SessionStorage: storage}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		return nil, err
	} else if resolver != nil {
		options.Resolver = resolver
	}
	client := telegram.NewClient(conf.AppId, conf.AppHash, options)
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var result *setupGroupResult
	err = client.Run(runCtx, func(ctx context.Context) error {
		api := client.API()
		updates, err := api.ChannelsCreateChannel(ctx, &tg.ChannelsCreateChannelRequest{
			Megagroup: true,
			Forum:     true,
			Title:     title,
			About:     "悦伴双向机器人管理群",
		})
		if err != nil {
			return gerror.Wrap(err, "创建TG管理群失败")
		}
		channel := channelFromUpdates(updates)
		if channel == nil || channel.ID == 0 {
			return gerror.New("创建TG管理群成功，但未读取到群信息")
		}
		accessHash, ok := channel.GetAccessHash()
		if !ok {
			return gerror.New("TG管理群AccessHash缺失")
		}
		inputChannel := &tg.InputChannel{ChannelID: channel.ID, AccessHash: accessHash}
		_, _ = api.ChannelsToggleForum(ctx, &tg.ChannelsToggleForumRequest{Channel: inputChannel, Enabled: true, Tabs: true})
		inputUser, err := resolveBotInputUser(ctx, api, botUsername)
		if err != nil {
			return err
		}
		if _, err = api.ChannelsInviteToChannel(ctx, &tg.ChannelsInviteToChannelRequest{
			Channel: inputChannel,
			Users:   []tg.InputUserClass{inputUser},
		}); err != nil && !isTelegramUserAlreadyParticipant(err) {
			return gerror.Wrap(err, "拉Bot进入管理群失败")
		}
		if _, err = api.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
			Channel: inputChannel,
			UserID:  inputUser,
			AdminRights: tg.ChatAdminRights{
				ChangeInfo:     true,
				PostMessages:   true,
				EditMessages:   true,
				DeleteMessages: true,
				BanUsers:       false,
				InviteUsers:    true,
				PinMessages:    true,
				AddAdmins:      false,
				ManageCall:     true,
				Other:          true,
				ManageTopics:   true,
				PostStories:    true,
				EditStories:    true,
				DeleteStories:  true,
			},
			Rank: "双向机器人",
		}); err != nil {
			return gerror.Wrap(err, "设置Bot管理员权限失败")
		}
		invite, err := api.MessagesExportChatInvite(ctx, &tg.MessagesExportChatInviteRequest{
			Peer:  &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: accessHash},
			Title: "双向通知群",
		})
		if err != nil {
			return gerror.Wrap(err, "生成管理群邀请链接失败")
		}
		link := ""
		if exported, ok := invite.(*tg.ChatInviteExported); ok {
			link = exported.Link
		}
		result = &setupGroupResult{
			AccessHash:   accessHash,
			InviteLink:   strings.TrimSpace(link),
			SupergroupId: "-100" + strings.TrimSpace(gconvString(channel.ID)),
			Title:        channel.Title,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.SupergroupId == "" {
		return nil, gerror.New("创建TG管理群失败")
	}
	if result.Title == "" {
		result.Title = title
	}
	return result, nil
}

func (s *sSysTwoWayBot) setupExistingManagementGroup(ctx context.Context, account *publishAccount, tgAccount *publishTgAccount, groupId string, botUsername string) (*setupGroupResult, error) {
	channel, err := selectedPublishGroupCache(ctx, account.TenantId, tgAccount.Id, groupId)
	if err != nil {
		return nil, err
	}
	conf, err := publishTelegramConfig(ctx)
	if err != nil {
		return nil, err
	}
	if conf == nil || conf.AppId <= 0 || strings.TrimSpace(conf.AppHash) == "" {
		return nil, gerror.New("请先配置Telegram App ID和App Hash")
	}
	storage, err := telegramSessionStorage(tgAccount.SessionKey)
	if err != nil {
		return nil, err
	}
	options := telegram.Options{SessionStorage: storage}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		return nil, err
	} else if resolver != nil {
		options.Resolver = resolver
	}
	client := telegram.NewClient(conf.AppId, conf.AppHash, options)
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var inviteLink string
	err = client.Run(runCtx, func(ctx context.Context) error {
		api := client.API()
		inputChannel := &tg.InputChannel{ChannelID: channel.ChannelId, AccessHash: channel.AccessHash}
		if _, err = api.ChannelsToggleForum(ctx, &tg.ChannelsToggleForumRequest{Channel: inputChannel, Enabled: true, Tabs: true}); err != nil {
			return gerror.Wrap(err, "开启管理群话题失败")
		}
		inputUser, err := resolveBotInputUser(ctx, api, botUsername)
		if err != nil {
			return err
		}
		if _, err = api.ChannelsInviteToChannel(ctx, &tg.ChannelsInviteToChannelRequest{
			Channel: inputChannel,
			Users:   []tg.InputUserClass{inputUser},
		}); err != nil && !isTelegramUserAlreadyParticipant(err) {
			return gerror.Wrap(err, "拉Bot进入管理群失败")
		}
		if _, err = api.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
			Channel: inputChannel,
			UserID:  inputUser,
			AdminRights: tg.ChatAdminRights{
				ChangeInfo:     true,
				PostMessages:   true,
				EditMessages:   true,
				DeleteMessages: true,
				BanUsers:       false,
				InviteUsers:    true,
				PinMessages:    true,
				AddAdmins:      false,
				ManageCall:     true,
				Other:          true,
				ManageTopics:   true,
				PostStories:    true,
				EditStories:    true,
				DeleteStories:  true,
			},
			Rank: "双向机器人",
		}); err != nil {
			return gerror.Wrap(err, "设置Bot管理员权限失败")
		}
		invite, err := api.MessagesExportChatInvite(ctx, &tg.MessagesExportChatInviteRequest{
			Peer:  &tg.InputPeerChannel{ChannelID: channel.ChannelId, AccessHash: channel.AccessHash},
			Title: "双向通知群",
		})
		if err != nil {
			return gerror.Wrap(err, "生成管理群邀请链接失败")
		}
		if exported, ok := invite.(*tg.ChatInviteExported); ok {
			inviteLink = strings.TrimSpace(exported.Link)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &setupGroupResult{
		AccessHash:   channel.AccessHash,
		InviteLink:   inviteLink,
		SupergroupId: "-100" + strconv.FormatInt(channel.ChannelId, 10),
		Title:        channel.ChannelTitle,
	}, nil
}

func selectedPublishGroupCache(ctx context.Context, tenantId int64, tgAccountId int64, groupId string) (*selectedPublishGroup, error) {
	ids := publishGroupCacheLookupIds(groupId)
	if len(ids) == 0 {
		return nil, gerror.New("请选择已有群聊")
	}
	var row *publishChannelCache
	if err := g.DB().Model("hg_youban_publish_tg_channel").Safe().Ctx(ctx).
		Fields("channel_id,access_hash,channel_title,is_megagroup,can_invite_users,can_add_admins").
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		WhereIn("channel_id", ids).
		Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "读取群聊缓存失败")
	}
	if row == nil || strings.TrimSpace(row.ChannelId) == "" {
		return nil, gerror.New("请先刷新群聊缓存，并选择当前TG账号下的群聊")
	}
	if row.IsMegagroup != 1 {
		return nil, gerror.New("请选择超级群，普通群聊无法开启话题")
	}
	if row.CanInviteUsers != 1 || row.CanAddAdmins != 1 {
		return nil, gerror.New("当前TG账号缺少邀请用户或添加管理员权限，请选择自己创建或可管理的群聊")
	}
	channelId, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(row.ChannelId), "-100"), 10, 64)
	if err != nil || channelId <= 0 {
		return nil, gerror.New("群聊ID无效，请刷新群聊缓存")
	}
	accessHash, err := strconv.ParseInt(strings.TrimSpace(row.AccessHash), 10, 64)
	if err != nil || accessHash == 0 {
		return nil, gerror.New("群聊AccessHash无效，请刷新群聊缓存")
	}
	return &selectedPublishGroup{AccessHash: accessHash, ChannelId: channelId, ChannelTitle: row.ChannelTitle}, nil
}

func publishGroupCacheLookupIds(groupId string) []string {
	raw := strings.TrimSpace(groupId)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "-") && !strings.HasPrefix(raw, "-100") {
		return []string{raw}
	}
	plain := strings.TrimPrefix(raw, "-100")
	return uniqueSetupStrings([]string{raw, plain, "-100" + plain})
}

func uniqueSetupStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func resolveBotInputUser(ctx context.Context, api *tg.Client, username string) (*tg.InputUser, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return nil, gerror.New("Bot缺少用户名")
	}
	resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, gerror.Wrap(err, "解析Bot用户名失败")
	}
	if resolved == nil {
		return nil, gerror.New("未找到Bot账号")
	}
	for _, item := range resolved.GetUsers() {
		user, ok := item.(*tg.User)
		if !ok || !user.Bot {
			continue
		}
		accessHash, ok := user.GetAccessHash()
		if !ok {
			return nil, gerror.New("Bot AccessHash缺失")
		}
		return &tg.InputUser{UserID: user.ID, AccessHash: accessHash}, nil
	}
	return nil, gerror.New("未找到Bot账号")
}

func channelFromUpdates(updates tg.UpdatesClass) *tg.Channel {
	var chats []tg.ChatClass
	switch data := updates.(type) {
	case *tg.Updates:
		chats = data.GetChats()
	case *tg.UpdatesCombined:
		chats = data.GetChats()
	default:
		return nil
	}
	for _, chat := range chats {
		channel, ok := chat.(*tg.Channel)
		if ok && channel.ID > 0 {
			return channel
		}
	}
	return nil
}

func defaultManagementGroupTitle(account *publishAccount) string {
	name := ""
	if account != nil {
		name = strings.TrimSpace(account.Nickname)
		if name == "" {
			name = strings.TrimSpace(account.Username)
		}
	}
	if name == "" {
		name = "悦伴"
	}
	return truncateText(name, 40) + "双向通知群"
}

func (s *telegramDBSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	if s == nil || strings.TrimSpace(s.key) == "" {
		return nil, session.ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ensureTelegramSessionTable(ctx); err != nil {
		return nil, err
	}
	count, err := g.DB().Model(publishTgSessionTable).Safe().Ctx(ctx).
		Where("session_key", s.key).
		Count()
	if err != nil {
		return nil, gerror.Wrap(err, "检查TG会话失败")
	}
	if count == 0 {
		return s.loadFallbackSession(ctx)
	}
	row, err := g.DB().Model(publishTgSessionTable).Safe().Ctx(ctx).
		Fields("session_data").
		Where("session_key", s.key).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG会话失败")
	}
	if row.IsEmpty() {
		return s.loadFallbackSession(ctx)
	}
	data := decodeTelegramSessionData(row["session_data"].Bytes())
	if len(data) > 0 {
		return data, nil
	}
	return s.loadFallbackSession(ctx)
}

func (s *telegramDBSessionStorage) loadFallbackSession(ctx context.Context) ([]byte, error) {
	if strings.TrimSpace(s.fallbackPath) == "" {
		return nil, session.ErrNotFound
	}
	data, err := os.ReadFile(s.fallbackPath)
	if os.IsNotExist(err) {
		return nil, session.ErrNotFound
	}
	if err != nil {
		return nil, gerror.Wrap(err, "读取文件TG会话失败")
	}
	if len(data) == 0 {
		return nil, session.ErrNotFound
	}
	if err = s.storeSession(ctx, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *telegramDBSessionStorage) StoreSession(ctx context.Context, data []byte) error {
	if s == nil || strings.TrimSpace(s.key) == "" {
		return gerror.New("TG账号会话键不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.storeSession(ctx, data)
}

func (s *telegramDBSessionStorage) storeSession(ctx context.Context, data []byte) error {
	now := gtime.Now()
	if err := ensureTelegramSessionTable(ctx); err != nil {
		return err
	}
	count, err := g.DB().Model(publishTgSessionTable).Safe().Ctx(ctx).
		Where("session_key", s.key).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG会话失败")
	}
	saveData := g.Map{
		"session_key":  s.key,
		"session_data": []byte(base64.StdEncoding.EncodeToString(data)),
		"updated_at":   now,
	}
	if count > 0 {
		_, err = g.DB().Model(publishTgSessionTable).Safe().Ctx(ctx).
			Where("session_key", s.key).
			Data(saveData).
			Update()
	} else {
		saveData["created_at"] = now
		_, err = g.DB().Model(publishTgSessionTable).Safe().Ctx(ctx).
			Data(saveData).
			Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存TG会话失败")
	}
	return nil
}

func telegramSessionStorage(sessionKey string) (session.Storage, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil, gerror.New("TG账号会话键不能为空")
	}
	fallbackPath, err := telegramSessionPathByKey(sessionKey)
	if err != nil {
		return nil, err
	}
	return &telegramDBSessionStorage{key: sessionKey, fallbackPath: fallbackPath}, nil
}

func telegramSessionPathByKey(sessionKey string) (string, error) {
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
	return filepath.Join(gfile.Pwd(), "runtime", "youban_publish", "telegram_sessions", "tenant_"+tenantPart, "account_"+accountPart+"_"+token+".json"), nil
}

func ensureTelegramSessionTable(ctx context.Context) error {
	dbType := strings.ToLower(g.DB().GetConfig().Type)
	if strings.Contains(dbType, "pgsql") || strings.Contains(dbType, "postgres") {
		_, err := g.DB().Exec(ctx, `CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_session" (
  "id" BIGSERIAL PRIMARY KEY,
  "session_key" varchar(255) NOT NULL DEFAULT '',
  "session_data" bytea NOT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
)`)
		if err != nil {
			return gerror.Wrap(err, "创建TG会话表失败")
		}
		_, err = g.DB().Exec(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS "idx_ybp_tg_session_key" ON "hg_youban_publish_tg_session" ("session_key")`)
		if err != nil {
			return gerror.Wrap(err, "创建TG会话索引失败")
		}
		return nil
	}
	_, err := g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_session` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`session_key` varchar(255) NOT NULL DEFAULT '' COMMENT '会话Key',`session_data` longblob NOT NULL COMMENT '会话数据',`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `idx_ybp_tg_session_key` (`session_key`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4")
	if err != nil {
		return gerror.Wrap(err, "创建TG会话表失败")
	}
	return nil
}

func decodeTelegramSessionData(data []byte) []byte {
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return nil
	}
	if json.Valid(data) {
		return append([]byte(nil), data...)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil || !json.Valid(decoded) {
		return nil
	}
	return decoded
}

func telegramMTProtoResolver(proxyURL string) (dcs.Resolver, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return nil, nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, gerror.Wrap(err, "Telegram代理地址格式错误")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "socks5" && scheme != "socks5h" && scheme != "s5" {
		return nil, gerror.New("Telegram协议号仅支持socks5代理，例如 socks5://127.0.0.1:7890")
	}
	address := u.Host
	if address == "" {
		return nil, gerror.New("Telegram代理地址缺少host")
	}
	var authInfo *proxy.Auth
	if u.User != nil {
		password, _ := u.User.Password()
		authInfo = &proxy.Auth{User: u.User.Username(), Password: password}
	}
	dialer, err := proxy.SOCKS5("tcp", address, authInfo, proxy.Direct)
	if err != nil {
		return nil, gerror.Wrap(err, "初始化Telegram SOCKS5代理失败")
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, gerror.New("当前SOCKS5代理不支持ContextDialer")
	}
	return dcs.Plain(dcs.PlainOptions{Dial: contextDialer.DialContext}), nil
}

func isTelegramUserAlreadyParticipant(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "user_already_participant") || strings.Contains(message, "already participant")
}

func gconvString(value int64) string {
	return strings.TrimSpace(g.NewVar(value).String())
}
