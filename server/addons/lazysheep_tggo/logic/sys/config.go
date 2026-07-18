// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/logic/bangchat"
	"hotgo/addons/lazysheep_tggo/model"
	lsysin "hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/addons/lazysheep_tggo/service"
	"hotgo/internal/dao"
	"hotgo/internal/library/hgrds/lock"
)

type sLazySheepTGGo struct {
	runtime *runtimeStore
}

func NewLazySheepTGGo() *sLazySheepTGGo {
	return &sLazySheepTGGo{runtime: newRuntimeStore()}
}

func init() {
	service.RegisterSysLazysheepTggo(NewLazySheepTGGo())
}

func (s *sLazySheepTGGo) Install(ctx context.Context) error {
	if err := s.ensureTables(ctx); err != nil {
		return err
	}
	_, err := s.GetState(ctx)
	return err
}

func (s *sLazySheepTGGo) Upgrade(ctx context.Context) error {
	if err := s.ensureTables(ctx); err != nil {
		return err
	}
	return s.SyncAllBots(ctx)
}

func (s *sLazySheepTGGo) UnInstall(ctx context.Context) error {
	s.runtime.Reset()
	return nil
}

func (s *sLazySheepTGGo) BootBots(ctx context.Context) error {
	if err := s.ensureTables(ctx); err != nil {
		return err
	}
	return s.bootBots(ctx)
}

func (s *sLazySheepTGGo) GetState(ctx context.Context) (res *model.State, err error) {
	return s.loadState(ctx)
}

func (s *sLazySheepTGGo) SaveState(ctx context.Context, state *model.State) error {
	if err := s.saveState(ctx, state); err != nil {
		return err
	}
	clearAutoPullBindingsCache(ctx)
	return nil
}

func (s *sLazySheepTGGo) SaveConfig(ctx context.Context, in *lsysin.UpdateConfigInp) error {
	if in == nil {
		return gerror.New("配置参数不能为空")
	}
	state := in.ToState()
	switch in.Group {
	case "plugins":
		if err := s.savePluginConfig(ctx, state); err != nil {
			return err
		}
	default:
		if err := s.saveState(ctx, state); err != nil {
			return err
		}
	}
	clearAutoPullBindingsCache(ctx)
	return nil
}

func (s *sLazySheepTGGo) InspectBot(ctx context.Context, in *lsysin.BotInspectInp) (*lsysin.BotInspectModel, error) {
	return s.inspectBot(ctx, in)
}

func (s *sLazySheepTGGo) DeleteBot(ctx context.Context, in *lsysin.BotDeleteInp) error {
	return s.deleteBot(ctx, in)
}

func (s *sLazySheepTGGo) StartBot(ctx context.Context, in *lsysin.BotStartInp) error {
	if in == nil || in.Key == "" {
		return gerror.New("机器人标识不能为空")
	}
	return s.SyncBot(ctx, in.Key)
}

func (s *sLazySheepTGGo) BotUsers(ctx context.Context, in *lsysin.BotUserListInp) ([]*lsysin.BotUserListModel, error) {
	return s.botUsers(ctx, in)
}

func (s *sLazySheepTGGo) UpdateBotUser(ctx context.Context, in *lsysin.BotUserEditInp) error {
	return s.updateBotUser(ctx, in)
}

func (s *sLazySheepTGGo) TouchUser(ctx context.Context, in *lsysin.TouchUserInp) error {
	if in == nil || in.TelegramID == 0 {
		return nil
	}
	if err := s.ensureUserBotKey(ctx); err != nil {
		return err
	}
	return s.upsertUser(ctx, in.TelegramID, &model.UserRecord{
		TelegramID:   in.TelegramID,
		BotKey:       in.BotKey,
		Username:     in.Username,
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		LanguageCode: in.LanguageCode,
		IsBot:        in.IsBot,
	})
}

func (s *sLazySheepTGGo) IsBotAdmin(ctx context.Context, botKey string, telegramID int64) (bool, error) {
	if strings.TrimSpace(botKey) == "" || telegramID == 0 {
		return false, nil
	}
	cols := dao.AddonLazysheepTggoUser.Columns()
	val, err := dao.AddonLazysheepTggoUser.Ctx(ctx).
		Fields(cols.MemberLevel).
		Where("bot_key", botKey).
		Where(cols.TelegramId, telegramID).
		Value()
	if err != nil {
		return false, gerror.Wrap(err, "查询机器人管理员失败")
	}
	return !val.IsNil() && val.Int() >= 9, nil
}

