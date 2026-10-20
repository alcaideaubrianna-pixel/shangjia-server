// Package adminsecurity protects the management entry without changing API routes.
package adminsecurity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

const cookieName = "youban_admin_entry"
const entryTTL = 12 * time.Hour

var entryPattern = regexp.MustCompile(`^/[A-Za-z0-9_-]{16,128}$`)

type Entry struct {
	Path      string
	Key       string
	Prefix    string
	IndexFile string
}

func Enabled(ctx context.Context) bool {
	return strings.TrimSpace(g.Cfg().MustGet(ctx, "security.admin.entryPath", "").String()) != ""
}

func FromConfig(ctx context.Context) (*Entry, error) {
	e := &Entry{
		Path:      strings.TrimSpace(g.Cfg().MustGet(ctx, "security.admin.entryPath", "").String()),
		Key:       g.Cfg().MustGet(ctx, "security.admin.entryKey", "").String(),
		Prefix:    g.Cfg().MustGet(ctx, "router.admin.prefix", "/admin").String(),
		IndexFile: "resource/public/admin/index.html",
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	if e.Path != "" && g.Cfg().MustGet(ctx, "cache.adapter", "").String() != "redis" {
		return nil, gerror.New("启用后台安全入口需要将 cache.adapter 配置为 redis，以支持多实例登录保护和验证码")
	}
	return e, nil
}

func (e *Entry) Validate() error {
	if e.Path == "" {
		return nil
	}
	if !entryPattern.MatchString(e.Path) || len(e.Key) < 32 {
		return gerror.New("后台安全入口需为 / 加16至128位字母、数字、下划线或短横线，签名密钥至少32位")
	}
	if !strings.HasPrefix(e.Prefix, "/") || e.Prefix == "/" || path.Clean(e.Prefix) != strings.TrimSuffix(e.Prefix, "/") {
		return gerror.New("后台接口前缀配置不合法")
	}
	return nil
}

func (e *Entry) signature(value string) string {
	mac := hmac.New(sha256.New, []byte(e.Key))
	_, _ = mac.Write([]byte(e.Path + "\x00" + value))
	return hex.EncodeToString(mac.Sum(nil))
}

func (e *Entry) cookieValue(now time.Time) string {
	value := strconv.FormatInt(now.Add(entryTTL).Unix(), 10)
	return value + "." + e.signature(value)
}

func (e *Entry) validCookie(r *http.Request, now time.Time) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 {
		return false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || expires <= now.Unix() || expires > now.Add(entryTTL+2*time.Minute).Unix() {
		return false
	}
	return hmac.Equal([]byte(parts[1]), []byte(e.signature(parts[0])))
}

// BeforeServe is a hook, rather than route middleware: static files must also
// pass the entry check. The credential is not an administrator login token.
func (e *Entry) BeforeServe(r *ghttp.Request) {
	if e.Path == "" {
		return
	}
	p := path.Clean(r.URL.Path)
	prefix := strings.TrimSuffix(e.Prefix, "/")
	if p == e.Path && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		r.Response.Header().Set("Cache-Control", "no-store")
		r.Response.Header().Set("Referrer-Policy", "no-referrer")
		http.SetCookie(r.Response.Writer, &http.Cookie{
			Name: cookieName, Value: e.cookieValue(time.Now()), Path: "/",
			HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
			MaxAge: int(entryTTL.Seconds()),
		})
		r.Response.ServeFile(e.IndexFile)
		// GoFrame may write its own access log after ExitAll. Do not persist
		// the secret URL there, even when the regular middleware is skipped.
		r.RequestURI = "/[admin-entry]"
		r.URL.Path = "/[admin-entry]"
		r.URL.RawQuery = ""
		r.ExitAll()
		return
	}
	lower := strings.ToLower(p)
	protected := lower == strings.ToLower(prefix) || strings.HasPrefix(lower, strings.ToLower(prefix)+"/")
	if !protected && lower != "/login" {
		return
	}
	r.Response.Header().Set("Cache-Control", "no-store")
	r.Response.Header().Set("Referrer-Policy", "no-referrer")
	index := strings.TrimPrefix(lower, strings.ToLower(prefix)+"/")
	if lower == "/login" || lower == strings.ToLower(prefix) || index == "index.html" || index == "index.html.gz" || index == "index.html.br" || !e.validCookie(r.Request, time.Now()) {
		r.Response.Header().Set("X-Admin-Entry-Required", "1")
		r.Response.WriteStatus(http.StatusNotFound)
		r.ExitAll()
	}
}
