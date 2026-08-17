package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	collectorservice "hotgo/addons/telegram_collector/service"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type accountCollectSourceRuntime struct {
	Id                    int64  `json:"id"`
	TenantId              int64  `json:"tenantId"`
	AccountId             int64  `json:"accountId"`
	TgAccountId           int64  `json:"tgAccountId"`
	SourceChatId          string `json:"sourceChatId"`
	SourceUsername        string `json:"sourceUsername"`
	HistoryCollectEnabled int    `json:"historyCollectEnabled"`
	HistoryCollectMode    string `json:"historyCollectMode"`
	HistoryCollectDays    int    `json:"historyCollectDays"`
}

type accountCollectWorker struct {
	service          *sSysPublish
	tgAccountId      int64
	clientMu         sync.RWMutex
	client           *telegram.Client
	configMu         sync.RWMutex
	signature        string
	listeners        []accountListenPlanRuntime
	listenerGroupMu  sync.Mutex
	listenerGroups   map[string]*listenerMessageGroup
	listenerSenderMu sync.Mutex
	listenerSenders  map[string]listenerMessageSenderInfo
}

type accountCollectOperation func(context.Context, *telegram.Client) error

func (s *sSysPublish) refreshAccountCollectSupervisor() { collectorservice.AccountRuntime().Refresh() }

func (s *sSysPublish) enabledAccountCollectSources(ctx context.Context) (map[int64][]accountCollectSourceRuntime, error) {
	groups := make(map[int64][]accountCollectSourceRuntime)
	if !s.collectGlobalEnabled(ctx) {
		return groups, nil
	}
	var rows []accountCollectSourceRuntime
	err := g.DB().Model(pdao.YoubanPublishCollectSource.Table()+" s").Safe().Ctx(ctx).
		InnerJoin(publishTgAccountTable+" ta", "ta.id=s.tg_account_id").
		InnerJoin(pdao.YoubanPublishTenantVip.Table()+" vip", "vip.tenant_id=s.tenant_id AND vip.status=1 AND vip.level>0 AND vip.deleted_at IS NULL").
		Fields("s.id,s.tenant_id,s.account_id,s.tg_account_id,s.source_chat_id,s.source_username,s.history_collect_enabled,s.history_collect_mode,s.history_collect_days").
		Where("s.source_type", sysin.CollectSourceTypeAccount).Where("s.collect_enabled", 1).Where("s.status", 1).
		WhereGT("s.tg_account_id", 0).Where("ta.status", sysin.PublishTgAccountStatusAuthorized).WhereNot("ta.session_key", "").
		Where("(vip.expired_at IS NULL OR vip.expired_at>?)", gtime.Now()).WhereNull("s.deleted_at").WhereNull("ta.deleted_at").
		OrderAsc("s.tg_account_id").OrderAsc("s.id").Scan(&rows)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row.HistoryCollectEnabled, row.HistoryCollectMode, row.HistoryCollectDays = sysin.NormalizeCollectHistoryConfig(sysin.CollectSourceTypeAccount, row.HistoryCollectEnabled, row.HistoryCollectMode, row.HistoryCollectDays)
		groups[row.TgAccountId] = append(groups[row.TgAccountId], row)
	}
	return groups, nil
}

func (w *accountCollectWorker) updateConfig(signature string, listeners []accountListenPlanRuntime) bool {
	w.configMu.Lock()
	defer w.configMu.Unlock()
	if w.signature == signature {
		return false
	}
	w.signature = signature
	w.listeners = append([]accountListenPlanRuntime(nil), listeners...)
	return true
}

func (w *accountCollectWorker) configSnapshot() []accountListenPlanRuntime {
	w.configMu.RLock()
	defer w.configMu.RUnlock()
	return append([]accountListenPlanRuntime(nil), w.listeners...)
}