func (s *sLazySheepTGGo) UpsertBot(ctx context.Context, in *lsysin.BotUpsertInp) (key string, err error) {
	if in == nil {
		return "", gerror.New("机器人配置不能为空")
	}
	key = in.Key
	if key == "" {
		key = shortHash(in.Token)
	}
	if state, stateErr := s.GetState(ctx); stateErr == nil && state != nil {
		if existing := state.Bots[key]; existing != nil && strings.TrimSpace(existing.Token) == strings.TrimSpace(in.Token) {
			if strings.TrimSpace(in.Username) == "" {
				in.Username = existing.Username
			}
			if strings.TrimSpace(in.DisplayName) == "" {
				in.DisplayName = existing.DisplayName
			}
		}
	}
	if strings.TrimSpace(in.Token) != "" && (strings.TrimSpace(in.Username) == "" || strings.TrimSpace(in.DisplayName) == "") {
		if info, inspectErr := s.inspectBot(ctx, &lsysin.BotInspectInp{Token: in.Token}); inspectErr == nil && info != nil {
			if strings.TrimSpace(in.Username) == "" {
				in.Username = info.Username
			}
			if strings.TrimSpace(in.DisplayName) == "" {
				in.DisplayName = info.DisplayName
			}
		}
	}
	cfg := &model.BotConfig{
		Key:           key,
		Role:          in.Role,
		MemberId:      in.MemberId,
		Token:         in.Token,
		DisplayName:   in.DisplayName,
		Username:      in.Username,
		WebhookSecret: in.WebhookSecret,
		WebhookPath:   in.WebhookPath,
		Enabled:       in.Enabled,
		AutoPull:      in.AutoPull,
		AutoForward:   in.AutoForward,
		ReviewEnabled: in.ReviewEnabled,
	}
	s.normalizeBotConfig(key, cfg)
	g.Log().Debugf(ctx, "保存机器人配置 botKey:%s role:%s enabled:%t username:%s", key, cfg.Role, cfg.Enabled, cfg.Username)
	if err = s.ensureBotRoleField(ctx); err != nil {
		return key, err
	}
	if err = s.upsertBot(ctx, key, cfg, nil); err != nil {
		return key, err
	}
	if err = s.syncBotAfterSave(ctx, key); err != nil {
		return key, err
	}
	return key, nil
}

func (s *sLazySheepTGGo) BindSource(ctx context.Context, in *lsysin.BindSourceInp) error {
	if in == nil || strings.TrimSpace(in.BotKey) == "" {
		return gerror.New("botKey 不能为空")
	}
	sourceURL := strings.TrimSpace(in.SourceURL)
	if sourceURL == "" {
		return gerror.New("BangChat 链接不能为空")
	}
	if err := validateBangchatSourceURL(sourceURL); err != nil {
		return err
	}
	mode := strings.TrimSpace(in.Mode)
	reviewChatID := in.ReviewChatID
	publishChatID := in.PublishChatID
	autoPush := in.AutoPush
	if in.ChatID != 0 {
		if mode == "review" {
			reviewChatID = in.ChatID
			autoPush = false
		} else if mode == "publish" || autoPush {
			publishChatID = in.ChatID
			autoPush = true
		} else if reviewChatID == 0 && publishChatID == 0 {
			reviewChatID = in.ChatID
			autoPush = false
		}
	}
	key := fmt.Sprintf("%s:%d", in.BotKey, in.ChatID)
	pluginState := s.defaultBindingPluginState(ctx, in.BotKey)
	var lastPullID int64
	var lastCursor string
	if existing, findErr := s.findBinding(ctx, in.BotKey, sourceURL, in.ChatID); findErr == nil && existing != nil {
		if len(existing.PluginState) > 0 {
			pluginState = existing.PluginState
		}
		if existing.Key == key && strings.TrimSpace(existing.SourceURL) == sourceURL {
			lastPullID = existing.LastPullID
			lastCursor = existing.LastCursor
		}
	}
	if in.OperatorID > 0 {
		pluginState[collectorBindOperatorIDKey] = in.OperatorID
	}
	if err := s.upsertBinding(ctx, key, &model.BindingRecord{
		Key:             key,
		BotKey:          in.BotKey,
		SourceURL:       sourceURL,
		SourceToken:     in.SourceToken,
		ReviewChatID:    reviewChatID,
		PublishChatID:   publishChatID,
		Status:          "enabled",
		AutoPush:        autoPush,
		VerifyEnabled:   true,
		LocationEnabled: true,
		PluginState:     pluginState,
		LastPullID:      lastPullID,
		LastCursor:      lastCursor,
	}); err != nil {
		return err
	}
	clearAutoPullBindingsCache(ctx)
	return nil
}

