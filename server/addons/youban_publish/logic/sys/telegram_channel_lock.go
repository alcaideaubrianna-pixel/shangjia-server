package sys

import (
	"context"
	"strings"
)

func (s *sSysPublish) withTelegramChannelLock(ctx context.Context, chatId string, fn func() error) error {
	lock := s.telegramChannelLock(chatId)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}

func (s *sSysPublish) telegramChannelLock(chatId string) *publishRuntimeMutex {
	key := strings.TrimSpace(normalizeTelegramChannelChatID(chatId))
	if key == "" {
		key = "default"
	}
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
