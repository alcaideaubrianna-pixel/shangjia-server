package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const accountCollectSupervisorInterval = 15 * time.Second

type accountCollectSupervisor struct {
	workers map[int64]*accountCollectWorker
}

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
	service     *sSysPublish
	tgAccountId int64
	signature   string
	sources     []accountCollectSourceRuntime
	cancel      context.CancelFunc
	done        chan struct{}
}

func (s *sSysPublish) runAccountCollectSupervisor(ctx context.Context) {
	supervisor := &accountCollectSupervisor{workers: make(map[int64]*accountCollectWorker)}
	defer supervisor.stopAll()
	supervisor.sync(ctx, s)
	ticker := time.NewTicker(accountCollectSupervisorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			supervisor.sync(ctx, s)
		}
	}
}

func (m *accountCollectSupervisor) sync(ctx context.Context, service *sSysPublish) {
	groups, err := service.enabledAccountCollectSources(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取账号采集源失败：%+v", err)
		return
	}
	active := make(map[int64]struct{}, len(groups))
	for tgAccountId, sources := range groups {
		if tgAccountId <= 0 || len(sources) == 0 {
			continue
		}
		active[tgAccountId] = struct{}{}
		signature := accountCollectSourceSignature(sources)
		if worker := m.workers[tgAccountId]; worker != nil && worker.signature == signature {
			continue
		}
		if worker := m.workers[tgAccountId]; worker != nil {
			worker.stop()
		}
		m.workers[tgAccountId] = startAccountCollectWorker(ctx, service, tgAccountId, signature, sources)
	}
	for tgAccountId, worker := range m.workers {
		if _, ok := active[tgAccountId]; ok {
			continue
		}
		worker.stop()
		delete(m.workers, tgAccountId)
	}
}

func (m *accountCollectSupervisor) stopAll() {
	for tgAccountId, worker := range m.workers {
		worker.stop()
		delete(m.workers, tgAccountId)
	}
}

func (s *sSysPublish) enabledAccountCollectSources(ctx context.Context) (map[int64][]accountCollectSourceRuntime, error) {
	if !s.collectGlobalEnabled(ctx) {
		return map[int64][]accountCollectSourceRuntime{}, nil
	}
	if err := ensureCollectSourceColumns(ctx); err != nil {
		return nil, err
	}
	var rows []accountCollectSourceRuntime
	err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields("id,tenant_id,account_id,tg_account_id,source_chat_id,source_username,history_collect_enabled,history_collect_mode,history_collect_days").
		Where("source_type", sysin.CollectSourceTypeAccount).
		Where("collect_enabled", 1).
		Where("status", 1).
		WhereGT("tg_account_id", 0).
		WhereNull("deleted_at").
		OrderAsc("tg_account_id").
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		return nil, err
	}
	groups := make(map[int64][]accountCollectSourceRuntime)
	for _, row := range rows {
		row.HistoryCollectEnabled, row.HistoryCollectMode, row.HistoryCollectDays = sysin.NormalizeCollectHistoryConfig(
			sysin.CollectSourceTypeAccount,
			row.HistoryCollectEnabled,
			row.HistoryCollectMode,
			row.HistoryCollectDays,
		)
		groups[row.TgAccountId] = append(groups[row.TgAccountId], row)
	}
	return groups, nil
}

func startAccountCollectWorker(ctx context.Context, service *sSysPublish, tgAccountId int64, signature string, sources []accountCollectSourceRuntime) *accountCollectWorker {
	workerCtx, cancel := context.WithCancel(ctx)
	worker := &accountCollectWorker{
		service:     service,
		tgAccountId: tgAccountId,
		signature:   signature,
		sources:     append([]accountCollectSourceRuntime(nil), sources...),
		cancel:      cancel,
		done:        make(chan struct{}),
	}
	go worker.run(workerCtx)
	return worker
}

func (w *accountCollectWorker) run(ctx context.Context) {
	defer close(w.done)
	g.Log().Infof(ctx, "账号采集 worker 启动 tgAccountId:%d sources:%d", w.tgAccountId, len(w.sources))
	defer g.Log().Infof(context.Background(), "账号采集 worker 停止 tgAccountId:%d", w.tgAccountId)
	if err := w.runGotdDispatcher(ctx); err != nil && !isContextDone(ctx) {
		g.Log().Warningf(ctx, "账号采集 worker 异常 tgAccountId:%d err:%+v", w.tgAccountId, err)
	}
}

func (w *accountCollectWorker) runGotdDispatcher(ctx context.Context) error {
	if w.service == nil {
		return gerror.New("账号采集服务未初始化")
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return err
	}
	item, err := w.service.accountCollectTgAccount(ctx, w.tgAccountId)
	if err != nil {
		return err
	}
	dispatcher := tg.NewUpdateDispatcher()
	w.bindGotdHandlers(dispatcher)
	client, err := w.service.newAccountCollectClient(ctx, conf, item, dispatcher)
	if err != nil {
		return err
	}
	return client.Run(ctx, func(runCtx context.Context) error {
		if _, err := client.Self(runCtx); err != nil {
			return err
		}
		g.Log().Infof(runCtx, "账号采集 dispatcher 已连接 tgAccountId:%d", w.tgAccountId)
		<-runCtx.Done()
		return runCtx.Err()
	})
}

func (w *accountCollectWorker) stop() {
	if w == nil || w.cancel == nil {
		return
	}
	w.cancel()
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
	}
}

func accountCollectSourceSignature(sources []accountCollectSourceRuntime) string {
	parts := make([]string, 0, len(sources))
	for _, item := range sources {
		parts = append(parts, fmt.Sprintf("%d:%s:%s:%d:%s:%d",
			item.Id,
			item.SourceChatId,
			item.SourceUsername,
			item.HistoryCollectEnabled,
			item.HistoryCollectMode,
			item.HistoryCollectDays,
		))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

type accountCollectTgAccount struct {
	Id         int64  `json:"id"`
	TenantId   int64  `json:"tenantId"`
	SessionKey string `json:"sessionKey"`
	Status     string `json:"status"`
}

func (s *sSysPublish) accountCollectTgAccount(ctx context.Context, tgAccountId int64) (*accountCollectTgAccount, error) {
	var item *accountCollectTgAccount
	err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,session_key,status").
		Where("id", tgAccountId).
		WhereNull("deleted_at").
		Scan(&item)
	if err != nil {
		return nil, gerror.Wrap(err, "读取账号采集TG账号失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("账号采集TG账号不存在")
	}
	if strings.TrimSpace(item.SessionKey) == "" {
		return nil, gerror.New("账号采集TG账号未登录")
	}
	return item, nil
}

func (s *sSysPublish) newAccountCollectClient(ctx context.Context, conf *model.TelegramConfig, item *accountCollectTgAccount, dispatcher tg.UpdateDispatcher) (*telegram.Client, error) {
	if conf == nil || conf.AppId <= 0 || strings.TrimSpace(conf.AppHash) == "" {
		return nil, gerror.New("请先配置Telegram App ID和App Hash")
	}
	sessionPath, err := s.telegramSessionPathByKey(item.SessionKey)
	if err != nil {
		return nil, err
	}
	options := telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{Path: sessionPath},
		UpdateHandler:  dispatcher,
	}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		return nil, err
	} else if resolver != nil {
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
