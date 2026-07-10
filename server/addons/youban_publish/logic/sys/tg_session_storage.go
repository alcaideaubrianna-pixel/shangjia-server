package sys

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/session"
)

const publishTgSessionTable = "hg_youban_publish_tg_session"

type telegramDBSessionStorage struct {
	key          string
	fallbackPath string
	mu           sync.Mutex
}

func (s *sSysPublish) telegramSessionStorage(sessionKey string) (session.Storage, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil, gerror.New("TG账号会话键不能为空")
	}
	fallbackPath, err := s.telegramSessionPathByKey(sessionKey)
	if err != nil {
		return nil, err
	}
	return &telegramDBSessionStorage{key: sessionKey, fallbackPath: fallbackPath}, nil
}

func (s *telegramDBSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	if s == nil || strings.TrimSpace(s.key) == "" {
		return nil, session.ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var row struct {
		SessionData []byte `json:"sessionData" orm:"session_data"`
	}
	err := g.DB().Model(publishTgSessionTable).Safe().Ctx(ctx).
		Fields("session_data").
		Where("session_key", s.key).
		Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG会话失败")
	}
	if len(row.SessionData) > 0 {
		return append([]byte(nil), row.SessionData...), nil
	}
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
	count, err := g.DB().Model(publishTgSessionTable).Safe().Ctx(ctx).
		Where("session_key", s.key).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG会话失败")
	}
	saveData := g.Map{
		"session_key":  s.key,
		"session_data": append([]byte(nil), data...),
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