func (w *accountCollectWorker) clearListenerGroups() {
	w.listenerGroupMu.Lock()
	defer w.listenerGroupMu.Unlock()
	for key, group := range w.listenerGroups {
		if group != nil && group.timer != nil {
			group.timer.Stop()
		}
		delete(w.listenerGroups, key)
	}
}

func (w *accountCollectWorker) setClient(client *telegram.Client) {
	w.clientMu.Lock()
	w.client = client
	w.clientMu.Unlock()
}

func (w *accountCollectWorker) currentClient() *telegram.Client {
	w.clientMu.RLock()
	defer w.clientMu.RUnlock()
	return w.client
}

func (s *sSysPublish) restartAccountCollectWorker(ctx context.Context, tgAccountId int64, reason error) {
	if tgAccountId <= 0 {
		return
	}
	g.Log().Warningf(ctx, "请求重建Telegram账号运行连接 tgAccountId:%d err:%+v", tgAccountId, reason)
	collectorservice.AccountRuntime().Restart(tgAccountId)
}

func accountCollectMediaConcurrency(ctx context.Context) int {
	concurrency := g.Cfg().MustGet(ctx, "youbanPublish.collect.mediaFileConcurrency", 2).Int()
	if concurrency < 1 {
		return 1
	}
	if concurrency > 16 {
		return 16
	}
	return concurrency
}

func accountCollectSourceSignature(sources []accountCollectSourceRuntime) string {
	parts := make([]string, 0, len(sources))
	for _, item := range sources {
		parts = append(parts, fmt.Sprintf("%d:%s:%s:%d:%s:%d", item.Id, item.SourceChatId, item.SourceUsername, item.HistoryCollectEnabled, item.HistoryCollectMode, item.HistoryCollectDays))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

type accountCollectTgAccount struct {
	Id         int64  `json:"id"`
	TenantId   int64  `json:"tenantId"`
	AccountId  int64  `json:"accountId"`
	SessionKey string `json:"sessionKey"`
	Status     string `json:"status"`
}

func (s *sSysPublish) accountCollectTgAccount(ctx context.Context, tgAccountId int64) (*accountCollectTgAccount, error) {
	var item *accountCollectTgAccount
	err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).Fields("id,tenant_id,account_id,session_key,status").Where("id", tgAccountId).WhereNull("deleted_at").Scan(&item)
	if err != nil {
		return nil, gerror.Wrap(err, "读取账号采集TG账号失败")
	}
	if err = validateAccountCollectTgAccount(item); err != nil {
		return nil, err
	}
	return item, nil
}

func validateAccountCollectTgAccount(item *accountCollectTgAccount) error {
	if item == nil || item.Id <= 0 {
		return gerror.New("账号采集TG账号不存在或已被删除")
	}
	if strings.TrimSpace(item.Status) != sysin.PublishTgAccountStatusAuthorized {
		return gerror.New("账号采集TG账号未授权或已失效")
	}
	if strings.TrimSpace(item.SessionKey) == "" {
		return gerror.New("账号采集TG账号未登录")
	}
	return nil
}

func (s *sSysPublish) newAccountCollectClient(ctx context.Context, conf *model.TelegramConfig, item *accountCollectTgAccount, dispatcher tg.UpdateDispatcher) (*telegram.Client, error) {
	if conf == nil || conf.AppId <= 0 || strings.TrimSpace(conf.AppHash) == "" {
		return nil, gerror.New("请先配置Telegram App ID和App Hash")
	}
	storage, err := s.telegramSessionStorage(item.SessionKey)
	if err != nil {
		return nil, err
	}
	options := telegram.Options{AllowCDN: true, SessionStorage: storage, UpdateHandler: dispatcher}
	resolver, err := telegramMTProtoResolver(conf.ProxyUrl)
	if err != nil {
		return nil, err
	}
	if resolver != nil {
		options.Resolver = resolver
	}
	return telegram.NewClient(conf.AppId, conf.AppHash, options), nil
}

func isContextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