func validateBangchatSourceURL(sourceURL string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(sourceURL))
	if err != nil || parsed == nil || parsed.Host == "" {
		return gerror.New("BangChat 链接格式不正确，请输入完整的 http/https 地址")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return gerror.New("BangChat 链接必须是 http 或 https 地址")
	}
	return nil
}

func (s *sLazySheepTGGo) PullNow(ctx context.Context, in *lsysin.PullInp) (message string, err error) {
	started := time.Now()
	if in == nil || in.BotKey == "" {
		return "", gerror.New("botKey 不能为空")
	}
	binding, err := s.findBinding(ctx, in.BotKey, in.SourceURL, in.ChatID)
	if err != nil {
		return "", err
	}
	if binding == nil {
		return "", gerror.New("未找到可触发的绑定关系")
	}
	traceID := shortHash(fmt.Sprintf("%s:%d:%s:%d", in.BotKey, in.ChatID, binding.SourceURL, time.Now().UnixNano()))
	ctx = withPullTrace(ctx, traceID)
	var summary *pullSummary
	var timer *pullTimer
	defer func() {
		var steps []lsysin.PullMonitorStep
		if timer != nil {
			steps = timer.Steps()
		}
		recordPullMonitorEvent(ctx, newPullMonitorEvent(ctx, in, binding.Key, binding.SourceURL, in.Auto, started, steps, summary, message, err))
	}()
	pullKey := fmt.Sprintf("lazysheep_tggo:pull:%s:%s", in.BotKey, binding.Key)
	mutex := lock.Mutex(pullKey)
	if lockErr := mutex.TryLock(ctx); lockErr != nil {
		if gerror.Is(lockErr, lock.ErrLockFailed) {
			if !in.Auto && cancelRunningPull(in.BotKey, binding) {
				g.Log().Warningf(ctx, "%s 手动拉取优先，已取消正在执行的采集任务 botKey:%s binding:%s", pullTraceTag(ctx), in.BotKey, binding.Key)
				for i := 0; i < 15; i++ {
					select {
					case <-ctx.Done():
						return "", ctx.Err()
					case <-time.After(200 * time.Millisecond):
					}
					mutex = lock.Mutex(pullKey)
					lockErr = mutex.TryLock(ctx)
					if lockErr == nil {
						goto pullLocked
					}
					if !gerror.Is(lockErr, lock.ErrLockFailed) {
						return "", gerror.Wrap(lockErr, "创建采集执行锁失败")
					}
				}
			}
			g.Log().Debugf(ctx, "%s 采集执行锁已存在 botKey:%s binding:%s auto:%t", pullTraceTag(ctx), in.BotKey, binding.Key, in.Auto)
			return "已有采集任务正在执行，请稍后再试。", nil
		}
		return "", gerror.Wrap(lockErr, "创建采集执行锁失败")
	}
pullLocked:
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if unlockErr := mutex.Unlock(unlockCtx); unlockErr != nil && !gerror.Is(unlockErr, lock.ErrNotExist) {
			g.Log().Warningf(ctx, "释放采集执行锁失败 key:%s err:%+v", pullKey, unlockErr)
		}
	}()
	ctx, unregisterPull := registerRunningPull(ctx, in.BotKey, binding)
	defer unregisterPull()

	limit := in.Limit
	timer = newPullTimer(ctx)
	if err = s.configureBangchatProxy(ctx); err != nil {
		return "", err
	}
	timer.Report("代理配置完成。")
	g.Log().Debugf(ctx, "%s 开始 BangChat 采集 botKey:%s binding:%s source:%s limit:%d autoPush:%t", pullTraceTag(ctx), in.BotKey, binding.Key, binding.SourceURL, limit, binding.AutoPush)
	summary = &pullSummary{}
	dedupScope := pullDedupScope(in.BotKey, binding, in.ChatID)
	successMaxContentID := binding.LastPullID
	successLatestCursor := binding.LastCursor
	successLatestCursorID := parseInt(binding.LastCursor)
	bindingLastCursorID := successLatestCursorID
	batchSeen := make(map[string]struct{})
	currentFingerprints := make(map[string]struct{})
	currentPushFingerprints := make(map[string]struct{})
	processed := 0
	noteProcessed := 0
	pullLimit := limit
	if limit > 0 {
		pullLimit = 0
	}
	pairID, err := bangchat.PullPages(ctx, bangchat.PullOption{URL: binding.SourceURL, Limit: pullLimit, MaxPages: 0, PageSize: 50}, func(page *bangchat.PullPage) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if page == nil {
			return nil
		}
		if summary.PairID == "" {
			summary.PairID = page.PairID
		}
		summary.RawFetched += len(page.Messages)
		g.Log().Debugf(ctx, "%s BangChat 采集页完成 botKey:%s binding:%s pair:%s page:%d fetched:%d total:%d", pullTraceTag(ctx), in.BotKey, binding.Key, page.PairID, page.Page, len(page.Messages), summary.RawFetched)
		timer.Report("已拉取第 %d 页 %d 条，开始处理...", page.Page, len(page.Messages))
		for idx, raw := range page.Messages {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			processed++
			if limit > 0 {
				timer.Report("正在处理第 %d/%d 条...", processed, limit)
			} else {
				timer.Report("正在处理第 %d 页第 %d 条，累计 %d 条...", page.Page, idx+1, processed)
			}
			if !isBangchatNote(raw) {
				summary.NonNotes++
				continue
			}
			if limit > 0 && noteProcessed >= limit {
				return errPullNoteLimitReached
			}
			noteProcessed++
			summary.Fetched++
			var msg sourceMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				summary.Failed++
				summary.AddError("解析采集消息失败", err)
				g.Log().Warningf(ctx, "解析采集消息失败 botKey:%s binding:%s err:%+v", in.BotKey, binding.Key, err)
				continue
			}
			g.Log().Debugf(ctx, "采集处理笔记 botKey:%s binding:%s page:%d index:%d contentID:%s autoPush:%t", in.BotKey, binding.Key, page.Page, idx+1, msg.ContentId, binding.AutoPush)
			contentID := parseInt(msg.ContentId)
			cursorID := parseInt(msg.Id)
			if !in.Retry && !in.Sync && isOldBangchatMessage(binding, cursorID, contentID) {
				summary.Skipped++
				summary.OldCursorSkipped++
				timer.Report("跳过旧笔记 contentID:%s cursor:%s。", msg.ContentId, msg.Id)
				return errPullOldCursorReached
			}
			var note noteContent
			if err := json.Unmarshal([]byte(msg.Content), &note); err != nil {
				summary.Failed++
				summary.AddError(fmt.Sprintf("解析采集笔记失败 contentID:%s", msg.ContentId), err)
				g.Log().Warningf(ctx, "解析采集笔记失败 botKey:%s binding:%s err:%+v", in.BotKey, binding.Key, err)
				continue
			}
			fingerprint, sourceURLs := noteFingerprint(note)
			if fingerprint != "" {
				currentFingerprints[fingerprint] = struct{}{}
				if _, ok := batchSeen[fingerprint]; ok {
					summary.Deduped++
					continue
				}
				if !in.Sync {
					seen, seenErr := pullDedupSeen(ctx, dedupScope, fingerprint)
					if seenErr != nil {
						return seenErr
					}
					if seen {
						summary.Deduped++
						timer.Report("当前频道发现重复笔记，已跳过。")
						continue
					}
				}
			}
			var stored *lsysin.NoteStoreModel
			if storeErr := retryPullAction(ctx, fmt.Sprintf("保存 botKey:%s binding:%s contentID:%s", in.BotKey, binding.Key, msg.ContentId), func() error {
				res, err := s.StoreNote(ctx, &lsysin.NoteStoreInp{
					BotKey:     in.BotKey,
					BindingKey: binding.Key,
					Payload:    string(raw),
				})
				if err != nil {
					return err
				}
				stored = res
				return nil
			}); storeErr != nil {
				summary.Failed++
				summary.AddError(fmt.Sprintf("保存失败 contentID:%s", msg.ContentId), storeErr)
				g.Log().Warningf(ctx, "%s 保存 BangChat 笔记失败 botKey:%s binding:%s err:%+v", pullTraceTag(ctx), in.BotKey, binding.Key, storeErr)
				continue
			}
			summary.Stored++
			if stored == nil {
				summary.Failed++
				summary.AddError(fmt.Sprintf("保存失败 contentID:%s", msg.ContentId), gerror.New("笔记保存结果为空"))
				continue
			}
			if in.Sync {
				if pushFingerprints, fpErr := s.pushMediaDedupFingerprints(ctx, stored.NoteId); fpErr != nil {
					g.Log().Warningf(ctx, "计算同步推送指纹失败 botKey:%s binding:%s note:%d err:%+v", in.BotKey, binding.Key, stored.NoteId, fpErr)
				} else {
					for _, pushFingerprint := range pushFingerprints {
						currentPushFingerprints[pushFingerprint] = struct{}{}
					}
				}
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			task, queued, pushErr := s.enqueuePushNote(ctx, in.BotKey, binding, stored.NoteId, contentID, in.ChatID)
			if pushErr != nil {
				summary.PushFailed++
				summary.AddError(fmt.Sprintf("推送入队失败 contentID:%s", msg.ContentId), pushErr)
				g.Log().Warningf(ctx, "%s BangChat 笔记推送任务入队失败 botKey:%s binding:%s err:%+v", pullTraceTag(ctx), in.BotKey, binding.Key, pushErr)
				continue
			}
			if queued {
				summary.PushQueued++
			} else {
				summary.Skipped++
				summary.PushDeduped++
				if in.Sync && task != nil && task.TaskID == 0 && stored.NoteId > 0 {
					if err := s.deleteNoteRows(ctx, []int64{stored.NoteId}); err != nil {
						g.Log().Warningf(ctx, "清理同步重复笔记失败 botKey:%s binding:%s note:%d err:%+v", in.BotKey, binding.Key, stored.NoteId, err)
					}
				}
			}
			if cursorID > 0 && cursorID > successLatestCursorID {
				successLatestCursorID = cursorID
				successLatestCursor = msg.Id
			}
			if contentID > successMaxContentID {
				successMaxContentID = contentID
				if successLatestCursor == "" {
					successLatestCursor = msg.Id
				}
			}
			if queued {
				timer.Report("准备开始推送 contentID:%s。", msg.ContentId)
			} else {
				timer.Report("当前频道已存在相同推送记录，已跳过 contentID:%s。", msg.ContentId)
			}
			if fingerprint != "" && stored != nil {
				batchSeen[fingerprint] = struct{}{}
				dedupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				if err := pullDedupRemember(dedupCtx, dedupScope, fingerprint, stored.NoteId, sourceURLs); err != nil {
					g.Log().Warningf(ctx, "记录采集去重信息失败 botKey:%s binding:%s err:%+v", in.BotKey, binding.Key, err)
				}
				cancel()
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errPullNoteLimitReached) && !errors.Is(err, errPullOldCursorReached) {
		if errors.Is(err, context.Canceled) {
			return timer.Append("采集已暂停。"), nil
		}
		return "", gerror.Wrap(err, "采集 BangChat 消息失败")
	}
	if summary.PairID == "" {
		summary.PairID = pairID
	}
	if in.Sync && limit == 0 && len(currentFingerprints) > 0 {
		removedNotes, deletedMessages, failedMessages, syncErr := s.deleteBindingNotesNotInFingerprints(ctx, in.BotKey, binding, in.ChatID, currentFingerprints, currentPushFingerprints)
		if syncErr != nil {
			summary.AddError("差异清理下架内容失败", syncErr)
			g.Log().Warningf(ctx, "%s 差异清理下架内容失败 botKey:%s binding:%s err:%+v", pullTraceTag(ctx), in.BotKey, binding.Key, syncErr)
		} else if removedNotes > 0 || deletedMessages > 0 || failedMessages > 0 {
			timer.Report("差异清理完成：下架 %d 条，删除频道消息 %d 条，失败 %d 条。", removedNotes, deletedMessages, failedMessages)
		}
	}
	g.Log().Debugf(ctx, "%s BangChat 采集完成 botKey:%s binding:%s pair:%s fetched:%d summary:%s", pullTraceTag(ctx), in.BotKey, binding.Key, summary.PairID, summary.Fetched, summary.Message())
	if successMaxContentID > binding.LastPullID || successLatestCursorID > bindingLastCursorID {
		if err := s.updateBindingPullState(ctx, binding.Key, successMaxContentID, successLatestCursor); err != nil {
			g.Log().Warningf(ctx, "%s 更新采集游标失败 botKey:%s binding:%s err:%+v", pullTraceTag(ctx), in.BotKey, binding.Key, err)
		}
	}
	if in.Auto {
		s.updateAutoPullIdleState(ctx, binding, summary.Stored > 0 || summary.PushQueued > 0)
	}
	return timer.Append(summary.Message()), nil
}

func (s *sLazySheepTGGo) ResetBindingPull(ctx context.Context, botKey string, chatID int64) (message string, err error) {
	binding, err := s.findBinding(ctx, botKey, "", chatID)
	if err != nil {
		return "", err
	}
	if binding == nil {
		return "", gerror.New("当前频道还没有绑定关系")
	}
	if err = s.updateBindingPullState(ctx, binding.Key, 0, ""); err != nil {
		return "", err
	}
	if err = clearPullDedupScope(ctx, pullDedupScope(botKey, binding, chatID)); err != nil {
		g.Log().Warningf(ctx, "清理采集去重缓存失败 bot:%s binding:%s err:%+v", botKey, binding.Key, err)
	}
	if err = s.clearPushChannelState(ctx, botKey, binding, chatID); err != nil {
		g.Log().Warningf(ctx, "清理频道推送状态失败 bot:%s binding:%s err:%+v", botKey, binding.Key, err)
	}
	return "当前频道采集记录已重置，可以重新拉取。", nil
}

func (s *sLazySheepTGGo) ClearBindingNotes(ctx context.Context, botKey string, chatID int64) (message string, err error) {
	binding, err := s.findBinding(ctx, botKey, "", chatID)
	if err != nil {
		return "", err
	}
	if binding == nil {
		return "", gerror.New("当前频道还没有绑定关系")
	}
	deletedMessages, failedMessages, deleteErr := s.deleteBindingPushedMessages(ctx, botKey, binding, chatID)
	if deleteErr != nil {
		g.Log().Warningf(ctx, "删除频道已推送消息失败 bot:%s binding:%s err:%+v", botKey, binding.Key, deleteErr)
	}
	noteCols := dao.AddonLazysheepTggoNote.Columns()
	itemCols := dao.AddonLazysheepTggoNoteItem.Columns()
	assetCols := dao.AddonLazysheepTggoNoteAsset.Columns()
	bindingCols := dao.AddonLazysheepTggoBinding.Columns()
	bindingID, err := dao.AddonLazysheepTggoBinding.Ctx(ctx).
		Fields(bindingCols.Id).
		Where(bindingCols.BindingKey, binding.Key).
		Value()
	if err != nil {
		return "", gerror.Wrap(err, "查询频道绑定失败")
	}
	if bindingID.IsNil() {
		return "", gerror.New("当前频道绑定不存在")
	}
	noteIds, err := dao.AddonLazysheepTggoNote.Ctx(ctx).
		Fields(noteCols.Id).
		Where(noteCols.BindingId, bindingID.Int64()).
		Array()
	if err != nil {
		return "", gerror.Wrap(err, "查询频道笔记失败")
	}
	count := len(noteIds)
	if count > 0 {
		if _, err = dao.AddonLazysheepTggoNoteAsset.Ctx(ctx).WhereIn(assetCols.NoteId, noteIds).Delete(); err != nil {
			return "", gerror.Wrap(err, "删除频道笔记资源失败")
		}
		if _, err = dao.AddonLazysheepTggoNoteItem.Ctx(ctx).WhereIn(itemCols.NoteId, noteIds).Delete(); err != nil {
			return "", gerror.Wrap(err, "删除频道笔记项失败")
		}
		if _, err = dao.AddonLazysheepTggoNote.Ctx(ctx).WhereIn(noteCols.Id, noteIds).Delete(); err != nil {
			return "", gerror.Wrap(err, "删除频道笔记失败")
		}
	}
	if err = s.updateBindingPullState(ctx, binding.Key, 0, ""); err != nil {
		return "", err
	}
	if err = clearPullDedupScope(ctx, pullDedupScope(botKey, binding, chatID)); err != nil {
		g.Log().Warningf(ctx, "清理采集去重缓存失败 bot:%s binding:%s err:%+v", botKey, binding.Key, err)
	}
	if err = s.clearPushChannelState(ctx, botKey, binding, chatID); err != nil {
		g.Log().Warningf(ctx, "清理频道推送状态失败 bot:%s binding:%s err:%+v", botKey, binding.Key, err)
	}
	if failedMessages > 0 {
		return fmt.Sprintf("当前频道已清空 %d 条笔记，删除频道消息 %d 条，%d 条消息删除失败，并重置采集记录。", count, deletedMessages, failedMessages), nil
	}
	return fmt.Sprintf("当前频道已清空 %d 条笔记，删除频道消息 %d 条，并重置采集记录。", count, deletedMessages), nil
}

func retryPullAction(ctx context.Context, label string, action func() error) error {
	var err error
	const maxAttempts = 6
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = action()
		if err == nil {
			return nil
		}
		if !isRetriablePullError(err) || attempt == maxAttempts {
			return err
		}
		g.Log().Warningf(ctx, "%s %s 第%d次失败，准备重试 err:%+v", pullTraceTag(ctx), label, attempt, err)
		delay := time.Duration(attempt*attempt)*time.Second + time.Duration(attempt*137)*time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

func isRetriablePullError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "timeout"),
		strings.Contains(text, "temporarily"),
		strings.Contains(text, "connection reset"),
		strings.Contains(text, "eof"),
		strings.Contains(text, "deadlock"),
		strings.Contains(text, "lock wait timeout"),
		strings.Contains(text, "try restarting transaction"),
		strings.Contains(text, "too many requests"),
		strings.Contains(text, "1213"),
		strings.Contains(text, "1205"),
		strings.Contains(text, "40001"),
		strings.Contains(text, "429"),
		strings.Contains(text, "try again"),
		strings.Contains(text, "deadline exceeded"):
		return true
	default:
		return false
	}
}

