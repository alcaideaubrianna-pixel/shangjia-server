package adminsecurity

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
	"hotgo/utility/ownedlease"
)

type attemptWindow struct {
	Count int   `json:"count"`
	Until int64 `json:"until"`
}

// reserveAttempt uses the shared cache and a fixed, owner-checked Redis lease.
// A cache/Redis outage refuses only management login, never public business APIs.
func reserveAttempt(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter", "").String() != "redis" {
		return false, gerror.New("后台登录保护需要共享Redis缓存")
	}
	lease := ownedlease.New(consts.CacheAdminLoginLock+key, "admin-login", 5*time.Second)
	locked, err := lease.TryAcquire(ctx)
	if err != nil || !locked {
		return false, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := lease.Unlock(cleanup); err != nil {
			g.Log().Warning(cleanup, "释放后台登录限流锁失败")
		}
	}()
	value, err := cache.Instance().Get(ctx, consts.CacheAdminLoginAttempts+key)
	if err != nil {
		return false, err
	}
	var state attemptWindow
	if value != nil && !value.IsNil() {
		if err := value.Scan(&state); err != nil {
			return false, err
		}
	}
	now := time.Now()
	if state.Until <= now.Unix() {
		state = attemptWindow{Until: now.Add(window).Unix()}
	}
	if state.Count >= limit {
		return false, nil
	}
	state.Count++
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	err = cache.Instance().Set(ctx, consts.CacheAdminLoginAttempts+key, state, time.Until(time.Unix(state.Until, 0)))
	return err == nil, err
}

type LoginLimiter struct {
	Prefix  string
	Reserve func(context.Context, string, int, time.Duration) (bool, error)
}

func NewLoginLimiter(prefix string) *LoginLimiter {
	return &LoginLimiter{Prefix: strings.TrimSuffix(prefix, "/"), Reserve: reserveAttempt}
}

func loginKey(kind, value string) string {
	return fmt.Sprintf("%s:%x", kind, sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value)))))
}

// Middleware runs before input decryption/captcha/database work. Both password
// and SMS logins consume the same account and IP budgets, including isLock.
func (l *LoginLimiter) Middleware(r *ghttp.Request) {
	p := strings.ToLower(path.Clean(r.URL.Path))
	captchaRequest := r.Method == http.MethodGet && p == strings.ToLower(l.Prefix)+"/site/captcha"
	if !captchaRequest && (r.Method != http.MethodPost || (p != strings.ToLower(l.Prefix)+"/site/accountlogin" && p != strings.ToLower(l.Prefix)+"/site/mobilelogin")) {
		r.Middleware.Next()
		return
	}
	// Do not trust client-supplied forwarding headers for a security limit.
	// Behind a proxy this is deliberately conservative (shared proxy IP budget).
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	account := r.Get("username").String()
	if strings.HasSuffix(p, "/mobilelogin") {
		account = r.Get("mobile").String()
	}
	budgets := []struct {
		key    string
		limit  int
		window time.Duration
	}{
		{loginKey("ip", ip), 30, 5 * time.Minute},
		{loginKey("account", account), 10, 15 * time.Minute},
	}
	if captchaRequest {
		budgets = budgets[:1]
		budgets[0].key = loginKey("captcha-ip", ip)
		budgets[0].limit = 60
	}
	for _, budget := range budgets {
		allowed, err := l.Reserve(ctx, budget.key, budget.limit, budget.window)
		if err != nil {
			g.Log().Warning(r.Context(), "后台登录保护暂不可用，已拒绝本次登录")
			r.Response.WriteStatus(http.StatusServiceUnavailable, "登录保护暂不可用，请稍后重试")
			r.ExitAll()
			return
		}
		if !allowed {
			r.Response.Header().Set("Retry-After", fmt.Sprint(int(budget.window.Seconds())))
			r.Response.WriteStatus(http.StatusTooManyRequests, "登录尝试过于频繁，请稍后重试")
			r.ExitAll()
			return
		}
	}
	r.Middleware.Next()
}
