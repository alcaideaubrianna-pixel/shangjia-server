package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

func (s *sSysPublish) withTelegramChannelLock(ctx context.Context, chatId string, fn func() error) error {
	key := telegramChannelLockKey(chatId)
	lock := s.telegramChannelLock(chatId)
	lock.Lock()
	defer lock.Unlock()
	return s.withTelegramChannelDBLock(ctx, key, fn)
}

func (s *sSysPublish) telegramChannelLock(chatId string) *publishRuntimeMutex {
	key := telegramChannelLockKey(chatId)
	s.telegramChannelMu.Lock()
	defer s.telegramChannelMu.Unlock()
	if s.telegramChannelLocks == nil {
		s.telegramChannelLocks = make(map[string]*publishRuntimeMutex)
	}
	lock := s.telegramChannelLocks[key]
	if lock == nil {
		lock = &publishRuntimeMutex{}
		s.telegramChannelLocks[key] = lock
	}
	return lock
}

func telegramChannelLockKey(chatId string) string {
	key := strings.TrimSpace(normalizeTelegramChannelChatID(chatId))
	if key == "" {
		key = "default"
	}
	return "youban_publish:tg_channel:" + key
}

func (s *sSysPublish) withTelegramChannelDBLock(ctx context.Context, key string, fn func() error) error {
	dbType := strings.ToLower(g.DB().GetConfig().Type)
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		switch dbType {
		case consts.DBPgsql:
			if _, err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", key); err != nil {
				return gerror.Wrap(err, "获取TG频道发送锁失败")
			}
			return fn()
		default:
			value, err := tx.GetValue("SELECT GET_LOCK(?, 60)", key)
			if err != nil {
				return gerror.Wrap(err, "获取TG频道发送锁失败")
			}
			if value.Int() != 1 {
				return gerror.New("获取TG频道发送锁超时")
			}
			defer func() {
				_, _ = tx.Exec("SELECT RELEASE_LOCK(?)", key)
			}()
			return fn()
		}
	})
}