type pullSummary struct {
	PairID           string
	RawFetched       int
	Fetched          int
	Stored           int
	Pushed           int
	PushQueued       int
	Deduped          int
	Skipped          int
	OldCursorSkipped int
	PushDeduped      int
	NonNotes         int
	Failed           int
	PushFailed       int
	Errors           []string
}

func (s *pullSummary) Message() string {
	message := fmt.Sprintf("采集完成：获取 %d 条，准备开始推送 %d 条。", s.Fetched, s.PushQueued)
	if s.Deduped > 0 || s.Skipped > 0 || s.Failed > 0 || s.PushFailed > 0 {
		message += fmt.Sprintf("\n已跳过 %d 条，失败 %d 条。", s.Deduped+s.Skipped, s.Failed+s.PushFailed)
		if detail := s.SkipDetailText(); detail != "" {
			message += "\n跳过原因：" + detail
		}
	}
	if errText := s.ErrorText(); errText != "" {
		message += "\n失败原因：" + errText
	}
	return message
}

func (s *pullSummary) SkipDetailText() string {
	if s == nil {
		return ""
	}
	details := make([]string, 0, 4)
	if s.Deduped > 0 {
		details = append(details, fmt.Sprintf("采集重复 %d 条", s.Deduped))
	}
	if s.PushDeduped > 0 {
		details = append(details, fmt.Sprintf("推送重复 %d 条", s.PushDeduped))
	}
	if s.OldCursorSkipped > 0 {
		details = append(details, fmt.Sprintf("旧笔记 %d 条", s.OldCursorSkipped))
	}
	otherSkipped := s.Skipped - s.OldCursorSkipped - s.PushDeduped
	if otherSkipped > 0 {
		details = append(details, fmt.Sprintf("其他 %d 条", otherSkipped))
	}
	return strings.Join(details, "，")
}

func (s *pullSummary) AddError(label string, err error) {
	if s == nil || err == nil {
		return
	}
	text := strings.TrimSpace(label)
	if text != "" {
		text += "："
	}
	text += trimPullErrorText(err)
	if text == "" {
		return
	}
	const maxErrors = 3
	if len(s.Errors) >= maxErrors {
		s.Errors = s.Errors[1:]
	}
	s.Errors = append(s.Errors, text)
}

func (s *pullSummary) ErrorText() string {
	if s == nil || len(s.Errors) == 0 {
		return ""
	}
	return strings.Join(s.Errors, "；")
}

func trimPullErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "duplicate entry"):
		text = "记录已存在，已按重复内容处理"
	case strings.Contains(lower, "context canceled"):
		text = "任务已取消"
	case strings.Contains(lower, "too many requests"):
		text = "Telegram 限流，已进入重试"
	}
	const maxLen = 180
	if len([]rune(text)) > maxLen {
		text = string([]rune(text)[:maxLen]) + "..."
	}
	return text
}

func (s *sLazySheepTGGo) SetBindingPublishChat(ctx context.Context, botKey string, chatID int64) (message string, err error) {
	if strings.TrimSpace(botKey) == "" {
		return "", gerror.New("botKey 不能为空")
	}
	if chatID == 0 {
		return "", gerror.New("发布频道ID不能为空")
	}
	cols := dao.AddonLazysheepTggoBinding.Columns()
	result, err := dao.AddonLazysheepTggoBinding.Ctx(ctx).
		Where(cols.BotKey, botKey).
		WhereGT(cols.ReviewChatId, 0).
		Data(g.Map{cols.PublishChatId: chatID, cols.PublishEnabled: 1}).
		Update()
	if err != nil {
		return "", gerror.Wrap(err, "绑定发布频道失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return "当前机器人还没有审核绑定。请先在审核群发送 /绑定审核 <BangChat链接>。", nil
	}
	return "发布频道绑定成功。审核群中的内容点击“发布”后，会推送到当前频道。", nil
}

func isBangchatNote(raw json.RawMessage) bool {
	var msg struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(raw, &msg)
	return msg.Type == "MESSAGE_TYPE_NOTES"
}

func isOldBangchatMessage(binding *model.BindingRecord, cursorID int64, contentID int64) bool {
	if binding == nil {
		return false
	}
	lastCursorID := parseInt(binding.LastCursor)
	if lastCursorID > 0 && cursorID > 0 {
		return cursorID <= lastCursorID
	}
	if lastCursorID > 0 {
		return false
	}
	return binding.LastPullID > 0 && contentID > 0 && contentID <= binding.LastPullID
}

func (s *sLazySheepTGGo) configureBangchatProxy(ctx context.Context) error {
	state, err := s.GetState(ctx)
	if err != nil {
		return err
	}
	proxyURL := ""
	if state != nil && state.Global != nil {
		proxyURL = strings.TrimSpace(state.Global.TelegramProxy)
	}
	if err = bangchat.SetProxy(proxyURL); err != nil {
		return gerror.Wrap(err, "配置 BangChat 代理失败")
	}
	return nil
}

func (s *sLazySheepTGGo) SignIn(ctx context.Context, in *lsysin.SignInInp) (message string, err error) {
	return "签到功能框架已接入，后续补充关注校验和验证码。", nil
}

func (s *sLazySheepTGGo) findBinding(ctx context.Context, botKey, sourceURL string, chatID int64) (*model.BindingRecord, error) {
	state, err := s.GetState(ctx)
	if err != nil {
		return nil, err
	}
	if chatID != 0 {
		expectedKey := fmt.Sprintf("%s:%d", botKey, chatID)
		for _, v := range state.Bindings {
			if v == nil || v.BotKey != botKey {
				continue
			}
			if v.Key == expectedKey {
				return v, nil
			}
		}
	}
	for _, v := range state.Bindings {
		if v == nil {
			continue
		}
		if v.BotKey != botKey {
			continue
		}
		if chatID != 0 && (v.ReviewChatID == chatID || v.PublishChatID == chatID) {
			return v, nil
		}
	}
	if strings.TrimSpace(sourceURL) != "" {
		for _, v := range state.Bindings {
			if v == nil || v.BotKey != botKey {
				continue
			}
			if v.SourceURL == sourceURL {
				return v, nil
			}
		}
	}
	return nil, nil
}

func (s *sLazySheepTGGo) updateBindingPullState(ctx context.Context, bindingKey string, lastPullID int64, lastCursor string) error {
	if strings.TrimSpace(bindingKey) == "" {
		return nil
	}
	cols := dao.AddonLazysheepTggoBinding.Columns()
	_, err := dao.AddonLazysheepTggoBinding.Ctx(ctx).
		Where(cols.BindingKey, bindingKey).
		Data(g.Map{
			cols.LastPullId: lastPullID,
			cols.LastCursor: strings.TrimSpace(lastCursor),
			cols.UpdatedAt:  gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集游标失败")
	}
	clearAutoPullBindingsCache(ctx)
	return nil
}
